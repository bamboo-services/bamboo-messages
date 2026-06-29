package relay

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

// TokenSplitter token 切分器，维护跨帧 pendingTail。
//
// 用于流式平滑缓冲场景：将连续到达的文本增量切分为可输出的 token 列表，
// 同时保留可能不完整的尾部片段（pendingTail），在下一帧拼接后继续切分。
//
// 切分规则：
//   - CJK 字符（中日韩）：每个 rune 独立 token
//   - Latin 字母/数字：连续序列合并为一个 token
//   - 空格：附着到下一个 token 的前缀
//   - 标点（CJK/Latin）：附着到前一个 token 的后缀
//   - Emoji：独立 token
//   - 换行符：附着到前一个 token
type TokenSplitter struct {
	pendingTail string
}

// NewTokenSplitter 创建 token 切分器。
func NewTokenSplitter() *TokenSplitter {
	return &TokenSplitter{}
}

// Split 切分文本为 token 列表。
//
// 将本次 text 与上一次保留的 pendingTail 拼接后进行切分。
// 最后一个 token 如果以 Latin 字母/数字结尾（可能跨帧延续），
// 或仅由空格组成（等待下一个字符附着），则保留到 pendingTail，不在本次返回。
func (s *TokenSplitter) Split(text string) []string {
	// 拼接跨帧残余
	full := s.pendingTail + text
	s.pendingTail = ""

	if full == "" {
		return nil
	}

	runes := []rune(full)
	var tokens []string
	var current []rune

	// flushCurrent 将 current 作为一个完整 token 输出并清空。
	flushCurrent := func() {
		if len(current) > 0 {
			tokens = append(tokens, string(current))
			current = current[:0]
		}
	}

	// attachToPrev 将 rune 追加到 current（非空时）或 tokens 的最后一个 token；
	// 若两者均空则独立成 token。用于标点/换行的"附着"语义。
	attachToPrev := func(r rune) {
		if len(current) > 0 {
			current = append(current, r)
		} else if len(tokens) > 0 {
			tokens[len(tokens)-1] += string(r)
		} else {
			tokens = append(tokens, string(r))
		}
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		switch {
		case r == '\n' || r == '\r':
			// 换行符：附着到前一个 token
			attachToPrev(r)

		case isCJK(r):
			// CJK 字符独立 token；
			// 但如果 current 全是空格（作为前缀等待），则空格 + CJK 合并为一个 token。
			if isAllSpaces(current) {
				current = append(current, r)
				flushCurrent()
			} else {
				flushCurrent()
				tokens = append(tokens, string(r))
			}

		case isEmoji(r):
			// Emoji 独立 token
			flushCurrent()
			tokens = append(tokens, string(r))

		case isLatinAlnum(r):
			// Latin 字母/数字：与前方连续的字母数字或空格前缀合并
			if len(current) > 0 {
				last := current[len(current)-1]
				if isLatinAlnum(last) || isSpace(last) {
					current = append(current, r)
				} else {
					// 前方以标点/换行收束 → flush 再开始
					flushCurrent()
					current = append(current, r)
				}
			} else {
				current = append(current, r)
			}

		case isSpace(r):
			// 空格：作为下一个 token 的前缀
			// current 以非空格结尾 → flush 当前 token，空格开始新 token
			// current 已有空格前缀 → 继续累积
			if len(current) > 0 {
				last := current[len(current)-1]
				if isSpace(last) {
					current = append(current, r)
				} else {
					flushCurrent()
					current = append(current, r)
				}
			} else {
				current = append(current, r)
			}

		case isPunctuation(r):
			// 标点（CJK/Latin）：附着到前一个 token 后缀
			attachToPrev(r)

		default:
			// 其他字符：独立 token
			flushCurrent()
			tokens = append(tokens, string(r))
		}
	}

	// 检查最后的 current 是否需要保留为 pendingTail
	if len(current) > 0 {
		last := current[len(current)-1]
		switch {
		case isLatinAlnum(last):
			// 以字母/数字结尾 → 可能跨帧延续，保留
			s.pendingTail = string(current)
		case isAllSpaces(current):
			// 全是空格 → 等待下一个字符附着，保留
			s.pendingTail = string(current)
		default:
			// 以标点/换行/其他结尾 → 完整 token，输出
			tokens = append(tokens, string(current))
		}
	}

	return tokens
}

