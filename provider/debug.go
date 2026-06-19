package provider

import (
	"encoding/json"
	"log"
	"os"
	"strings"
)

// DebugEnabled debug 日志全局开关。
//
// 通过以下任一方式启用：
//   - 环境变量 BAMBOO_DEBUG=1 / true / on
//   - 调用 provider.SetDebug(true)
//   - 适配器构造时传入 WithDebug() Option
var DebugEnabled = false

// SetDebug 全局开启或关闭 Provider 层 debug 日志。
//
// 供适配器的 WithDebug() Option 内部调用，也可被上层业务直接调用来
// 在运行时动态控制 debug 开关。
func SetDebug(enabled bool) {
	DebugEnabled = enabled
}

// init 从环境变量 BAMBOO_DEBUG 初始化 debug 开关。
//
// 支持的值：1 / true / on（不区分大小写）。默认关闭，不影响生产性能。
func init() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("BAMBOO_DEBUG")))
	DebugEnabled = v == "1" || v == "true" || v == "on"
}

// MaxDebugBodyLen debug 日志中请求体（content 正文）的最大长度。
//
// 超过此长度的 content 字段会被截断并追加 "..." 标记。
const MaxDebugBodyLen = 500

// DebugRequest 输出请求的 debug 日志。
//
// 用于适配器在调用底层 SDK 前打印实际发送的参数信息，
// 方便排查"参数有误"类错误。
//
// 参数：
//   - providerType: Provider 类型标识（如 "openai-completions"）
//   - endpoint: 目标端点 URL（如 "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions"）
//   - headers: 请求头键值对（可为空）
//   - params: 可序列化的请求参数（通过 json.Marshal 输出 body）
func DebugRequest(providerType, endpoint string, headers map[string]string, params any) {
	if !DebugEnabled {
		return
	}

	// 序列化 params 为 JSON
	var bodyJSON string
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			bodyJSON = "<marshal error: " + err.Error() + ">"
		} else {
			bodyJSON = truncateContent(string(raw))
		}
	}

	// 构建 headers 字符串
	var headerStr string
	if len(headers) > 0 {
		parts := make([]string, 0, len(headers))
		for k, v := range headers {
			// API Key 脱敏处理
			if isSensitiveHeader(k) {
				v = maskSensitive(v)
			}
			parts = append(parts, k+": "+v)
		}
		headerStr = strings.Join(parts, ", ")
	} else {
		headerStr = "(none)"
	}

	log.Printf(
		"[bamboo/debug] provider=%s endpoint=%s | headers: {%s} | body: %s",
		providerType, endpoint, headerStr, bodyJSON,
	)
}

// truncateContent 截断 JSON 字符串中的 content 字段值，避免日志过长。
//
// 遍历 JSON 顶层键，对 "content" / "text" 等长文本字段做截断。
func truncateContent(jsonStr string) string {
	// 如果整体 JSON 不长，直接返回
	if len(jsonStr) <= MaxDebugBodyLen*2 {
		return jsonStr
	}

	// 尝试解析为 map 做 content 截断
	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		// 解析失败，直接截断整体
		if len(jsonStr) > MaxDebugBodyLen {
			return jsonStr[:MaxDebugBodyLen] + "...(truncated)"
		}
		return jsonStr
	}

	// 截断长文本字段
	truncateLongFields(raw, MaxDebugBodyLen)

	out, err := json.Marshal(raw)
	if err != nil {
		return jsonStr
	}
	return string(out)
}

// truncateLongFields 递归截断 JSON 中已知的长文本字段。
func truncateLongFields(data map[string]any, maxLen int) {
	for key, val := range data {
		switch v := val.(type) {
		case string:
			if len(v) > maxLen && isContentField(key) {
				data[key] = v[:maxLen] + "...(truncated)"
			}
		case map[string]any:
			truncateLongFields(v, maxLen)
		case []any:
			for i, item := range v {
				if m, ok := item.(map[string]any); ok {
					truncateLongFields(m, maxLen)
					v[i] = m
				}
				if s, ok := item.(string); ok && len(s) > maxLen {
					v[i] = s[:maxLen] + "...(truncated)"
				}
			}
		}
	}
}

// isContentField 判断字段是否为需要截断的长文本字段。
func isContentField(key string) bool {
	switch strings.ToLower(key) {
	case "content", "text", "system", "thinking", "reasoning_content", "arguments":
		return true
	}
	return false
}

// isSensitiveHeader 判断 header 是否为敏感字段（需要脱敏）。
func isSensitiveHeader(key string) bool {
	lk := strings.ToLower(key)
	switch lk {
	case "authorization", "x-api-key", "api-key", "x-goog-api-key":
		return true
	}
	return false
}

// maskSensitive 对敏感值做脱敏处理，仅保留前 4 和后 4 个字符。
func maskSensitive(val string) string {
	if len(val) <= 12 {
		return "****"
	}
	return val[:4] + "..." + val[len(val)-4:]
}
