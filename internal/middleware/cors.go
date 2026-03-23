package middleware

import "github.com/gofiber/fiber/v3"

// CORS returns a Fiber middleware that validates the request Origin against the
// allowed list and sets credentials-enabled CORS headers. This is required for
// cross-subdomain requests (e.g. orchestra-mcp.dev → api.orchestra-mcp.dev).
func CORS(allowedOrigins []string) fiber.Handler {
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = true
	}

	return func(c fiber.Ctx) error {
		origin := c.Get("Origin")
		if originSet[origin] {
			c.Set("Access-Control-Allow-Origin", origin)
			c.Set("Access-Control-Allow-Credentials", "true")
			c.Set("Vary", "Origin")
		}

		c.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Set("Access-Control-Max-Age", "86400")

		if c.Method() == fiber.MethodOptions {
			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}
