package main

import (
  "bufio"
  "encoding/json"
  "fmt"
  "log"
  "net/http"
  "path/filepath"
  "regexp"
  "strings"
  "sync"
  "time"

  "github.com/gin-gonic/gin"
)

type lineBuffer struct {
  mu      sync.Mutex
  buffers map[string]*fileBuffer
}

type fileBuffer struct {
  lines   []bufLine
  pending []pendingMatch
}

type bufLine struct {
  content string
  ts      time.Time
}

type pendingMatch struct {
  matchIdx int
  matchAt  time.Time
  need     int
}

var ingestBuffer *lineBuffer
var ingestFilter *regexp.Regexp

const bufferSize = 10

func InitIngest() {
  ingestBuffer = &lineBuffer{buffers: make(map[string]*fileBuffer)}
  reloadIngestFilter()
  log.Println("[ingest] initialized")
}

// PLACEHOLDER_INGEST

func reloadIngestFilter() {
  cfgMu.RLock()
  defer cfgMu.RUnlock()
  for _, src := range cfg.LogSources {
    if src.Type == "filebeat" && src.Filter != "" {
      ingestFilter, _ = regexp.Compile("(?i)" + src.Filter)
      return
    }
  }
  for _, src := range cfg.LogSources {
    if src.Filter != "" {
      ingestFilter, _ = regexp.Compile("(?i)" + src.Filter)
      return
    }
  }
  ingestFilter, _ = regexp.Compile("(?i)ERROR|CRASH|exception|panic|fatal|崩溃|错误")
}

func RegisterIngestRoutes(r *gin.Engine) {
  r.POST("/_bulk", handleBulk)
  r.PUT("/:index/_bulk", handleBulk)
  r.POST("/:index/_bulk", handleBulk)
  r.GET("/", handleESRoot)
  r.PUT("/:index", handleESCreateIndex)
  r.HEAD("/:index", handleESHeadIndex)
  r.GET("/_template/:name", handleESTemplate)
  r.PUT("/_template/:name", handleESTemplate)
  r.GET("/_xpack", handleESXpack)
  r.GET("/_license", handleESXpack)
  r.POST("/_ilm/policy/:name", handleESAck)
  r.PUT("/_ilm/policy/:name", handleESAck)
  r.GET("/_ilm/policy/:name", handleESAck)
  r.POST("/_index_template/:name", handleESAck)
  r.PUT("/_index_template/:name", handleESAck)
  r.GET("/_index_template/:name", handleESAck)
  r.PUT("/_data_stream/:name", handleESAck)

  api := r.Group("/api")
  api.POST("/ingest", AuthMiddleware(), handleIngest)
}

func handleESRoot(c *gin.Context) {
  accept := c.GetHeader("Accept")
  if strings.Contains(accept, "text/html") || accept == "" {
    data, _ := templateFS.ReadFile("templates/index.html")
    c.Data(http.StatusOK, "text/html; charset=utf-8", data)
    return
  }
  c.JSON(http.StatusOK, gin.H{
    "name": "opspanel", "cluster_name": "opspanel",
    "version": gin.H{"number": "7.17.0"},
    "tagline": "You Know, for Logs",
  })
}

func handleESCreateIndex(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"acknowledged": true}) }
func handleESHeadIndex(c *gin.Context)   { c.Status(http.StatusOK) }
func handleESTemplate(c *gin.Context) {
  if c.Request.Method == "PUT" {
    c.JSON(http.StatusOK, gin.H{"acknowledged": true})
  } else {
    c.JSON(http.StatusOK, gin.H{})
  }
}
func handleESXpack(c *gin.Context) {
  c.JSON(http.StatusOK, gin.H{
    "build": gin.H{"hash": "unknown", "date": "2023-01-01"},
    "license": gin.H{"status": "active", "uid": "none", "type": "basic"},
    "features": gin.H{}, "tagline": "You know, for X-Pack",
  })
}
func handleESAck(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"acknowledged": true}) }

func handleBulk(c *gin.Context) {
  scanner := bufio.NewScanner(c.Request.Body)
  scanner.Buffer(make([]byte, 256*1024), 256*1024)
  var action map[string]interface{}
  lineNum := 0
  docCount := 0
  for scanner.Scan() {
    line := scanner.Text()
    if line == "" { continue }
    lineNum++
    if lineNum%2 == 1 {
      json.Unmarshal([]byte(line), &action)
      continue
    }
    var doc map[string]interface{}
    if err := json.Unmarshal([]byte(line), &doc); err != nil { continue }
    processIngestDoc(doc)
    docCount++
  }
  items := make([]gin.H, docCount)
  for i := range items {
    items[i] = gin.H{"index": gin.H{"_index": "filebeat", "_id": fmt.Sprintf("%d", i), "status": 200, "result": "created"}}
  }
  c.JSON(http.StatusOK, gin.H{"took": 1, "errors": false, "items": items})
}

