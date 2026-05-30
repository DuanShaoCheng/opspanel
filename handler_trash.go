package main

import (
  "net/http"

  "github.com/gin-gonic/gin"
)

// RegisterTrashRoutes 注册回收站路由
func RegisterTrashRoutes(r *gin.RouterGroup) {
  trash := r.Group("/trash", AuthMiddleware(), AdminOnly())
  {
    trash.GET("", handleListTrash)
    trash.POST("/restore", handleRestoreTrash)
    trash.DELETE("", handlePurgeTrash)
  }
}

func handleListTrash(c *gin.Context) {
  var logs []LogEntry
  db.Unscoped().Where("deleted_at IS NOT NULL").Order("deleted_at desc").Limit(200).Find(&logs)

  var jobs []Job
  db.Unscoped().Where("deleted_at IS NOT NULL").Order("deleted_at desc").Find(&jobs)

  c.JSON(http.StatusOK, gin.H{
    "logs": logs,
    "jobs": jobs,
  })
}

func handleRestoreTrash(c *gin.Context) {
  var req struct {
    Type string `json:"type" binding:"required"` // logs, jobs
    IDs  []uint `json:"ids" binding:"required"`
  }
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }

  switch req.Type {
  case "logs":
    db.Unscoped().Model(&LogEntry{}).Where("id IN ?", req.IDs).Update("deleted_at", nil)
  case "jobs":
    db.Unscoped().Model(&Job{}).Where("id IN ?", req.IDs).Update("deleted_at", nil)
    // 恢复后重新同步调度
    var jobs []Job
    db.Where("id IN ? AND enabled = ?", req.IDs, true).Find(&jobs)
    for _, j := range jobs {
      scheduler.SyncJob(j)
    }
  default:
    c.JSON(http.StatusBadRequest, gin.H{"error": "无效类型"})
    return
  }
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handlePurgeTrash(c *gin.Context) {
  var req struct {
    Type string `json:"type"` // logs, jobs, 空=全部
    IDs  []uint `json:"ids"`  // 空=该类型全部
  }
  c.ShouldBindJSON(&req)

  if req.Type == "" || req.Type == "logs" {
    if len(req.IDs) > 0 {
      db.Unscoped().Where("id IN ? AND deleted_at IS NOT NULL", req.IDs).Delete(&LogEntry{})
    } else {
      db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&LogEntry{})
    }
  }
  if req.Type == "" || req.Type == "jobs" {
    if len(req.IDs) > 0 {
      db.Unscoped().Where("id IN ? AND deleted_at IS NOT NULL", req.IDs).Delete(&Job{})
    } else {
      db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&Job{})
    }
  }
  c.JSON(http.StatusOK, gin.H{"ok": true})
}
