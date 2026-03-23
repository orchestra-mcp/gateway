package models

import (
	"time"

	"gorm.io/datatypes"
)

// TunnelStatus represents the connection state of a tunnel.
type TunnelStatus string

const (
	TunnelStatusOnline     TunnelStatus = "online"
	TunnelStatusOffline    TunnelStatus = "offline"
	TunnelStatusConnecting TunnelStatus = "connecting"
)

// Tunnel represents a registered machine running `orchestra serve --web-gate`.
// Each tunnel provides remote access to a machine's MCP tools via WebSocket.
type Tunnel struct {
	Base
	UserID          uint           `gorm:"index;not null" json:"user_id"`
	TeamID          *string        `gorm:"index" json:"team_id,omitempty"`
	Name            string         `json:"name"`
	Hostname        string         `json:"hostname"`
	OS              string         `json:"os"`
	Architecture    string         `json:"architecture"`
	GateAddress     string         `json:"gate_address"`
	ConnectionToken string         `json:"-"` // encrypted, never exposed in API responses
	Status          TunnelStatus   `gorm:"default:offline" json:"status"`
	LastSeenAt      *time.Time     `json:"last_seen_at,omitempty"`
	Labels          datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"labels"`
	Meta            datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"meta"`
	Version         string         `json:"version"`
	ToolCount       int            `gorm:"default:0" json:"tool_count"`
	LocalIP         string         `json:"local_ip,omitempty"`
	Workspace       string         `json:"workspace,omitempty"`
}