func handleIngest(c *gin.Context) {
  var docs []map[string]interface{}
  if err := c.ShouldBindJSON(&docs); err != nil {
    var single map[string]interface{}
    if err2 := json.NewDecoder(c.Request.Body).Decode(&single); err2 != nil {
      c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
      return
    }
    docs = []map[string]interface{}{single}
  }
  for _, doc := range docs { processIngestDoc(doc) }
  c.JSON(http.StatusOK, gin.H{"ok": true, "received": len(docs)})
}

// PLACEHOLDER_INGEST2

func processIngestDoc(doc map[string]interface{}) {
  message := getStr(doc, "message")
  if message == "" { message = getStr(doc, "log") }
  if message == "" { return }

  source := getStr(doc, "source")
  if source == "" || source == "<nil>" {
    source = getNestedStr(doc, "log.file.path")
  }
  if source == "" || source == "<nil>" {
    if fields, ok := doc["fields"].(map[string]interface{}); ok {
      if s, ok := fields["source"]; ok { source = fmt.Sprintf("%v", s) }
    }
  }

  host := ""
  cfgMu.RLock()
  hostField := ""
  for _, src := range cfg.LogSources {
    if src.Type == "filebeat" && src.HostField != "" {
      hostField = src.HostField
      break
    }
  }
  cfgMu.RUnlock()
  if hostField != "" { host = getNestedStr(doc, hostField) }
  if host == "" {
    if fields, ok := doc["fields"].(map[string]interface{}); ok {
      if sid, ok := fields["server_id"]; ok { host = fmt.Sprintf("%v", sid) }
    }
  }
  if host == "" {
    if hostObj, ok := doc["host"].(map[string]interface{}); ok { host = fmt.Sprintf("%v", hostObj["name"]) }
  }
  if host == "" {
    if agentObj, ok := doc["agent"].(map[string]interface{}); ok { host = fmt.Sprintf("%v", agentObj["hostname"]) }
  }

  file := getNestedStr(doc, "log.file.path")
  if file == "" { file = source }

  key := source + "|" + file
  ingestBuffer.addLine(key, source, host, file, message)
}

func (lb *lineBuffer) addLine(key, source, host, file, content string) {
  lb.mu.Lock()
  defer lb.mu.Unlock()
  fb, ok := lb.buffers[key]
  if !ok {
    fb = &fileBuffer{}
    lb.buffers[key] = fb
  }
  fb.lines = append(fb.lines, bufLine{content: content, ts: time.Now()})
  if len(fb.lines) > bufferSize { fb.lines = fb.lines[len(fb.lines)-bufferSize:] }
  curIdx := len(fb.lines) - 1
  if ingestFilter != nil && ingestFilter.MatchString(content) {
    fb.pending = append(fb.pending, pendingMatch{matchIdx: curIdx, matchAt: time.Now(), need: 2})
  }
  var completed []pendingMatch
  var remaining []pendingMatch
  for _, p := range fb.pending {
    if curIdx-p.matchIdx >= p.need || time.Since(p.matchAt) > 10*time.Second {
      completed = append(completed, p)
    } else {
      remaining = append(remaining, p)
    }
  }
  fb.pending = remaining
  for _, p := range completed { lb.storeMatch(fb, p, source, host, file) }
}

func (lb *lineBuffer) storeMatch(fb *fileBuffer, p pendingMatch, source, host, file string) {
  if p.matchIdx >= len(fb.lines) { return }
  matchLine := fb.lines[p.matchIdx].content
  var ctxLines []string
  for i := p.matchIdx - 2; i <= p.matchIdx+2; i++ {
    if i < 0 || i >= len(fb.lines) || i == p.matchIdx { continue }
    ctxLines = append(ctxLines, fb.lines[i].content)
  }
  entry := LogEntry{
    Source: source, Host: host, File: filepath.Base(file),
    Content: matchLine, Context: strings.Join(ctxLines, "\n"), Level: classifyLevel(matchLine),
  }
  db.Create(&entry)
}

func (lb *lineBuffer) cleanup() {
  lb.mu.Lock()
  defer lb.mu.Unlock()
  for key, fb := range lb.buffers {
    var remaining []pendingMatch
    for _, p := range fb.pending {
      if time.Since(p.matchAt) > 30*time.Second {
        lb.storeMatch(fb, p, key, "", key)
      } else {
        remaining = append(remaining, p)
      }
    }
    fb.pending = remaining
    if len(fb.lines) > 0 && time.Since(fb.lines[len(fb.lines)-1].ts) > 5*time.Minute {
      delete(lb.buffers, key)
    }
  }
}

func getStr(m map[string]interface{}, key string) string {
  if v, ok := m[key]; ok { return fmt.Sprintf("%v", v) }
  return ""
}

func getNestedStr(m map[string]interface{}, path string) string {
  parts := strings.Split(path, ".")
  current := m
  for i, p := range parts {
    v, ok := current[p]
    if !ok { return "" }
    if i == len(parts)-1 { return fmt.Sprintf("%v", v) }
    next, ok := v.(map[string]interface{})
    if !ok { return "" }
    current = next
  }
  return ""
}