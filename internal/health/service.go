package health

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/lib/pq"
	"github.com/orchestra-mcp/gateway/internal/models"
	"gorm.io/gorm"
)

// ErrHealthNotFound is returned when a health entity has no matching row.
var ErrHealthNotFound = errors.New("health entity not found")

// ErrHealthForbidden is returned when the caller does not own the health entity.
var ErrHealthForbidden = errors.New("forbidden")

// ---------------------------------------------------------------------------
// Request DTOs
// ---------------------------------------------------------------------------

// UpdateProfileRequest is the input shape for HealthService.UpdateProfile.
type UpdateProfileRequest struct {
	HeightCm             *float64 `json:"height_cm,omitempty"`
	WeightKg             *float64 `json:"weight_kg,omitempty"`
	TargetWeightKg       *float64 `json:"target_weight_kg,omitempty"`
	TargetWaterMl        *int     `json:"target_water_ml,omitempty"`
	WorkWindowStart      string   `json:"work_window_start,omitempty"`
	WorkWindowEnd        string   `json:"work_window_end,omitempty"`
	TargetSleepTime      string   `json:"target_sleep_time,omitempty"`
	WakeTime             string   `json:"wake_time,omitempty"`
	PomodoroDurationMin  *int     `json:"pomodoro_duration_min,omitempty"`
	PomodoroBreakMin     *int     `json:"pomodoro_break_min,omitempty"`
	PomodoroLongBreakMin *int     `json:"pomodoro_long_break_min,omitempty"`
	PomodoroDailyTarget  *int     `json:"pomodoro_daily_target,omitempty"`
	GerdShutdownHours    *int     `json:"gerd_shutdown_hours,omitempty"`
	CaffeineDelayMin     *int     `json:"caffeine_delay_min,omitempty"`
	Conditions           []string `json:"conditions,omitempty"`
}

// LogWaterRequest is the input shape for HealthService.LogWater.
type LogWaterRequest struct {
	AmountMl    int    `json:"amount_ml"`
	Source      string `json:"source,omitempty"`
	IsGoutFlush bool   `json:"is_gout_flush"`
}

// LogMealRequest is the input shape for HealthService.LogMeal.
type LogMealRequest struct {
	Name     string   `json:"name"`
	IsSafe   bool     `json:"is_safe"`
	Category string   `json:"category,omitempty"`
	Triggers []string `json:"triggers,omitempty"`
	Notes    string   `json:"notes,omitempty"`
}

// LogCaffeineRequest is the input shape for HealthService.LogCaffeine.
type LogCaffeineRequest struct {
	DrinkType  string  `json:"drink_type"`
	IsClean    bool    `json:"is_clean"`
	CaffeineMg int     `json:"caffeine_mg,omitempty"`
	SugarG     float64 `json:"sugar_g,omitempty"`
}

// StartPomodoroRequest is the input shape for HealthService.StartPomodoro.
type StartPomodoroRequest struct {
	Type string `json:"type"` // work, break, stand
}

// EndPomodoroRequest is the input shape for HealthService.EndPomodoro.
type EndPomodoroRequest struct {
	Completed bool    `json:"completed"`
	StoodUp   bool    `json:"stood_up"`
	WalkedMin float64 `json:"walked_min"`
}

// LogSleepRequest is the input shape for HealthService.LogSleep.
type LogSleepRequest struct {
	BedTime       string `json:"bed_time"`
	WakeTime      string `json:"wake_time"`
	QualityRating int    `json:"quality_rating,omitempty"`
}

