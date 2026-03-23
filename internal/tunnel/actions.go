package tunnel

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/orchestra-mcp/gateway/internal/middleware"
	"github.com/orchestra-mcp/gateway/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// bridgeLongRunningTools are action types that support progress streaming.
var bridgeLongRunningTools = map[string]bool{
	"run_prompt":  true,
	"run_command": true,
	"run_bridge":  true,
	"run_tests":   true,
}

const (
	// smartActionTimeout is the max time to wait for a tool call response.
	smartActionTimeout = 120 * time.Second
)

// SmartActionHandler dispatches smart actions through the tunnel to a local CLI.
// It provides a REST API bridge: HTTP POST → relay envelope → reverse tunnel → response.
// All executions are recorded in the action_histories table.
type SmartActionHandler struct {
	db        *gorm.DB
	tunnelHub *TunnelHub

	// reqID is an atomic counter for generating unique JSON-RPC request IDs.
	reqID atomic.Int64
}

// NewSmartActionHandler creates a new SmartActionHandler.
func NewSmartActionHandler(db *gorm.DB, hub *TunnelHub) *SmartActionHandler {
	return &SmartActionHandler{
		db:        db,
		tunnelHub: hub,
	}
}

// smartActionRequest is the REST request body for POST /api/tunnels/:id/actions
// and POST /api/tunnels/:id/action (JSON-RPC dispatch).
type smartActionRequest struct {
	Type   string          `json:"type"`   // run_prompt, run_command, run_tool, run_bridge, file_read, file_write, sync_workspace, run_tests, get_status, list_tools
	Params json.RawMessage `json:"params"` // action-specific params
}

// Execute handles POST /api/tunnels/:id/actions — dispatches a smart action
// through the tunnel and returns the result synchronously.
func (h *SmartActionHandler) Execute(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tunnelID := c.Params("id")
	if tunnelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tunnel id is required"})
	}

	// Verify tunnel ownership.
	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND user_id = ?", tunnelID, user.ID).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	// Check reverse connection.
	if !h.tunnelHub.HasReverse(tunnelID) {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "tunnel_not_connected",
			"message": "tunnel is not connected — ensure orchestra serve is running",
		})
	}

	var body smartActionRequest
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Type == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "action type is required"})
	}

	// Map the smart action to an MCP tool call.
	toolName, toolArgs, err := h.mapAction(body)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	start := time.Now()

	// Build a JSON-RPC request for tools/call.
	reqID := fmt.Sprintf("sa-%d", h.reqID.Add(1))
	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      reqID,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": toolArgs,
		},
	}

	rpcBytes, err := json.Marshal(rpcReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to marshal request"})
	}

	// Register a response channel (and optional progress channel) via the hub.
	respCh := make(chan json.RawMessage, 1)
	var progressCh chan string
	if bridgeLongRunningTools[body.Type] {
		progressCh = make(chan string, 32)
	}
	sessionID := generateBrowserSessionID()
	h.tunnelHub.RegisterSmartAction(sessionID, respCh, progressCh)
	defer h.tunnelHub.UnregisterSmartAction(sessionID)

	// Forward the request through the tunnel.
	if err := h.tunnelHub.ForwardToGate(tunnelID, sessionID, json.RawMessage(rpcBytes)); err != nil {
		h.recordHistory(user.ID, tunnelID, body.Type, toolName, body.Params, false, 0, "failed to forward to tunnel")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "failed to forward to tunnel"})
	}

	// Wait for the response with timeout.
	timeout := smartActionTimeout
	if body.Type == "get_status" || body.Type == "list_tools" {
		timeout = 15 * time.Second
	}

	var progressMsgs []string
	if progressCh != nil {
		// Drain progress messages until final response arrives.
	drainLoop:
		for {
			select {
			case msg := <-progressCh:
				progressMsgs = append(progressMsgs, msg)
			case resp := <-respCh:
				durationMs := time.Since(start).Milliseconds()
				return h.handleActionResponse(c, user.ID, tunnelID, body, toolName, resp, progressMsgs, durationMs)
			case <-time.After(timeout):
				durationMs := time.Since(start).Milliseconds()
				h.recordHistory(user.ID, tunnelID, body.Type, toolName, body.Params, false, durationMs, "timeout")
				return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{
					"error": "action timed out waiting for response from tunnel",
				})
			}
			// Check if final response arrived alongside last progress message.
			select {
			case resp := <-respCh:
				durationMs := time.Since(start).Milliseconds()
				return h.handleActionResponse(c, user.ID, tunnelID, body, toolName, resp, progressMsgs, durationMs)
			default:
				continue drainLoop
			}
		}
	}

	select {
	case resp := <-respCh:
		durationMs := time.Since(start).Milliseconds()
		return h.handleActionResponse(c, user.ID, tunnelID, body, toolName, resp, nil, durationMs)
	case <-time.After(timeout):
		durationMs := time.Since(start).Milliseconds()
		h.recordHistory(user.ID, tunnelID, body.Type, toolName, body.Params, false, durationMs, "timeout")
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{
			"error": "action timed out waiting for response from tunnel",
		})
	}
}