// Flush 返回 pendingTail 残余并清空。
//
// 在流结束时调用，确保最后一帧保留的不完整片段被输出。
func (s *TokenSplitter) Flush() string {
	tail := s.pendingTail
	s.pendingTail = ""
	return tail
}

// ── 内部辅助函数 ──

// isCJK 判断是否为 CJK（中日韩）字符。
// 包含汉字、平假名、片假名、谚文。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// isEmoji 判断是否为 Emoji 相关字符。
// 覆盖 Emoticons/Pictographs、Misc Symbols/Dingbats、Misc Technical 范围。
func isEmoji(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1FAFF) || // Emoticons & Pictographs
		(r >= 0x2600 && r <= 0x27BF) || // Misc Symbols & Dingbats
		(r >= 0x2300 && r <= 0x23FF) // Misc Technical
}

// isLatinAlnum 判断是否为 ASCII Latin 字母或数字。
func isLatinAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}

// isSpace 判断是否为空白字符（不含换行符 \n \r）。
// 换行符有独立的附着语义，不作为空格处理。
func isSpace(r rune) bool {
	return unicode.IsSpace(r) && r != '\n' && r != '\r'
}

// isAllSpaces 判断 rune 切片是否全部由空白字符组成（至少 1 个元素，不含换行）。
func isAllSpaces(runes []rune) bool {
	if len(runes) == 0 {
		return false
	}
	for _, r := range runes {
		if !isSpace(r) {
			return false
		}
	}
	return true
}

// isPunctuation 判断是否为标点符号（含 CJK 标点）。
// 使用 unicode.IsPunct 覆盖通用标点，额外检查 CJK 标点和全角符号范围。
func isPunctuation(r rune) bool {
	return unicode.IsPunct(r) ||
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // Halfwidth and Fullwidth Forms（含全角标点）
}

// ════════════════════════════════════════════════════════════════════════════
// FrameParser — SSE 帧解析器
// ════════════════════════════════════════════════════════════════════════════

// frameKind 微帧类型。
type frameKind int

const (
	frameControl frameKind = iota // 控制事件（block_start/message_start）— 直接透传
	frameText                     // 文本 delta
	frameThinking                 // 思考 delta
	frameTool                     // 工具调用 delta — 仅分类标记，不切分
	frameBarrier                  // 屏障事件（block_stop/message_stop/error）— 需排空前面积压
)

// microFrame 微帧 — 切分后的最小输出单元。
type microFrame struct {
	kind       frameKind
	data       []byte // 完整 SSE 帧 bytes
	tokenCount int    // 仅 text/thinking 有效
	isBarrier  bool   // barrier 事件标记
}

// FrameParser SSE 帧解析器，将原始 SSE 帧解析为 microFrame 列表。
//
// 根据 outFormat 分派到对应协议格式的解析函数。
// 对于 text/thinking delta，会通过 TokenSplitter 切分为微帧。
// 对于控制事件（block_start/message_start），直接透传。
// 对于屏障事件（block_stop/message_stop/error），标记 isBarrier。
//
// 为支持流结束时的尾部残余（pendingTail）构建正确的 SSE 帧，
// 解析器会记录最后一次 text/thinking delta 的构建上下文（index/id 等字段）。
// FlushRemaining() 方法基于该上下文生成残余帧。
type FrameParser struct {
	format           codec.FormatType
	textSplitter     *TokenSplitter
	thinkingSplitter *TokenSplitter

	// 残余帧构建上下文（每次 split delta 时更新）
	textCtx     *deltaBuildContext
	thinkingCtx *deltaBuildContext
}

