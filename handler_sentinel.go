package main

import (
  "encoding/json"
  "fmt"
  "log"
  "net/http"
  "strings"
  "time"

  "github.com/gin-gonic/gin"
)

// RegisterSentinelRoutes 注册日志分析相关路由
func RegisterSentinelRoutes(r *gin.RouterGroup) {
  // 读操作 — viewer 可访问
  read := r.Group("", AuthMiddleware())
  {
    read.GET("/config", handleGetConfig)
    read.GET("/history", handleGetHistory)
    read.GET("/status", handleGetStatus)
    read.GET("/issues", handleGetIssues)
  }

  // 写操作 — admin only
  write := r.Group("", AuthMiddleware(), AdminOnly())
  {
    write.POST("/config", handlePostConfig)
    write.POST("/trigger", handleTrigger)
    write.POST("/test-notify", handleTestNotify)
    write.POST("/test-collect", handleTestCollect)
    write.POST("/test-llm", handleTestLLM)
    write.POST("/issues", handlePostIssues)
  }
}

func handleGetConfig(c *gin.Context) {
  cfgMu.RLock()
  safe := *cfg
  cfgMu.RUnlock()
  if len(safe.LLM.APIKey) > 8 {
    safe.LLM.APIKey = safe.LLM.APIKey[:4] + "****" + safe.LLM.APIKey[len(safe.LLM.APIKey)-4:]
  }
  for i := range safe.Notifications {
    if s := safe.Notifications[i].Secret; len(s) > 4 {
      safe.Notifications[i].Secret = s[:2] + "****"
    }
  }
  c.JSON(http.StatusOK, safe)
}

// PLACEHOLDER_SENTINEL_HANDLERS

func handlePostConfig(c *gin.Context) {
  var newCfg Config
  if err := c.ShouldBindJSON(&newCfg); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }
  cfgMu.Lock()
  if strings.Contains(newCfg.LLM.APIKey, "****") {
    newCfg.LLM.APIKey = cfg.LLM.APIKey
  }
  for i := range newCfg.Notifications {
    if strings.Contains(newCfg.Notifications[i].Secret, "****") && i < len(cfg.Notifications) {
      newCfg.Notifications[i].Secret = cfg.Notifications[i].Secret
    }
  }
  *cfg = newCfg
  SaveConfig(cfg)
  cfgMu.Unlock()

  if cfg.Schedule.Enabled && cfg.Schedule.CronExpr != "" {
    setCron(cfg.Schedule.CronExpr)
  } else {
    clearCron()
  }
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleTrigger(c *gin.Context) {
  go runAnalysis()
  c.JSON(http.StatusOK, gin.H{"ok": true, "msg": "triggered"})
}

func handleGetHistory(c *gin.Context) {
  histMu.RLock()
  defer histMu.RUnlock()
  c.JSON(http.StatusOK, history)
}

func handleGetStatus(c *gin.Context) {
  cfgMu.RLock()
  co := *cfg
  cfgMu.RUnlock()

  histMu.RLock()
  histCopy := make([]AnalysisRecord, len(history))
  copy(histCopy, history)
  histMu.RUnlock()

  enabledChannels := 0
  for _, ch := range co.Notifications {
    if ch.Enabled {
      enabledChannels++
    }
  }

  var lastRun, lastStatus string
  if len(histCopy) > 0 {
    lastRun = histCopy[0].Time
    lastStatus = histCopy[0].Status
  }

  var nextRun string
  if cronEntryID != 0 {
    entry := cronScheduler.Entry(cronEntryID)
    if !entry.Next.IsZero() {
      nextRun = entry.Next.Format("2006-01-02 15:04:05")
    }
  }

  c.JSON(http.StatusOK, gin.H{
    "sources_count":    len(co.LogSources),
    "channels_count":   len(co.Notifications),
    "channels_enabled": enabledChannels,
    "schedule_enabled": co.Schedule.Enabled,
    "schedule_cron":    co.Schedule.CronExpr,
    "next_run":         nextRun,
    "last_run":         lastRun,
    "last_status":      lastStatus,
    "llm_configured":   co.LLM.APIURL != "" && co.LLM.APIKey != "",
    "recent_history":   histCopy,
  })
}