// UpsertSnapshotRequest is the input shape for HealthService.UpsertSnapshot.
type UpsertSnapshotRequest struct {
	SnapshotDate            string   `json:"snapshot_date"`
	WeightKg                *float64 `json:"weight_kg,omitempty"`
	BodyFatPct              *float64 `json:"body_fat_pct,omitempty"`
	VisceralFat             *int     `json:"visceral_fat,omitempty"`
	BodyWaterPct            *float64 `json:"body_water_pct,omitempty"`
	MetabolicAge            *int     `json:"metabolic_age,omitempty"`
	Steps                   int      `json:"steps"`
	ActiveEnergyCal         float64  `json:"active_energy_cal"`
	AvgHeartRate            *int     `json:"avg_heart_rate,omitempty"`
	SleepHours              float64  `json:"sleep_hours"`
	WaterTotalMl            int      `json:"water_total_ml"`
	MealsSafe               int      `json:"meals_safe"`
	MealsUnsafe             int      `json:"meals_unsafe"`
	CaffeineCleanCount      int      `json:"caffeine_clean_count"`
	CaffeineSugarCount      int      `json:"caffeine_sugar_count"`
	PomodorosCompleted      int      `json:"pomodoros_completed"`
	StandSessions           int      `json:"stand_sessions"`
	GerdShutdownCompliant   *bool    `json:"gerd_shutdown_compliant,omitempty"`
	NutritionSafetyScore    *int     `json:"nutrition_safety_score,omitempty"`
	CaffeineTransitionScore *int     `json:"caffeine_transition_score,omitempty"`
	Source                  string   `json:"source,omitempty"`
}

// ---------------------------------------------------------------------------
// Computed Status Types
// ---------------------------------------------------------------------------

// HydrationStatus is the computed real-time hydration state.
type HydrationStatus struct {
	TotalMl           int     `json:"total_ml"`
	TargetMl          int     `json:"target_ml"`
	DeficitMl         int     `json:"deficit_ml"`
	Percentage        float64 `json:"percentage"`
	HourlyRateNeeded  float64 `json:"hourly_rate_needed"`
	Status            string  `json:"status"` // on_track, slightly_behind, dehydrated, goal_reached
	IsGoutFlushNeeded bool    `json:"is_gout_flush_needed"`
}

// CaffeineScore is the computed caffeine transition state.
type CaffeineScore struct {
	CleanCount           int    `json:"clean_count"`
	SugarCount           int    `json:"sugar_count"`
	TransitionScore      int    `json:"transition_score"` // 0-100
	Status               string `json:"status"`           // clean, transitioning, dependent
	WithinCortisolWindow bool   `json:"within_cortisol_window"`
}

// ShutdownStatus is the computed GERD shutdown state.
type ShutdownStatus struct {
	Active               bool     `json:"active"`
	MinutesUntilShutdown int      `json:"minutes_until_shutdown"`
	MinutesSinceLastMeal int      `json:"minutes_since_last_meal"`
	AllowedItems         []string `json:"allowed_items"`
	Compliant            bool     `json:"compliant"`
}

// NutritionSafety is the computed meal safety state.
type NutritionSafety struct {
	MealsSafe     int            `json:"meals_safe"`
	MealsUnsafe   int            `json:"meals_unsafe"`
	SafetyScore   int            `json:"safety_score"` // 0-100
	TriggerCounts map[string]int `json:"trigger_counts"`
}

// HealthSummary aggregates all computed health statuses into one response.
type HealthSummary struct {
	Hydration          HydrationStatus `json:"hydration"`
	Caffeine           CaffeineScore   `json:"caffeine"`
	Shutdown           ShutdownStatus  `json:"shutdown"`
	Nutrition          NutritionSafety `json:"nutrition"`
	PomodorosCompleted int             `json:"pomodoros_completed"`
	PomodoroTarget     int             `json:"pomodoro_target"`
	OverallScore       int             `json:"overall_score"` // 0-100
}

// ---------------------------------------------------------------------------
// Interface
// ---------------------------------------------------------------------------