// deltaBuildContext 记录构建 SSE 帧所需的上下文（跨 4 种协议格式的并集）。
// 仅在 flush 残余 tail 时使用，保证 flush 帧与原始 delta 帧格式一致。
type deltaBuildContext struct {
	// Anthropic
	index      int
	deltaType  string // "text_delta" / "thinking_delta"
	deltaField string // "text" / "thinking"

	// OpenAI Completions
	openaiID      string
	openaiObject  string
	openaiCreated int64
	openaiModel   string
	openaiField   string // "content" / "reasoning_content"

	// OpenAI Responses
	responsesEventType    string
	responsesOutputIndex  int
	responsesContentIndex int

	// Gemini
	geminiRole      string
	geminiIsThinking bool
}

// NewFrameParser 创建帧解析器。
// text 和 thinking 各持有独立的 TokenSplitter（各自维护 pendingTail）。
func NewFrameParser(format codec.FormatType) *FrameParser {
	return &FrameParser{
		format:           format,
		textSplitter:     NewTokenSplitter(),
		thinkingSplitter: NewTokenSplitter(),
	}
}

// FlushText 返回 text splitter 的 pendingTail 残余。
func (p *FrameParser) FlushText() string {
	return p.textSplitter.Flush()
}

// FlushThinking 返回 thinking splitter 的 pendingTail 残余。
func (p *FrameParser) FlushThinking() string {
	return p.thinkingSplitter.Flush()
}

// Parse 解析 SSE 帧，返回 microFrame 列表。
//
// 对于控制事件和屏障事件，返回单个 microFrame（原 data）。
// 对于 text/thinking delta，切分为多个 microFrame。
// 解析失败时返回包含原数据的单个 control 帧（安全降级）。
func (p *FrameParser) Parse(data []byte) []microFrame {
	switch p.format {
	case codec.FormatAnthropic:
		return p.parseAnthropic(data)
	case codec.FormatOpenAI:
		return p.parseOpenAI(data)
	case codec.FormatResponses:
		return p.parseResponses(data)
	case codec.FormatGemini:
		return p.parseGemini(data)
	default:
		return []microFrame{{kind: frameControl, data: data}}
	}
}

// ── SSE 解析辅助函数 ──

// extractSSEData 从 SSE 帧中提取 data: 行的 JSON payload 和 event: 行的事件类型。
//
// SSE 格式:
//
//	event: {type}\n    （可选，Anthropic/Responses 使用）
//	data: {json}\n     （必有）
//	\n                 （帧结束空行）
//
// 返回:
//   - jsonData: data: 行之后的 JSON 内容（不含 "data: " 前缀）
//   - eventType: event: 行的事件类型（无 event: 行时为空）
//
// 特殊处理: OpenAI 终止帧 "data: [DONE]" 返回 jsonData=[]byte("[DONE]").
func extractSSEData(raw []byte) (jsonData []byte, eventType string) {
	var dataLines []string
	rawStr := string(raw)

	for _, line := range strings.Split(rawStr, "\n") {
		line = strings.TrimRight(line, "\r")

		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			dataLines = append(dataLines, dataStr)
		}
	}

	if len(dataLines) == 0 {
		return nil, eventType
	}

	// 多行 data 按 SSE 规范拼接为单条（多行间无分隔符）
	return []byte(strings.Join(dataLines, "")), eventType
}

// ── Anthropic 格式解析 ──

// anthropicSSEFrame Anthropic SSE 帧的顶层 JSON 结构。
type anthropicSSEFrame struct {
	Type  string          `json:"type"`
	Index int             `json:"index"`
	Delta json.RawMessage `json:"delta,omitempty"`
}

// anthropicDelta Anthropic delta 子对象。
type anthropicDelta struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