func handleTestNotify(c *gin.Context) {
  var req struct {
    Index int `json:"index"`
  }
  c.ShouldBindJSON(&req)

  cfgMu.RLock()
  defer cfgMu.RUnlock()
  if req.Index < 0 || req.Index >= len(cfg.Notifications) {
    c.JSON(http.StatusBadRequest, gin.H{"error": "invalid index"})
    return
  }
  ch := cfg.Notifications[req.Index]
  err := NotifySingle(ch, "🔔 测试消息", "OpsPanel 通知测试成功！\n时间: "+time.Now().Format("2006-01-02 15:04:05"))
  if err != nil {
    c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
    return
  }
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleTestCollect(c *gin.Context) {
  var src LogSource
  if err := c.ShouldBindJSON(&src); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }
  result := TestCollect(src)
  c.JSON(http.StatusOK, result)
}

func handleTestLLM(c *gin.Context) {
  cfgMu.RLock()
  llmCfg := cfg.LLM
  cfgMu.RUnlock()

  if llmCfg.APIURL == "" || llmCfg.APIKey == "" {
    c.JSON(http.StatusOK, gin.H{"ok": false, "error": "LLM 未配置"})
    return
  }
  reply, err := CallLLM(llmCfg, "测试日志: [INFO] 系统运行正常", 1)
  if err != nil {
    c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
    return
  }
  c.JSON(http.StatusOK, gin.H{"ok": true, "reply": truncate(reply, 200)})
}

func handleGetIssues(c *gin.Context) {
  issueMu.RLock()
  defer issueMu.RUnlock()
  c.JSON(http.StatusOK, issues)
}

func handlePostIssues(c *gin.Context) {
  var req struct {
    ID     string `json:"id"`
    Status string `json:"status"`
  }
  if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }
  issueMu.Lock()
  for i := range issues {
    if issues[i].ID == req.ID {
      issues[i].Status = req.Status
      break
    }
  }
  SaveIssues(issues)
  issueMu.Unlock()
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

// runAnalysis 保持原有逻辑不变
func runAnalysis() {
  log.Println("[opspanel] running analysis...")
  cfgMu.RLock()
  c := *cfg
  cfgMu.RUnlock()

  enabledChannels := 0
  for _, ch := range c.Notifications {
    if ch.Enabled {
      enabledChannels++
    }
  }
  if enabledChannels == 0 {
    addRecord(AnalysisRecord{Time: now(), Status: "error", Error: "无启用的通知渠道"})
    return
  }

  logs := CollectLogs(c.LogSources, c.Schedule.MaxBytes)

  if len(strings.TrimSpace(logs)) < 10 {
    msg := fmt.Sprintf("过去 %d 小时无匹配的错误日志，服务运行正常。", c.Schedule.LogHours)
    title := fmt.Sprintf("✅ OpsPanel 日志报告 - %s", today())
    err := Notify(c.Notifications, title, msg)
    status := "ok"
    errMsg := ""
    if err != nil {
      status = "error"
      errMsg = err.Error()
    }
    addRecord(AnalysisRecord{Time: now(), Status: status, Summary: msg, Error: errMsg})
    return
  }

  var summary string
  var newIssues []Issue
  if c.LLM.APIURL != "" && c.LLM.APIKey != "" {
    var err error
    summary, err = CallLLM(c.LLM, logs, c.Schedule.LogHours)
    if err != nil {
      log.Printf("[opspanel] LLM failed: %v", err)
      summary = "⚠️ LLM 分析失败（" + err.Error() + "），原始日志摘要：\n\n" + truncate(logs, 3000)
    } else {
      newIssues = ParseIssuesFromLLM(summary)
      if len(newIssues) > 0 {
        addIssues(newIssues)
        log.Printf("[opspanel] parsed %d issues from LLM", len(newIssues))
      }
    }
  } else {
    summary = "ℹ️ LLM 未配置，原始错误日志：\n\n" + truncate(logs, 3000)
  }

  var notifyBody string
  if len(newIssues) > 0 {
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("发现 %d 个问题：\n\n", len(newIssues)))
    for _, iss := range newIssues {
      sb.WriteString(fmt.Sprintf("[%s] %s\n原因: %s\n影响: %s\n\n", iss.Priority, iss.Title, iss.Cause, iss.Impact))
    }
    notifyBody = sb.String()
  } else {
    notifyBody = summary
  }

  title := fmt.Sprintf("🔍 OpsPanel 日志分析报告 - %s", today())
  err := Notify(c.Notifications, title, notifyBody)
  status := "ok"
  errMsg := ""
  if err != nil {
    status = "error"
    errMsg = err.Error()
  }
  recordSummary := truncate(notifyBody, 500)
  if len(newIssues) > 0 {
    recordSummary = fmt.Sprintf("发现 %d 个问题（%s）", len(newIssues), newIssues[0].Priority+": "+newIssues[0].Title)
  }
  addRecord(AnalysisRecord{Time: now(), Status: status, Summary: recordSummary, Error: errMsg})
}
