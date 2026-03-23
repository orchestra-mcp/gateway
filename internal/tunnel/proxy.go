package tunnel

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/orchestra-mcp/gateway/internal/config"
	"github.com/orchestra-mcp/gateway/internal/middleware"
	"github.com/orchestra-mcp/gateway/internal/models"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

const (
	// proxyReadLimit is the maximum message size for proxied messages (1MB).
	proxyReadLimit = 1024 * 1024

	// proxyWriteTimeout is the max time to write a message to either side.
	proxyWriteTimeout = 10 * time.Second

	// proxyPongTimeout is how long to wait for a pong from the gate.
	proxyPongTimeout = 60 * time.Second

	// proxyPingInterval sends pings to the gate at this interval.
	proxyPingInterval = 30 * time.Second
)

// TunnelProxyHandler handles WebSocket proxy connections from browsers to tunnel gates.
// It upgrades the browser connection and relays messages through the TunnelHub's
// reverse connection to the local machine.
//
// Auth is done via JWT token query parameter (?token=<jwt>) because browsers
// cannot send custom headers during the WebSocket upgrade handshake.
type TunnelProxyHandler struct {
	db        *gorm.DB
	cfg       *config.Config
	tunnelHub *TunnelHub
	upgrader  websocket.FastHTTPUpgrader
}

// NewTunnelProxyHandler creates a new TunnelProxyHandler.
func NewTunnelProxyHandler(db *gorm.DB, cfg *config.Config, hub *TunnelHub) *TunnelProxyHandler {
	return &TunnelProxyHandler{
		db:        db,
		cfg:       cfg,
		tunnelHub: hub,
		upgrader: websocket.FastHTTPUpgrader{
			ReadBufferSize:  16 * 1024,
			WriteBufferSize: 16 * 1024,
			CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
				return true // CORS handled at HTTP level.
			},
		},
	}
}

// Handle upgrades an HTTP request to a WebSocket connection and relays messages
// to the tunnel's reverse connection via the TunnelHub.
//
// Route: GET /api/tunnels/:id/ws?token=<jwt>
func (h *TunnelProxyHandler) Handle(c fiber.Ctx) error {
	// Authenticate via query param JWT (browsers can't send headers on WS upgrade).
	tokenStr := c.Query("token")
	if tokenStr == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing token query parameter"})
	}

	// Support both raw JWT and "Bearer <jwt>" format.
	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")

	claims := &middleware.Claims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fiber.NewError(fiber.StatusUnauthorized, "unexpected signing method")
		}
		return []byte(h.cfg.JWTSecret), nil
	})
	if err != nil || !parsed.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired token"})
	}

	var user models.User
	if err := h.db.First(&user, claims.UserID).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "user not found"})
	}
	if user.Status == "blocked" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "account is blocked"})
	}

	tunnelID := c.Params("id")
	if tunnelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tunnel id is required"})
	}

	// Look up the tunnel and verify ownership.
	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND user_id = ?", tunnelID, user.ID).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	// Check that a reverse connection exists for this tunnel.
	if !h.tunnelHub.HasReverse(tunnelID) {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "tunnel_not_connected",
			"message": "tunnel is not connected — ensure orchestra serve is running with --cloud-url",
		})
	}

	// Generate a unique browser session ID for this connection.
	sessionID := generateBrowserSessionID()

	// Upgrade the browser connection.
	err = h.upgrader.Upgrade(c.Context(), func(browserConn *websocket.Conn) {
		// Register the browser session in the hub.
		h.tunnelHub.RegisterBrowser(sessionID, tunnelID, browserConn)
		defer func() {
			h.tunnelHub.UnregisterBrowser(sessionID)
			h.tunnelHub.NotifyGateClose(tunnelID, sessionID)
		}()

		browserConn.SetReadLimit(proxyReadLimit)

		// Read from browser, wrap in relay envelope, forward to reverse tunnel.
		for {
			_, msg, err := browserConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.Printf("[tunnel-proxy] browser read error for session %s: %v", sessionID, err)
				}
				return
			}

			// Forward to the reverse tunnel via the hub.
			if err := h.tunnelHub.ForwardToGate(tunnelID, sessionID, json.RawMessage(msg)); err != nil {
				log.Printf("[tunnel-proxy] forward to gate failed for session %s: %v", sessionID, err)
				// Reverse tunnel dropped — close the browser connection.
				return
			}
		}
	})
	if err != nil {
		log.Printf("[tunnel-proxy] upgrade error for tunnel %s: %v", tunnelID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "websocket upgrade failed",
		})
	}

	return nil
}

// generateBrowserSessionID creates a random session ID for a browser connection.
func generateBrowserSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "bs-" + hex.EncodeToString(b)
}
