package tunnel

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/orchestra-mcp/gateway/internal/middleware"
	"github.com/orchestra-mcp/gateway/internal/models"
	"gorm.io/gorm"
)

// claimEntry stores tunnel credentials waiting to be claimed by the local CLI.
type claimEntry struct {
	TunnelID        string
	ConnectionToken string
	TeamID          string // user's active team (for sync scoping)
	AuthToken       string // user's JWT (for sync API auth)
	Workspace       string // workspace path from the CLI
	ExpiresAt       time.Time
}

// TunnelHandler handles tunnel CRUD and registration endpoints.
type TunnelHandler struct {
	db        *gorm.DB
	tunnelHub *TunnelHub

	// claimMap stores nonce → claimEntry for the claim polling flow.
	// Entries are short-lived (5 minutes) and consumed on first claim.
	claimMu  sync.Mutex
	claimMap map[string]*claimEntry
}

// NewTunnelHandler creates a new TunnelHandler.
func NewTunnelHandler(db *gorm.DB, hub *TunnelHub) *TunnelHandler {
	h := &TunnelHandler{
		db:        db,
		tunnelHub: hub,
		claimMap:  make(map[string]*claimEntry),
	}
	// Background reaper for expired claim entries.
	go h.reapClaims()
	return h
}

// reapClaims removes expired claim entries every minute.
func (h *TunnelHandler) reapClaims() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		h.claimMu.Lock()
		for nonce, entry := range h.claimMap {
			if now.After(entry.ExpiresAt) {
				delete(h.claimMap, nonce)
			}
		}
		h.claimMu.Unlock()
	}
}

// List handles GET /api/tunnels — returns all tunnels for the authenticated user.
func (h *TunnelHandler) List(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var tunnels []models.Tunnel
	q := h.db.Where("user_id = ?", user.ID).Order("created_at desc")

	// Filter by status if provided.
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}

	if err := q.Find(&tunnels).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch tunnels"})
	}

	return c.JSON(tunnels)
}

// Show handles GET /api/tunnels/:id — returns a single tunnel.
func (h *TunnelHandler) Show(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND user_id = ?", c.Params("id"), user.ID).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	return c.JSON(tunnel)
}

// registerBody is the request body for POST /api/tunnels/register.
type registerBody struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// Register handles POST /api/tunnels/register — validates a tunnel registration
// token from `orchestra serve --web-gate`, stores it in the database, and makes
// the tunnel credentials available for the local CLI to claim.
//
// Unlike the old flow, we do NOT verify reachability by dialing the gate — the
// reverse tunnel pattern means the local machine connects outbound to us.
func (h *TunnelHandler) Register(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var body registerBody
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required"})
	}

	// Decode the registration token to extract machine info.
	tokenInfo, err := decodeRegistrationToken(body.Token)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("invalid token: %v", err)})
	}

	// Generate a persistent connection token for future API calls.
	connToken, err := generateConnectionToken()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate connection token"})
	}

	name := body.Name
	if name == "" && tokenInfo.Workspace != "" {
		name = filepath.Base(tokenInfo.Workspace)
	}
	if name == "" {
		name = tokenInfo.Hostname
	}

	// Look up the user's active team (first membership).
	var membership models.Membership
	var teamID string
	if err := h.db.Where("user_id = ?", user.ID).Order("created_at asc").First(&membership).Error; err == nil {
		teamID = membership.TeamID
	}

	now := time.Now()
	tunnel := models.Tunnel{
		UserID:          user.ID,
		Name:            name,
		Hostname:        tokenInfo.Hostname,
		OS:              tokenInfo.OS,
		Architecture:    tokenInfo.Arch,
		GateAddress:     tokenInfo.GateAddress,
		ConnectionToken: connToken,
		Status:          models.TunnelStatusConnecting,
		LastSeenAt:      &now,
		ToolCount:       tokenInfo.ToolCount,
		LocalIP:         tokenInfo.LocalIP,
		Workspace:       tokenInfo.Workspace,
		Version:         "1.0.0",
	}
	if teamID != "" {
		tunnel.TeamID = &teamID
	}

	if err := h.db.Create(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to register tunnel"})
	}

	// Extract the user's auth token from the request header for sync.
	authToken := ""
	if header := c.Get("Authorization"); header != "" {
		authToken = strings.TrimPrefix(header, "Bearer ")
	}

	// Store claim entry so the local CLI can poll for credentials.
	if tokenInfo.Nonce != "" {
		h.claimMu.Lock()
		h.claimMap[tokenInfo.Nonce] = &claimEntry{
			TunnelID:        tunnel.ID,
			ConnectionToken: connToken,
			TeamID:          teamID,
			AuthToken:       authToken,
			Workspace:       tokenInfo.Workspace,
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		h.claimMu.Unlock()
	}

	// Return the tunnel with the connection token (only returned at registration time).
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"tunnel":           tunnel,
		"connection_token": connToken,
	})
}

