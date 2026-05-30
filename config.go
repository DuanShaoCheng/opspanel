package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Config 全局配置
type Config struct {
	LogSources    []LogSource       `json:"log_sources"`
	Notifications []Notification    `json:"notifications"`
	LLMProviders  []LLMProvider     `json:"llm_providers"`
	LLM           LLMConfig         `json:"llm"`
	Schedule      ScheduleConfig    `json:"schedule"`
	SMTP          SMTPConfig        `json:"smtp"`
	LogAnalysis   LogAnalysisConfig `json:"log_analysis"`
}

// LogAnalysisConfig 日志分析模块配置
type LogAnalysisConfig struct {
	LLMProvider    string `json:"llm_provider"`
	LLMPrompt      string `json:"llm_prompt"`
	NotifyChannels []int  `json:"notify_channels"`
	HistoryMax     int    `json:"history_max"`
	TitleOk        string `json:"title_ok"`
	TitleAlert     string `json:"title_alert"`
	MsgOk          string `json:"msg_ok"`
	MsgHeader      string `json:"msg_header"`
}

type LogSource struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	Path             string `json:"path"`
	Pattern          string `json:"pattern"`
	Filter           string `json:"filter"`
	Format           string `json:"format"`
	MultilinePattern string `json:"multiline_pattern"`
	JSONField        string `json:"json_field"`
	JSONLevelField   string `json:"json_level_field"`
	JSONLevelFilter  string `json:"json_level_filter"`
	HostField        string `json:"host_field"`
}

type Notification struct {
	Type    string `json:"type"`    // feishu, wecom, dingtalk, webhook
	Name    string `json:"name"`    // 显示名称
	Webhook string `json:"webhook"`
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

type LLMConfig struct {
	APIURL       string `json:"api_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
}

type ScheduleConfig struct {
	Enabled  bool   `json:"enabled"`
	CronExpr string `json:"cron_expr"`
	LogHours int    `json:"log_hours"`
	MaxBytes int    `json:"max_bytes"`
}

type AnalysisRecord struct {
	Time    string `json:"time"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Error   string `json:"error,omitempty"`
}

// Issue LLM 分析出的结构化问题
type Issue struct {
	ID       string `json:"id"`
	Time     string `json:"time"`
	Priority string `json:"priority"` // P0/P1/P2/P3
	Title    string `json:"title"`
	Cause    string `json:"cause"`
	Impact   string `json:"impact"`
	RawLog   string `json:"raw_log"`
	Status   string `json:"status"` // open/resolved
}

// ScanFileState 单个文件的扫描状态
type ScanFileState struct {
	Offset  int64  `json:"offset"`
	ScannedAt string `json:"scanned_at"`
}

var (
	cfg     *Config
	cfgMu   sync.RWMutex
	history []AnalysisRecord
	histMu  sync.RWMutex
	issues  []Issue
	issueMu sync.RWMutex
	scanState map[string]ScanFileState
	scanMu    sync.RWMutex
)

func configPath() string    { return filepath.Join(dataDir, "config.json") }
func historyPath() string   { return filepath.Join(dataDir, "history.json") }
func issuesPath() string    { return filepath.Join(dataDir, "issues.json") }
func scanStatePath() string { return filepath.Join(dataDir, "scan_state.json") }

func DefaultConfig() *Config {
	return &Config{
		LogSources: []LogSource{
			{
				Name:    "默认日志",
				Path:    "/mnt/logs",
				Pattern: "*.log",
				Filter:  "ERROR|CRASH|exception|panic|fatal",
				Format:  "plain",
			},
		},
		Notifications: []Notification{},
		LLM: LLMConfig{
			SystemPrompt: "你是一个服务器运维专家。请分析以下错误日志，输出结构化摘要：\n1. 关键错误（按严重程度排序）\n2. 重复出现的错误模式\n3. 影响范围\n4. 处理建议\n\n如果没有严重问题，简短说明即可。输出使用中文，不超过 500 字。",
		},
		Schedule: ScheduleConfig{
			Enabled:  false,
			CronExpr: "0 9 * * *",
			LogHours: 24,
			MaxBytes: 12000,
		},
	}
}

func LoadConfig() *Config {
	c := DefaultConfig()
	data, err := os.ReadFile(configPath())
	if err != nil {
		return c
	}
	json.Unmarshal(data, c)
	if c.Schedule.LogHours == 0 {
		c.Schedule.LogHours = 24
	}
	if c.Schedule.MaxBytes == 0 {
		c.Schedule.MaxBytes = 12000
	}
	if c.Schedule.CronExpr == "" {
		c.Schedule.CronExpr = "0 9 * * *"
	}
	return c
}

// LoadEnvOverrides 用环境变量覆盖配置，优先级高于 config.json
func LoadEnvOverrides(c *Config) {
	if v := os.Getenv("LOG_SENTINEL_LLM_URL"); v != "" {
		c.LLM.APIURL = v
	}
	if v := os.Getenv("LOG_SENTINEL_LLM_KEY"); v != "" {
		c.LLM.APIKey = v
	}
	if v := os.Getenv("LOG_SENTINEL_LLM_MODEL"); v != "" {
		c.LLM.Model = v
	}
	if v := os.Getenv("LOG_SENTINEL_LLM_PROMPT"); v != "" {
		c.LLM.SystemPrompt = v
	}
	if v := os.Getenv("LOG_SENTINEL_SCHEDULE_CRON"); v != "" {
		c.Schedule.CronExpr = v
	}
	if v := os.Getenv("LOG_SENTINEL_SCHEDULE_ENABLED"); v != "" {
		c.Schedule.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("LOG_SENTINEL_SCHEDULE_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Schedule.LogHours = n
		}
	}
	if v := os.Getenv("LOG_SENTINEL_SCHEDULE_MAX_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Schedule.MaxBytes = n
		}
	}
	// 快速添加单个通知渠道
	if webhook := os.Getenv("LOG_SENTINEL_NOTIFY_WEBHOOK"); webhook != "" {
		chType := os.Getenv("LOG_SENTINEL_NOTIFY_TYPE")
		if chType == "" {
			chType = "webhook"
		}
		chName := os.Getenv("LOG_SENTINEL_NOTIFY_NAME")
		if chName == "" {
			chName = "env-" + chType
		}
		chSecret := os.Getenv("LOG_SENTINEL_NOTIFY_SECRET")
		// 仅当没有已配置渠道时追加
		if len(c.Notifications) == 0 {
			c.Notifications = append(c.Notifications, Notification{
				Type:    chType,
				Name:    chName,
				Webhook: webhook,
				Secret:  chSecret,
				Enabled: true,
			})
		}
	}
}

