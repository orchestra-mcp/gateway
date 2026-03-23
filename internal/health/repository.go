package health

import (
	"context"
	"fmt"
	"time"

	"github.com/orchestra-mcp/gateway/internal/models"
	"gorm.io/gorm"
)

// HealthRepository defines the data-access contract for all health entities.
type HealthRepository interface {
	// Profile
	GetProfile(ctx context.Context, userID uint) (*models.HealthProfile, error)
	UpsertProfile(ctx context.Context, profile *models.HealthProfile) error

	// Water
	CreateWaterLog(ctx context.Context, log *models.WaterLog) error
	ListWaterLogs(ctx context.Context, userID uint, since, until time.Time) ([]models.WaterLog, error)
	GetDailyWaterTotal(ctx context.Context, userID uint, date time.Time) (int, error)

	// Meals
	CreateMealLog(ctx context.Context, log *models.MealLog) error
	ListMealLogs(ctx context.Context, userID uint, since, until time.Time) ([]models.MealLog, error)

	// Caffeine
	CreateCaffeineLog(ctx context.Context, log *models.CaffeineLog) error
	ListCaffeineLogs(ctx context.Context, userID uint, since, until time.Time) ([]models.CaffeineLog, error)

	// Pomodoro
	CreatePomodoroSession(ctx context.Context, session *models.PomodoroSession) error
	UpdatePomodoroSession(ctx context.Context, session *models.PomodoroSession) error
	FindPomodoroByID(ctx context.Context, id string) (*models.PomodoroSession, error)
	ListPomodoroSessions(ctx context.Context, userID uint, since, until time.Time) ([]models.PomodoroSession, error)
	GetDailyPomodoroCount(ctx context.Context, userID uint, date time.Time) (int, error)

	// Sleep
	GetSleepConfig(ctx context.Context, userID uint) (*models.SleepConfig, error)
	UpsertSleepConfig(ctx context.Context, config *models.SleepConfig) error
	CreateSleepLog(ctx context.Context, log *models.SleepLog) error
	ListSleepLogs(ctx context.Context, userID uint, since, until time.Time) ([]models.SleepLog, error)

	// Snapshots
	GetSnapshot(ctx context.Context, userID uint, date time.Time) (*models.HealthSnapshot, error)
	UpsertSnapshot(ctx context.Context, snapshot *models.HealthSnapshot) error
	ListSnapshots(ctx context.Context, userID uint, since, until time.Time) ([]models.HealthSnapshot, error)
}

type healthRepository struct {
	db *gorm.DB
}

// NewHealthRepository returns a GORM-backed HealthRepository.
func NewHealthRepository(db *gorm.DB) HealthRepository {
	return &healthRepository{db: db}
}

// --- Profile ---

func (r *healthRepository) GetProfile(ctx context.Context, userID uint) (*models.HealthProfile, error) {
	var p models.HealthProfile
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&p).Error
	if err != nil {
		return nil, fmt.Errorf("health profile get: %w", err)
	}
	return &p, nil
}

func (r *healthRepository) UpsertProfile(ctx context.Context, profile *models.HealthProfile) error {
	var existing models.HealthProfile
	err := r.db.WithContext(ctx).Where("user_id = ?", profile.UserID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		if err := r.db.WithContext(ctx).Create(profile).Error; err != nil {
			return fmt.Errorf("health profile create: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("health profile lookup: %w", err)
	}
	profile.ID = existing.ID
	profile.Version = existing.Version + 1
	if err := r.db.WithContext(ctx).Save(profile).Error; err != nil {
		return fmt.Errorf("health profile update: %w", err)
	}
	return nil
}

// --- Water ---

func (r *healthRepository) CreateWaterLog(ctx context.Context, log *models.WaterLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("water log create: %w", err)
	}
	return nil
}

func (r *healthRepository) ListWaterLogs(ctx context.Context, userID uint, since, until time.Time) ([]models.WaterLog, error) {
	var logs []models.WaterLog
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND logged_at >= ? AND logged_at < ?", userID, since, until).
		Order("logged_at DESC").
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("water logs list: %w", err)
	}
	return logs, nil
}

func (r *healthRepository) GetDailyWaterTotal(ctx context.Context, userID uint, date time.Time) (int, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.AddDate(0, 0, 1)
	var total int64
	err := r.db.WithContext(ctx).Model(&models.WaterLog{}).
		Where("user_id = ? AND logged_at >= ? AND logged_at < ?", userID, start, end).
		Select("COALESCE(SUM(amount_ml), 0)").
		Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("water daily total: %w", err)
	}
	return int(total), nil
}

// --- Meals ---

func (r *healthRepository) CreateMealLog(ctx context.Context, log *models.MealLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("meal log create: %w", err)
	}
	return nil
}

func (r *healthRepository) ListMealLogs(ctx context.Context, userID uint, since, until time.Time) ([]models.MealLog, error) {
	var logs []models.MealLog
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND logged_at >= ? AND logged_at < ?", userID, since, until).
		Order("logged_at DESC").
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("meal logs list: %w", err)
	}
	return logs, nil
}

// --- Caffeine ---

func (r *healthRepository) CreateCaffeineLog(ctx context.Context, log *models.CaffeineLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("caffeine log create: %w", err)
	}
	return nil
}

