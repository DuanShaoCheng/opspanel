package main

import (
  "net/http"
  "os"
  "strings"

  "github.com/gin-gonic/gin"
)

// AuthMiddleware JWT 认证中间件
func AuthMiddleware() gin.HandlerFunc {
  return func(c *gin.Context) {
    auth := c.GetHeader("Authorization")
    if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
      c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
      return
    }
    tokenStr := strings.TrimPrefix(auth, "Bearer ")
    claims, err := ParseToken(tokenStr)
    if err != nil {
      c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
      return
    }
    c.Set("uid", claims["uid"])
    c.Set("username", claims["username"])
    c.Set("role", claims["role"])
    c.Next()
  }
}

// AdminOnly 管理员权限中间件
func AdminOnly() gin.HandlerFunc {
  return func(c *gin.Context) {
    role, _ := c.Get("role")
    if role != "admin" {
      c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "权限不足"})
      return
    }
    c.Next()
  }
}

// IngestAuthMiddleware Ingest 端点认证中间件
// 通过环境变量 INGEST_API_KEY 设置，未设置时允许所有请求（向后兼容）
func IngestAuthMiddleware() gin.HandlerFunc {
  apiKey := os.Getenv("INGEST_API_KEY")
  return func(c *gin.Context) {
    if apiKey == "" {
      c.Next()
      return
    }
    key := c.GetHeader("X-API-Key")
    if key == "" {
      // 也支持 Authorization: ApiKey <key> 格式
      auth := c.GetHeader("Authorization")
      if strings.HasPrefix(auth, "ApiKey ") {
        key = strings.TrimPrefix(auth, "ApiKey ")
      }
    }
    if key != apiKey {
      c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
      return
    }
    c.Next()
  }
}
