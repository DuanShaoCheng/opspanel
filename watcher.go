package main

import (
  "log"
  "os"
  "path/filepath"
  "strings"
  "sync"
  "time"
)

// Watcher 持续日志采集器
type Watcher struct {
  mu       sync.RWMutex
  watching map[string]*fileWatcher
  stop     chan struct{}
}

type fileWatcher struct {
  path   string
  source string
  offset int64
  filter string
}

var watcher *Watcher

func NewWatcher() *Watcher {
  return &Watcher{
    watching: make(map[string]*fileWatcher),
    stop:     make(chan struct{}),
  }
}

func (w *Watcher) Start() {
  go w.loop()
  log.Println("[watcher] started")
}

func (w *Watcher) loop() {
  ticker := time.NewTicker(10 * time.Second)
  defer ticker.Stop()
  w.scan()
  for {
    select {
    case <-ticker.C:
      w.scan()
    case <-w.stop:
      return
    }
  }
}

// PLACEHOLDER_WATCHER

func (w *Watcher) scan() {
  cfgMu.RLock()
  sources := make([]LogSource, len(cfg.LogSources))
  copy(sources, cfg.LogSources)
  cfgMu.RUnlock()

  for _, src := range sources {
    if src.Type == "filebeat" {
      continue
    }
    files := findLogFiles(src.Path, src.Pattern)
    for _, f := range files {
      w.processFile(f, src)
    }
  }
}

func (w *Watcher) processFile(path string, src LogSource) {
  info, err := os.Stat(path)
  if err != nil || info.Size() == 0 {
    return
  }

  w.mu.Lock()
  fw, exists := w.watching[path]
  if !exists {
    fw = &fileWatcher{
      path:   path,
      source: src.Name,
      offset: info.Size(), // 首次只记录位置，不读历史
      filter: src.Filter,
    }
    w.watching[path] = fw
    w.mu.Unlock()
    log.Printf("[watcher] tracking: %s (size=%d)", path, info.Size())
    return
  }
  w.mu.Unlock()

  if info.Size() < fw.offset {
    // 文件被截断（轮转）
    log.Printf("[watcher] file rotated: %s", path)
    fw.offset = 0
  }

  if info.Size() == fw.offset {
    return
  }

  // 读取新增内容
  f, err := os.Open(path)
  if err != nil {
    return
  }
  defer f.Close()

  f.Seek(fw.offset, 0)
  buf := make([]byte, info.Size()-fw.offset)
  n, _ := f.Read(buf)
  fw.offset = info.Size()

  if n > 0 {
    w.filterAndStore(string(buf[:n]), fw)
  }
}

func (w *Watcher) filterAndStore(data string, fw *fileWatcher) {
  lines := strings.Split(data, "\n")
  for _, line := range lines {
    line = strings.TrimSpace(line)
    if line == "" {
      continue
    }
    if fw.filter != "" && !matchFilter(line, fw.filter) {
      continue
    }
    entry := LogEntry{
      Source:  fw.source,
      File:    filepath.Base(fw.path),
      Content: line,
      Level:   classifyLevel(line),
    }
    db.Create(&entry)
  }
}

func (w *Watcher) Stop() {
  close(w.stop)
}

func (w *Watcher) GetStats() map[string]interface{} {
  w.mu.RLock()
  defer w.mu.RUnlock()
  var unanalyzed int64
  db.Model(&LogEntry{}).Where("analyzed = ?", false).Count(&unanalyzed)
  return map[string]interface{}{
    "tracking_files": len(w.watching),
    "unanalyzed":     unanalyzed,
  }
}

// InitWatcher 初始化并启动采集器
func InitWatcher() {
  watcher = NewWatcher()
  watcher.Start()
}