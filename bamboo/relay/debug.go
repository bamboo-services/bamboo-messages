package relay

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// envDebugEnabled 通过环境变量 BAMBOO_DEBUG 控制的 relay 层 debug 开关。
//
// 作为 Config.Debug 的兜底默认值，WithDebug() Option 可覆盖。
var envDebugEnabled = false

func init() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("BAMBOO_DEBUG")))
	envDebugEnabled = v == "1" || v == "true" || v == "on"
}

// maxDebugBodyLen debug 日志中单字段内容的最大长度（字符数）。
//
// 超过此长度的 content / text / system 等长文本字段会被截断并追加 "...(truncated)"。
const maxDebugBodyLen = 500

// shouldDebug 判断本次 relay 调用是否应输出 debug 日志。
//
// 优先使用 Config.Debug（WithDebug Option），未设置时回退到环境变量 BAMBOO_DEBUG。
func shouldDebug(cfg *Config) bool {
	if cfg != nil && cfg.Debug {
		return true
	}
	return envDebugEnabled
}

// debugRelayInput 输出 relay 层接收到的原始请求体 debug 日志。
//
// 在 codec.ParseRequest 之前调用，打印上游调用方（如 newapi）传入的原始 JSON body，
// 是排查"上游传了什么参数"的第一手数据。
//
// 参数：
//   - enabled: 是否启用 debug（由 shouldDebug(cfg) 判断）
//   - fn: 调用场景标识（"Relay" 或 "RelayStream"）
//   - inFormat: 输入协议格式（如 codec.FormatOpenAI）
//   - outFormat: 输出协议格式
//   - body: 原始请求体 JSON 字节
func debugRelayInput(enabled bool, fn string, inFormat, outFormat any, body []byte) {
	if !enabled {
		return
	}
	log.Printf(
		"[bamboo/debug] relay input | fn=%s in=%v out=%v | raw body: %s",
		fn, inFormat, outFormat, truncateContent(string(body)),
	)
}

// FormatRelayInput 格式化 relay 输入的 debug 信息并返回字符串。
//
// 与 debugRelayInput 功能相同，但返回字符串而非打日志。
// 不受 debug 开关影响，调用即返回。
//
// 适用于上层业务需要自定义日志格式、写入文件、或通过 HTTP 接口暴露 debug 信息的场景。
func FormatRelayInput(fn string, inFormat, outFormat any, body []byte) string {
	return fmt.Sprintf(
		"[bamboo/debug] relay input | fn=%s in=%v out=%v | raw body: %s",
		fn, inFormat, outFormat, truncateContent(string(body)),
	)
}

// debugRelayParsed 输出 relay 层 codec 解析后的统一请求 debug 日志。
//
// 在 codec.ParseRequest 之后调用，打印解析得到的 RelayRequest 中间表示，
// 用于与 debugRelayInput 的原始 body 对比，排查 codec 转换是否有偏差。
//
// 参数：
//   - enabled: 是否启用 debug（由 shouldDebug(cfg) 判断）
//   - fn: 调用场景标识（"Relay" 或 "RelayStream"）
//   - inFormat: 输入协议格式
//   - req: codec 解析后的 RelayRequest
func debugRelayParsed(enabled bool, fn string, inFormat any, req any) {
	if !enabled {
		return
	}
	var reqJSON string
	if req != nil {
		raw, err := json.Marshal(req)
		if err != nil {
			reqJSON = "<marshal error: " + err.Error() + ">"
		} else {
			reqJSON = truncateContent(string(raw))
		}
	}
	log.Printf(
		"[bamboo/debug] relay parsed | fn=%s in=%v | RelayRequest: %s",
		fn, inFormat, reqJSON,
	)
}

// FormatRelayParsed 格式化 relay 解析结果的 debug 信息并返回字符串。
//
// 与 debugRelayParsed 功能相同，但返回字符串而非打日志。
// 不受 debug 开关影响，调用即返回。
func FormatRelayParsed(fn string, inFormat any, req any) string {
	var reqJSON string
	if req != nil {
		raw, err := json.Marshal(req)
		if err != nil {
			reqJSON = "<marshal error: " + err.Error() + ">"
		} else {
			reqJSON = truncateContent(string(raw))
		}
	}
	return fmt.Sprintf(
		"[bamboo/debug] relay parsed | fn=%s in=%v | RelayRequest: %s",
		fn, inFormat, reqJSON,
	)
}

// truncateContent 截断 JSON 字符串中已知长文本字段的值，避免 debug 日志过长。
//
// 解析 JSON 后先调用 provider.SummarizeTools 简化 tools 数组（保留首个完整、后续摘要），
// 再截断 content / text / system 等长文本字段，最后重新序列化。
func truncateContent(jsonStr string) string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		if len(jsonStr) > maxDebugBodyLen {
			return jsonStr[:maxDebugBodyLen] + "...(truncated)"
		}
		return jsonStr
	}
	provider.SummarizeTools(raw)
	truncateLongFields(raw, maxDebugBodyLen)
	out, err := json.Marshal(raw)
	if err != nil {
		return jsonStr
	}
	return string(out)
}

// truncateLongFields 递归遍历 JSON map，对匹配 isContentField 的长字符串做截断。
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
					v[i] = s[:maxDebugBodyLen] + "...(truncated)"
				}
			}
		}
	}
}

// isContentField 判断字段名是否属于需要截断的长文本字段。
func isContentField(key string) bool {
	switch strings.ToLower(key) {
	case "content", "text", "system", "thinking", "reasoning_content", "arguments":
		return true
	}
	return false
}
