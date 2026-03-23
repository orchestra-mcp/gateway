package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// User represents a registered user of the platform.
// This is a MINIMAL subset needed for auth + tunnel ownership.
type User struct {
	ID                    uint           `gorm:"primarykey" json:"id"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"deleted_at"`
	Name                  string         `json:"name"`
	Email                 string         `gorm:"uniqueIndex" json:"email"`
	Password              string         `json:"-"`
	Role                  string         `gorm:"default:user" json:"role"` // admin | team_owner | team_manager | user
	AvatarURL             string         `json:"avatar_url"`
	EmailVerifiedAt       *time.Time     `json:"email_verified_at"`
	Status                string         `gorm:"default:active" json:"status"`
	PasswordSet           bool           `gorm:"default:false" json:"password_set"`
	OnboardingCompletedAt *time.Time     `json:"onboarding_completed_at"`
	Settings              datatypes.JSON `json:"settings"`
	TwoFactorSecret       string         `json:"-"`
	TwoFactorEnabled      bool           `gorm:"default:false" json:"two_factor_enabled"`
	TwoFactorVerifiedAt   *time.Time     `json:"two_factor_verified_at"`
	DeletionScheduledAt   *time.Time     `json:"deletion_scheduled_at"`
	RememberToken         string         `json:"-"`
	IsVerified            bool           `gorm:"default:false" json:"is_verified"`
	VerifiedAt            *time.Time     `json:"verified_at"`
}
