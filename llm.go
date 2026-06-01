package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const structuredPrompt = `你是一个服务器运维专家。请分析以下错误日志，以 JSON 数组格式输出问题列表。

每个问题包含以下字段：
- priority: 优先级，P0(致命)/P1(严重)/P2(警告)/P3(提示)
- title: 问题标题，简短描述（不超过30字）
- cause: 原因分析，详细说明为什么会出现这个问题
- impact: 影响范围，说明这个问题会造成什么后果
- raw_log: 相关的日志原文（保留关键的1-3行）

要求：
1. 按优先级从高到低排序
2. 相同错误合并为一条，在 cause 中说明出现次数
3. 如果没有严重问题，返回空数组 []
4. 只输出 JSON，不要输出其他内容

输出格式示例：
[{"priority":"P1","title":"数据库连接超时","cause":"MySQL 连接池耗尽，过去1小时出现12次","impact":"部分用户请求会返回500错误","raw_log":"[ERROR] 2024-01-01 10:00:00 db connection timeout after 30s"}]`

// CallLLM 调用 LLM 获取结构化分析结果
func CallLLM(provider *LLMProvider, systemPrompt string, logs string, hours int) (string, error) {
	if provider == nil || provider.APIURL == "" || provider.APIKey == "" {
		return "", fmt.Errorf("LLM not configured")
	}

	if systemPrompt == "" {
		systemPrompt = structuredPrompt
	}

	userMsg := fmt.Sprintf("以下是过去 %d 小时的错误日志：\n\n%s", hours, logs)

	payload := map[string]interface{}{
		"model": provider.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"max_tokens":  2000,
		"temperature": 0.1,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", provider.APIURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	json.Unmarshal(respBody, &result)

	var content string
	if len(result.Choices) > 0 && result.Choices[0].Message.Content != "" {
		content = result.Choices[0].Message.Content
	} else if len(result.Content) > 0 && result.Content[0].Text != "" {
		content = result.Content[0].Text
	} else {
		return "", fmt.Errorf("empty response: %s", truncate(string(respBody), 200))
	}

	return content, nil
}

// ParseIssuesFromLLM 尝试从 LLM 返回内容中解析结构化问题列表
func ParseIssuesFromLLM(content string) []Issue {
	// 提取 JSON 部分（LLM 可能包裹在 markdown code block 中）
	jsonStr := content
	if idx := strings.Index(content, "["); idx >= 0 {
		if end := strings.LastIndex(content, "]"); end > idx {
			jsonStr = content[idx : end+1]
		}
	}

	var raw []struct {
		Priority string `json:"priority"`
		Title    string `json:"title"`
		Cause    string `json:"cause"`
		Impact   string `json:"impact"`
		RawLog   string `json:"raw_log"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	var issues []Issue
	for i, r := range raw {
		issues = append(issues, Issue{
			ID:       fmt.Sprintf("%s-%d", time.Now().Format("20060102150405"), i),
			Time:     now,
			Priority: r.Priority,
			Title:    r.Title,
			Cause:    r.Cause,
			Impact:   r.Impact,
			RawLog:   r.RawLog,
			Status:   "open",
		})
	}
	return issues
}

// CallLLMWithProvider 使用 LLMProvider 调用 LLM
func CallLLMWithProvider(provider *LLMProvider, systemPrompt, userMsg string) (string, error) {
	if provider == nil || provider.APIURL == "" || provider.APIKey == "" {
		return "", fmt.Errorf("LLM not configured")
	}

	payload := map[string]interface{}{
		"model": provider.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"max_tokens":  2000,
		"temperature": 0.1,
	}

	body, _ := json.Marshal(payload)
	url := provider.APIURL
	if !strings.HasSuffix(url, "/completions") {
		if strings.HasSuffix(url, "/v1") {
			url += "/chat/completions"
		} else {
			url += "/v1/chat/completions"
		}
	}

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(respBody, &result)

	if len(result.Choices) > 0 && result.Choices[0].Message.Content != "" {
		return result.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("empty response: %s", truncate(string(respBody), 200))
}
