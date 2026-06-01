package main

import (
  "encoding/json"
  "fmt"
  "log"
  "strings"
  "sync"
  "time"

  "github.com/robfig/cron/v3"
)

// Job 定时任务模型
type Job struct {
  ID        uint       `gorm:"primaryKey" json:"id"`
  Name      string     `gorm:"size:128;not null" json:"name"`
  Module    string     `gorm:"size:64;not null" json:"module"`
  Handler   string     `gorm:"size:64;not null" json:"handler"`
  CronExpr  string     `gorm:"size:64;not null" json:"cron_expr"`
  Enabled   bool       `gorm:"default:false" json:"enabled"`
  Config    string     `gorm:"type:text" json:"config"`
  LastRun   *time.Time `json:"last_run"`
  LastError string     `gorm:"size:512" json:"last_error"`
  CreatedAt time.Time  `json:"created_at"`
  UpdatedAt time.Time  `json:"updated_at"`
}

// JobHandler 任务执行函数签名
type JobHandler func(config string) error

// HandlerInfo handler 元信息
type HandlerInfo struct {
  Fn    JobHandler `json:"-"`
  Label string     `json:"label"`
}

// ModuleInfo 模块元信息
type ModuleInfo struct {
  Value string `json:"value"`
  Label string `json:"label"`
}

// HandlerOption handler 选项（供前端下拉）
type HandlerOption struct {
  Value string `json:"value"`
  Label string `json:"label"`
}

// PLACEHOLDER_SCHEDULER_CONTINUE

// Scheduler 通用调度器
type Scheduler struct {
  cron     *cron.Cron
  entries  map[uint]cron.EntryID
  handlers map[string]HandlerInfo
  modules  []ModuleInfo
  mu       sync.RWMutex
}

var scheduler *Scheduler

func NewScheduler() *Scheduler {
  return &Scheduler{
    cron:     cron.New(cron.WithLocation(time.FixedZone("CST", 8*3600))),
    entries:  make(map[uint]cron.EntryID),
    handlers: make(map[string]HandlerInfo),
    modules:  []ModuleInfo{},
  }
}

func (s *Scheduler) RegisterHandler(name string, label string, fn JobHandler) {
  s.mu.Lock()
  defer s.mu.Unlock()
  s.handlers[name] = HandlerInfo{Fn: fn, Label: label}
  log.Printf("[scheduler] registered handler: %s (%s)", name, label)
}

func (s *Scheduler) RegisterModule(value string, label string) {
  s.mu.Lock()
  defer s.mu.Unlock()
  s.modules = append(s.modules, ModuleInfo{Value: value, Label: label})
}

func (s *Scheduler) Start() {
  var jobs []Job
  db.Where("enabled = ?", true).Find(&jobs)
  for _, job := range jobs {
    s.addJob(job)
  }
  s.cron.Start()
  log.Printf("[scheduler] started with %d active jobs", len(jobs))
}

func (s *Scheduler) addJob(job Job) {
  s.mu.RLock()
  info, ok := s.handlers[job.Handler]
  s.mu.RUnlock()
  if !ok {
    log.Printf("[scheduler] unknown handler %q for job %d", job.Handler, job.ID)
    return
  }
  jobID := job.ID
  jobConfig := job.Config
  entryID, err := s.cron.AddFunc(job.CronExpr, func() {
    s.executeJob(jobID, info.Fn, jobConfig)
  })
  if err != nil {
    log.Printf("[scheduler] invalid cron %q for job %d: %v", job.CronExpr, job.ID, err)
    return
  }
  s.mu.Lock()
  s.entries[job.ID] = entryID
  s.mu.Unlock()
}

func (s *Scheduler) executeJob(jobID uint, handler JobHandler, config string) {
  now := time.Now()
  err := handler(config)
  errMsg := ""
  if err != nil {
    errMsg = err.Error()
    log.Printf("[scheduler] job %d failed: %v", jobID, err)
  }
  db.Model(&Job{}).Where("id = ?", jobID).Updates(map[string]interface{}{
    "last_run":   now,
    "last_error": errMsg,
  })
}

// PLACEHOLDER_SCHEDULER_PART2

func (s *Scheduler) SyncJob(job Job) {
  s.mu.Lock()
  if entryID, ok := s.entries[job.ID]; ok {
    s.cron.Remove(entryID)
    delete(s.entries, job.ID)
  }
  s.mu.Unlock()
  if job.Enabled {
    s.addJob(job)
  }
}

func (s *Scheduler) RemoveJob(jobID uint) {
  s.mu.Lock()
  defer s.mu.Unlock()
  if entryID, ok := s.entries[jobID]; ok {
    s.cron.Remove(entryID)
    delete(s.entries, jobID)
  }
}

func (s *Scheduler) GetNextRun(jobID uint) string {
  s.mu.RLock()
  defer s.mu.RUnlock()
  if entryID, ok := s.entries[jobID]; ok {
    entry := s.cron.Entry(entryID)
    if !entry.Next.IsZero() {
      return entry.Next.Format("2006-01-02 15:04:05")
    }
  }
  return ""
}

func (s *Scheduler) RunJobNow(jobID uint) error {
  var job Job
  if err := db.First(&job, jobID).Error; err != nil {
    return fmt.Errorf("任务不存在")
  }
  s.mu.RLock()
  info, ok := s.handlers[job.Handler]
  s.mu.RUnlock()
  if !ok {
    return fmt.Errorf("handler %q 未注册", job.Handler)
  }
  go s.executeJob(jobID, info.Fn, job.Config)
  return nil
}

