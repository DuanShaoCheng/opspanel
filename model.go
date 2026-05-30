package main

import (
  "time"

  "gorm.io/gorm"
)

// User 用户模型
type User struct {
  ID        uint      `gorm:"primaryKey" json:"id"`
  Username  string    `gorm:"uniqueIndex;size:64;not null" json:"username"`
  Password  string    `gorm:"size:128;not null" json:"-"`
  Role      string    `gorm:"size:16;not null;default:viewer" json:"role"` // admin / viewer
  CreatedAt time.Time `json:"created_at"`
  UpdatedAt time.Time `json:"updated_at"`
}

// LogEntry 采集到的日志条目
type LogEntry struct {
  ID         uint           `gorm:"primaryKey" json:"id"`
  Source     string         `gorm:"size:128;index" json:"source"`
  Host       string         `gorm:"size:128;index" json:"host"`
  File       string         `gorm:"size:256" json:"file"`
  Content    string         `gorm:"type:text" json:"content"`
  Context    string         `gorm:"type:text" json:"context"`
  Level      string         `gorm:"size:16;index" json:"level"`
  AIScore    int            `gorm:"default:0" json:"ai_score"`
  AIAnalysis string         `gorm:"type:text" json:"ai_analysis"`
  Analyzed   bool           `gorm:"default:false;index" json:"analyzed"`
  CreatedAt  time.Time      `gorm:"index" json:"created_at"`
  DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}
