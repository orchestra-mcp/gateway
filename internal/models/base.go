package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base provides UUID primary key and standard timestamps for models.
type Base struct {
	ID        string         `gorm:"type:uuid;primarykey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// BeforeCreate generates a UUID if the ID is empty, preventing GORM from
// sending an empty string to the uuid column.
func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}
