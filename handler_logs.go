package main

import (
  "encoding/json"
  "fmt"
  "net/http"
  "strconv"

  "github.com/gin-gonic/gin"
)

// RegisterLogRoutes 注册日志采集相关路由
func RegisterLogRoutes(r *gin.RouterGroup) {
  logs := r.Group("/logs", AuthMiddleware())
  {
    logs.GET("", handleGetLogs)
    logs.GET("/:id", handleGetLogDetail)
    logs.GET("/stats", handleLogStats)
    logs.POST("/:id/analyze", AdminOnly(), handleAnalyzeLog)
    logs.DELETE("", AdminOnly(), handleClearLogs)
    logs.DELETE("/:id", AdminOnly(), handleDeleteLog)
  }
}

func handleGetLogs(c *gin.Context) {
  page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
  size, _ := strconv.Atoi(c.DefaultQuery("size", "50"))
  level := c.Query("level")
  source := c.Query("source")
  host := c.Query("host")

  if page < 1 {
    page = 1
  }
  if size < 1 || size > 200 {
    size = 50
  }

  query := db.Model(&LogEntry{}).Order("id desc")
  if level != "" {
    query = query.Where("level = ?", level)
  }
  if source != "" {
    query = query.Where("source = ?", source)
  }
  if host != "" {
    query = query.Where("host = ?", host)
  }

  var total int64
  query.Count(&total)

// PLACEHOLDER_CONTINUE

  var entries []LogEntry
  query.Offset((page - 1) * size).Limit(size).Find(&entries)

  // 返回可用的 host 列表供前端筛选
  var hosts []string
  db.Model(&LogEntry{}).Distinct("host").Where("host != ''").Pluck("host", &hosts)

  c.JSON(http.StatusOK, gin.H{
    "total":   total,
    "page":    page,
    "size":    size,
    "entries": entries,
    "hosts":   hosts,
  })
}

func handleGetLogDetail(c *gin.Context) {
  id, err := strconv.ParseUint(c.Param("id"), 10, 64)
  if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
    return
  }
  var entry LogEntry
  if err := db.First(&entry, id).Error; err != nil {
    c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
    return
  }
  c.JSON(http.StatusOK, entry)
}

func handleAnalyzeLog(c *gin.Context) {
  id, err := strconv.ParseUint(c.Param("id"), 10, 64)
  if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
    return
  }
  var entry LogEntry
  if err := db.First(&entry, id).Error; err != nil {
    c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
    return
  }

  cfgMu.RLock()
  providerID := cfg.LogAnalysis.LLMProvider
  modulePrompt := cfg.LogAnalysis.LLMPrompt
  cfgMu.RUnlock()

  provider := GetLLMProvider(providerID)
  if provider == nil || provider.APIURL == "" {
    c.JSON(http.StatusBadRequest, gin.H{"error": "LLM 未配置，请在系统 → LLM 中添加配置，并在日志分析 → AI 配置中选择"})
    return
  }

  // 构建分析 prompt
  logContent := fmt.Sprintf("错误日志:\n%s\n\n上下文:\n%s", entry.Content, entry.Context)
  if modulePrompt == "" {
    modulePrompt = "你是服务器运维专家。请分析错误日志，返回 JSON：{\"score\":<1-10>,\"analysis\":\"<说明>\"}"
  }

  reply, err := CallLLMWithProvider(provider, modulePrompt, logContent)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "LLM 调用失败: " + err.Error()})
    return
  }

  // 解析 LLM 返回的 JSON
  var result struct {
    Score    int    `json:"score"`
    Analysis string `json:"analysis"`
  }
  jsonStr := extractJSON(reply)
  if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
    result.Score = 5
    result.Analysis = reply
  }

  db.Model(&entry).Updates(map[string]interface{}{
    "ai_score":    result.Score,
    "ai_analysis": result.Analysis,
    "analyzed":    true,
  })

  c.JSON(http.StatusOK, gin.H{
    "ok":       true,
    "score":    result.Score,
    "analysis": result.Analysis,
  })
}

// extractJSON 从文本中提取第一个 JSON 对象
func extractJSON(text string) string {
  start := -1
  for i, c := range text {
    if c == '{' {
      start = i
      break
    }
  }
  if start < 0 {
    return text
  }
  depth := 0
  for i := start; i < len(text); i++ {
    if text[i] == '{' {
      depth++
    } else if text[i] == '}' {
      depth--
      if depth == 0 {
        return text[start : i+1]
      }
    }
  }
  return text[start:]
}

func handleLogStats(c *gin.Context) {
  stats := watcher.GetStats()
  c.JSON(http.StatusOK, stats)
}

func handleClearLogs(c *gin.Context) {
  var req struct {
    Before string `json:"before"`
    IDs    []uint `json:"ids"`
  }
  c.ShouldBindJSON(&req)

  if len(req.IDs) > 0 {
    db.Where("id IN ?", req.IDs).Delete(&LogEntry{})
  } else if req.Before != "" {
    db.Where("created_at < ?", req.Before).Delete(&LogEntry{})
  } else {
    db.Where("analyzed = ?", true).Delete(&LogEntry{})
  }
  c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleDeleteLog(c *gin.Context) {
  id, err := strconv.ParseUint(c.Param("id"), 10, 64)
  if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
    return
  }
  result := db.Delete(&LogEntry{}, id)
  if result.RowsAffected == 0 {
    c.JSON(http.StatusNotFound, gin.H{"error": "日志不存在"})
    return
  }
  c.JSON(http.StatusOK, gin.H{"ok": true})
}