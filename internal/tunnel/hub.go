package tunnel

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
)

// RelayEnvelope wraps JSON-RPC messages with a browser session ID so multiple
// browser connections can share a single reverse tunnel WebSocket.
type RelayEnvelope struct {
	RelayTo string          `json:"relay_to"`       // browser session ID (opaque)
	Type    string          `json:"type,omitempty"` // "close" when browser disconnects
	Message json.RawMessage `json:"message"`        // raw JSON-RPC message
}

// ReverseConn represents a persistent WebSocket from a local machine to the cloud.
type ReverseConn struct {
	TunnelID string
	Conn     *websocket.Conn
	WriteMu  sync.Mutex
}

// WriteJSON sends a JSON message to the reverse connection (thread-safe).
func (rc *ReverseConn) WriteJSON(v any) error {
	rc.WriteMu.Lock()
	defer rc.WriteMu.Unlock()
	rc.Conn.SetWriteDeadline(time.Now().Add(proxyWriteTimeout))
	return rc.Conn.WriteJSON(v)
}

// BrowserConn represents a browser's WebSocket connection to a tunnel.
type BrowserConn struct {
	TunnelID  string
	SessionID string
	Conn      *websocket.Conn
	WriteMu   sync.Mutex
}

// WriteMessage sends a raw message to the browser (thread-safe).
func (bc *BrowserConn) WriteMessage(msgType int, data []byte) error {
	bc.WriteMu.Lock()
	defer bc.WriteMu.Unlock()
	bc.Conn.SetWriteDeadline(time.Now().Add(proxyWriteTimeout))
	return bc.Conn.WriteMessage(msgType, data)
}

// smartActionEntry holds the response channel and optional progress channel
// for a REST-based smart action.
type smartActionEntry struct {
	respCh     chan json.RawMessage // receives the final JSON-RPC response
	progressCh chan string          // receives intermediate progress strings (buffered, optional)
}

// TunnelHub manages active reverse tunnel connections and browser sessions.
// It acts as a relay: browser messages are forwarded to the reverse tunnel,
// and responses from the reverse tunnel are routed back to the correct browser.
type TunnelHub struct {
	mu           sync.RWMutex
	tunnels      map[string]*ReverseConn          // tunnel ID → active reverse connection
	browsers     map[string]*BrowserConn          // session ID → browser websocket
	smartActions map[string]*smartActionEntry      // session ID → response+progress channels (REST smart actions)
}

// NewTunnelHub creates a new TunnelHub.
func NewTunnelHub() *TunnelHub {
	return &TunnelHub{
		tunnels:      make(map[string]*ReverseConn),
		browsers:     make(map[string]*BrowserConn),
		smartActions: make(map[string]*smartActionEntry),
	}
}

// RegisterReverse registers a reverse tunnel connection. Returns false if a
// connection already exists for this tunnel (caller should close the old one first).
func (h *TunnelHub) RegisterReverse(tunnelID string, conn *websocket.Conn) *ReverseConn {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If an existing connection exists, close it (new connection supersedes).
	if old, ok := h.tunnels[tunnelID]; ok {
		old.Conn.Close()
		log.Printf("[tunnel-hub] replaced existing reverse connection for tunnel %s", tunnelID)
	}

	rc := &ReverseConn{
		TunnelID: tunnelID,
		Conn:     conn,
	}
	h.tunnels[tunnelID] = rc
	log.Printf("[tunnel-hub] registered reverse connection for tunnel %s", tunnelID)
	return rc
}

// UnregisterReverse removes a reverse tunnel connection and closes all browser
// sessions connected to that tunnel.
func (h *TunnelHub) UnregisterReverse(tunnelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.tunnels, tunnelID)

	// Close all browser connections for this tunnel.
	for sid, bc := range h.browsers {
		if bc.TunnelID == tunnelID {
			bc.Conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "tunnel disconnected"),
				time.Now().Add(time.Second),
			)
			bc.Conn.Close()
			delete(h.browsers, sid)
		}
	}
	log.Printf("[tunnel-hub] unregistered reverse connection for tunnel %s", tunnelID)
}

// GetReverse returns the active reverse connection for a tunnel, or nil.
func (h *TunnelHub) GetReverse(tunnelID string) *ReverseConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.tunnels[tunnelID]
}

