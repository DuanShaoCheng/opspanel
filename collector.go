package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CollectLogs 根据配置的日志源收集日志（增量扫描，只读新增内容）
func CollectLogs(sources []LogSource, maxBytes int) string {
	var buf strings.Builder
	cutoff := time.Now().Add(-24 * time.Hour)

	scanMu.Lock()
	defer scanMu.Unlock()

	for _, src := range sources {
		files := findLogFiles(src.Path, src.Pattern)
		if len(files) == 0 {
			continue
		}

		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil || info.Size() == 0 {
				continue
			}
			// 跳过 24h 未修改的文件
			if info.ModTime().Before(cutoff) {
				continue
			}

			// 获取上次扫描偏移
			state := scanState[f]
			offset := state.Offset
			// 文件被截断（日志轮转），重置
			if info.Size() < offset {
				offset = 0
			}
			// 无新增内容
			if info.Size() <= offset {
				continue
			}

			data, err := readFrom(f, offset, maxBytes)
			if err != nil || len(data) == 0 {
				continue
			}

			// 更新扫描状态
			scanState[f] = ScanFileState{
				Offset:    info.Size(),
				ScannedAt: time.Now().Format("2006-01-02 15:04:05"),
			}

			var lines []string
			switch src.Format {
			case "json":
				lines = collectJSON(string(data), src)
			case "multiline":
				lines = collectMultiline(string(data), src)
			default:
				lines = collectPlain(string(data), src)
			}

			if len(lines) == 0 {
				continue
			}
			if len(lines) > 80 {
				lines = lines[len(lines)-80:]
			}

			label := src.Name
			if label == "" {
				label = filepath.Base(src.Path)
			}
			buf.WriteString(fmt.Sprintf("=== %s: %s ===\n", label, filepath.Base(f)))
			buf.WriteString(strings.Join(lines, "\n"))
			buf.WriteString("\n\n")
		}
	}

	// 清理超过 7 天未更新的条目
	cleanCutoff := time.Now().Add(-7 * 24 * time.Hour)
	for k, v := range scanState {
		t, _ := time.Parse("2006-01-02 15:04:05", v.ScannedAt)
		if !t.IsZero() && t.Before(cleanCutoff) {
			delete(scanState, k)
		}
	}
	SaveScanState(scanState)

	result := buf.String()
	if len(result) > maxBytes {
		result = result[:maxBytes]
	}
	return result
}

// collectPlain 按行读取，正则过滤
func collectPlain(data string, src LogSource) []string {
	var filter *regexp.Regexp
	if src.Filter != "" {
		filter, _ = regexp.Compile("(?i)" + src.Filter)
	}

	var lines []string
	for _, line := range strings.Split(data, "\n") {
		if filter != nil {
			if filter.MatchString(line) {
				lines = append(lines, line)
			}
		} else {
			lines = append(lines, line)
		}
	}
	return lines
}

// collectJSON 解析 JSON 行日志，按级别和内容过滤
func collectJSON(data string, src LogSource) []string {
	msgField := src.JSONField
	if msgField == "" {
		msgField = "message"
	}
	levelField := src.JSONLevelField
	if levelField == "" {
		levelField = "level"
	}

	var levelFilter *regexp.Regexp
	if src.JSONLevelFilter != "" {
		levelFilter, _ = regexp.Compile("(?i)" + src.JSONLevelFilter)
	}

	var contentFilter *regexp.Regexp
	if src.Filter != "" {
		contentFilter, _ = regexp.Compile("(?i)" + src.Filter)
	}

	var lines []string
	for _, raw := range strings.Split(data, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || raw[0] != '{' {
			continue
		}

		var obj map[string]interface{}
		if json.Unmarshal([]byte(raw), &obj) != nil {
			continue
		}

		if levelFilter != nil {
			level := fmt.Sprintf("%v", obj[levelField])
			if !levelFilter.MatchString(level) {
				continue
			}
		}

		msg := fmt.Sprintf("%v", obj[msgField])
		if contentFilter != nil && !contentFilter.MatchString(msg) {
			continue
		}

		level := fmt.Sprintf("%v", obj[levelField])
		ts := ""
		if t, ok := obj["time"]; ok {
			ts = fmt.Sprintf("%v", t)
		} else if t, ok := obj["timestamp"]; ok {
			ts = fmt.Sprintf("%v", t)
		} else if t, ok := obj["@timestamp"]; ok {
			ts = fmt.Sprintf("%v", t)
		}
		if ts != "" {
			lines = append(lines, fmt.Sprintf("[%s] [%s] %s", ts, level, msg))
		} else {
			lines = append(lines, fmt.Sprintf("[%s] %s", level, msg))
		}
	}
	return lines
}

