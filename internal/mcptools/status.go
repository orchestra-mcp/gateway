package mcptools

import (
	"github.com/orchestra-mcp/gateway/internal/mcptypes"
)

// newCheckStatusTool returns the check_status tool definition and handler.
// PUBLIC — no authentication required.
func newCheckStatusTool() Tool {
	readOnly := true
	return Tool{
		Permission: "", // public
		Definition: mcptypes.ToolDefinition{
			Name:  "check_status",
			Title: "Check Orchestra Status",
			Description: "Check if Orchestra MCP is installed on the user's machine and return the current version. " +
				"Returns a shell command for Claude to run locally.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &mcptypes.ToolAnnotations{
				Title:        "Check Orchestra Status",
				ReadOnlyHint: &readOnly,
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (mcptypes.ToolResult, error) {
			script := `which orchestra 2>/dev/null && orchestra version || echo "Orchestra is not installed"`
			return mcptypes.ToolResult{
				Content: []mcptypes.Content{
					{
						Type: "text",
						Text: "Run this command to check if Orchestra is installed on your machine:\n\n```bash\n" + script + "\n```\n\nIf it says \"Orchestra is not installed\", call `install_orchestra` to set it up.",
					},
				},
			}, nil
		},
	}
}
