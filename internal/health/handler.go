package health

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/orchestra-mcp/gateway/internal/middleware"
)

// HealthHandler handles HTTP requests for the Health Debug API.
type HealthHandler struct {
	svc HealthService
}

// NewHealthHandler creates a HealthHandler with the given service.
func NewHealthHandler(svc HealthService) *HealthHandler {
	return &HealthHandler{svc: svc}
}

// GetProfile returns the health profile for the authenticated user.
// GET /api/health/profile
func (h *HealthHandler) GetProfile(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	profile, err := h.svc.GetProfile(c.Context(), user.ID)
	if err != nil {
		return healthErrorResponse(c, err)
	}
	return c.JSON(fiber.Map{"data": profile})
}

// UpdateProfile updates the health profile for the authenticated user.
// PUT /api/health/profile
func (h *HealthHandler) UpdateProfile(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	var req UpdateProfileRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   "invalid_body",
			"message": err.Error(),
		})
	}

	profile, err := h.svc.UpdateProfile(c.Context(), user.ID, req)
	if err != nil {
		return healthErrorResponse(c, err)
	}
	return c.JSON(fiber.Map{"data": profile})
}

// LogWater records a water intake entry for the authenticated user.
// POST /api/health/water
func (h *HealthHandler) LogWater(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	var req LogWaterRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   "invalid_body",
			"message": err.Error(),
		})
	}

	entry, err := h.svc.LogWater(c.Context(), user.ID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": entry})
}

// ListWaterLogs returns water intake entries for a given date.
// GET /api/health/water?date=2006-01-02
func (h *HealthHandler) ListWaterLogs(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	date := healthParseDate(c.Query("date"))

	logs, err := h.svc.ListWaterLogs(c.Context(), user.ID, date)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{"data": logs})
}

// GetHydrationStatus returns the computed hydration status for the authenticated user.
// GET /api/health/water/status
func (h *HealthHandler) GetHydrationStatus(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	status, err := h.svc.GetHydrationStatus(c.Context(), user.ID)
	if err != nil {
		return healthErrorResponse(c, err)
	}
	return c.JSON(fiber.Map{"data": status})
}

// LogMeal records a meal entry for the authenticated user.
// POST /api/health/meals
func (h *HealthHandler) LogMeal(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	var req LogMealRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   "invalid_body",
			"message": err.Error(),
		})
	}

	entry, err := h.svc.LogMeal(c.Context(), user.ID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": entry})
}

// ListMealLogs returns meal entries for a given date.
// GET /api/health/meals?date=2006-01-02
func (h *HealthHandler) ListMealLogs(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	date := healthParseDate(c.Query("date"))

	logs, err := h.svc.ListMealLogs(c.Context(), user.ID, date)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{"data": logs})
}

// LogCaffeine records a caffeine intake entry for the authenticated user.
// POST /api/health/caffeine
func (h *HealthHandler) LogCaffeine(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	var req LogCaffeineRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   "invalid_body",
			"message": err.Error(),
		})
	}

	entry, err := h.svc.LogCaffeine(c.Context(), user.ID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": entry})
}

// ListCaffeineLogs returns caffeine intake entries for a given date.
// GET /api/health/caffeine?date=2006-01-02
func (h *HealthHandler) ListCaffeineLogs(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	date := healthParseDate(c.Query("date"))

	logs, err := h.svc.ListCaffeineLogs(c.Context(), user.ID, date)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{"data": logs})
}

// GetCaffeineScore returns the computed caffeine score for the authenticated user.
// GET /api/health/caffeine/score
func (h *HealthHandler) GetCaffeineScore(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	score, err := h.svc.GetCaffeineScore(c.Context(), user.ID)
	if err != nil {
		return healthErrorResponse(c, err)
	}
	return c.JSON(fiber.Map{"data": score})
}

// StartPomodoro begins a new pomodoro session for the authenticated user.
// POST /api/health/pomodoro/start
func (h *HealthHandler) StartPomodoro(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	var req StartPomodoroRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   "invalid_body",
			"message": err.Error(),
		})
	}

	session, err := h.svc.StartPomodoro(c.Context(), user.ID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": session})
}

// EndPomodoro completes an active pomodoro session.
// POST /api/health/pomodoro/:id/end
func (h *HealthHandler) EndPomodoro(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	var req EndPomodoroRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   "invalid_body",
			"message": err.Error(),
		})
	}

	session, err := h.svc.EndPomodoro(c.Context(), user.ID, c.Params("id"), req)
	if err != nil {
		return healthErrorResponse(c, err)
	}
	return c.JSON(fiber.Map{"data": session})
}