// collectMultiline 按起始模式合并多行日志
func collectMultiline(data string, src LogSource) []string {
	startPattern := src.MultilinePattern
	if startPattern == "" {
		startPattern = `^\d{4}[-/]`
	}
	startRe, err := regexp.Compile(startPattern)
	if err != nil {
		return collectPlain(data, src)
	}

	var filter *regexp.Regexp
	if src.Filter != "" {
		filter, _ = regexp.Compile("(?i)" + src.Filter)
	}

	rawLines := strings.Split(data, "\n")
	var entries []string
	var current strings.Builder

	for _, line := range rawLines {
		if startRe.MatchString(line) {
			if current.Len() > 0 {
				entries = append(entries, current.String())
				current.Reset()
			}
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)
	}
	if current.Len() > 0 {
		entries = append(entries, current.String())
	}

	if filter == nil {
		return entries
	}
	var matched []string
	for _, entry := range entries {
		if filter.MatchString(entry) {
			matched = append(matched, entry)
		}
	}
	return matched
}

func findLogFiles(dir, pattern string) []string {
	if pattern == "" {
		pattern = "*.log"
	}
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	return matches
}

// readFrom 从指定偏移读取文件内容
func readFrom(path string, offset int64, maxBytes int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if offset > 0 {
		f.Seek(offset, 0)
	}
	// 限制单次读取量
	reader := io.LimitReader(f, int64(maxBytes))
	return io.ReadAll(reader)
}

// readTail 从文件尾部读取（用于测试采集）
func readTail(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, _ := f.Stat()
	if info == nil || info.Size() == 0 {
		return nil, nil
	}

	if info.Size() > maxBytes {
		f.Seek(info.Size()-maxBytes, 0)
	}
	return io.ReadAll(f)
}

// TestCollectResult 测试采集结果
type TestCollectResult struct {
	Files   []string `json:"files"`
	Count   int      `json:"count"`
	Lines   int      `json:"lines"`
	Samples []string `json:"samples"`
	Error   string   `json:"error,omitempty"`
}

// TestCollect 测试单个日志源的采集效果（不影响扫描状态）
func TestCollect(src LogSource) TestCollectResult {
	files := findLogFiles(src.Path, src.Pattern)
	if len(files) == 0 {
		return TestCollectResult{Error: fmt.Sprintf("路径 %s 下未找到匹配 %s 的文件", src.Path, src.Pattern)}
	}

	var allLines []string
	for _, f := range files {
		data, err := readTail(f, 4000)
		if err != nil || len(data) == 0 {
			continue
		}
		var lines []string
		switch src.Format {
		case "json":
			lines = collectJSON(string(data), src)
		case "multiline":
			lines = collectMultiline(string(data), src)
		default:
			lines = collectPlain(string(data), src)
		}
		allLines = append(allLines, lines...)
	}

	samples := allLines
	if len(samples) > 10 {
		samples = samples[len(samples)-10:]
	}

	return TestCollectResult{
		Files:   files,
		Count:   len(files),
		Lines:   len(allLines),
		Samples: samples,
	}
}

// classifyLevel 根据配置的规则分类日志级别
func classifyLevel(line string) string {
	lower := strings.ToLower(line)
	cfgMu.RLock()
	rules := cfg.LevelRules
	cfgMu.RUnlock()
	for _, rule := range rules {
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return rule.Level
			}
		}
	}
	return "info"
}

// matchFilter 检查行是否匹配过滤正则
var filterCache sync.Map // map[string]*regexp.Regexp

func matchFilter(line, filter string) bool {
	if filter == "" {
		return true
	}
	var re *regexp.Regexp
	if cached, ok := filterCache.Load(filter); ok {
		re = cached.(*regexp.Regexp)
	} else {
		var err error
		re, err = regexp.Compile("(?i)" + filter)
		if err != nil {
			return strings.Contains(strings.ToLower(line), strings.ToLower(filter))
		}
		filterCache.Store(filter, re)
	}
	return re.MatchString(line)
}
