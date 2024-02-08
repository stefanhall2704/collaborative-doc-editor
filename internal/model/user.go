package model

import (
    "gorm.io/gorm"
    "time"
)

type Document struct {
    gorm.Model
    Title    string `gorm:"type:varchar(100);not null"`
    Content  string `gorm:"type:text;not null"`
    OwnerID  uint   `gorm:"not null"`
    LastEdit    time.Time
}


type User struct {
    gorm.Model
    Username     string
    PasswordHash string
    Email        string
    Documents    []Document `gorm:"foreignKey:OwnerID"`
}