// HealthService defines the business-logic contract for the health protocol.
type HealthService interface {
	GetProfile(ctx context.Context, userID uint) (*models.HealthProfile, error)
	UpdateProfile(ctx context.Context, userID uint, req UpdateProfileRequest) (*models.HealthProfile, error)

	LogWater(ctx context.Context, userID uint, req LogWaterRequest) (*models.WaterLog, error)
	ListWaterLogs(ctx context.Context, userID uint, date time.Time) ([]models.WaterLog, error)
	GetHydrationStatus(ctx context.Context, userID uint) (*HydrationStatus, error)

	LogMeal(ctx context.Context, userID uint, req LogMealRequest) (*models.MealLog, error)
	ListMealLogs(ctx context.Context, userID uint, date time.Time) ([]models.MealLog, error)

	LogCaffeine(ctx context.Context, userID uint, req LogCaffeineRequest) (*models.CaffeineLog, error)
	ListCaffeineLogs(ctx context.Context, userID uint, date time.Time) ([]models.CaffeineLog, error)
	GetCaffeineScore(ctx context.Context, userID uint) (*CaffeineScore, error)

	StartPomodoro(ctx context.Context, userID uint, req StartPomodoroRequest) (*models.PomodoroSession, error)
	EndPomodoro(ctx context.Context, userID uint, sessionID string, req EndPomodoroRequest) (*models.PomodoroSession, error)
	ListPomodoroSessions(ctx context.Context, userID uint, date time.Time) ([]models.PomodoroSession, error)

	GetShutdownStatus(ctx context.Context, userID uint) (*ShutdownStatus, error)
	StartShutdown(ctx context.Context, userID uint) (*models.SleepConfig, error)

	LogSleep(ctx context.Context, userID uint, req LogSleepRequest) (*models.SleepLog, error)
	ListSleepLogs(ctx context.Context, userID uint, date time.Time) ([]models.SleepLog, error)

	UpsertSnapshot(ctx context.Context, userID uint, req UpsertSnapshotRequest) (*models.HealthSnapshot, error)
	ListSnapshots(ctx context.Context, userID uint, from, to time.Time) ([]models.HealthSnapshot, error)

	GetHealthSummary(ctx context.Context, userID uint) (*HealthSummary, error)
}

type healthService struct {
	repo HealthRepository
}

// NewHealthService returns a HealthService backed by the given repository.
func NewHealthService(repo HealthRepository) HealthService {
	return &healthService{repo: repo}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// dayRange returns the start (00:00:00) and end (00:00:00 next day) of the given date.
func dayRange(date time.Time) (since, until time.Time) {
	since = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	until = since.AddDate(0, 0, 1)
	return
}

// parseTimeOfDay parses "HH:MM" into a full time.Time for the given date.
// Returns the date at 00:00 if parsing fails.
func parseTimeOfDay(hhmm string, date time.Time) time.Time {
	var h, m int
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &h, &m); err != nil {
		return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	}
	return time.Date(date.Year(), date.Month(), date.Day(), h, m, 0, 0, date.Location())
}

// containsCondition checks if a pq.StringArray contains a specific condition.
func containsCondition(conditions pq.StringArray, condition string) bool {
	for _, c := range conditions {
		if c == condition {
			return true
		}
	}
	return false
}

// defaultProfile returns a HealthProfile populated with sensible defaults.
func defaultProfile(userID uint) *models.HealthProfile {
	return &models.HealthProfile{
		UserID:               userID,
		TargetWaterMl:        2500,
		WorkWindowStart:      "09:00",
		WorkWindowEnd:        "19:00",
		TargetSleepTime:      "23:00",
		WakeTime:             "08:00",
		PomodoroDurationMin:  25,
		PomodoroBreakMin:     5,
		PomodoroLongBreakMin: 15,
		PomodoroDailyTarget:  8,
		GerdShutdownHours:    4,
		CaffeineDelayMin:     120,
	}
}

