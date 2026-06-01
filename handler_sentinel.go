package main

import (
  "encoding/json"
  "fmt"
  "log"
  "net/http"
  "strconv"
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
    write.DELETE("/history", handleClearHistory)
    write.DELETE("/history/:index", handleDeleteHistoryItem)
  }
}

func handleGetConfig(c *gin.Context) {
  cfgMu.RLock()
  defer cfgMu.RUnlock()

  // 深拷贝配置以避免修改原始数据
  safe := Config{
    LogSources:       append([]LogSource{}, cfg.LogSources...),
    Notifications:    make([]Notification, len(cfg.Notifications)),
    LLMProviders:     make([]LLMProvider, len(cfg.LLMProviders)),
    Schedule:         cfg.Schedule,
    SMTP:             cfg.SMTP,
    LogAnalysis:      cfg.LogAnalysis,
    HostFieldOptions: cfg.HostFieldOptions,
    LevelRules:       cfg.LevelRules,
  }

  // 如果提示词为空，返回默认提示词供前端展示
  if safe.LogAnalysis.LLMPrompt == "" {
    safe.LogAnalysis.LLMPrompt = structuredPrompt
  }

  // 深拷贝并脱敏 LLM Providers
  for i := range cfg.LLMProviders {
    safe.LLMProviders[i] = cfg.LLMProviders[i]
    if len(safe.LLMProviders[i].APIKey) > 8 {
      safe.LLMProviders[i].APIKey = safe.LLMProviders[i].APIKey[:4] + "****" + safe.LLMProviders[i].APIKey[len(safe.LLMProviders[i].APIKey)-4:]
    }
  }

  // 深拷贝并脱敏 Notifications
  for i := range cfg.Notifications {
    safe.Notifications[i] = cfg.Notifications[i]
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
  for i := range newCfg.LLMProviders {
    if strings.Contains(newCfg.LLMProviders[i].APIKey, "****") && i < len(cfg.LLMProviders) {
      newCfg.LLMProviders[i].APIKey = cfg.LLMProviders[i].APIKey
    }
  }
  for i := range newCfg.Notifications {
    if strings.Contains(newCfg.Notifications[i].Secret, "****") && i < len(cfg.Notifications) {
      newCfg.Notifications[i].Secret = cfg.Notifications[i].Secret
    }
  }
  *cfg = newCfg
  SaveConfig(cfg)
  schedEnabled := cfg.Schedule.Enabled
  schedCron := cfg.Schedule.CronExpr
  cfgMu.Unlock()

  if schedEnabled && schedCron != "" {
    setCron(schedCron)
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

func handleClearHistory(c *gin.Context) {
  histMu.Lock()
  history = []AnalysisRecord{}
  SaveHistory(history)
  histMu.Unlock()
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleDeleteHistoryItem(c *gin.Context) {
  idx, err := strconv.Atoi(c.Param("index"))
  if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "invalid index"})
    return
  }
  histMu.Lock()
  if idx < 0 || idx >= len(history) {
    histMu.Unlock()
    c.JSON(http.StatusBadRequest, gin.H{"error": "index out of range"})
    return
  }
  history = append(history[:idx], history[idx+1:]...)
  SaveHistory(history)
  histMu.Unlock()
  c.JSON(http.StatusOK, gin.H{"ok": true})
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

  // 从新的 scheduler 系统获取定时任务状态
  var jobs []Job
  db.Where("enabled = ?", true).Find(&jobs)

  var scheduleEnabled bool
  var scheduleName string
  var scheduleCron string
  var nextRun string

  if len(jobs) > 0 {
    // 使用第一个启用的任务作为主要显示
    job := jobs[0]
    scheduleEnabled = true
    scheduleName = job.Name
    scheduleCron = job.CronExpr
    nextRun = scheduler.GetNextRun(job.ID)
  } else {
    // 如果没有启用的任务，检查是否有任何任务存在
    var allJobs []Job
    db.Find(&allJobs)
    if len(allJobs) > 0 {
      job := allJobs[0]
      scheduleEnabled = false
      scheduleName = job.Name
      scheduleCron = job.CronExpr
    }
  }

  llmConfigured := GetLLMProvider("") != nil

  c.JSON(http.StatusOK, gin.H{
    "sources_count":    len(co.LogSources),
    "channels_count":   len(co.Notifications),
    "channels_enabled": enabledChannels,
    "schedule_enabled": scheduleEnabled,
    "schedule_name":    scheduleName,
    "schedule_cron":    scheduleCron,
    "next_run":         nextRun,
    "last_run":         lastRun,
    "last_status":      lastStatus,
    "llm_configured":   llmConfigured,
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
  var req struct {
    Index int `json:"index"`
  }
  c.ShouldBindJSON(&req)

  log.Printf("[test-llm] received index: %d", req.Index)

  var provider *LLMProvider
  if req.Index >= 0 {
    // 测试指定索引的提供商
    cfgMu.RLock()
    log.Printf("[test-llm] cfg.LLMProviders length: %d", len(cfg.LLMProviders))
    if req.Index < len(cfg.LLMProviders) {
      provider = &cfg.LLMProviders[req.Index]
      log.Printf("[test-llm] found provider: %s (id=%s)", provider.Name, provider.ID)
    } else {
      log.Printf("[test-llm] index %d out of range", req.Index)
    }
    cfgMu.RUnlock()
  } else {
    // 测试当前日志分析配置的提供商
    cfgMu.RLock()
    providerID := cfg.LogAnalysis.LLMProvider
    cfgMu.RUnlock()
    log.Printf("[test-llm] using log_analysis provider: %s", providerID)
    provider = GetLLMProvider(providerID)
  }

  if provider == nil {
    log.Printf("[test-llm] provider is nil, returning error")
    c.JSON(http.StatusOK, gin.H{"ok": false, "error": "LLM 未配置"})
    return
  }
  log.Printf("[test-llm] calling LLM with provider: %s", provider.ID)
  reply, err := CallLLM(provider, cfg.LogAnalysis.LLMPrompt, "测试日志: [INFO] 系统运行正常", 1)
  if err != nil {
    log.Printf("[test-llm] LLM call failed: %v", err)
    c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
    return
  }
  log.Printf("[test-llm] LLM call succeeded")
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
  provider := GetLLMProvider(c.LogAnalysis.LLMProvider)
  if provider != nil {
    var err error
    summary, err = CallLLM(provider, c.LogAnalysis.LLMPrompt, logs, c.Schedule.LogHours)
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
