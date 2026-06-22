package provider

import (
	"encoding/json"
	"fmt"
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

	log.Printf(
		"[bamboo/debug] provider=%s endpoint=%s | headers: {%s} | body: %s",
		providerType, endpoint, formatHeaders(headers), formatParams(params),
	)
}

// FormatDebugRequest 格式化请求的 debug 信息并返回字符串。
//
// 与 DebugRequest 功能相同，但返回字符串而非打日志。
// 不受 DebugEnabled 开关影响，调用即返回。
//
// 适用于上层业务需要自定义日志格式、写入文件、或通过 HTTP 接口暴露 debug 信息的场景。
func FormatDebugRequest(providerType, endpoint string, headers map[string]string, params any) string {
	return fmt.Sprintf(
		"[bamboo/debug] provider=%s endpoint=%s | headers: {%s} | body: %s",
		providerType, endpoint, formatHeaders(headers), formatParams(params),
	)
}

func formatHeaders(headers map[string]string) string {
	if len(headers) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(headers))
	for k, v := range headers {
		if isSensitiveHeader(k) {
			v = maskSensitive(v)
		}
		parts = append(parts, k+": "+v)
	}
	return strings.Join(parts, ", ")
}

func formatParams(params any) string {
	if params == nil {
		return "(none)"
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return "<marshal error: " + err.Error() + ">"
	}
	return truncateContent(string(raw))
}

// truncateContent 截断 JSON 字符串中的 content 字段值，避免日志过长。
//
// 遍历 JSON 顶层键，对 "content" / "text" 等长文本字段做截断。
func truncateContent(jsonStr string) string {
	// 尝试解析为 map 做 tools 简化 + content 截断
	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		// 解析失败，直接截断整体
		if len(jsonStr) > MaxDebugBodyLen {
			return jsonStr[:MaxDebugBodyLen] + "...(truncated)"
		}
		return jsonStr
	}

	// 先简化 tools 数组（减少 debug 输出中的工具噪音）
	summarizeTools(raw)
	// 再截断长文本字段
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

// summarizeTools 简化 tools 数组的 debug 输出。
//
// 当 tools 包含 2 个或以上元素时，保留第一个元素完整不变，
// 对后续元素调用 summarizeToolElement 生成字段级摘要。
// 非数组、空数组、单元素、无 tools 键时均为 no-op。
func summarizeTools(raw map[string]any) {
	tools, ok := raw["tools"].([]any)
	if !ok || len(tools) <= 1 {
		return
	}
	for i := 1; i < len(tools); i++ {
		tool, ok := tools[i].(map[string]any)
		if !ok {
			continue
		}
		summarizeToolElement(tool)
		tools[i] = tool
	}
}

// summarizeToolElement 就地简化单个 tool 元素的内容字段。
//
// 遍历 tool 的所有键值对：
//   - 标识字段（name / type）保留原值
//   - 其他字段按 summarizeValue 规则替换为长度摘要
//
// 嵌套感知：若存在 "function" 键且其值为 map，则递归处理 function 内部
// 字段（保留 function.name / function.type，简化 function.description 等）。
func summarizeToolElement(tool map[string]any) {
	for key, val := range tool {
		if key == "function" {
			if functionMap, ok := val.(map[string]any); ok {
				summarizeToolElement(functionMap)
			}
			continue
		}
		tool[key] = summarizeValue(key, val)
	}
}

// summarizeValue 根据字段名和值类型生成摘要。
//
// 规则：
//   - 标识字段（name / type）→ 返回原值
//   - string   → "(N chars)"，N = len(bytes)
//   - map[string]any → "(N keys)"
//   - []any    → "(N items)"
//   - 其他（number/bool/nil）→ 返回原值
func summarizeValue(key string, val any) any {
	if isIdentifierField(key) {
		return val
	}
	switch v := val.(type) {
	case string:
		return fmt.Sprintf("(%d chars)", len(v))
	case map[string]any:
		return fmt.Sprintf("(%d keys)", len(v))
	case []any:
		return fmt.Sprintf("(%d items)", len(v))
	default:
		return val
	}
}

// isIdentifierField 判断字段是否为 tool 标识字段（应保留原值不简化）。
func isIdentifierField(key string) bool {
	switch strings.ToLower(key) {
	case "name", "type":
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