// parseAnthropic 解析 Anthropic SSE 帧。
//
// 帧格式: `event: {type}\ndata: {json}\n\n`
// 关键事件:
//   - content_block_start / message_start → frameControl
//   - content_block_stop / message_stop → frameBarrier
//   - content_block_delta + text_delta → frameText (切分)
//   - content_block_delta + thinking_delta → frameThinking (切分)
//   - error → frameBarrier
func (p *FrameParser) parseAnthropic(data []byte) []microFrame {
	jsonData, _ := extractSSEData(data)
	if jsonData == nil {
		return []microFrame{{kind: frameControl, data: data}}
	}

	var frame anthropicSSEFrame
	if err := json.Unmarshal(jsonData, &frame); err != nil {
		return []microFrame{{kind: frameControl, data: data}}
	}

	switch frame.Type {
	case "message_start", "ping":
		return []microFrame{{kind: frameControl, data: data}}

	case "content_block_start":
		return []microFrame{{kind: frameControl, data: data}}

	case "content_block_stop", "message_delta", "message_stop":
		return []microFrame{{kind: frameBarrier, data: data, isBarrier: true}}

	case "error":
		return []microFrame{{kind: frameBarrier, data: data, isBarrier: true}}

	case "content_block_delta":
		// 解析 delta 子对象
		if len(frame.Delta) == 0 {
			return []microFrame{{kind: frameControl, data: data}}
		}

		var delta anthropicDelta
		if err := json.Unmarshal(frame.Delta, &delta); err != nil {
			return []microFrame{{kind: frameControl, data: data}}
		}

		switch delta.Type {
		case "text_delta":
			return p.splitAnthropicDelta(data, frame.Index, "text_delta", "text", delta.Text, true)

		case "thinking_delta":
			return p.splitAnthropicDelta(data, frame.Index, "thinking_delta", "thinking", delta.Thinking, false)

		case "input_json_delta":
			return []microFrame{{kind: frameTool, data: data}}
		}
	}

	return []microFrame{{kind: frameControl, data: data}}
}

// splitAnthropicDelta 将 Anthropic text/thinking delta 切分为多个微帧。
//
// 每个微帧保留原帧的 index 字段，只替换 delta.text / delta.thinking 内容。
func (p *FrameParser) splitAnthropicDelta(
	origData []byte,
	index int,
	deltaType string,
	deltaField string, // "text" or "thinking"
	text string,
	isText bool,
) []microFrame {
	splitter := p.textSplitter
	if !isText {
		splitter = p.thinkingSplitter
	}

	tokens := splitter.Split(text)
	if len(tokens) == 0 {
		return nil
	}

	kind := frameText
	if !isText {
		kind = frameThinking
	}

	// 记录构建上下文（用于 FlushRemaining）
	ctx := &deltaBuildContext{
		index:      index,
		deltaType:  deltaType,
		deltaField: deltaField,
	}
	if isText {
		p.textCtx = ctx
	} else {
		p.thinkingCtx = ctx
	}

	frames := make([]microFrame, 0, len(tokens))
	for _, token := range tokens {
		frameData := buildAnthropicDeltaFrame(index, deltaType, deltaField, token)
		frames = append(frames, microFrame{
			kind: kind,
			data: frameData,
		})
	}
	return frames
}

// buildAnthropicDeltaFrame 构建一个 Anthropic content_block_delta SSE 帧。
//
// 输出格式:
//
//	event: content_block_delta
//	data: {"type":"content_block_delta","index":N,"delta":{"type":"text_delta","text":"TOKEN"}}
func buildAnthropicDeltaFrame(index int, deltaType, deltaField, text string) []byte {
	deltaObj := map[string]any{
		"type":       deltaType,
		deltaField:   text,
	}
	payload := map[string]any{
		"type":  "content_block_delta",
		"index": index,
		"delta": deltaObj,
	}

	data, _ := json.Marshal(payload)
	return []byte("event: content_block_delta\ndata: " + string(data) + "\n\n")
}

// ── OpenAI 格式解析 ──

