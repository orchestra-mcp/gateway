// Package mcp contains MCP 2025-11-25 transport handlers.
// Shared types live in mcptypes to break the mcp ↔ mcptools import cycle.
// This file re-exports them so existing handler code continues to compile.
package mcp

import "github.com/orchestra-mcp/gateway/internal/mcptypes"

// Re-exported constants.
const (
	ProtocolVersion = mcptypes.ProtocolVersion

	CodeParseError     = mcptypes.CodeParseError
	CodeInvalidRequest = mcptypes.CodeInvalidRequest
	CodeMethodNotFound = mcptypes.CodeMethodNotFound
	CodeInvalidParams  = mcptypes.CodeInvalidParams
	CodeInternalError  = mcptypes.CodeInternalError
)

// Re-exported type aliases so handler.go compiles without changes.
type (
	Request            = mcptypes.Request
	Response           = mcptypes.Response
	RPCError           = mcptypes.RPCError
	ServerInfo         = mcptypes.ServerInfo
	Icon               = mcptypes.Icon
	ToolAnnotations    = mcptypes.ToolAnnotations
	ToolDefinition     = mcptypes.ToolDefinition
	Content            = mcptypes.Content
	ToolResult         = mcptypes.ToolResult
	ServerCapabilities = mcptypes.ServerCapabilities
	ToolsCapability    = mcptypes.ToolsCapability
	InitializeResult   = mcptypes.InitializeResult
)

// Re-exported helper functions.
var (
	TextResult  = mcptypes.TextResult
	ErrorResult = mcptypes.ErrorResult
	BoolPtr     = mcptypes.BoolPtr
)
