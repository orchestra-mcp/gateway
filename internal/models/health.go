package models

import (
	"time"

	"github.com/lib/pq"
)

// HealthProfile stores per-user health targets, work window, and condition flags.
// One row per user (unique constraint on user_id).
type HealthProfile struct {
	Base
	Version              int            `gorm:"not null;default:1"                json:"version"`
	UserID               uint           `gorm:"not null;uniqueIndex:idx_health_profiles_user,where:deleted_at IS NULL" json:"user_id"`
	HeightCm             *float64       `gorm:"type:real"                         json:"height_cm,omitempty"`
	WeightKg             *float64       `gorm:"type:real"                         json:"weight_kg,omitempty"`
	TargetWeightKg       *float64       `gorm:"type:real"                         json:"target_weight_kg,omitempty"`
	TargetWaterMl        int            `gorm:"not null;default:2500"             json:"target_water_ml"`
	WorkWindowStart      string         `gorm:"type:varchar(5);not null;default:'09:00'" json:"work_window_start"`
	WorkWindowEnd        string         `gorm:"type:varchar(5);not null;default:'19:00'" json:"work_window_end"`
	TargetSleepTime      string         `gorm:"type:varchar(5);not null;default:'23:00'" json:"target_sleep_time"`
	WakeTime             string         `gorm:"type:varchar(5);not null;default:'08:00'" json:"wake_time"`
	PomodoroDurationMin  int            `gorm:"not null;default:25"               json:"pomodoro_duration_min"`
	PomodoroBreakMin     int            `gorm:"not null;default:5"                json:"pomodoro_break_min"`
	PomodoroLongBreakMin int            `gorm:"not null;default:15"               json:"pomodoro_long_break_min"`
	PomodoroDailyTarget  int            `gorm:"not null;default:8"                json:"pomodoro_daily_target"`
	GerdShutdownHours    int            `gorm:"not null;default:4"                json:"gerd_shutdown_hours"`
	CaffeineDelayMin     int            `gorm:"not null;default:120"              json:"caffeine_delay_min"`
	Conditions           pq.StringArray `gorm:"type:text[]"                       json:"conditions,omitempty"`
}

func (HealthProfile) TableName() string { return "health_profiles" }

// WaterLog records a single hydration event.
type WaterLog struct {
	Base
	Version     int       `gorm:"not null;default:1"                json:"version"`
	UserID      uint      `gorm:"not null;index"                    json:"user_id"`
	AmountMl    int       `gorm:"not null"                          json:"amount_ml"`
	LoggedAt    time.Time `gorm:"type:timestamptz;not null"         json:"logged_at"`
	Source      string    `gorm:"type:varchar(50);not null;default:'manual'" json:"source"`
	IsGoutFlush bool     `gorm:"not null;default:false"            json:"is_gout_flush"`
}

func (WaterLog) TableName() string { return "water_logs" }

// MealLog records a meal with boolean safe/unsafe classification and trigger warnings.
type MealLog struct {
	Base
	Version  int            `gorm:"not null;default:1"                json:"version"`
	UserID   uint           `gorm:"not null;index"                    json:"user_id"`
	Name     string         `gorm:"type:varchar(500);not null"        json:"name"`
	IsSafe   bool           `gorm:"not null"                          json:"is_safe"`
	Category string         `gorm:"type:varchar(50);not null;default:'other'" json:"category"`
	Triggers pq.StringArray `gorm:"type:text[]"                       json:"triggers,omitempty"`
	LoggedAt time.Time      `gorm:"type:timestamptz;not null"         json:"logged_at"`
	Notes    string         `gorm:"type:text;not null;default:''"     json:"notes"`
}

func (MealLog) TableName() string { return "meal_logs" }

// CaffeineLog records a caffeine intake event with Red Bull deprecation tracking.
type CaffeineLog struct {
	Base
	Version              int       `gorm:"not null;default:1"                json:"version"`
	UserID               uint      `gorm:"not null;index"                    json:"user_id"`
	DrinkType            string    `gorm:"type:varchar(100);not null"        json:"drink_type"`
	IsClean              bool      `gorm:"not null"                          json:"is_clean"`
	CaffeineMg           int       `gorm:"not null;default:0"               json:"caffeine_mg"`
	SugarG               float64   `gorm:"type:real;not null;default:0"      json:"sugar_g"`
	LoggedAt             time.Time `gorm:"type:timestamptz;not null"         json:"logged_at"`
	WithinCortisolWindow bool      `gorm:"not null;default:false"            json:"within_cortisol_window"`
}

func (CaffeineLog) TableName() string { return "caffeine_logs" }

