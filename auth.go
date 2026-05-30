package main

import (
  "errors"
  "os"
  "time"

  "github.com/golang-jwt/jwt/v5"
  "golang.org/x/crypto/bcrypt"
  "gorm.io/gorm"
)

var jwtSecret []byte

func InitJWTSecret() {
  secret := os.Getenv("JWT_SECRET")
  if secret == "" {
    secret = "opspanel-default-secret-change-me"
  }
  jwtSecret = []byte(secret)
}

// HashPassword 生成 bcrypt 哈希
func HashPassword(password string) (string, error) {
  hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
  return string(hash), err
}

// CheckPassword 验证密码
func CheckPassword(hash, password string) bool {
  return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateToken 生成 JWT
func GenerateToken(user *User) (string, error) {
  claims := jwt.MapClaims{
    "uid":      user.ID,
    "username": user.Username,
    "role":     user.Role,
    "exp":      time.Now().Add(24 * time.Hour).Unix(),
    "iat":      time.Now().Unix(),
  }
  token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
  return token.SignedString(jwtSecret)
}

// ParseToken 解析并验证 JWT
func ParseToken(tokenStr string) (jwt.MapClaims, error) {
  token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
    if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
      return nil, errors.New("unexpected signing method")
    }
    return jwtSecret, nil
  })
  if err != nil {
    return nil, err
  }
  if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
    return claims, nil
  }
  return nil, errors.New("invalid token")
}

// Authenticate 验证用户名密码，返回用户
func Authenticate(username, password string) (*User, error) {
  var user User
  if err := db.Where("username = ?", username).First(&user).Error; err != nil {
    if errors.Is(err, gorm.ErrRecordNotFound) {
      return nil, errors.New("用户名或密码错误")
    }
    return nil, err
  }
  if !CheckPassword(user.Password, password) {
    return nil, errors.New("用户名或密码错误")
  }
  return &user, nil
}
