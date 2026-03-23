package middleware

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

// Logger returns a middleware that logs every HTTP request with method, path,
// status code, duration, request body, and response body.
func Logger() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		// Capture request body (skip for GET/HEAD/OPTIONS and large bodies).
		var reqBody string
		if c.Method() != "GET" && c.Method() != "HEAD" && c.Method() != "OPTIONS" {
			body := c.Body()
			if len(body) > 0 && len(body) <= 4096 {
				reqBody = string(body)
			} else if len(body) > 4096 {
				reqBody = string(body[:4096]) + "...(truncated)"
			}
		}

		err := c.Next()
		duration := time.Since(start)

		status := c.Response().StatusCode()
		resBody := c.Response().Body()
		var resStr string
		if len(resBody) > 0 && len(resBody) <= 4096 {
			resStr = string(resBody)
		} else if len(resBody) > 4096 {
			resStr = string(resBody[:4096]) + "...(truncated)"
		}

		// Skip logging for static files / robots.txt.
		path := c.Path()
		if path == "/robots.txt" || strings.HasPrefix(path, "/uploads/") {
			return err
		}

		var b strings.Builder
		b.WriteString(c.Method())
		b.WriteByte(' ')
		b.WriteString(path)
		b.WriteString(" → ")
		b.WriteString(strconv.Itoa(status))
		b.WriteString(" (")
		b.WriteString(duration.Round(time.Microsecond).String())
		b.WriteByte(')')

		if reqBody != "" {
			b.WriteString("\n  ← ")
			b.WriteString(reqBody)
		}
		if resStr != "" {
			b.WriteString("\n  → ")
			b.WriteString(resStr)
		}

		log.Println(b.String())
		return err
	}
}