// openaiSSEChunk OpenAI 流式 chunk 的 JSON 结构（FrameParser 视角）。
type openaiSSEChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []openaiSSEChoice `json:"choices"`
}

type openaiSSEChoice struct {
	Index        int                `json:"index"`
	Delta        openaiSSEDeltaMsg  `json:"delta"`
	FinishReason *string            `json:"finish_reason"`
}

type openaiSSEDeltaMsg struct {
	Role             string                 `json:"role,omitempty"`
	Content          string                 `json:"content,omitempty"`
	ReasoningContent string                 `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiSSEToolCall    `json:"tool_calls,omitempty"`
}

type openaiSSEToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// parseOpenAI 解析 OpenAI Chat Completions SSE 帧。
//
// 帧格式: `data: {json}\n\n`（无 event 行）
// 关键事件:
//   - choices[0].delta.content 非空 → frameText
//   - choices[0].delta.reasoning_content 非空 → frameThinking
//   - choices[0].finish_reason 非空 → frameBarrier
//   - data: [DONE] → frameBarrier
//   - 无 choices（usage 帧） → frameControl
func (p *FrameParser) parseOpenAI(data []byte) []microFrame {
	jsonData, _ := extractSSEData(data)
	if jsonData == nil {
		return []microFrame{{kind: frameControl, data: data}}
	}

	// OpenAI 终止帧: "data: [DONE]"
	if string(jsonData) == "[DONE]" {
		return []microFrame{{kind: frameBarrier, data: data, isBarrier: true}}
	}

	var chunk openaiSSEChunk
	if err := json.Unmarshal(jsonData, &chunk); err != nil {
		return []microFrame{{kind: frameControl, data: data}}
	}

	// 无 choices（如 usage 帧）→ control
	if len(chunk.Choices) == 0 {
		return []microFrame{{kind: frameControl, data: data}}
	}

	choice := chunk.Choices[0]

	// finish_reason 非空 → barrier
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		return []microFrame{{kind: frameBarrier, data: data, isBarrier: true}}
	}

	// content 非空 → text
	if choice.Delta.Content != "" {
		tokens := p.textSplitter.Split(choice.Delta.Content)
		if len(tokens) == 0 {
			return nil
		}
		p.textCtx = &deltaBuildContext{
			openaiID:      chunk.ID,
			openaiObject:  chunk.Object,
			openaiCreated: chunk.Created,
			openaiModel:   chunk.Model,
			openaiField:   "content",
		}
		frames := make([]microFrame, 0, len(tokens))
		for _, token := range tokens {
			frameData := buildOpenAIChunkFrame(chunk.ID, chunk.Object, chunk.Created, chunk.Model, "content", token)
			frames = append(frames, microFrame{
				kind: frameText,
				data: frameData,
			})
		}
		return frames
	}

	// reasoning_content 非空 → thinking
	if choice.Delta.ReasoningContent != "" {
		tokens := p.thinkingSplitter.Split(choice.Delta.ReasoningContent)
		if len(tokens) == 0 {
			return nil
		}
		p.thinkingCtx = &deltaBuildContext{
			openaiID:      chunk.ID,
			openaiObject:  chunk.Object,
			openaiCreated: chunk.Created,
			openaiModel:   chunk.Model,
			openaiField:   "reasoning_content",
		}
		frames := make([]microFrame, 0, len(tokens))
		for _, token := range tokens {
			frameData := buildOpenAIChunkFrame(chunk.ID, chunk.Object, chunk.Created, chunk.Model, "reasoning_content", token)
			frames = append(frames, microFrame{
				kind: frameThinking,
				data: frameData,
			})
		}
		return frames
	}

	// tool_calls 非空 → frameTool（不切分，原帧透传）
	if len(choice.Delta.ToolCalls) > 0 {
		return []microFrame{{kind: frameTool, data: data}}
	}

	// 仅 role 的初始帧或其他 → control
	return []microFrame{{kind: frameControl, data: data}}
}

// buildOpenAIChunkFrame 构建一个 OpenAI Chat Completions chunk SSE 帧。
//
// 输出格式:
//
//	data: {"id":"...","object":"chat.completion.chunk","created":N,"model":"...","choices":[{"index":0,"delta":{"FIELD":"TOKEN"},"finish_reason":null}]}
//
// FIELD 为 "content" 或 "reasoning_content"。
func buildOpenAIChunkFrame(id, object string, created int64, model, field, text string) []byte {
	deltaJSON := `{"` + field + `":` + jsonString(text) + `}`
	choicesJSON := `[{"index":0,"delta":` + deltaJSON + `,"finish_reason":null}]`

	payload := map[string]any{
		"id":      id,
		"object":  object,
		"created": created,
		"model":   model,
		"choices": json.RawMessage(choicesJSON),
	}

	data, _ := json.Marshal(payload)
	return []byte("data: " + string(data) + "\n\n")
}

// jsonString 将字符串编码为 JSON 字符串（含引号）。
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ── Responses 格式解析 ──

// responsesSSEEvent OpenAI Responses 流式事件的 JSON 结构（通用）。
type responsesSSEEvent struct {
	Type         string `json:"type"`
	OutputIndex  int    `json:"output_index,omitempty"`
	ContentIndex int    `json:"content_index,omitempty"`
	Delta        string `json:"delta,omitempty"`
}

// parseResponses 解析 OpenAI Responses SSE 帧。
//
// 帧格式: `event: {type}\ndata: {json}\n\n`
// 关键事件:
//   - response.output_text.delta → frameText (提取 .delta)
//   - response.reasoning_text.delta → frameThinking (提取 .delta)
//   - response.output_text.done / response.completed → frameBarrier
//   - response.failed / response.error → frameBarrier
//   - response.created / response.output_item.added → frameControl
func (p *FrameParser) parseResponses(data []byte) []microFrame {
	jsonData, eventType := extractSSEData(data)
	if jsonData == nil {
		return []microFrame{{kind: frameControl, data: data}}
	}

	// 优先使用 event: 行的事件类型，回退到 .type 字段
	evType := eventType
	if evType == "" {
		var probe responsesSSEEvent
		if err := json.Unmarshal(jsonData, &probe); err != nil {
			return []microFrame{{kind: frameControl, data: data}}
		}
		evType = probe.Type
	}

	switch {
	case evType == "response.output_text.delta":
		var ev responsesSSEEvent
		if err := json.Unmarshal(jsonData, &ev); err != nil {
			return []microFrame{{kind: frameControl, data: data}}
		}
		tokens := p.textSplitter.Split(ev.Delta)
		if len(tokens) == 0 {
			return nil
		}
		p.textCtx = &deltaBuildContext{
			responsesEventType:    evType,
			responsesOutputIndex:  ev.OutputIndex,
			responsesContentIndex: ev.ContentIndex,
		}
		frames := make([]microFrame, 0, len(tokens))
		for _, token := range tokens {
			frameData := buildResponsesDeltaFrame(evType, ev.OutputIndex, ev.ContentIndex, token)
			frames = append(frames, microFrame{
				kind: frameText,
				data: frameData,
			})
		}
		return frames

	case evType == "response.reasoning_text.delta":
		var ev responsesSSEEvent
		if err := json.Unmarshal(jsonData, &ev); err != nil {
			return []microFrame{{kind: frameControl, data: data}}
		}
		tokens := p.thinkingSplitter.Split(ev.Delta)
		if len(tokens) == 0 {
			return nil
		}
		p.thinkingCtx = &deltaBuildContext{
			responsesEventType:    evType,
			responsesOutputIndex:  ev.OutputIndex,
			responsesContentIndex: ev.ContentIndex,
		}
		frames := make([]microFrame, 0, len(tokens))
		for _, token := range tokens {
			frameData := buildResponsesDeltaFrame(evType, ev.OutputIndex, ev.ContentIndex, token)
			frames = append(frames, microFrame{
				kind: frameThinking,
				data: frameData,
			})
		}
		return frames

	case evType == "response.function_call_arguments.delta",
		evType == "response.function_call_arguments.done":
		return []microFrame{{kind: frameTool, data: data}}

	case evType == "response.output_text.done",
		evType == "response.reasoning_text.done",
		evType == "response.completed",
		evType == "response.failed",
		evType == "response.incomplete",
		evType == "error":
		return []microFrame{{kind: frameBarrier, data: data, isBarrier: true}}

	default:
		// response.created / response.output_item.added / response.function_call_arguments.* / ping 等
		return []microFrame{{kind: frameControl, data: data}}
	}
}

// buildResponsesDeltaFrame 构建一个 Responses delta SSE 帧。
//
// 输出格式:
//
//	event: response.output_text.delta
//	data: {"type":"response.output_text.delta","output_index":N,"content_index":N,"delta":"TOKEN"}
func buildResponsesDeltaFrame(eventType string, outputIndex, contentIndex int, text string) []byte {
	payload := map[string]any{
		"type":          eventType,
		"output_index":  outputIndex,
		"content_index": contentIndex,
		"delta":         text,
	}

	data, _ := json.Marshal(payload)
	return []byte("event: " + eventType + "\ndata: " + string(data) + "\n\n")
}

// ── Gemini 格式解析 ──

// geminiSSEChunk Gemini 流式 chunk 的 JSON 结构（FrameParser 视角）。
type geminiSSEChunk struct {
	Candidates    []geminiSSECandidate `json:"candidates,omitempty"`
	UsageMetadata json.RawMessage      `json:"usageMetadata,omitempty"`
}

type geminiSSECandidate struct {
	Content      *geminiSSEContent `json:"content,omitempty"`
	FinishReason string            `json:"finishReason,omitempty"`
	Index        int               `json:"index"`
}

type geminiSSEContent struct {
	Role  string            `json:"role,omitempty"`
	Parts []geminiSSEPart   `json:"parts,omitempty"`
}

type geminiSSEPart struct {
	Text          string          `json:"text,omitempty"`
	Thought       bool            `json:"thought,omitempty"`
	FunctionCall  *geminiFuncCall `json:"functionCall,omitempty"`
}

type geminiFuncCall struct {
	Name string          `json:"name,omitempty"`
	Args json.RawMessage `json:"args,omitempty"`
}

// parseGemini 解析 Gemini SSE 帧。
//
// 帧格式: `data: {json}\n\n`（无 event 行）
// 关键事件:
//   - candidates[0].content.parts[0].text 非空 → frameText (或 frameThinking if thought=true)
//   - candidates[0].finishReason 非空 → frameBarrier
//   - 无 candidates (usage 帧) → frameControl
func (p *FrameParser) parseGemini(data []byte) []microFrame {
	jsonData, _ := extractSSEData(data)
	if jsonData == nil {
		return []microFrame{{kind: frameControl, data: data}}
	}

	var chunk geminiSSEChunk
	if err := json.Unmarshal(jsonData, &chunk); err != nil {
		return []microFrame{{kind: frameControl, data: data}}
	}

	// 无 candidates（如 usageMetadata 帧）→ control
	if len(chunk.Candidates) == 0 {
		return []microFrame{{kind: frameControl, data: data}}
	}

	candidate := chunk.Candidates[0]

	// finishReason 非空 → barrier
	if candidate.FinishReason != "" {
		return []microFrame{{kind: frameBarrier, data: data, isBarrier: true}}
	}

	// 检查 parts 中的文本内容
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return []microFrame{{kind: frameControl, data: data}}
	}

	part := candidate.Content.Parts[0]

	// functionCall 非空 → frameTool（不切分）
	if part.FunctionCall != nil {
		return []microFrame{{kind: frameTool, data: data}}
	}

	if part.Text == "" {
		return []microFrame{{kind: frameControl, data: data}}
	}

	// thought=true → thinking delta
	isThinking := part.Thought

	splitter := p.textSplitter
	if isThinking {
		splitter = p.thinkingSplitter
	}

	tokens := splitter.Split(part.Text)
	if len(tokens) == 0 {
		return nil
	}

	kind := frameText
	if isThinking {
		kind = frameThinking
	}

	if isThinking {
		p.thinkingCtx = &deltaBuildContext{
			geminiRole:       candidate.Content.Role,
			geminiIsThinking: true,
		}
	} else {
		p.textCtx = &deltaBuildContext{
			geminiRole:       candidate.Content.Role,
			geminiIsThinking: false,
		}
	}

	frames := make([]microFrame, 0, len(tokens))
	for _, token := range tokens {
		frameData := buildGeminiChunkFrame(candidate.Content.Role, token, isThinking)
		frames = append(frames, microFrame{
			kind: kind,
			data: frameData,
		})
	}
	return frames
}

// buildGeminiChunkFrame 构建一个 Gemini chunk SSE 帧。
//
// 输出格式:
//
//	data: {"candidates":[{"content":{"role":"model","parts":[{"text":"TOKEN"}]},"index":0}]}
//
// 对于 thinking delta，parts 中添加 "thought":true。
func buildGeminiChunkFrame(role, text string, isThinking bool) []byte {
	part := map[string]any{
		"text": text,
	}
	if isThinking {
		part["thought"] = true
	}

	if role == "" {
		role = "model"
	}

	payload := map[string]any{
		"candidates": []any{
			map[string]any{
				"index": 0,
				"content": map[string]any{
					"role":  role,
					"parts": []any{part},
				},
			},
		},
	}

	data, _ := json.Marshal(payload)
	return []byte("data: " + string(data) + "\n\n")
}

// ── Flush 残余帧 ──

// FlushRemaining 输出 text/thinking splitter 的 pendingTail 残余帧。
//
// 在流结束时调用，确保最后一帧保留的不完整 token 被正确输出。
// 返回的 microFrame 列表为 text 残余 + thinking 残余（空 tail 跳过）。
func (p *FrameParser) FlushRemaining() []microFrame {
	var frames []microFrame

	if textTail := p.textSplitter.Flush(); textTail != "" {
		if frame := p.buildFlushFrame(textTail, true); frame != nil {
			frames = append(frames, microFrame{kind: frameText, data: frame})
		}
	}
	if thinkingTail := p.thinkingSplitter.Flush(); thinkingTail != "" {
		if frame := p.buildFlushFrame(thinkingTail, false); frame != nil {
			frames = append(frames, microFrame{kind: frameThinking, data: frame})
		}
	}
	return frames
}

// buildFlushFrame 基于已记录的构建上下文生成一帧 SSE 数据。
// isText=true 使用 textCtx，false 使用 thinkingCtx；上下文为 nil 时返回 nil（无残余可输出）。
func (p *FrameParser) buildFlushFrame(text string, isText bool) []byte {
	var ctx *deltaBuildContext
	if isText {
		ctx = p.textCtx
	} else {
		ctx = p.thinkingCtx
	}
	if ctx == nil {
		return nil
	}

	switch p.format {
	case codec.FormatAnthropic:
		return buildAnthropicDeltaFrame(ctx.index, ctx.deltaType, ctx.deltaField, text)

	case codec.FormatOpenAI:
		return buildOpenAIChunkFrame(ctx.openaiID, ctx.openaiObject, ctx.openaiCreated, ctx.openaiModel, ctx.openaiField, text)

	case codec.FormatResponses:
		return buildResponsesDeltaFrame(ctx.responsesEventType, ctx.responsesOutputIndex, ctx.responsesContentIndex, text)

	case codec.FormatGemini:
		return buildGeminiChunkFrame(ctx.geminiRole, text, ctx.geminiIsThinking)
	}
	return nil
}