// ListPomodoroSessions returns pomodoro sessions for a given date.
// GET /api/health/pomodoro?date=2006-01-02
func (h *HealthHandler) ListPomodoroSessions(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	date := healthParseDate(c.Query("date"))

	sessions, err := h.svc.ListPomodoroSessions(c.Context(), user.ID, date)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{"data": sessions})
}

// LogSleep records a sleep session for the authenticated user.
// POST /api/health/sleep
func (h *HealthHandler) LogSleep(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	var req LogSleepRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   "invalid_body",
			"message": err.Error(),
		})
	}

	entry, err := h.svc.LogSleep(c.Context(), user.ID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": entry})
}

// ListSleepLogs returns sleep log entries for a given date.
// GET /api/health/sleep?date=2006-01-02
func (h *HealthHandler) ListSleepLogs(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	date := healthParseDate(c.Query("date"))

	logs, err := h.svc.ListSleepLogs(c.Context(), user.ID, date)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{"data": logs})
}

// GetShutdownStatus returns the computed shutdown ritual status for the authenticated user.
// GET /api/health/shutdown/status
func (h *HealthHandler) GetShutdownStatus(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	status, err := h.svc.GetShutdownStatus(c.Context(), user.ID)
	if err != nil {
		return healthErrorResponse(c, err)
	}
	return c.JSON(fiber.Map{"data": status})
}

// StartShutdown initiates the shutdown ritual for the authenticated user.
// POST /api/health/shutdown/start
func (h *HealthHandler) StartShutdown(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	result, err := h.svc.StartShutdown(c.Context(), user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{"data": result})
}

// UpsertSnapshot creates or updates a health snapshot for the authenticated user.
// POST /api/health/snapshots
func (h *HealthHandler) UpsertSnapshot(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	var req UpsertSnapshotRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   "invalid_body",
			"message": err.Error(),
		})
	}

	snapshot, err := h.svc.UpsertSnapshot(c.Context(), user.ID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{"data": snapshot})
}

// ListSnapshots returns health snapshots within a date range.
// GET /api/health/snapshots?from=2006-01-02&to=2006-01-02
func (h *HealthHandler) ListSnapshots(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	from := healthParseDate(c.Query("from"))
	to := healthParseDate(c.Query("to"))

	snapshots, err := h.svc.ListSnapshots(c.Context(), user.ID, from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{"data": snapshots})
}

// GetHealthSummary returns a comprehensive daily health summary for the authenticated user.
// GET /api/health/summary
func (h *HealthHandler) GetHealthSummary(c fiber.Ctx) error {
	user := middleware.CurrentUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	summary, err := h.svc.GetHealthSummary(c.Context(), user.ID)
	if err != nil {
		return healthErrorResponse(c, err)
	}
	return c.JSON(fiber.Map{"data": summary})
}

// RegisterHealthRoutes registers all health debug routes on a Fiber route group.
func RegisterHealthRoutes(group fiber.Router, h *HealthHandler) {
	health := group.Group("/health")

	health.Get("/profile", h.GetProfile)
	health.Put("/profile", h.UpdateProfile)

	health.Post("/water", h.LogWater)
	health.Get("/water", h.ListWaterLogs)
	health.Get("/water/status", h.GetHydrationStatus)

	health.Post("/meals", h.LogMeal)
	health.Get("/meals", h.ListMealLogs)

	health.Post("/caffeine", h.LogCaffeine)
	health.Get("/caffeine", h.ListCaffeineLogs)
	health.Get("/caffeine/score", h.GetCaffeineScore)

	health.Post("/pomodoro/start", h.StartPomodoro)
	health.Post("/pomodoro/:id/end", h.EndPomodoro)
	health.Get("/pomodoro", h.ListPomodoroSessions)

	health.Post("/sleep", h.LogSleep)
	health.Get("/sleep", h.ListSleepLogs)

	health.Get("/shutdown/status", h.GetShutdownStatus)
	health.Post("/shutdown/start", h.StartShutdown)

	health.Post("/snapshots", h.UpsertSnapshot)
	health.Get("/snapshots", h.ListSnapshots)

	health.Get("/summary", h.GetHealthSummary)
}

// healthErrorResponse maps known service errors to the correct HTTP status.
func healthErrorResponse(c fiber.Ctx, err error) error {
	if strings.Contains(err.Error(), "not found") {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":   "not_found",
			"message": err.Error(),
		})
	}
	if strings.Contains(err.Error(), "forbidden") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":   "forbidden",
			"message": err.Error(),
		})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"error":   "internal_error",
		"message": err.Error(),
	})
}

// healthParseDate parses a YYYY-MM-DD string into a time.Time, defaulting to today on failure.
func healthParseDate(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Now()
	}
	return t
}
