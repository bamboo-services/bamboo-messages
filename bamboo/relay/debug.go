package relay

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// maxDebugBodyLen debug 日志中单字段内容的最大长度（字符数）。
//
// 超过此长度的 content / text / system 等长文本字段会被截断并追加 "...(truncated)"。
const maxDebugBodyLen = 500

// debugRelayInput 输出 relay 层接收到的原始请求体 debug 日志。
//
// 在 codec.ParseRequest 之前调用，打印上游调用方（如 newapi）传入的原始 JSON body，
// 是排查"上游传了什么参数"的第一手数据。
//
// 参数：
//   - fn: 调用场景标识（"Relay" 或 "RelayStream"）
//   - inFormat: 输入协议格式（如 codec.FormatOpenAI）
//   - outFormat: 输出协议格式
//   - body: 原始请求体 JSON 字节
func debugRelayInput(fn string, inFormat, outFormat any, body []byte) {
	if !provider.DebugEnabled {
		return
	}
	fmt.Println(FormatRelayInput(fn, inFormat, outFormat, body))
}

// formatRelayDebugLine 生成统一的 relay debug 日志行。
//
// 格式：[bamboo/debug] {prefix} | fn={fn} in={inFormat} out={outFormat} | {label}: {content}
func formatRelayDebugLine(prefix, fn string, inFormat, outFormat any, label, content string) string {
	return fmt.Sprintf(
		"[bamboo/debug] %s | fn=%s in=%v out=%v | %s: %s",
		prefix, fn, inFormat, outFormat, label, content,
	)
}

// FormatRelayInput 格式化 relay 输入的 debug 信息并返回字符串。
//
// 与 debugRelayInput 功能相同，但返回字符串而非打日志。
// 不受 debug 开关影响，调用即返回。
//
// 适用于上层业务需要自定义日志格式、写入文件、或通过 HTTP 接口暴露 debug 信息的场景。
func FormatRelayInput(fn string, inFormat, outFormat any, body []byte) string {
	return formatRelayDebugLine("relay input", fn, inFormat, outFormat, "raw body", truncateContent(string(body)))
}

// debugRelayParsed 输出 relay 层 codec 解析后的统一请求 debug 日志。
//
// 在 codec.ParseRequest 之后调用，打印解析得到的 RelayRequest 中间表示，
// 用于与 debugRelayInput 的原始 body 对比，排查 codec 转换是否有偏差。
//
// 参数：
//   - fn: 调用场景标识（"Relay" 或 "RelayStream"）
//   - inFormat: 输入协议格式
//   - req: codec 解析后的 RelayRequest
func debugRelayParsed(fn string, inFormat any, req any) {
	if !provider.DebugEnabled {
		return
	}
	fmt.Println(FormatRelayParsed(fn, inFormat, req))
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

// debugRelayResponse 输出 relay 非流式响应 debug 日志。
//
// 在 codec.SerializeResponse 之后调用，打印 relay 返回给上游调用方的响应 body。
//
// 参数：
//   - fn: 调用场景标识（"Relay" 或 "RelayStream"）
//   - inFormat: 输入协议格式
//   - outFormat: 输出协议格式
//   - data: 响应体 JSON 字节
func debugRelayResponse(fn string, inFormat, outFormat any, data []byte) {
	if !provider.DebugEnabled {
		return
	}
	fmt.Println(FormatRelayResponse(fn, inFormat, outFormat, data))
}

// FormatRelayResponse 格式化 relay 非流式响应的 debug 信息并返回字符串。
//
// 与 debugRelayResponse 功能相同，但返回字符串而非打日志。
// 不受 debug 开关影响，调用即返回。
func FormatRelayResponse(fn string, inFormat, outFormat any, data []byte) string {
	return formatRelayDebugLine("relay response", fn, inFormat, outFormat, "body", truncateBytes(data, maxDebugBodyLen))
}

// debugRelayResponseFrame 输出 relay 流式逐帧响应 debug 日志。
//
// 在 RelayStream 每次向输出 channel 写入 SSE 帧之前调用，打印即将发送的原始帧内容。
//
// 参数：
//   - fn: 调用场景标识（通常为 "RelayStream"）
//   - inFormat: 输入协议格式
//   - outFormat: 输出协议格式
//   - data: 原始 SSE 帧字节
func debugRelayResponseFrame(fn string, inFormat, outFormat any, data []byte) {
	if !provider.DebugEnabled {
		return
	}
	fmt.Println(FormatRelayResponseFrame(fn, inFormat, outFormat, data))
}

// FormatRelayResponseFrame 格式化 relay 流式逐帧响应的 debug 信息并返回字符串。
//
// 与 debugRelayResponseFrame 功能相同，但返回字符串而非打日志。
// 不受 debug 开关影响，调用即返回。
func FormatRelayResponseFrame(fn string, inFormat, outFormat any, data []byte) string {
	return formatRelayDebugLine("relay response stream", fn, inFormat, outFormat, "frame", truncateBytes(data, maxDebugBodyLen))
}

// truncateBytes 将字节切片转换为字符串并按字符数简单截断。
//
// 超过 maxLen 时返回前 maxLen 个字符并追加 "...(truncated)"。
func truncateBytes(data []byte, maxLen int) string {
	s := string(data)
	if len(s) > maxLen {
		return s[:maxLen] + "...(truncated)"
	}
	return s
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
