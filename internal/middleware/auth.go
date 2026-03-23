package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/orchestra-mcp/gateway/internal/config"
	"github.com/orchestra-mcp/gateway/internal/models"
	"gorm.io/gorm"
)

// Claims extends jwt.RegisteredClaims with user identity fields.
type Claims struct {
	UserID    uint     `json:"user_id"`
	Email     string   `json:"email"`
	Abilities []string `json:"abilities"`
	jwt.RegisteredClaims
}

// Auth returns a Fiber middleware that validates JWT Bearer tokens or orch_* API keys.
func Auth(db *gorm.DB, cfg *config.Config) fiber.Handler {
	return func(c fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing authorization header",
			})
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid authorization header format",
			})
		}

		tokenStr := parts[1]

		// API key authentication: orch_* tokens are looked up by hash.
		if strings.HasPrefix(tokenStr, "orch_") {
			user, err := authenticateAPIKey(db, tokenStr)
			if err != nil {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "invalid API key",
				})
			}
			c.Locals("user", user)
			return c.Next()
		}

		// JWT authentication.
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.NewError(fiber.StatusUnauthorized, "unexpected signing method")
			}
			return []byte(cfg.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or expired token",
			})
		}

		var user models.User
		if err := db.First(&user, claims.UserID).Error; err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "user not found",
			})
		}

		if user.Status == "blocked" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "account is blocked",
			})
		}

		c.Locals("user", &user)
		return c.Next()
	}
}

// authenticateAPIKey finds a user whose settings contain an API key matching the given token.
func authenticateAPIKey(db *gorm.DB, token string) (*models.User, error) {
	h := sha256.Sum256([]byte(token))
	keyHash := hex.EncodeToString(h[:])

	// Query users whose settings JSON contains this hash.
	var users []models.User
	if err := db.Where("settings::text LIKE ?", "%"+keyHash+"%").Find(&users).Error; err != nil {
		return nil, err
	}

	for i := range users {
		var meta map[string]interface{}
		if err := json.Unmarshal(users[i].Settings, &meta); err != nil {
			continue
		}

		// Check mcp_token_hash (tokens generated before api_keys migration).
		if mcpHash, _ := meta["mcp_token_hash"].(string); mcpHash == keyHash {
			if users[i].Status == "blocked" {
				return nil, fiber.NewError(fiber.StatusForbidden, "account is blocked")
			}
			return &users[i], nil
		}

		// Check api_keys list.
		raw, ok := meta["api_keys"]
		if !ok {
			continue
		}
		b, _ := json.Marshal(raw)
		var keys []struct {
			ID   string `json:"id"`
			Hash string `json:"hash"`
		}
		if json.Unmarshal(b, &keys) != nil {
			continue
		}
		for _, k := range keys {
			if k.Hash == keyHash {
				if users[i].Status == "blocked" {
					return nil, fiber.NewError(fiber.StatusForbidden, "account is blocked")
				}
				// Update last_used timestamp.
				go updateKeyLastUsed(db, &users[i], k.ID)
				return &users[i], nil
			}
		}
	}
	return nil, fiber.NewError(fiber.StatusUnauthorized, "API key not found")
}

// updateKeyLastUsed updates the last_used field of the matching API key.
func updateKeyLastUsed(db *gorm.DB, user *models.User, keyID string) {
	var meta map[string]interface{}
	if json.Unmarshal(user.Settings, &meta) != nil {
		return
	}
	raw, ok := meta["api_keys"]
	if !ok {
		return
	}
	b, _ := json.Marshal(raw)
	var keys []map[string]interface{}
	if json.Unmarshal(b, &keys) != nil {
		return
	}
	for i, k := range keys {
		if id, _ := k["id"].(string); id == keyID {
			keys[i]["last_used"] = time.Now().Format(time.RFC3339)
			break
		}
	}
	meta["api_keys"] = keys
	metaJSON, _ := json.Marshal(meta)
	db.Model(user).Update("settings", metaJSON)
}

// CurrentUser extracts the authenticated user from the Fiber context.
func CurrentUser(c fiber.Ctx) *models.User {
	if u, ok := c.Locals("user").(*models.User); ok {
		return u
	}
	return nil
}

// OptionalCurrentUser tries to extract a user from the Authorization header
// on public routes (where auth middleware doesn't run). Returns nil on failure.
func OptionalCurrentUser(c fiber.Ctx, db *gorm.DB) *models.User {
	auth := c.Get("Authorization")
	if auth == "" {
		return nil
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return nil
	}
	tokenStr := parts[1]
	if tokenStr == "" || strings.HasPrefix(tokenStr, "orch_") {
		return nil
	}

	cfg := config.Load()
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fiber.NewError(fiber.StatusUnauthorized, "bad method")
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil
	}

	var user models.User
	if err := db.First(&user, claims.UserID).Error; err != nil {
		return nil
	}
	return &user
}