// getOrDefaultProfile fetches the health profile for the given user, returning
// a sensible default profile (without persisting it) when none exists yet.
func (s *healthService) getOrDefaultProfile(ctx context.Context, userID uint) (*models.HealthProfile, error) {
	p, err := s.repo.GetProfile(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultProfile(userID), nil
		}
		return nil, err
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

func (s *healthService) GetProfile(ctx context.Context, userID uint) (*models.HealthProfile, error) {
	return s.getOrDefaultProfile(ctx, userID)
}

func (s *healthService) UpdateProfile(ctx context.Context, userID uint, req UpdateProfileRequest) (*models.HealthProfile, error) {
	p, err := s.getOrDefaultProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	if req.HeightCm != nil {
		p.HeightCm = req.HeightCm
	}
	if req.WeightKg != nil {
		p.WeightKg = req.WeightKg
	}
	if req.TargetWeightKg != nil {
		p.TargetWeightKg = req.TargetWeightKg
	}
	if req.TargetWaterMl != nil {
		p.TargetWaterMl = *req.TargetWaterMl
	}
	if req.WorkWindowStart != "" {
		p.WorkWindowStart = req.WorkWindowStart
	}
	if req.WorkWindowEnd != "" {
		p.WorkWindowEnd = req.WorkWindowEnd
	}
	if req.TargetSleepTime != "" {
		p.TargetSleepTime = req.TargetSleepTime
	}
	if req.WakeTime != "" {
		p.WakeTime = req.WakeTime
	}
	if req.PomodoroDurationMin != nil {
		p.PomodoroDurationMin = *req.PomodoroDurationMin
	}
	if req.PomodoroBreakMin != nil {
		p.PomodoroBreakMin = *req.PomodoroBreakMin
	}
	if req.PomodoroLongBreakMin != nil {
		p.PomodoroLongBreakMin = *req.PomodoroLongBreakMin
	}
	if req.PomodoroDailyTarget != nil {
		p.PomodoroDailyTarget = *req.PomodoroDailyTarget
	}
	if req.GerdShutdownHours != nil {
		p.GerdShutdownHours = *req.GerdShutdownHours
	}
	if req.CaffeineDelayMin != nil {
		p.CaffeineDelayMin = *req.CaffeineDelayMin
	}
	if len(req.Conditions) > 0 {
		p.Conditions = pq.StringArray(req.Conditions)
	}

	if err := s.repo.UpsertProfile(ctx, p); err != nil {
		return nil, fmt.Errorf("update health profile: %w", err)
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Water
// ---------------------------------------------------------------------------

func (s *healthService) LogWater(ctx context.Context, userID uint, req LogWaterRequest) (*models.WaterLog, error) {
	if req.AmountMl <= 0 {
		return nil, fmt.Errorf("amount_ml is required and must be positive")
	}

	source := req.Source
	if source == "" {
		source = "manual"
	}

	log := &models.WaterLog{
		UserID:      userID,
		AmountMl:    req.AmountMl,
		LoggedAt:    time.Now().UTC(),
		Source:      source,
		IsGoutFlush: req.IsGoutFlush,
	}

	if err := s.repo.CreateWaterLog(ctx, log); err != nil {
		return nil, fmt.Errorf("log water: %w", err)
	}
	return log, nil
}

func (s *healthService) ListWaterLogs(ctx context.Context, userID uint, date time.Time) ([]models.WaterLog, error) {
	since, until := dayRange(date)
	return s.repo.ListWaterLogs(ctx, userID, since, until)
}

func (s *healthService) GetHydrationStatus(ctx context.Context, userID uint) (*HydrationStatus, error) {
	profile, err := s.getOrDefaultProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	totalMl, err := s.repo.GetDailyWaterTotal(ctx, userID, now)
	if err != nil {
		return nil, fmt.Errorf("hydration status: %w", err)
	}

	targetMl := profile.TargetWaterMl
	deficitMl := targetMl - totalMl
	if deficitMl < 0 {
		deficitMl = 0
	}

	var percentage float64
	if targetMl > 0 {
		percentage = math.Min(float64(totalMl)/float64(targetMl)*100, 100)
	}

	workEnd := parseTimeOfDay(profile.WorkWindowEnd, now)
	remainingHours := workEnd.Sub(now).Hours()
	var hourlyRateNeeded float64
	if remainingHours > 0 && deficitMl > 0 {
		hourlyRateNeeded = math.Ceil(float64(deficitMl) / remainingHours)
	}

	var status string
	switch {
	case percentage >= 90:
		status = "goal_reached"
	case percentage >= 70:
		status = "on_track"
	case percentage >= 40:
		status = "slightly_behind"
	default:
		status = "dehydrated"
	}

	isGoutFlushNeeded := deficitMl > 500 && containsCondition(profile.Conditions, "gout")

	return &HydrationStatus{
		TotalMl:           totalMl,
		TargetMl:          targetMl,
		DeficitMl:         deficitMl,
		Percentage:        math.Round(percentage*10) / 10,
		HourlyRateNeeded:  hourlyRateNeeded,
		Status:            status,
		IsGoutFlushNeeded: isGoutFlushNeeded,
	}, nil
}

// ---------------------------------------------------------------------------
// Meals
// ---------------------------------------------------------------------------

func (s *healthService) LogMeal(ctx context.Context, userID uint, req LogMealRequest) (*models.MealLog, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	category := req.Category
	if category == "" {
		category = "other"
	}

	log := &models.MealLog{
		UserID:   userID,
		Name:     req.Name,
		IsSafe:   req.IsSafe,
		Category: category,
		Triggers: pq.StringArray(req.Triggers),
		LoggedAt: time.Now().UTC(),
		Notes:    req.Notes,
	}

	if err := s.repo.CreateMealLog(ctx, log); err != nil {
		return nil, fmt.Errorf("log meal: %w", err)
	}
	return log, nil
}

func (s *healthService) ListMealLogs(ctx context.Context, userID uint, date time.Time) ([]models.MealLog, error) {
	since, until := dayRange(date)
	return s.repo.ListMealLogs(ctx, userID, since, until)
}

// ---------------------------------------------------------------------------
// Caffeine
// ---------------------------------------------------------------------------

func (s *healthService) LogCaffeine(ctx context.Context, userID uint, req LogCaffeineRequest) (*models.CaffeineLog, error) {
	if req.DrinkType == "" {
		return nil, fmt.Errorf("drink_type is required")
	}

	now := time.Now()
	withinCortisolWindow := false
	profile, _ := s.getOrDefaultProfile(ctx, userID)
	if profile != nil {
		wakeTime := parseTimeOfDay(profile.WakeTime, now)
		cortisolEnd := wakeTime.Add(time.Duration(profile.CaffeineDelayMin) * time.Minute)
		if now.Before(cortisolEnd) {
			withinCortisolWindow = true
		}
	}

	log := &models.CaffeineLog{
		UserID:               userID,
		DrinkType:            req.DrinkType,
		IsClean:              req.IsClean,
		CaffeineMg:           req.CaffeineMg,
		SugarG:               req.SugarG,
		LoggedAt:             now.UTC(),
		WithinCortisolWindow: withinCortisolWindow,
	}

	if err := s.repo.CreateCaffeineLog(ctx, log); err != nil {
		return nil, fmt.Errorf("log caffeine: %w", err)
	}
	return log, nil
}

func (s *healthService) ListCaffeineLogs(ctx context.Context, userID uint, date time.Time) ([]models.CaffeineLog, error) {
	since, until := dayRange(date)
	return s.repo.ListCaffeineLogs(ctx, userID, since, until)
}

func (s *healthService) GetCaffeineScore(ctx context.Context, userID uint) (*CaffeineScore, error) {
	now := time.Now()
	since, until := dayRange(now)

	logs, err := s.repo.ListCaffeineLogs(ctx, userID, since, until)
	if err != nil {
		return nil, fmt.Errorf("caffeine score: %w", err)
	}

	var cleanCount, sugarCount int
	for _, l := range logs {
		if l.IsClean {
			cleanCount++
		} else {
			sugarCount++
		}
	}

	var transitionScore int
	total := cleanCount + sugarCount
	if total > 0 {
		transitionScore = int(math.Round(float64(cleanCount) / float64(total) * 100))
	}

	var status string
	switch {
	case total == 0:
		status = "clean"
		transitionScore = 100
	case transitionScore == 100:
		status = "clean"
	case transitionScore > 50:
		status = "transitioning"
	default:
		status = "dependent"
	}

	withinCortisolWindow := false
	profile, _ := s.getOrDefaultProfile(ctx, userID)
	if profile != nil {
		wakeTime := parseTimeOfDay(profile.WakeTime, now)
		cortisolEnd := wakeTime.Add(time.Duration(profile.CaffeineDelayMin) * time.Minute)
		if now.Before(cortisolEnd) {
			withinCortisolWindow = true
		}
	}

	return &CaffeineScore{
		CleanCount:           cleanCount,
		SugarCount:           sugarCount,
		TransitionScore:      transitionScore,
		Status:               status,
		WithinCortisolWindow: withinCortisolWindow,
	}, nil
}

// ---------------------------------------------------------------------------
// Pomodoro
// ---------------------------------------------------------------------------

func (s *healthService) StartPomodoro(ctx context.Context, userID uint, req StartPomodoroRequest) (*models.PomodoroSession, error) {
	sessionType := req.Type
	if sessionType == "" {
		sessionType = "work"
	}
	if sessionType != "work" && sessionType != "break" && sessionType != "stand" {
		return nil, fmt.Errorf("type must be one of: work, break, stand")
	}

	durationMin := 25
	breakDurationMin := 0
	profile, _ := s.getOrDefaultProfile(ctx, userID)
	if profile != nil {
		switch sessionType {
		case "work":
			durationMin = profile.PomodoroDurationMin
			breakDurationMin = profile.PomodoroBreakMin
		case "break":
			durationMin = profile.PomodoroBreakMin
		case "stand":
			durationMin = 5
		}
	}

	session := &models.PomodoroSession{
		UserID:           userID,
		StartedAt:        time.Now().UTC(),
		DurationMin:      durationMin,
		BreakDurationMin: breakDurationMin,
		Type:             sessionType,
		Completed:        false,
		StoodUp:          false,
		WalkedMin:        0,
	}

	if err := s.repo.CreatePomodoroSession(ctx, session); err != nil {
		return nil, fmt.Errorf("start pomodoro: %w", err)
	}
	return session, nil
}

func (s *healthService) EndPomodoro(ctx context.Context, userID uint, sessionID string, req EndPomodoroRequest) (*models.PomodoroSession, error) {
	session, err := s.repo.FindPomodoroByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrHealthNotFound
		}
		return nil, err
	}

	if session.UserID != userID {
		return nil, ErrHealthForbidden
	}

	if session.EndedAt != nil {
		return nil, fmt.Errorf("pomodoro session already ended")
	}

	now := time.Now().UTC()
	session.EndedAt = &now
	session.Completed = req.Completed
	session.StoodUp = req.StoodUp
	session.WalkedMin = req.WalkedMin

	if err := s.repo.UpdatePomodoroSession(ctx, session); err != nil {
		return nil, fmt.Errorf("end pomodoro: %w", err)
	}
	return session, nil
}

func (s *healthService) ListPomodoroSessions(ctx context.Context, userID uint, date time.Time) ([]models.PomodoroSession, error) {
	since, until := dayRange(date)
	return s.repo.ListPomodoroSessions(ctx, userID, since, until)
}

// ---------------------------------------------------------------------------
// Shutdown
// ---------------------------------------------------------------------------

func (s *healthService) GetShutdownStatus(ctx context.Context, userID uint) (*ShutdownStatus, error) {
	profile, err := s.getOrDefaultProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	targetBedtime := parseTimeOfDay(profile.TargetSleepTime, now)
	shutdownStart := targetBedtime.Add(-time.Duration(profile.GerdShutdownHours) * time.Hour)

	active := now.After(shutdownStart) || now.Equal(shutdownStart)

	minutesUntilShutdown := 0
	if !active {
		minutesUntilShutdown = int(shutdownStart.Sub(now).Minutes())
	}

	allowedItems := []string{"water", "chamomile_tea", "anise_tea"}
	sleepConfig, err := s.repo.GetSleepConfig(ctx, userID)
	if err == nil && len(sleepConfig.AllowedItems) > 0 {
		allowedItems = []string(sleepConfig.AllowedItems)
	}

	compliant := true
	minutesSinceLastMeal := 0
	if active {
		since, until := dayRange(now)
		meals, mealErr := s.repo.ListMealLogs(ctx, userID, since, until)
		if mealErr == nil {
			for _, meal := range meals {
				if meal.LoggedAt.After(shutdownStart) || meal.LoggedAt.Equal(shutdownStart) {
					compliant = false
					break
				}
			}
			if len(meals) > 0 {
				lastMeal := meals[0]
				minutesSinceLastMeal = int(now.Sub(lastMeal.LoggedAt).Minutes())
			}
		}
	} else {
		since, until := dayRange(now)
		meals, mealErr := s.repo.ListMealLogs(ctx, userID, since, until)
		if mealErr == nil && len(meals) > 0 {
			lastMeal := meals[0]
			minutesSinceLastMeal = int(now.Sub(lastMeal.LoggedAt).Minutes())
		}
	}

	return &ShutdownStatus{
		Active:               active,
		MinutesUntilShutdown: minutesUntilShutdown,
		MinutesSinceLastMeal: minutesSinceLastMeal,
		AllowedItems:         allowedItems,
		Compliant:            compliant,
	}, nil
}

func (s *healthService) StartShutdown(ctx context.Context, userID uint) (*models.SleepConfig, error) {
	profile, err := s.getOrDefaultProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	config := &models.SleepConfig{
		UserID:            userID,
		TargetBedtime:     profile.TargetSleepTime,
		ShutdownStartedAt: &now,
		ShutdownActive:    true,
		AllowedItems:      pq.StringArray{"water", "chamomile_tea", "anise_tea"},
	}

	since, until := dayRange(now)
	meals, mealErr := s.repo.ListMealLogs(ctx, userID, since, until)
	if mealErr == nil && len(meals) > 0 {
		lastMealAt := meals[0].LoggedAt
		config.LastMealAt = &lastMealAt
	}

	if err := s.repo.UpsertSleepConfig(ctx, config); err != nil {
		return nil, fmt.Errorf("start shutdown: %w", err)
	}
	return config, nil
}

// ---------------------------------------------------------------------------
// Sleep Logs
// ---------------------------------------------------------------------------

func (s *healthService) LogSleep(ctx context.Context, userID uint, req LogSleepRequest) (*models.SleepLog, error) {
	if req.BedTime == "" || req.WakeTime == "" {
		return nil, fmt.Errorf("bed_time and wake_time are required")
	}

	bedTime, err := time.Parse(time.RFC3339, req.BedTime)
	if err != nil {
		return nil, fmt.Errorf("bed_time must be RFC3339 format: %w", err)
	}
	wakeTime, err := time.Parse(time.RFC3339, req.WakeTime)
	if err != nil {
		return nil, fmt.Errorf("wake_time must be RFC3339 format: %w", err)
	}

	durationHours := wakeTime.Sub(bedTime).Hours()
	if durationHours <= 0 {
		return nil, fmt.Errorf("wake_time must be after bed_time")
	}

	qualityRating := req.QualityRating
	if qualityRating < 1 || qualityRating > 5 {
		qualityRating = 3
	}

	log := &models.SleepLog{
		UserID:        userID,
		BedTime:       bedTime.UTC().Format(time.RFC3339),
		WakeTime:      wakeTime.UTC().Format(time.RFC3339),
		QualityRating: qualityRating,
		DurationHours: math.Round(durationHours*100) / 100,
		LoggedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.repo.CreateSleepLog(ctx, log); err != nil {
		return nil, fmt.Errorf("log sleep: %w", err)
	}
	return log, nil
}

func (s *healthService) ListSleepLogs(ctx context.Context, userID uint, date time.Time) ([]models.SleepLog, error) {
	since, until := dayRange(date)
	return s.repo.ListSleepLogs(ctx, userID, since, until)
}

// ---------------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------------

func (s *healthService) UpsertSnapshot(ctx context.Context, userID uint, req UpsertSnapshotRequest) (*models.HealthSnapshot, error) {
	if req.SnapshotDate == "" {
		req.SnapshotDate = time.Now().Format("2006-01-02")
	}

	source := req.Source
	if source == "" {
		source = "healthkit"
	}

	snapshot := &models.HealthSnapshot{
		UserID:                  userID,
		SnapshotDate:            req.SnapshotDate,
		WeightKg:                req.WeightKg,
		BodyFatPct:              req.BodyFatPct,
		VisceralFat:             req.VisceralFat,
		BodyWaterPct:            req.BodyWaterPct,
		MetabolicAge:            req.MetabolicAge,
		Steps:                   req.Steps,
		ActiveEnergyCal:         req.ActiveEnergyCal,
		AvgHeartRate:            req.AvgHeartRate,
		SleepHours:              req.SleepHours,
		WaterTotalMl:            req.WaterTotalMl,
		MealsSafe:               req.MealsSafe,
		MealsUnsafe:             req.MealsUnsafe,
		CaffeineCleanCount:      req.CaffeineCleanCount,
		CaffeineSugarCount:      req.CaffeineSugarCount,
		PomodorosCompleted:      req.PomodorosCompleted,
		StandSessions:           req.StandSessions,
		GerdShutdownCompliant:   req.GerdShutdownCompliant,
		NutritionSafetyScore:    req.NutritionSafetyScore,
		CaffeineTransitionScore: req.CaffeineTransitionScore,
		Source:                  source,
	}

	if err := s.repo.UpsertSnapshot(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("upsert snapshot: %w", err)
	}
	return snapshot, nil
}

func (s *healthService) ListSnapshots(ctx context.Context, userID uint, from, to time.Time) ([]models.HealthSnapshot, error) {
	return s.repo.ListSnapshots(ctx, userID, from, to)
}

// ---------------------------------------------------------------------------
// Health Summary
// ---------------------------------------------------------------------------

func (s *healthService) GetHealthSummary(ctx context.Context, userID uint) (*HealthSummary, error) {
	hydration, err := s.GetHydrationStatus(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("health summary hydration: %w", err)
	}

	caffeine, err := s.GetCaffeineScore(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("health summary caffeine: %w", err)
	}

	shutdown, err := s.GetShutdownStatus(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("health summary shutdown: %w", err)
	}

	now := time.Now()
	since, until := dayRange(now)
	meals, err := s.repo.ListMealLogs(ctx, userID, since, until)
	if err != nil {
		return nil, fmt.Errorf("health summary meals: %w", err)
	}

	var mealsSafe, mealsUnsafe int
	triggerCounts := make(map[string]int)
	for _, m := range meals {
		if m.IsSafe {
			mealsSafe++
		} else {
			mealsUnsafe++
		}
		for _, t := range m.Triggers {
			triggerCounts[t]++
		}
	}

	var nutritionSafetyScore int
	totalMeals := mealsSafe + mealsUnsafe
	if totalMeals > 0 {
		nutritionSafetyScore = int(math.Round(float64(mealsSafe) / float64(totalMeals) * 100))
	} else {
		nutritionSafetyScore = 100
	}

	nutrition := NutritionSafety{
		MealsSafe:     mealsSafe,
		MealsUnsafe:   mealsUnsafe,
		SafetyScore:   nutritionSafetyScore,
		TriggerCounts: triggerCounts,
	}

	pomodoroCount, err := s.repo.GetDailyPomodoroCount(ctx, userID, now)
	if err != nil {
		return nil, fmt.Errorf("health summary pomodoro: %w", err)
	}

	pomodoroTarget := 8
	profile, _ := s.getOrDefaultProfile(ctx, userID)
	if profile != nil {
		pomodoroTarget = profile.PomodoroDailyTarget
	}

	var pomodoroScore float64
	if pomodoroTarget > 0 {
		pomodoroScore = math.Min(float64(pomodoroCount)/float64(pomodoroTarget)*100, 100)
	}

	var shutdownScore float64
	if shutdown.Compliant {
		shutdownScore = 100
	} else {
		shutdownScore = 0
	}

	overallScore := int(math.Round(
		hydration.Percentage*0.25 +
			float64(nutritionSafetyScore)*0.25 +
			float64(caffeine.TransitionScore)*0.20 +
			pomodoroScore*0.20 +
			shutdownScore*0.10,
	))

	return &HealthSummary{
		Hydration:          *hydration,
		Caffeine:           *caffeine,
		Shutdown:           *shutdown,
		Nutrition:          nutrition,
		PomodorosCompleted: pomodoroCount,
		PomodoroTarget:     pomodoroTarget,
		OverallScore:       overallScore,
	}, nil
}
