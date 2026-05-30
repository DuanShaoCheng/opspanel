package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Notify 向所有启用的渠道发送通知
func Notify(channels []Notification, title, body string) error {
	var errs []string
	for _, ch := range channels {
		if !ch.Enabled || ch.Webhook == "" {
			continue
		}
		var err error
		switch ch.Type {
		case "feishu":
			err = sendFeishu(ch.Webhook, ch.Secret, title, body)
		case "wecom":
			err = sendWecom(ch.Webhook, title, body)
		case "dingtalk":
			err = sendDingtalk(ch.Webhook, ch.Secret, title, body)
		case "webhook":
			err = sendGenericWebhook(ch.Webhook, title, body)
		default:
			err = fmt.Errorf("unknown channel type: %s", ch.Type)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s(%s): %v", ch.Type, ch.Name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// NotifySingle 向单个渠道发送（用于测试）
func NotifySingle(ch Notification, title, body string) error {
	switch ch.Type {
	case "feishu":
		return sendFeishu(ch.Webhook, ch.Secret, title, body)
	case "wecom":
		return sendWecom(ch.Webhook, title, body)
	case "dingtalk":
		return sendDingtalk(ch.Webhook, ch.Secret, title, body)
	case "webhook":
		return sendGenericWebhook(ch.Webhook, title, body)
	}
	return fmt.Errorf("unknown type: %s", ch.Type)
}

// === 飞书 ===
func sendFeishu(webhook, secret, title, body string) error {
	var contentLines [][]map[string]interface{}
	for _, line := range strings.Split(body, "\n") {
		contentLines = append(contentLines, []map[string]interface{}{
			{"tag": "text", "text": line + "\n"},
		})
	}

	payload := map[string]interface{}{
		"msg_type": "post",
		"content": map[string]interface{}{
			"post": map[string]interface{}{
				"zh_cn": map[string]interface{}{
					"title":   title,
					"content": contentLines,
				},
			},
		},
	}

	if secret != "" {
		ts := fmt.Sprintf("%d", time.Now().Unix())
		payload["timestamp"] = ts
		payload["sign"] = feishuSign(ts, secret)
	}

	return postJSON(webhook, payload)
}

func feishuSign(timestamp, secret string) string {
	h := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// === 企业微信 ===
func sendWecom(webhook, title, body string) error {
	content := title + "\n\n" + body
	if len(content) > 4096 {
		content = content[:4096]
	}
	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": content,
		},
	}
	return postJSON(webhook, payload)
}

// === 钉钉 ===
func sendDingtalk(webhook, secret, title, body string) error {
	if secret != "" {
		ts := fmt.Sprintf("%d", time.Now().UnixMilli())
		sign := dingtalkSign(ts, secret)
		if strings.Contains(webhook, "?") {
			webhook += "&timestamp=" + ts + "&sign=" + sign
		} else {
			webhook += "?timestamp=" + ts + "&sign=" + sign
		}
	}

	content := title + "\n\n" + body
	if len(content) > 4096 {
		content = content[:4096]
	}
	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"title": title,
			"text":  content,
		},
	}
	return postJSON(webhook, payload)
}

func dingtalkSign(timestamp, secret string) string {
	stringToSign := timestamp + "\n" + secret
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// === 通用 Webhook ===
func sendGenericWebhook(webhook, title, body string) error {
	payload := map[string]interface{}{
		"title":     title,
		"body":      body,
		"timestamp": time.Now().Unix(),
	}
	return postJSON(webhook, payload)
}

// === 公共 ===
func postJSON(url string, payload interface{}) error {
	data, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	// 检查常见的错误码字段
	var result struct {
		Code    int    `json:"code"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Msg     string `json:"msg"`
	}
	json.Unmarshal(respBody, &result)
	if result.Code != 0 && result.Msg != "" {
		return fmt.Errorf("code=%d msg=%s", result.Code, result.Msg)
	}
	if result.ErrCode != 0 && result.ErrMsg != "" {
		return fmt.Errorf("errcode=%d errmsg=%s", result.ErrCode, result.ErrMsg)
	}

	return nil
}
