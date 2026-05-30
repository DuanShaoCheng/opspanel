package main

import (
  "net/http"

  "github.com/gin-gonic/gin"
)

// RegisterSchedulerRoutes 注册定时任务路由
func RegisterSchedulerRoutes(r *gin.RouterGroup) {
  jobs := r.Group("/jobs", AuthMiddleware())
  {
    jobs.GET("", handleListJobs)
    jobs.GET("/handlers", handleListHandlers)
    jobs.GET("/modules", handleListModules)
    jobs.POST("", AdminOnly(), handleCreateJob)
    jobs.PUT("/:id", AdminOnly(), handleUpdateJob)
    jobs.DELETE("/:id", AdminOnly(), handleDeleteJob)
    jobs.POST("/:id/run", AdminOnly(), handleRunJob)
  }
}

func handleListJobs(c *gin.Context) {
  var jobs []Job
  db.Order("id asc").Find(&jobs)
  type JobWithNext struct {
    Job
    NextRun string `json:"next_run"`
  }
  result := make([]JobWithNext, len(jobs))
  for i, j := range jobs {
    result[i] = JobWithNext{Job: j, NextRun: scheduler.GetNextRun(j.ID)}
  }
  c.JSON(http.StatusOK, result)
}

func handleListHandlers(c *gin.Context) {
  c.JSON(http.StatusOK, scheduler.ListHandlers())
}

func handleListModules(c *gin.Context) {
  c.JSON(http.StatusOK, scheduler.ListModules())
}

// PLACEHOLDER_HANDLER_SCHEDULER

func handleCreateJob(c *gin.Context) {
  var req struct {
    Name     string `json:"name" binding:"required"`
    Module   string `json:"module" binding:"required"`
    Handler  string `json:"handler" binding:"required"`
    CronExpr string `json:"cron_expr" binding:"required"`
    Enabled  bool   `json:"enabled"`
    Config   string `json:"config"`
  }
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }
  job := Job{
    Name: req.Name, Module: req.Module, Handler: req.Handler,
    CronExpr: req.CronExpr, Enabled: req.Enabled, Config: req.Config,
  }
  if err := db.Create(&job).Error; err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
  }
  scheduler.SyncJob(job)
  c.JSON(http.StatusOK, gin.H{"ok": true, "id": job.ID})
}

func handleUpdateJob(c *gin.Context) {
  id := c.Param("id")
  var job Job
  if err := db.First(&job, id).Error; err != nil {
    c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
    return
  }
  var req struct {
    Name     *string `json:"name"`
    CronExpr *string `json:"cron_expr"`
    Enabled  *bool   `json:"enabled"`
    Config   *string `json:"config"`
  }
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }
  if req.Name != nil { job.Name = *req.Name }
  if req.CronExpr != nil { job.CronExpr = *req.CronExpr }
  if req.Enabled != nil { job.Enabled = *req.Enabled }
  if req.Config != nil { job.Config = *req.Config }
  db.Save(&job)
  scheduler.SyncJob(job)
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleDeleteJob(c *gin.Context) {
  id := c.Param("id")
  var job Job
  if err := db.First(&job, id).Error; err != nil {
    c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
    return
  }
  scheduler.RemoveJob(job.ID)
  db.Delete(&job)
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleRunJob(c *gin.Context) {
  id := c.Param("id")
  var job Job
  if err := db.First(&job, id).Error; err != nil {
    c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
    return
  }
  if err := scheduler.RunJobNow(job.ID); err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
  }
  c.JSON(http.StatusOK, gin.H{"ok": true, "msg": "已触发执行"})
}