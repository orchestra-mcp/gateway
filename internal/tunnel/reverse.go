package tunnel

import (
	"encoding/json"
	"log"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/orchestra-mcp/gateway/internal/models"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

const (
	// reversePingInterval is how often the cloud pings the local machine.
	reversePingInterval = 30 * time.Second

	// reversePongTimeout is how long to wait for a pong from the local machine.
	reversePongTimeout = 90 * time.Second
)

// TunnelReverseHandler handles inbound WebSocket connections from local machines
// that are establishing a reverse tunnel. The local machine connects outbound to
// the cloud, and the cloud relays browser traffic through this connection.
//
// Auth is via connection_token (not JWT — the local CLI doesn't have a user JWT).
//
// Route: GET /api/tunnels/reverse?tunnel_id=X&connection_token=Y
type TunnelReverseHandler struct {
	db       *gorm.DB
	hub      *TunnelHub
	upgrader websocket.FastHTTPUpgrader
}

// NewTunnelReverseHandler creates a new TunnelReverseHandler.
func NewTunnelReverseHandler(db *gorm.DB, hub *TunnelHub) *TunnelReverseHandler {
	return &TunnelReverseHandler{
		db:  db,
		hub: hub,
		upgrader: websocket.FastHTTPUpgrader{
			ReadBufferSize:  16 * 1024,
			WriteBufferSize: 16 * 1024,
			CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
				return true
			},
		},
	}
}

// Handle upgrades an HTTP request from a local machine to a persistent reverse
// tunnel WebSocket. Messages on this connection are relay envelopes containing
// a browser session ID and a JSON-RPC payload.
func (h *TunnelReverseHandler) Handle(c fiber.Ctx) error {
	tunnelID := c.Query("tunnel_id")
	connToken := c.Query("connection_token")

	if tunnelID == "" || connToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "tunnel_id and connection_token query parameters are required",
		})
	}

	// Authenticate via connection_token.
	var tunnel models.Tunnel
	if err := h.db.Where("id = ? AND connection_token = ?", tunnelID, connToken).
		First(&tunnel).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "invalid tunnel_id or connection_token",
		})
	}

	// Upgrade to WebSocket.
	err := h.upgrader.Upgrade(c.Context(), func(conn *websocket.Conn) {
		// Register in the hub (replaces any existing connection for this tunnel).
		rc := h.hub.RegisterReverse(tunnelID, conn)

		// Mark tunnel online.
		now := time.Now()
		h.db.Model(&tunnel).Updates(map[string]any{
			"status":       models.TunnelStatusOnline,
			"last_seen_at": now,
		})

		log.Printf("[tunnel-reverse] tunnel %s (%s) connected", tunnelID, tunnel.Name)

		// Run the read loop (blocks until connection closes).
		h.readLoop(rc, tunnelID)

		// Cleanup: unregister and mark offline.
		h.hub.UnregisterReverse(tunnelID)
		h.db.Model(&tunnel).Update("status", models.TunnelStatusOffline)
		log.Printf("[tunnel-reverse] tunnel %s (%s) disconnected", tunnelID, tunnel.Name)
	})
	if err != nil {
		log.Printf("[tunnel-reverse] upgrade error for tunnel %s: %v", tunnelID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "websocket upgrade failed",
		})
	}

	return nil
}

// readLoop reads relay envelopes from the local machine and routes responses
// to the correct browser session. It also manages ping/pong keepalive.
func (h *TunnelReverseHandler) readLoop(rc *ReverseConn, tunnelID string) {
	conn := rc.Conn
	conn.SetReadLimit(proxyReadLimit)

	// Pong handler resets the read deadline when the local machine responds.
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(reversePongTimeout))
		return nil
	})
	conn.SetReadDeadline(time.Now().Add(reversePongTimeout))

	// Ping ticker — keeps the reverse connection alive through proxies/firewalls.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(reversePingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rc.WriteMu.Lock()
				conn.SetWriteDeadline(time.Now().Add(proxyWriteTimeout))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				rc.WriteMu.Unlock()
				if err != nil {
					return
				}
				// Update last_seen_at on successful ping.
				h.db.Model(&models.Tunnel{}).Where("id = ?", tunnelID).
					Update("last_seen_at", time.Now())
			case <-done:
				return
			}
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[tunnel-reverse] read error for tunnel %s: %v", tunnelID, err)
			}
			return
		}

		// Parse the relay envelope from the local machine.
		var envelope RelayEnvelope
		if err := json.Unmarshal(msg, &envelope); err != nil {
			log.Printf("[tunnel-reverse] invalid envelope from tunnel %s: %v", tunnelID, err)
			continue
		}

		if envelope.RelayTo == "" {
			continue
		}

		// Route the response to the correct browser session.
		if err := h.hub.ForwardToBrowser(envelope.RelayTo, envelope.Message); err != nil {
			log.Printf("[tunnel-reverse] forward to browser %s failed: %v", envelope.RelayTo, err)
		}
	}
}
