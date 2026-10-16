package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/pkg/ws"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ResponsesSessionStore 维护 Responses WebSocket / 多轮会话的历史快照（response_id -> 完整上下文快照）。
//
// 当下游客户端使用 OpenAI Responses WebSocket 模式发送携带 `previous_response_id` 的增量请求，
// 而上游是无状态的 Anthropic / Gemini / OpenAI Chat Completions 时，
// 通过该 Store 自动还原前序轮次的完整对话历史（含思维签名、工具调用及工具结果），
// 并支持跨 stream_id 分叉（快照不可变）。
type ResponsesSessionStore struct {
	mu      sync.RWMutex
	records map[string]responsesSessionRecord
}

type responsesSessionRecord struct {
	system   string
	messages []bamboo.BambooMessage
}

// NewResponsesSessionStore 创建一个新的并发安全 Responses 会话存储实例。
func NewResponsesSessionStore() *ResponsesSessionStore {
	return &ResponsesSessionStore{
		records: make(map[string]responsesSessionRecord),
	}
}

// Save 保存指定 responseID 对应的完整对话历史快照（含本轮 assistant 输出）。
func (s *ResponsesSessionStore) Save(responseID, system string, history []bamboo.BambooMessage) {
	if s == nil || responseID == "" {
		return
	}
	cloned := make([]bamboo.BambooMessage, len(history))
	for i := range history {
		cloned[i] = bamboo.BambooMessage{
			Role:        history[i].Role,
			ReasoningID: history[i].ReasoningID,
			Content:     append([]bamboo.ContentBlock(nil), history[i].Content...),
		}
	}
	s.mu.Lock()
	s.records[responseID] = responsesSessionRecord{
		system:   system,
		messages: cloned,
	}
	s.mu.Unlock()
}

// Load 读取指定 responseID 对应的历史快照副本。
func (s *ResponsesSessionStore) Load(responseID string) (system string, messages []bamboo.BambooMessage, ok bool) {
	if s == nil || responseID == "" {
		return "", nil, false
	}
	s.mu.RLock()
	rec, exists := s.records[responseID]
	s.mu.RUnlock()
	if !exists {
		return "", nil, false
	}
	cloned := make([]bamboo.BambooMessage, len(rec.messages))
	for i := range rec.messages {
		cloned[i] = bamboo.BambooMessage{
			Role:        rec.messages[i].Role,
			ReasoningID: rec.messages[i].ReasoningID,
			Content:     append([]bamboo.ContentBlock(nil), rec.messages[i].Content...),
		}
	}
	return rec.system, cloned, true
}

// RelayResponsesWebSocket 将下游 HTTP 请求升级为 OpenAI Responses WebSocket 服务端连接，
// 并将每轮 `response.create` 事件桥接到任意底层 `provider.Provider`（Anthropic / Gemini / OpenAI / Responses）。
//
// 核心能力：
//  1. 协议升级：基于纯标准库 `pkg/ws.Upgrade` 完成 RFC 6455 握手；
//  2. 多路复用：支持默认通道与命名 `stream_id` 通道，同通道 FIFO 串行、跨通道并发执行，出站事件自动回填 `stream_id`；
//  3. 跨协议历史续接：内置连接级（或通过 opts 共享的）`ResponsesSessionStore`，当上游不是原生 `openai-responses` 时，
//     自动根据 `previous_response_id` 拼接历史消息与本轮新增输入（如 `function_call_output`），并在找不到父 ID 时返回标准 `previous_response_not_found` 错误事件；
//  4. 预热模式（`generate: false`）：支持仅保存上下文状态并返回 `response.created` + `response.completed` 空输出响应。
func RelayResponsesWebSocket(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	p provider.Provider,
	opts ...Option,
) error {
	conn, err := ws.Upgrade(w, r)
	if err != nil {
		return err
	}
	return ServeResponsesWebSocketConn(ctx, conn, p, opts...)
}

