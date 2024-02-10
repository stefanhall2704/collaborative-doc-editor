package model

import (
	"gorm.io/gorm"
	"time"
)

type Document struct {
	gorm.Model
	FileName    string `gorm:"type:varchar(100);"`
	ContentType string `gorm:"type:text;"`
	OwnerID     uint   `gorm:"not null"`
	LastEdit    time.Time
}

type User struct {
	gorm.Model
	Username     string
	PasswordHash string
	Email        string
	Documents    []Document `gorm:"foreignKey:OwnerID"`
}