// PomodoroSession records a work/break/stand timer session.
type PomodoroSession struct {
	Base
	Version          int        `gorm:"not null;default:1"                json:"version"`
	UserID           uint       `gorm:"not null;index"                    json:"user_id"`
	StartedAt        time.Time  `gorm:"type:timestamptz;not null"         json:"started_at"`
	EndedAt          *time.Time `gorm:"type:timestamptz"                  json:"ended_at,omitempty"`
	DurationMin      int        `gorm:"not null;default:0"                json:"duration_min"`
	BreakDurationMin int        `gorm:"not null;default:0"                json:"break_duration_min"`
	Type             string     `gorm:"type:varchar(20);not null;default:'work'" json:"type"`
	Completed        bool       `gorm:"not null;default:false"            json:"completed"`
	StoodUp          bool       `gorm:"not null;default:false"            json:"stood_up"`
	WalkedMin        float64    `gorm:"type:real;not null;default:0"      json:"walked_min"`
}

func (PomodoroSession) TableName() string { return "pomodoro_sessions" }

// SleepConfig stores GERD shutdown timer configuration. One row per user.
type SleepConfig struct {
	Base
	Version           int            `gorm:"not null;default:1"                json:"version"`
	UserID            uint           `gorm:"not null;uniqueIndex:idx_sleep_configs_user,where:deleted_at IS NULL" json:"user_id"`
	TargetBedtime     string         `gorm:"type:varchar(5);not null;default:'23:00'" json:"target_bedtime"`
	ShutdownStartedAt *time.Time     `gorm:"type:timestamptz"                  json:"shutdown_started_at,omitempty"`
	ShutdownActive    bool           `gorm:"not null;default:false"            json:"shutdown_active"`
	LastMealAt        *time.Time     `gorm:"type:timestamptz"                  json:"last_meal_at,omitempty"`
	AllowedItems      pq.StringArray `gorm:"type:text[];default:'{water,chamomile_tea,anise_tea}'" json:"allowed_items,omitempty"`
}

func (SleepConfig) TableName() string { return "sleep_configs" }

// SleepLog records a sleep session.
type SleepLog struct {
	Base
	UserID        uint    `gorm:"index" json:"user_id"`
	BedTime       string  `json:"bed_time"`
	WakeTime      string  `json:"wake_time"`
	QualityRating int     `gorm:"default:3" json:"quality_rating"`
	DurationHours float64 `json:"duration_hours"`
	LoggedAt      string  `json:"logged_at"`
}

// HealthSnapshot is a daily aggregate of all health metrics from HealthKit/Health
// Connect and in-app protocol logs. One row per user per day.
type HealthSnapshot struct {
	Base
	Version                 int      `gorm:"not null;default:1"                json:"version"`
	UserID                  uint     `gorm:"not null;index"                    json:"user_id"`
	SnapshotDate            string   `gorm:"type:date;not null"                json:"snapshot_date"`
	WeightKg                *float64 `gorm:"type:real"                         json:"weight_kg,omitempty"`
	BodyFatPct              *float64 `gorm:"type:real"                         json:"body_fat_pct,omitempty"`
	VisceralFat             *int     `gorm:"type:integer"                      json:"visceral_fat,omitempty"`
	BodyWaterPct            *float64 `gorm:"type:real"                         json:"body_water_pct,omitempty"`
	MetabolicAge            *int     `gorm:"type:integer"                      json:"metabolic_age,omitempty"`
	Steps                   int      `gorm:"not null;default:0"               json:"steps"`
	ActiveEnergyCal         float64  `gorm:"type:real;not null;default:0"      json:"active_energy_cal"`
	AvgHeartRate            *int     `gorm:"type:integer"                      json:"avg_heart_rate,omitempty"`
	SleepHours              float64  `gorm:"type:real;not null;default:0"      json:"sleep_hours"`
	WaterTotalMl            int      `gorm:"not null;default:0"               json:"water_total_ml"`
	MealsSafe               int      `gorm:"not null;default:0"               json:"meals_safe"`
	MealsUnsafe             int      `gorm:"not null;default:0"               json:"meals_unsafe"`
	CaffeineCleanCount      int      `gorm:"not null;default:0"               json:"caffeine_clean_count"`
	CaffeineSugarCount      int      `gorm:"not null;default:0"               json:"caffeine_sugar_count"`
	PomodorosCompleted      int      `gorm:"not null;default:0"               json:"pomodoros_completed"`
	StandSessions           int      `gorm:"not null;default:0"               json:"stand_sessions"`
	GerdShutdownCompliant   *bool    `gorm:"type:boolean"                     json:"gerd_shutdown_compliant,omitempty"`
	NutritionSafetyScore    *int     `gorm:"type:integer"                      json:"nutrition_safety_score,omitempty"`
	CaffeineTransitionScore *int     `gorm:"type:integer"                      json:"caffeine_transition_score,omitempty"`
	Source                  string   `gorm:"type:varchar(50);not null;default:'healthkit'" json:"source"`
}

func (HealthSnapshot) TableName() string { return "health_snapshots" }