// ServeResponsesWebSocketConn 在已建立的 `*ws.Conn` 上运行 Responses WebSocket 服务端事件循环。
func ServeResponsesWebSocketConn(
	ctx context.Context,
	conn *ws.Conn,
	p provider.Provider,
	opts ...Option,
) error {
	defer func() {
		_ = conn.Close()
	}()

	cfg := applyOptions(opts...)
	store := cfg.SessionStore
	if store == nil {
		store = NewResponsesSessionStore()
	}

	respCodec, err := codec.Get(codec.FormatResponses)
	if err != nil || respCodec == nil {
		return pkgErrors.NewBambooError("下游", "codec \"responses\" is not registered", 0)
	}

	// 每个 stream_id 对应独立的 FIFO 队列，实现“同通道串行、跨通道并行”
	type streamTask struct {
		rawFrame []byte
	}
	var streamsMu sync.Mutex
	streams := make(map[string]chan streamTask)
	var wg sync.WaitGroup

	connCtx, cancelConn := context.WithCancel(ctx)
	defer func() {
		cancelConn()
		streamsMu.Lock()
		for _, ch := range streams {
			close(ch)
		}
		streamsMu.Unlock()
		wg.Wait()
	}()

	dispatchTask := func(streamKey string, raw []byte) {
		streamsMu.Lock()
		ch, ok := streams[streamKey]
		if !ok {
			ch = make(chan streamTask, 32)
			streams[streamKey] = ch
			wg.Add(1)
			go func(taskCh <-chan streamTask) {
				defer wg.Done()
				for task := range taskCh {
					handleResponsesWSFrame(connCtx, conn, p, respCodec, store, cfg, task.rawFrame)
				}
			}(ch)
		}
		streamsMu.Unlock()

		select {
		case ch <- streamTask{rawFrame: raw}:
		case <-connCtx.Done():
		}
	}

	for {
		_, payload, readErr := conn.ReadMessage()
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}

		var env struct {
			Type     string `json:"type"`
			StreamID string `json:"stream_id"`
		}
		if err := json.Unmarshal(payload, &env); err != nil {
			_ = writeWSErrorEvent(conn, "", "invalid_request", "invalid JSON frame")
			continue
		}
		if env.Type != "" && env.Type != "response.create" {
			_ = writeWSErrorEvent(conn, env.StreamID, "invalid_request", fmt.Sprintf("unsupported websocket client event type %q", env.Type))
			continue
		}

		dispatchTask(env.StreamID, payload)
	}
}