// handleActionResponse parses a JSON-RPC response, records history + action log, and returns the HTTP response.
func (h *SmartActionHandler) handleActionResponse(c fiber.Ctx, userID uint, tunnelID string, body smartActionRequest, toolName string, resp json.RawMessage, progress []string, durationMs int64) error {
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &rpcResp); err != nil {
		h.recordHistory(userID, tunnelID, body.Type, toolName, body.Params, true, durationMs, "")
		return c.JSON(fiber.Map{"success": true, "output": string(resp)})
	}
	if rpcResp.Error != nil {
		h.recordHistory(userID, tunnelID, body.Type, toolName, body.Params, false, durationMs, rpcResp.Error.Message)
		h.recordActionLog(userID, tunnelID, body.Type, toolName, body.Params, "", nil, progress, false, durationMs, rpcResp.Error.Message)
		return c.JSON(fiber.Map{"success": false, "error": rpcResp.Error.Message})
	}
	// Extract text output from MCP tool result content array.
	output := extractMCPOutput(rpcResp.Result)
	files := extractMCPFiles(rpcResp.Result)
	h.recordHistory(userID, tunnelID, body.Type, toolName, body.Params, true, durationMs, "")
	h.recordActionLog(userID, tunnelID, body.Type, toolName, body.Params, output, files, progress, true, durationMs, "")
	return c.JSON(fiber.Map{"success": true, "result": rpcResp.Result})
}