// Claim handles POST /api/tunnels/claim — called by the local CLI to retrieve
// tunnel credentials after the user registers the tunnel in the web app.
// Auth is via the token nonce (the secret the CLI knows from token generation).
//
// The CLI polls this endpoint every ~2 seconds after displaying the token.
// Returns 404 if the token hasn't been registered yet.
func (h *TunnelHandler) Claim(c fiber.Ctx) error {
	var body struct {
		Nonce string `json:"nonce"`
	}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Nonce == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nonce is required"})
	}

	h.claimMu.Lock()
	entry, ok := h.claimMap[body.Nonce]
	if ok {
		delete(h.claimMap, body.Nonce) // One-time use.
	}
	h.claimMu.Unlock()

	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not_registered_yet"})
	}

	if time.Now().After(entry.ExpiresAt) {
		return c.Status(fiber.StatusGone).JSON(fiber.Map{"error": "claim expired"})
	}

	return c.JSON(fiber.Map{
		"tunnel_id":        entry.TunnelID,
		"connection_token": entry.ConnectionToken,
		"team_id":          entry.TeamID,
		"auth_token":       entry.AuthToken,
		"workspace":        entry.Workspace,
	})
}

// Update handles PUT /api/tunnels/:id — updates tunnel name/labels.
func (h *TunnelHandler) Update(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND user_id = ?", c.Params("id"), user.ID).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	var body struct {
		Name   string          `json:"name"`
		Labels json.RawMessage `json:"labels"`
	}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	updates := map[string]any{}
	if body.Name != "" {
		updates["name"] = body.Name
	}
	if body.Labels != nil {
		updates["labels"] = body.Labels
	}

	if len(updates) > 0 {
		if err := h.db.Model(&tunnel).Updates(updates).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update tunnel"})
		}
	}

	h.db.First(&tunnel, "id = ?", tunnel.ID)
	return c.JSON(tunnel)
}

// Delete handles DELETE /api/tunnels/:id — removes a tunnel.
func (h *TunnelHandler) Delete(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND user_id = ?", c.Params("id"), user.ID).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	if err := h.db.Delete(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete tunnel"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// Status handles GET /api/tunnels/:id/status — checks if the tunnel has an
// active reverse connection.
func (h *TunnelHandler) Status(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND user_id = ?", c.Params("id"), user.ID).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	// Check if the tunnel has an active reverse connection.
	connected := h.tunnelHub.HasReverse(tunnel.ID)

	now := time.Now()
	if connected {
		h.db.Model(&tunnel).Updates(map[string]any{
			"status":       models.TunnelStatusOnline,
			"last_seen_at": now,
		})
		tunnel.Status = models.TunnelStatusOnline
		tunnel.LastSeenAt = &now
	} else {
		h.db.Model(&tunnel).Update("status", models.TunnelStatusOffline)
		tunnel.Status = models.TunnelStatusOffline
	}

	return c.JSON(fiber.Map{
		"tunnel_id":  tunnel.ID,
		"status":     tunnel.Status,
		"connected":  connected,
		"checked_at": now.Format(time.RFC3339),
	})
}

// Heartbeat handles POST /api/tunnels/heartbeat — called periodically by the
// tunnel to report it's still alive. Uses the connection token for auth.
// Kept for backward compatibility — the reverse WebSocket connection is the
// primary liveness signal.
func (h *TunnelHandler) Heartbeat(c fiber.Ctx) error {
	var body struct {
		ConnectionToken string `json:"connection_token"`
		ToolCount       int    `json:"tool_count"`
		GateAddress     string `json:"gate_address"`
		LocalIP         string `json:"local_ip"`
	}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.ConnectionToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "connection_token is required"})
	}

	var tunnel models.Tunnel
	if err := h.db.Where("connection_token = ?", body.ConnectionToken).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	now := time.Now()
	updates := map[string]any{
		"status":       models.TunnelStatusOnline,
		"last_seen_at": now,
	}
	if body.ToolCount > 0 {
		updates["tool_count"] = body.ToolCount
	}
	if body.GateAddress != "" && body.GateAddress != tunnel.GateAddress {
		updates["gate_address"] = body.GateAddress
	}
	if body.LocalIP != "" && body.LocalIP != tunnel.LocalIP {
		updates["local_ip"] = body.LocalIP
	}

	h.db.Model(&tunnel).Updates(updates)

	return c.JSON(fiber.Map{"ok": true})
}

