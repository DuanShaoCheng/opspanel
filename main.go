package main

import (
  "embed"
  "log"
  "net/http"
  "os"
  "time"

  "github.com/gin-gonic/gin"
  "github.com/robfig/cron/v3"
)

//go:embed templates/*
var templateFS embed.FS

var (
  dataDir    = env("DATA_DIR", "/data")
  listenAddr = env("LISTEN_ADDR", ":9090")
)

var (
  cronScheduler *cron.Cron
  cronEntryID   cron.EntryID
)

func main() {
  log.SetFlags(log.LstdFlags | log.Lshortfile)
  log.Println("[opspanel] starting...")

  os.MkdirAll(dataDir, 0755)

  // 初始化数据库和 JWT
  InitDatabase()
  InitJWTSecret()

  // 加载应用配置（JSON 文件）
  cfg = LoadConfig()
  LoadEnvOverrides(cfg)
  history = LoadHistory()
  issues = LoadIssues()
  scanState = LoadScanState()

  // 定时任务
  cronScheduler = cron.New(cron.WithLocation(time.FixedZone("CST", 8*3600)))
  if cfg.Schedule.Enabled && cfg.Schedule.CronExpr != "" {
    setCron(cfg.Schedule.CronExpr)
  }
  cronScheduler.Start()

  // 初始化日志接收（Filebeat）
  InitIngest()

  // 初始化调度器
  InitScheduler()

  // 初始化文件监控
  InitWatcher()

  // Gin 路由
  if os.Getenv("GIN_MODE") == "" {
    gin.SetMode(gin.ReleaseMode)
  }
  r := gin.New()
  r.Use(gin.Recovery())

  // Filebeat/Ingest 路由（需要在 SPA 路由之前注册）
  RegisterIngestRoutes(r)

  // 静态页面
  r.GET("/healthz", func(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"status": "ok"})
  })

  // API 路由组
  api := r.Group("/api")
  RegisterAuthRoutes(api)
  RegisterSentinelRoutes(api)
  RegisterLogRoutes(api)
  RegisterSchedulerRoutes(api)
  RegisterTrashRoutes(api)

  log.Printf("[opspanel] listening on %s", listenAddr)
  log.Fatal(r.Run(listenAddr))
}

func serveIndex(c *gin.Context) {
  data, err := templateFS.ReadFile("templates/index.html")
  if err != nil {
    c.String(http.StatusInternalServerError, err.Error())
    return
  }
  c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func setCron(expr string) {
  if cronEntryID != 0 {
    cronScheduler.Remove(cronEntryID)
  }
  var err error
  cronEntryID, err = cronScheduler.AddFunc(expr, runAnalysis)
  if err != nil {
    log.Printf("[opspanel] invalid cron %q: %v", expr, err)
  } else {
    log.Printf("[opspanel] cron scheduled: %s", expr)
  }
}

func clearCron() {
  if cronEntryID != 0 {
    cronScheduler.Remove(cronEntryID)
    cronEntryID = 0
  }
}

func env(key, def string) string {
  if v := os.Getenv(key); v != "" {
    return v
  }
  return def
}

func now() string   { return time.Now().Format("2006-01-02 15:04:05") }
func today() string { return time.Now().Format("2006-01-02") }
func truncate(s string, n int) string {
  if len(s) <= n {
    return s
  }
  return s[:n] + "..."
}