// History handles GET /api/tunnels/:id/actions/history — returns recent action
// history for a tunnel.
func (h *SmartActionHandler) History(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tunnelID := c.Params("id")

	// Verify tunnel ownership.
	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND user_id = ?", tunnelID, user.ID).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	var entries []models.ActionHistory
	q := h.db.Where("tunnel_id = ? AND user_id = ?", tunnelID, user.ID).
		Order("created_at desc")

	if actionType := c.Query("type"); actionType != "" {
		q = q.Where("action_type = ?", actionType)
	}

	var total int64
	q.Model(&models.ActionHistory{}).Count(&total)

	q.Offset(offset).Limit(limit).Find(&entries)

	return c.JSON(fiber.Map{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// AllHistory handles GET /api/actions/history — returns action history across
// all tunnels for the authenticated user.
func (h *SmartActionHandler) AllHistory(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	var entries []models.ActionHistory
	q := h.db.Where("user_id = ?", user.ID).
		Order("created_at desc")

	if actionType := c.Query("type"); actionType != "" {
		q = q.Where("action_type = ?", actionType)
	}

	var total int64
	q.Model(&models.ActionHistory{}).Count(&total)

	q.Offset(offset).Limit(limit).Find(&entries)

	return c.JSON(fiber.Map{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// Dispatch handles POST /api/tunnels/:id/action — JSON-RPC style action dispatch.
// Supports run_tool (any MCP tool by name), run_bridge, file_read, file_write,
// plus all existing smart action types.
func (h *SmartActionHandler) Dispatch(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tunnelID := c.Params("id")
	if tunnelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tunnel id is required"})
	}

	// Verify tunnel ownership.
	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND user_id = ?", tunnelID, user.ID).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	// Check reverse connection.
	if !h.tunnelHub.HasReverse(tunnelID) {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "tunnel_not_connected",
			"message": "tunnel is not connected — ensure orchestra serve is running",
		})
	}

	var body smartActionRequest
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Type == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "action type is required"})
	}

	toolName, toolArgs, err := h.mapAction(body)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	start := time.Now()

	reqID := fmt.Sprintf("dispatch-%d", h.reqID.Add(1))
	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      reqID,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": toolArgs,
		},
	}

	rpcBytes, err := json.Marshal(rpcReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to marshal request"})
	}

	respCh := make(chan json.RawMessage, 1)
	var progressCh chan string
	if bridgeLongRunningTools[body.Type] {
		progressCh = make(chan string, 32)
	}
	sessionID := generateBrowserSessionID()
	h.tunnelHub.RegisterSmartAction(sessionID, respCh, progressCh)
	defer h.tunnelHub.UnregisterSmartAction(sessionID)

	if err := h.tunnelHub.ForwardToGate(tunnelID, sessionID, json.RawMessage(rpcBytes)); err != nil {
		h.recordHistory(user.ID, tunnelID, body.Type, toolName, body.Params, false, 0, "failed to forward to tunnel")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "failed to forward to tunnel"})
	}

	timeout := smartActionTimeout
	if body.Type == "get_status" || body.Type == "list_tools" {
		timeout = 15 * time.Second
	}

	var progressMsgs []string
	if progressCh != nil {
	drainLoop2:
		for {
			select {
			case msg := <-progressCh:
				progressMsgs = append(progressMsgs, msg)
			case resp := <-respCh:
				durationMs := time.Since(start).Milliseconds()
				return h.handleActionResponse(c, user.ID, tunnelID, body, toolName, resp, progressMsgs, durationMs)
			case <-time.After(timeout):
				durationMs := time.Since(start).Milliseconds()
				h.recordHistory(user.ID, tunnelID, body.Type, toolName, body.Params, false, durationMs, "timeout")
				return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{
					"error": "action timed out waiting for response from tunnel",
				})
			}
			select {
			case resp := <-respCh:
				durationMs := time.Since(start).Milliseconds()
				return h.handleActionResponse(c, user.ID, tunnelID, body, toolName, resp, progressMsgs, durationMs)
			default:
				continue drainLoop2
			}
		}
	}

	select {
	case resp := <-respCh:
		durationMs := time.Since(start).Milliseconds()
		return h.handleActionResponse(c, user.ID, tunnelID, body, toolName, resp, nil, durationMs)
	case <-time.After(timeout):
		durationMs := time.Since(start).Milliseconds()
		h.recordHistory(user.ID, tunnelID, body.Type, toolName, body.Params, false, durationMs, "timeout")
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{
			"error": "action timed out waiting for response from tunnel",
		})
	}
}

// SupportedActions handles GET /api/tunnels/:id/actions — returns the list of
// supported smart action types.
func (h *SmartActionHandler) SupportedActions(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"actions": []fiber.Map{
			{"type": "run_prompt", "description": "Send a prompt to the AI agent"},
			{"type": "run_command", "description": "Run a shell command in the workspace"},
			{"type": "run_tool", "description": "Execute any MCP tool by name"},
			{"type": "run_bridge", "description": "Execute a Claude Code bridge command"},
			{"type": "file_read", "description": "Read a file from the desktop workspace"},
			{"type": "file_write", "description": "Write a file to the desktop workspace"},
			{"type": "sync_workspace", "description": "Trigger an immediate cloud sync"},
			{"type": "run_tests", "description": "Run tests in the workspace"},
			{"type": "get_status", "description": "Get workspace and tunnel status"},
			{"type": "list_tools", "description": "List available MCP tools"},
		},
	})
}