func handleResponsesWSFrame(
	ctx context.Context,
	conn *ws.Conn,
	p provider.Provider,
	respCodec codec.Codec,
	store *ResponsesSessionStore,
	cfg *Config,
	rawFrame []byte,
) {
	req, err := respCodec.ParseRequest(rawFrame)
	if err != nil {
		cfg.triggerError(err)
		_ = writeWSErrorEvent(conn, "", "invalid_request", err.Error())
		return
	}

	streamID, _ := provider.GetExtraString(req.Config.ProviderExtra, "stream_id")
	prevID, _ := provider.GetExtraString(req.Config.ProviderExtra, "previous_response_id")

	// 若指定了 previous_response_id，尝试从 SessionStore 恢复前序历史
	if prevID != "" {
		prevSys, prevMsgs, ok := store.Load(prevID)
		if !ok {
			// 若底层不是原生 Responses Provider，则必须依赖网关侧历史，找不到时报错 previous_response_not_found
			if p.GetProviderType() != provider.ProviderOpenAIResponses {
				prevErr := pkgErrors.NewBambooError("下游", fmt.Sprintf("previous_response_id %q not found in session store", prevID), 400)
				cfg.triggerError(prevErr)
				_ = writeWSErrorEvent(conn, streamID, "previous_response_not_found", prevErr.Message)
				return
			}
		} else {
			if req.System == "" {
				req.System = prevSys
			}
			combined := make([]bamboo.BambooMessage, 0, len(prevMsgs)+len(req.Messages))
			combined = append(combined, prevMsgs...)
			combined = append(combined, req.Messages...)
			req.Messages = combined
		}
	}

	// 检查 generate:false（预热上下文模式，不调用上游生成，直接保存历史并返回 completed）
	if gen, hasGen := provider.GetExtraBool(req.Config.ProviderExtra, "generate"); hasGen && !gen {
		warmupID := fmt.Sprintf("resp_warmup_%d", len(req.Messages)+1)
		store.Save(warmupID, req.System, req.Messages)
		_ = writeWSWarmupEvents(conn, streamID, warmupID, req.Config.Model)
		return
	}

	client := bamboo.NewClient(p)
	eventCh, err := client.Chat(ctx, req.Messages, req.System, req.Config)
	if err != nil {
		cfg.triggerError(err)
		code := "server_error"
		msg := err.Error()
		var be *pkgErrors.BambooError
		if errors.As(err, &be) {
			msg = be.Message
			if be.StatusCode == 401 || be.StatusCode == 403 {
				code = "authentication_error"
			} else if be.StatusCode == 429 {
				code = "rate_limit_exceeded"
			} else if be.StatusCode >= 400 && be.StatusCode < 500 {
				code = "invalid_request"
			}
		}
		_ = writeWSErrorEvent(conn, streamID, code, msg)
		return
	}

	serializer := respCodec.NewSerializer(req.Config.Model)
	collector := newAssistantTurnCollector()
	var responseID string

	for event := range eventCh {
		if event.Type == bamboo.EventError && event.Error != nil {
			cfg.triggerError(event.Error)
		}
		if event.Usage != nil && (event.Usage.InputTokens > 0 || event.Usage.OutputTokens > 0) {
			cfg.triggerUsage(*event.Usage)
		}
		collector.observe(event)

		sseBytes, sErr := serializer.Serialize(event)
		if sErr != nil {
			cfg.triggerError(toBambooError(sErr))
			continue
		}
		for _, payload := range ExtractSSEDataPayloads(sseBytes) {
			if id := extractResponseIDFromEventJSON(payload); id != "" {
				responseID = id
			}
			outBytes := injectStreamID(payload, streamID)
			if writeErr := conn.WriteText(outBytes); writeErr != nil {
				return
			}
		}
	}

	if flushBytes, fErr := serializer.Flush(); fErr == nil && len(flushBytes) > 0 {
		for _, payload := range ExtractSSEDataPayloads(flushBytes) {
			outBytes := injectStreamID(payload, streamID)
			_ = conn.WriteText(outBytes)
		}
	}

	// 将本轮完整输入 + assistant 产出存入 SessionStore，供下一轮 previous_response_id 续接
	if responseID != "" && !collector.failed {
		updated := make([]bamboo.BambooMessage, 0, len(req.Messages)+1)
		updated = append(updated, req.Messages...)
		if assistantMsg, ok := collector.buildMessage(); ok {
			updated = append(updated, assistantMsg)
		}
		store.Save(responseID, req.System, updated)
	}
}

// assistantTurnCollector 收集流式输出中的 assistant 内容块，用于构建多轮会话历史。
type assistantTurnCollector struct {
	blocks      map[int]bamboo.ContentBlock
	inputJSON   map[int]*strings.Builder
	order       []int
	reasoningID string
	failed      bool
}

func newAssistantTurnCollector() *assistantTurnCollector {
	return &assistantTurnCollector{
		blocks:    make(map[int]bamboo.ContentBlock),
		inputJSON: make(map[int]*strings.Builder),
	}
}

