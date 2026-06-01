package main

import (
  "fmt"
  "log"

  "github.com/glebarez/sqlite"
  "golang.org/x/crypto/bcrypt"
  "gorm.io/driver/mysql"
  "gorm.io/driver/postgres"
  "gorm.io/gorm"
  "gorm.io/gorm/logger"
)

var db *gorm.DB

func InitDatabase() {
  driver := env("DB_DRIVER", "sqlite")
  dsn := env("DB_DSN", dataDir+"/sentinel.db")

  var dialector gorm.Dialector
  switch driver {
  case "sqlite":
    dialector = sqlite.Open(dsn)
  case "mysql":
    dialector = mysql.Open(dsn)
  case "postgres":
    dialector = postgres.Open(dsn)
  default:
    log.Fatalf("[database] unsupported driver: %s", driver)
  }

  var err error
  db, err = gorm.Open(dialector, &gorm.Config{
    Logger: logger.Default.LogMode(logger.Warn),
  })
  if err != nil {
    log.Fatalf("[database] connect failed: %v", err)
  }

  // Auto-migrate
  if err := db.AutoMigrate(&User{}, &LogEntry{}, &Job{}); err != nil {
    log.Fatalf("[database] migrate failed: %v", err)
  }

  // 首次启动创建默认管理员
  var count int64
  db.Model(&User{}).Count(&count)
  if count == 0 {
    hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
    admin := User{
      Username: "admin",
      Password: string(hash),
      Role:     "admin",
    }
    db.Create(&admin)
    fmt.Println("[database] created default admin user (admin / admin123)")
  }

  log.Printf("[database] initialized (%s)", driver)
}