func (s *Scheduler) ListHandlers() []HandlerOption {
  s.mu.RLock()
  defer s.mu.RUnlock()
  opts := make([]HandlerOption, 0, len(s.handlers))
  for name, info := range s.handlers {
    opts = append(opts, HandlerOption{Value: name, Label: info.Label})
  }
  return opts
}

func (s *Scheduler) ListModules() []ModuleInfo {
  s.mu.RLock()
  defer s.mu.RUnlock()
  return s.modules
}

func (s *Scheduler) MigrateFromLegacy(cfg *Config) {
  if !cfg.Schedule.Enabled && cfg.Schedule.CronExpr == "" {
    return
  }
  var count int64
  db.Model(&Job{}).Where("handler = ?", "log-analysis").Count(&count)
  if count > 0 {
    return
  }
  config, _ := json.Marshal(map[string]interface{}{
    "hours": cfg.Schedule.LogHours, "batch_len": cfg.Schedule.MaxBytes,
  })
  job := Job{
    Name: "日志采集推送", Module: "log-analysis", Handler: "log-analysis",
    CronExpr: cfg.Schedule.CronExpr, Enabled: cfg.Schedule.Enabled, Config: string(config),
  }
  db.Create(&job)
  log.Printf("[scheduler] migrated legacy schedule to job #%d", job.ID)
}

// InitScheduler 初始化调度器
func InitScheduler() {
  db.AutoMigrate(&Job{})
  scheduler = NewScheduler()
  scheduler.RegisterModule("log-analysis", "日志分析")
  scheduler.RegisterHandler("log-analysis", "日志采集推送", logAnalysisHandler)
  scheduler.MigrateFromLegacy(cfg)
  scheduler.Start()
}

// logAnalysisHandler 日志推送任务
func logAnalysisHandler(config string) error {
  var params struct {
    Hours      int    `json:"hours"`
    BatchLen   int    `json:"batch_len"`
    TitleOk    string `json:"title_ok"`
    TitleAlert string `json:"title_alert"`
    MsgOk      string `json:"msg_ok"`
    MsgHeader  string `json:"msg_header"`
  }
  if config != "" {
    json.Unmarshal([]byte(config), &params)
  }
  if params.Hours == 0 { params.Hours = 24 }
  if params.BatchLen == 0 { params.BatchLen = 3500 }
  if params.TitleOk == "" { params.TitleOk = cfg.LogAnalysis.TitleOk }
  if params.TitleAlert == "" { params.TitleAlert = cfg.LogAnalysis.TitleAlert }
  if params.MsgOk == "" { params.MsgOk = cfg.LogAnalysis.MsgOk }
  if params.MsgHeader == "" { params.MsgHeader = cfg.LogAnalysis.MsgHeader }
  if params.TitleOk == "" { params.TitleOk = "✅ OpsPanel 日志报告 - %s" }
  if params.TitleAlert == "" { params.TitleAlert = "🔍 OpsPanel 日志报告 - %s" }
  if params.MsgOk == "" { params.MsgOk = "过去 %d 小时无新增错误日志，服务运行正常。" }
  if params.MsgHeader == "" { params.MsgHeader = "过去 %d 小时共 %d 条错误日志：" }

  cutoff := time.Now().Add(-time.Duration(params.Hours) * time.Hour)
  var entries []LogEntry
  db.Where("created_at >= ?", cutoff).Order("id asc").Limit(5000).Find(&entries)

  channels := GetLogAnalysisChannels()
  if len(channels) == 0 {
    addRecord(AnalysisRecord{Time: now(), Status: "error", Error: "无启用的通知渠道"})
    return nil
  }

  if len(entries) == 0 {
    msg := fmt.Sprintf(params.MsgOk, params.Hours)
    title := fmt.Sprintf(params.TitleOk, today())
    Notify(channels, title, msg)
    addRecord(AnalysisRecord{Time: now(), Status: "ok", Summary: msg})
    return nil
  }

  // 按 source+level 分组，生成简略摘要
  type group struct {
    Source  string
    Level   string
    Count   int
    Latest  string
  }
  groupKey := func(e LogEntry) string { return e.Source + "|" + e.Level }
  groups := make(map[string]*group)
  var order []string
  for _, e := range entries {
    k := groupKey(e)
    if g, ok := groups[k]; ok {
      g.Count++
      g.Latest = e.Content
    } else {
      groups[k] = &group{Source: e.Source, Level: e.Level, Count: 1, Latest: e.Content}
      order = append(order, k)
    }
  }

  var buf strings.Builder
  buf.WriteString(fmt.Sprintf(params.MsgHeader+"\n\n", params.Hours, len(entries)))
  for _, k := range order {
    g := groups[k]
    latest := g.Latest
    if len(latest) > 80 {
      latest = latest[:80] + "..."
    }
    buf.WriteString(fmt.Sprintf("[%s][%s] × %d — %s\n", g.Source, g.Level, g.Count, latest))
  }

  title := fmt.Sprintf(params.TitleAlert, today())
  body := buf.String()
  var errMsgs []string
  if err := Notify(channels, title, body); err != nil {
    errMsgs = append(errMsgs, err.Error())
  }

  status := "ok"
  errMsg := ""
  if len(errMsgs) > 0 {
    status = "error"
    errMsg = strings.Join(errMsgs, "; ")
  }
  summary := fmt.Sprintf("推送 %d 条日志（%d 类错误）", len(entries), len(order))
  addRecord(AnalysisRecord{Time: now(), Status: status, Summary: summary, Error: errMsg})
  return nil
}