// recordHistory saves an action execution to the database (fire-and-forget).
func (h *SmartActionHandler) recordHistory(userID uint, tunnelID, actionType, toolName string, params json.RawMessage, success bool, durationMs int64, errMsg string) {
	entry := models.ActionHistory{
		UserID:     userID,
		TunnelID:   tunnelID,
		ActionType: actionType,
		ToolName:   toolName,
		Success:    success,
		DurationMs: durationMs,
		Error:      errMsg,
	}
	if params != nil {
		entry.Params = datatypes.JSON(params)
	}
	if err := h.db.Create(&entry).Error; err != nil {
		log.Printf("[smart-actions] failed to record history: %v", err)
	}
}

// mapAction converts a smart action request into an MCP tool name and arguments.
func (h *SmartActionHandler) mapAction(req smartActionRequest) (string, map[string]any, error) {
	switch req.Type {
	case "run_prompt":
		var p struct {
			Prompt   string `json:"prompt"`
			Provider string `json:"provider"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return "", nil, fmt.Errorf("invalid run_prompt params: %w", err)
			}
		}
		if p.Prompt == "" {
			return "", nil, fmt.Errorf("prompt is required")
		}
		args := map[string]any{"prompt": p.Prompt, "wait": true}
		if p.Provider != "" {
			args["provider"] = p.Provider
		}
		return "ai_prompt", args, nil

	case "run_command":
		var p struct {
			Command string `json:"command"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return "", nil, fmt.Errorf("invalid run_command params: %w", err)
			}
		}
		if p.Command == "" {
			return "", nil, fmt.Errorf("command is required")
		}
		return "ai_prompt", map[string]any{
			"prompt": fmt.Sprintf("Run this command and return the output: %s", p.Command),
			"wait":   true,
		}, nil

	case "sync_workspace":
		return "sync_now", map[string]any{}, nil

	case "run_tests":
		var p struct {
			Runner  string `json:"runner"`
			Pattern string `json:"pattern"`
		}
		if req.Params != nil {
			_ = json.Unmarshal(req.Params, &p)
		}
		prompt := "Run tests"
		if p.Runner != "" {
			prompt += " using " + p.Runner
		}
		if p.Pattern != "" {
			prompt += " matching " + p.Pattern
		}
		prompt += " and return the results"
		return "ai_prompt", map[string]any{"prompt": prompt, "wait": true}, nil

	case "get_status":
		return "sync_status", map[string]any{}, nil

	case "list_tools":
		return "sync_status", map[string]any{}, nil

	case "run_tool":
		// Execute any MCP tool by name with arbitrary arguments.
		var p struct {
			Tool string         `json:"tool"`
			Args map[string]any `json:"args"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return "", nil, fmt.Errorf("invalid run_tool params: %w", err)
			}
		}
		if p.Tool == "" {
			return "", nil, fmt.Errorf("tool name is required for run_tool")
		}
		args := p.Args
		if args == nil {
			args = map[string]any{}
		}
		return p.Tool, args, nil

	case "run_bridge":
		// Execute a Claude Code bridge command via ai_prompt with a system note.
		var p struct {
			Command string `json:"command"`
			Args    string `json:"args"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return "", nil, fmt.Errorf("invalid run_bridge params: %w", err)
			}
		}
		if p.Command == "" {
			return "", nil, fmt.Errorf("command is required for run_bridge")
		}
		prompt := fmt.Sprintf("Execute bridge command: %s", p.Command)
		if p.Args != "" {
			prompt += " " + p.Args
		}
		return "ai_prompt", map[string]any{"prompt": prompt, "wait": true}, nil

	case "file_read":
		// Read a file from the desktop workspace via the read_file tool.
		var p struct {
			Path string `json:"path"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return "", nil, fmt.Errorf("invalid file_read params: %w", err)
			}
		}
		if p.Path == "" {
			return "", nil, fmt.Errorf("path is required for file_read")
		}
		return "read_file", map[string]any{"path": p.Path}, nil

	case "file_write":
		// Write a file to the desktop workspace via the write_file tool.
		var p struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return "", nil, fmt.Errorf("invalid file_write params: %w", err)
			}
		}
		if p.Path == "" {
			return "", nil, fmt.Errorf("path is required for file_write")
		}
		return "write_file", map[string]any{"path": p.Path, "content": p.Content}, nil

	default:
		return "", nil, fmt.Errorf("unsupported action type: %s", req.Type)
	}
}

// ActionLog handles GET /api/tunnels/:id/action-log — returns stored action log entries
// (full output + files + progress) for a tunnel.
func (h *SmartActionHandler) ActionLog(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tunnelID := c.Params("id")

	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND user_id = ?", tunnelID, user.ID).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tunnel not found"})
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	var entries []models.ActionLog
	q := h.db.Where("tunnel_id = ? AND user_id = ?", tunnelID, user.ID).
		Order("created_at desc")

	if actionType := c.Query("type"); actionType != "" {
		q = q.Where("action_type = ?", actionType)
	}

	var total int64
	q.Model(&models.ActionLog{}).Count(&total)
	q.Offset(offset).Limit(limit).Find(&entries)

	return c.JSON(fiber.Map{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// recordActionLog saves the full action result to the action_log table (fire-and-forget).
func (h *SmartActionHandler) recordActionLog(userID uint, tunnelID, actionType, toolName string, params json.RawMessage, output string, files []string, progress []string, success bool, durationMs int64, errMsg string) {
	entry := models.ActionLog{
		UserID:     userID,
		TunnelID:   tunnelID,
		ActionType: actionType,
		ToolName:   toolName,
		Output:     output,
		Success:    success,
		DurationMs: durationMs,
		Error:      errMsg,
	}
	if params != nil {
		entry.Params = datatypes.JSON(params)
	}
	if files == nil {
		files = []string{}
	}
	if b, err := json.Marshal(files); err == nil {
		entry.Files = datatypes.JSON(b)
	}
	if progress == nil {
		progress = []string{}
	}
	if b, err := json.Marshal(progress); err == nil {
		entry.Progress = datatypes.JSON(b)
	}
	if err := h.db.Create(&entry).Error; err != nil {
		log.Printf("[smart-actions] failed to record action log: %v", err)
	}
}

// extractMCPOutput returns the concatenated text from an MCP tool result content array.
// MCP tool results have the shape: {"content": [{"type": "text", "text": "..."}], "isError": false}
func extractMCPOutput(result json.RawMessage) string {
	if result == nil {
		return ""
	}
	var r struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &r); err != nil {
		return string(result)
	}
	out := ""
	for _, c := range r.Content {
		if c.Type == "text" {
			if out != "" {
				out += "\n"
			}
			out += c.Text
		}
	}
	return out
}

// extractMCPFiles parses file paths from MCP tool result text output.
// Looks for lines starting with "Created: " or "Modified: " as a convention.
func extractMCPFiles(result json.RawMessage) []string {
	text := extractMCPOutput(result)
	var files []string
	for _, line := range splitLines(text) {
		switch {
		case len(line) > 9 && line[:9] == "Created: ":
			files = append(files, line[9:])
		case len(line) > 10 && line[:10] == "Modified: ":
			files = append(files, line[10:])
		}
	}
	return files
}

// splitLines splits a string by newlines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
