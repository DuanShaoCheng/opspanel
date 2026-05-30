package main

import (
  "fmt"
  "net/http"

  "github.com/gin-gonic/gin"
)

// RegisterAuthRoutes 注册认证相关路由
func RegisterAuthRoutes(r *gin.RouterGroup) {
  auth := r.Group("/auth")
  {
    auth.POST("/login", handleLogin)
    auth.GET("/me", AuthMiddleware(), handleMe)
    auth.POST("/password", AuthMiddleware(), handleChangePassword)
  }

  // 用户管理（admin only）
  users := r.Group("/users", AuthMiddleware(), AdminOnly())
  {
    users.GET("", handleListUsers)
    users.POST("", handleCreateUser)
    users.PUT("/:id", handleUpdateUser)
    users.DELETE("/:id", handleDeleteUser)
  }
}

func handleLogin(c *gin.Context) {
  var req struct {
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required"`
  }
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "请输入用户名和密码"})
    return
  }
  user, err := Authenticate(req.Username, req.Password)
  if err != nil {
    c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
    return
  }
  token, err := GenerateToken(user)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 token 失败"})
    return
  }
  c.JSON(http.StatusOK, gin.H{
    "token":    token,
    "username": user.Username,
    "role":     user.Role,
  })
}

func handleMe(c *gin.Context) {
  username, _ := c.Get("username")
  role, _ := c.Get("role")
  c.JSON(http.StatusOK, gin.H{"username": username, "role": role})
}

func handleChangePassword(c *gin.Context) {
  var req struct {
    OldPassword string `json:"old_password" binding:"required"`
    NewPassword string `json:"new_password" binding:"required,min=6"`
  }
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "密码至少 6 位"})
    return
  }
  username, _ := c.Get("username")
  var user User
  if err := db.Where("username = ?", username).First(&user).Error; err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "用户不存在"})
    return
  }
  if !CheckPassword(user.Password, req.OldPassword) {
    c.JSON(http.StatusBadRequest, gin.H{"error": "原密码错误"})
    return
  }
  hash, _ := HashPassword(req.NewPassword)
  db.Model(&user).Update("password", hash)
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleListUsers(c *gin.Context) {
  var users []User
  db.Order("id asc").Find(&users)
  c.JSON(http.StatusOK, users)
}

func handleCreateUser(c *gin.Context) {
  var req struct {
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required,min=6"`
    Role     string `json:"role" binding:"required,oneof=admin viewer"`
  }
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
    return
  }
  hash, _ := HashPassword(req.Password)
  user := User{Username: req.Username, Password: hash, Role: req.Role}
  if err := db.Create(&user).Error; err != nil {
    c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
    return
  }
  c.JSON(http.StatusOK, gin.H{"ok": true, "id": user.ID})
}

func handleUpdateUser(c *gin.Context) {
  id := c.Param("id")
  var user User
  if err := db.First(&user, id).Error; err != nil {
    c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
    return
  }
  var req struct {
    Role     string `json:"role"`
    Password string `json:"password"`
  }
  c.ShouldBindJSON(&req)
  if req.Role != "" {
    user.Role = req.Role
  }
  if req.Password != "" {
    hash, _ := HashPassword(req.Password)
    user.Password = hash
  }
  db.Save(&user)
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleDeleteUser(c *gin.Context) {
  id := c.Param("id")
  // 不允许删除自己
  uid, _ := c.Get("uid")
  if fmt.Sprintf("%v", uid) == id {
    c.JSON(http.StatusBadRequest, gin.H{"error": "不能删除自己"})
    return
  }
  db.Delete(&User{}, id)
  c.JSON(http.StatusOK, gin.H{"ok": true})
}