// autoRegisterBody is the request body for POST /api/tunnels/auto-register.
type autoRegisterBody struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	LocalIP      string `json:"local_ip"`
	GateAddress  string `json:"gate_address"`
	Workspace    string `json:"workspace"`
	ToolCount    int    `json:"tool_count"`
	Version      string `json:"version"`
}

// AutoRegister handles POST /api/tunnels/auto-register — registers or reconnects
// a tunnel using the caller's JWT authentication. No manual token paste needed.
// Upserts by (user_id, hostname, workspace) so the same machine reuses its tunnel.
func (h *TunnelHandler) AutoRegister(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var body autoRegisterBody
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Hostname == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "hostname is required"})
	}

	// Look up the user's active team.
	var membership models.Membership
	var teamID string
	if err := h.db.Where("user_id = ?", user.ID).Order("created_at asc").First(&membership).Error; err == nil {
		teamID = membership.TeamID
	}

	// Try to find an existing tunnel for this (user, hostname, workspace).
	var existing models.Tunnel
	q := h.db.Where("user_id = ? AND hostname = ?", user.ID, body.Hostname)
	if body.Workspace != "" {
		q = q.Where("workspace = ?", body.Workspace)
	}

	now := time.Now()

	if err := q.First(&existing).Error; err == nil {
		// Existing tunnel found — update it and generate a fresh connection token.
		connToken, err := generateConnectionToken()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate connection token"})
		}

		updates := map[string]any{
			"connection_token": connToken,
			"status":           string(models.TunnelStatusConnecting),
			"last_seen_at":     &now,
			"os":               body.OS,
			"architecture":     body.Architecture,
			"local_ip":         body.LocalIP,
			"gate_address":     body.GateAddress,
			"tool_count":       body.ToolCount,
		}
		if body.Version != "" {
			updates["version"] = body.Version
		}
		if teamID != "" {
			updates["team_id"] = teamID
		}

		h.db.Model(&existing).Updates(updates)

		// Extract auth token for sync.
		authToken := ""
		if header := c.Get("Authorization"); header != "" {
			authToken = strings.TrimPrefix(header, "Bearer ")
		}

		return c.JSON(fiber.Map{
			"tunnel_id":        existing.ID,
			"connection_token": connToken,
			"team_id":          teamID,
			"auth_token":       authToken,
			"workspace":        body.Workspace,
			"reconnected":      true,
		})
	}

	// No existing tunnel — create a new one.
	connToken, err := generateConnectionToken()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate connection token"})
	}

	name := body.Workspace
	if name != "" {
		name = filepath.Base(name)
	}
	if name == "" {
		name = body.Hostname
	}

	tunnel := models.Tunnel{
		UserID:          user.ID,
		Name:            name,
		Hostname:        body.Hostname,
		OS:              body.OS,
		Architecture:    body.Architecture,
		GateAddress:     body.GateAddress,
		ConnectionToken: connToken,
		Status:          models.TunnelStatusConnecting,
		LastSeenAt:      &now,
		ToolCount:       body.ToolCount,
		LocalIP:         body.LocalIP,
		Workspace:       body.Workspace,
		Version:         body.Version,
	}
	if teamID != "" {
		tunnel.TeamID = &teamID
	}

	if err := h.db.Create(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to register tunnel"})
	}

	authToken := ""
	if header := c.Get("Authorization"); header != "" {
		authToken = strings.TrimPrefix(header, "Bearer ")
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"tunnel_id":        tunnel.ID,
		"connection_token": connToken,
		"team_id":          teamID,
		"auth_token":       authToken,
		"workspace":        body.Workspace,
		"reconnected":      false,
	})
}

// --- Helpers ---

// tunnelTokenPayload represents the decoded registration token from the CLI.
type tunnelTokenPayload struct {
	Hostname    string `json:"hostname"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	LocalIP     string `json:"local_ip"`
	GateAddress string `json:"gate_address"`
	APIKeyHash  string `json:"api_key_hash,omitempty"`
	Nonce       string `json:"nonce"`
	ToolCount   int    `json:"tool_count"`
	CreatedAt   string `json:"created_at"`
	CloudURL    string `json:"cloud_url,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
}

// decodeRegistrationToken decodes the base64url-encoded JSON token from the CLI.
func decodeRegistrationToken(raw string) (*tunnelTokenPayload, error) {
	decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	var payload tunnelTokenPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &payload, nil
}

// generateConnectionToken creates a secure random token for persistent tunnel auth.
func generateConnectionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "tun_" + hex.EncodeToString(b), nil
}