func SaveConfig(c *Config) {
	data, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(configPath(), data, 0600); err != nil {
		log.Printf("[config] save failed: %v", err)
	}
}

func LoadHistory() []AnalysisRecord {
	var h []AnalysisRecord
	data, _ := os.ReadFile(historyPath())
	json.Unmarshal(data, &h)
	return h
}

func SaveHistory(h []AnalysisRecord) {
	data, _ := json.MarshalIndent(h, "", "  ")
	os.WriteFile(historyPath(), data, 0644)
}

func addRecord(rec AnalysisRecord) {
	histMu.Lock()
	defer histMu.Unlock()
	history = append([]AnalysisRecord{rec}, history...)
	cfgMu.RLock()
	max := cfg.LogAnalysis.HistoryMax
	cfgMu.RUnlock()
	if max <= 0 { max = 50 }
	if len(history) > max {
		history = history[:max]
	}
	SaveHistory(history)
}

// === Issues ===

func LoadIssues() []Issue {
	var list []Issue
	data, _ := os.ReadFile(issuesPath())
	json.Unmarshal(data, &list)
	return list
}

func SaveIssues(list []Issue) {
	data, _ := json.MarshalIndent(list, "", "  ")
	os.WriteFile(issuesPath(), data, 0644)
}

func addIssues(newIssues []Issue) {
	issueMu.Lock()
	defer issueMu.Unlock()
	issues = append(newIssues, issues...)
	if len(issues) > 200 {
		issues = issues[:200]
	}
	SaveIssues(issues)
}

// === Scan State ===

func LoadScanState() map[string]ScanFileState {
	m := make(map[string]ScanFileState)
	data, _ := os.ReadFile(scanStatePath())
	json.Unmarshal(data, &m)
	return m
}

func SaveScanState(m map[string]ScanFileState) {
	data, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(scanStatePath(), data, 0644)
}

// LLMProvider LLM 配置
type LLMProvider struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"` // openai, openai-responses, claude
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

// SMTPConfig SMTP 邮件配置
type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
}

// GetLLMProvider 根据 ID 获取 LLM 配置
func GetLLMProvider(id string) *LLMProvider {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	for i := range cfg.LLMProviders {
		if cfg.LLMProviders[i].ID == id {
			return &cfg.LLMProviders[i]
		}
	}
	if cfg.LLM.APIURL != "" {
		return &LLMProvider{
			ID: "legacy", APIURL: cfg.LLM.APIURL, APIKey: cfg.LLM.APIKey, Model: cfg.LLM.Model,
		}
	}
	if len(cfg.LLMProviders) > 0 {
		return &cfg.LLMProviders[0]
	}
	return nil
}

// GetLogAnalysisChannels 获取日志分析模块选择的通知渠道
func GetLogAnalysisChannels() []Notification {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	indices := cfg.LogAnalysis.NotifyChannels
	if len(indices) == 0 {
		// 未选择则使用所有启用的渠道
		var channels []Notification
		for _, ch := range cfg.Notifications {
			if ch.Enabled {
				channels = append(channels, ch)
			}
		}
		return channels
	}
	var channels []Notification
	for _, idx := range indices {
		if idx >= 0 && idx < len(cfg.Notifications) && cfg.Notifications[idx].Enabled {
			channels = append(channels, cfg.Notifications[idx])
		}
	}
	return channels
}
