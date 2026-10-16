# 类型、结构体字段与接口

## 类型与字段

类型注释说明职责与使用边界。每个导出字段的含义由类型注释或字段注释说明，不要求逐个字段单独注释。字段有来源、必填性、默认值或资源归属等约束时，在字段上方单独说明；一组字段共用一段说明时，注释写在这组字段之前：

```go
// Response 是非流式对话调用的结构化聚合响应。
//
// # 核心字段与链路追踪
//
//   - ProviderType: 底层实际承载请求的协议适配器（如 "anthropic"、"openai-completions"），
//     在中继网关执行跨协议互转时用于保留端点溯源凭证；
//   - ResponseID: 对应 OpenAI Responses API 的 response_id，用于长会话链路追踪与状态续接；
//   - ReasoningID: 推理项独立 ID（如 "rs_xxx"），与加密签名解耦。
type Response struct {
	ID           string         `json:"id"`                      // 消息唯一标识（服务商生成）
	Type         string         `json:"type"`                    // 消息类型，固定为 "message"
	Role         MessageRole    `json:"role"`                    // 消息角色，固定为 "assistant"
	Content      []ContentBlock `json:"content"`                 // 响应内容块列表
	Model        string         `json:"model"`                   // 实际响应的模型标识
	StopReason   FinishReason   `json:"stop_reason"`             // 停止原因（如 "end_turn"）
	StopSequence string         `json:"stop_sequence,omitempty"` // 触发停止的序列（可选）
	Usage        Usage          `json:"usage"`                   // Token 用量统计
	ProviderType string         `json:"provider_type"`          // 底层协议类型
	RequestID    string         `json:"request_id,omitempty"`   // 请求追踪 ID
	ResponseID   string         `json:"response_id,omitempty"`  // 响应 ID（用于会话续接）
	ReasoningID  string         `json:"reasoning_id,omitempty"` // reasoning item ID
	CreatedAt    int64          `json:"created_at,omitempty"`   // 响应创建时间戳
}
```

字段注释形式以结构体为单位保持风格一致，**严禁内部混搭**：

- **统一行尾注释（推荐优先）**：同一结构体内所有字段统一使用行尾 `// ...` 注释，由 `gofmt` 自动完成列对齐，保持紧凑、整洁、高可读性；
- **复杂字段提升至类型总 GoDoc**：当某些字段需要长篇解释、复杂校验规则或跨协议演进背景时，**严禁在结构体内部插入多行注释打断行尾注释的一致性**，必须提升至该 `type` 上方的总 GoDoc 中通过段落或列表进行详细展开说明。

## 接口与方法

接口方法的契约是调用方与实现方共同遵守的约定，写在方法注释或接口的类型注释中。标准库 `io.Reader`、`http.Handler` 把方法契约写在类型注释里；方法较多或契约各不相同时，逐个方法注释更清楚：

```go
// Keyer 抽象能够转换为基础通用 [Key] 的对象。
type Keyer interface {
	// Key 返回对象对应的通用键。
	Key() Key
}
```

方法的输入、返回值和调用约束按 [函数与方法](./functions-and-methods.md) 检查；函数签名类型按 [函数类型与回调](./function-types.md) 检查。