func (c *assistantTurnCollector) observe(ev bamboo.StreamEvent) {
	switch ev.Type {
	case bamboo.EventError:
		c.failed = true
	case bamboo.EventMessageStart, bamboo.EventMessageDelta:
		if md, ok := ev.Delta.(*bamboo.MessageDelta); ok && md.ReasoningID != "" {
			c.reasoningID = md.ReasoningID
		}
	case bamboo.EventContentBlockStart:
		if ev.ContentBlock == nil {
			return
		}
		c.order = append(c.order, ev.Index)
		switch b := ev.ContentBlock.(type) {
		case *bamboo.TextBlock:
			cp := *b
			c.blocks[ev.Index] = &cp
		case *bamboo.ThinkingBlock:
			cp := *b
			c.blocks[ev.Index] = &cp
		case *bamboo.ToolUseBlock:
			cp := *b
			c.blocks[ev.Index] = &cp
			c.inputJSON[ev.Index] = &strings.Builder{}
		}
	case bamboo.EventContentBlockDelta:
		delta, ok := ev.Delta.(*bamboo.StreamDelta)
		if !ok {
			return
		}
		switch delta.Type {
		case bamboo.DeltaTextDelta:
			if tb, ok := c.blocks[ev.Index].(*bamboo.TextBlock); ok {
				tb.Text += delta.Text
			}
		case bamboo.DeltaThinkingDelta:
			if th, ok := c.blocks[ev.Index].(*bamboo.ThinkingBlock); ok {
				th.Thinking += delta.Thinking
			}
		case bamboo.DeltaSignature:
			if th, ok := c.blocks[ev.Index].(*bamboo.ThinkingBlock); ok {
				th.Signature = delta.Signature
				if delta.SignatureProvider != "" {
					th.SignatureProvider = delta.SignatureProvider
				}
			}
		case bamboo.DeltaInputJSON:
			if sb, ok := c.inputJSON[ev.Index]; ok {
				sb.WriteString(delta.PartialJSON)
			}
		}
	case bamboo.EventContentBlockStop:
		if tu, ok := c.blocks[ev.Index].(*bamboo.ToolUseBlock); ok {
			if sb, ok := c.inputJSON[ev.Index]; ok {
				raw := strings.TrimSpace(sb.String())
				if raw == "" {
					raw = "{}"
				}
				tu.Input = json.RawMessage(raw)
			}
		}
	}
}

func (c *assistantTurnCollector) buildMessage() (bamboo.BambooMessage, bool) {
	if len(c.order) == 0 {
		return bamboo.BambooMessage{}, false
	}
	var content []bamboo.ContentBlock
	for _, idx := range c.order {
		if b, ok := c.blocks[idx]; ok && b != nil {
			content = append(content, b)
		}
	}
	if len(content) == 0 {
		return bamboo.BambooMessage{}, false
	}
	msg := bamboo.NewAssistantMessageBlocks(content...)
	msg.ReasoningID = c.reasoningID
	return msg, true
}

func extractResponseIDFromEventJSON(payload []byte) string {
	var env struct {
		ResponseID string `json:"response_id"`
		Response   *struct {
			ID string `json:"id"`
		} `json:"response"`
	}
	if json.Unmarshal(payload, &env) != nil {
		return ""
	}
	if env.Response != nil && env.Response.ID != "" {
		return env.Response.ID
	}
	return env.ResponseID
}

func injectStreamID(payload []byte, streamID string) []byte {
	if streamID == "" {
		return payload
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return payload
	}
	obj["stream_id"] = streamID
	out, err := json.Marshal(obj)
	if err != nil {
		return payload
	}
	return out
}

func writeWSErrorEvent(conn *ws.Conn, streamID, code, message string) error {
	payload := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    code,
			"code":    code,
			"message": message,
		},
	}
	if streamID != "" {
		payload["stream_id"] = streamID
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.WriteText(data)
}

func writeWSWarmupEvents(conn *ws.Conn, streamID, responseID, model string) error {
	for i, status := range []string{"in_progress", "completed"} {
		evType := "response.created"
		if i == 1 {
			evType = "response.completed"
		}
		payload := map[string]any{
			"type":            evType,
			"sequence_number": i + 1,
			"response_id":     responseID,
			"response": map[string]any{
				"id":     responseID,
				"object": "response",
				"status": status,
				"model":  model,
				"output": []any{},
			},
		}
		if streamID != "" {
			payload["stream_id"] = streamID
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if err := conn.WriteText(data); err != nil {
			return err
		}
	}
	return nil
}