// HasReverse returns true if a tunnel has an active reverse connection.
func (h *TunnelHub) HasReverse(tunnelID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.tunnels[tunnelID]
	return ok
}

// RegisterBrowser registers a browser WebSocket connection for a tunnel.
func (h *TunnelHub) RegisterBrowser(sessionID, tunnelID string, conn *websocket.Conn) *BrowserConn {
	h.mu.Lock()
	defer h.mu.Unlock()

	bc := &BrowserConn{
		TunnelID:  tunnelID,
		SessionID: sessionID,
		Conn:      conn,
	}
	h.browsers[sessionID] = bc
	log.Printf("[tunnel-hub] registered browser session %s for tunnel %s", sessionID, tunnelID)
	return bc
}

// UnregisterBrowser removes a browser session.
func (h *TunnelHub) UnregisterBrowser(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.browsers, sessionID)
	log.Printf("[tunnel-hub] unregistered browser session %s", sessionID)
}

// ForwardToGate wraps a browser message in a relay envelope and sends it to
// the reverse tunnel connection.
func (h *TunnelHub) ForwardToGate(tunnelID, sessionID string, message json.RawMessage) error {
	h.mu.RLock()
	rc, ok := h.tunnels[tunnelID]
	h.mu.RUnlock()

	if !ok {
		return ErrTunnelNotConnected
	}

	envelope := RelayEnvelope{
		RelayTo: sessionID,
		Message: message,
	}
	return rc.WriteJSON(envelope)
}

// ForwardToBrowser routes a response from the reverse tunnel to the correct
// browser session or smart action channel.
//
// Progress envelopes (type == "progress") are forwarded to the progress channel
// if one was registered; all other messages go to the response channel.
// Final responses (any non-progress message) are sent to respCh.
func (h *TunnelHub) ForwardToBrowser(sessionID string, message json.RawMessage) error {
	h.mu.RLock()
	// Check smart action entries first (REST-based actions).
	if entry, ok := h.smartActions[sessionID]; ok {
		h.mu.RUnlock()

		// Peek at the envelope type to detect progress messages.
		var peek struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if entry.progressCh != nil && json.Unmarshal(message, &peek) == nil && peek.Type == "progress" {
			select {
			case entry.progressCh <- peek.Text:
			default:
			}
			return nil
		}

		// Final response — send to respCh (buffered 1, non-blocking).
		select {
		case entry.respCh <- message:
		default:
		}
		return nil
	}
	bc, ok := h.browsers[sessionID]
	h.mu.RUnlock()

	if !ok {
		// Browser disconnected — not an error, just discard.
		return nil
	}

	return bc.WriteMessage(websocket.TextMessage, message)
}

// RegisterSmartAction registers a response channel for a REST-based smart action.
// Optionally provide a progress channel to receive intermediate progress strings.
// Pass nil for progressCh if progress streaming is not needed.
func (h *TunnelHub) RegisterSmartAction(sessionID string, respCh chan json.RawMessage, progressCh chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.smartActions[sessionID] = &smartActionEntry{
		respCh:     respCh,
		progressCh: progressCh,
	}
}

// UnregisterSmartAction removes a smart action response channel.
func (h *TunnelHub) UnregisterSmartAction(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.smartActions, sessionID)
}

// NotifyGateClose sends a close envelope to the reverse tunnel when a browser
// disconnects, so the local machine can clean up any session state.
func (h *TunnelHub) NotifyGateClose(tunnelID, sessionID string) {
	h.mu.RLock()
	rc, ok := h.tunnels[tunnelID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	envelope := RelayEnvelope{
		RelayTo: sessionID,
		Type:    "close",
	}
	if err := rc.WriteJSON(envelope); err != nil {
		log.Printf("[tunnel-hub] failed to send close to tunnel %s: %v", tunnelID, err)
	}
}

// ErrTunnelNotConnected is returned when no reverse connection exists for a tunnel.
var ErrTunnelNotConnected = &tunnelNotConnectedError{}

type tunnelNotConnectedError struct{}

func (e *tunnelNotConnectedError) Error() string {
	return "tunnel is not connected via reverse tunnel"
}