func (r *healthRepository) ListCaffeineLogs(ctx context.Context, userID uint, since, until time.Time) ([]models.CaffeineLog, error) {
	var logs []models.CaffeineLog
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND logged_at >= ? AND logged_at < ?", userID, since, until).
		Order("logged_at DESC").
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("caffeine logs list: %w", err)
	}
	return logs, nil
}

// --- Pomodoro ---

func (r *healthRepository) CreatePomodoroSession(ctx context.Context, session *models.PomodoroSession) error {
	if err := r.db.WithContext(ctx).Create(session).Error; err != nil {
		return fmt.Errorf("pomodoro create: %w", err)
	}
	return nil
}

func (r *healthRepository) UpdatePomodoroSession(ctx context.Context, session *models.PomodoroSession) error {
	session.Version++
	if err := r.db.WithContext(ctx).Save(session).Error; err != nil {
		return fmt.Errorf("pomodoro update: %w", err)
	}
	return nil
}

func (r *healthRepository) FindPomodoroByID(ctx context.Context, id string) (*models.PomodoroSession, error) {
	var s models.PomodoroSession
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if err != nil {
		return nil, fmt.Errorf("pomodoro find: %w", err)
	}
	return &s, nil
}

func (r *healthRepository) ListPomodoroSessions(ctx context.Context, userID uint, since, until time.Time) ([]models.PomodoroSession, error) {
	var sessions []models.PomodoroSession
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND started_at >= ? AND started_at < ?", userID, since, until).
		Order("started_at DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("pomodoro list: %w", err)
	}
	return sessions, nil
}

func (r *healthRepository) GetDailyPomodoroCount(ctx context.Context, userID uint, date time.Time) (int, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.AddDate(0, 0, 1)
	var count int64
	err := r.db.WithContext(ctx).Model(&models.PomodoroSession{}).
		Where("user_id = ? AND started_at >= ? AND started_at < ? AND completed = true", userID, start, end).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("pomodoro daily count: %w", err)
	}
	return int(count), nil
}

// --- Sleep ---

func (r *healthRepository) GetSleepConfig(ctx context.Context, userID uint) (*models.SleepConfig, error) {
	var c models.SleepConfig
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&c).Error
	if err != nil {
		return nil, fmt.Errorf("sleep config get: %w", err)
	}
	return &c, nil
}

func (r *healthRepository) UpsertSleepConfig(ctx context.Context, config *models.SleepConfig) error {
	var existing models.SleepConfig
	err := r.db.WithContext(ctx).Where("user_id = ?", config.UserID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		if err := r.db.WithContext(ctx).Create(config).Error; err != nil {
			return fmt.Errorf("sleep config create: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("sleep config lookup: %w", err)
	}
	config.ID = existing.ID
	config.Version = existing.Version + 1
	if err := r.db.WithContext(ctx).Save(config).Error; err != nil {
		return fmt.Errorf("sleep config update: %w", err)
	}
	return nil
}

func (r *healthRepository) CreateSleepLog(ctx context.Context, log *models.SleepLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("sleep log create: %w", err)
	}
	return nil
}

func (r *healthRepository) ListSleepLogs(ctx context.Context, userID uint, since, until time.Time) ([]models.SleepLog, error) {
	var logs []models.SleepLog
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND logged_at >= ? AND logged_at < ?", userID, since.Format("2006-01-02T15:04:05Z"), until.Format("2006-01-02T15:04:05Z")).
		Order("logged_at DESC").
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("sleep logs list: %w", err)
	}
	return logs, nil
}

// --- Snapshots ---

func (r *healthRepository) GetSnapshot(ctx context.Context, userID uint, date time.Time) (*models.HealthSnapshot, error) {
	dateStr := date.Format("2006-01-02")
	var s models.HealthSnapshot
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND snapshot_date = ?", userID, dateStr).
		First(&s).Error
	if err != nil {
		return nil, fmt.Errorf("snapshot get: %w", err)
	}
	return &s, nil
}

func (r *healthRepository) UpsertSnapshot(ctx context.Context, snapshot *models.HealthSnapshot) error {
	var existing models.HealthSnapshot
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND snapshot_date = ?", snapshot.UserID, snapshot.SnapshotDate).
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		if err := r.db.WithContext(ctx).Create(snapshot).Error; err != nil {
			return fmt.Errorf("snapshot create: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("snapshot lookup: %w", err)
	}
	snapshot.ID = existing.ID
	snapshot.Version = existing.Version + 1
	if err := r.db.WithContext(ctx).Save(snapshot).Error; err != nil {
		return fmt.Errorf("snapshot update: %w", err)
	}
	return nil
}

func (r *healthRepository) ListSnapshots(ctx context.Context, userID uint, since, until time.Time) ([]models.HealthSnapshot, error) {
	sinceStr := since.Format("2006-01-02")
	untilStr := until.Format("2006-01-02")
	var snapshots []models.HealthSnapshot
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND snapshot_date >= ? AND snapshot_date <= ?", userID, sinceStr, untilStr).
		Order("snapshot_date DESC").
		Find(&snapshots).Error
	if err != nil {
		return nil, fmt.Errorf("snapshots list: %w", err)
	}
	return snapshots, nil
}
