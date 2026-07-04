# bamboo-messages × new-api 对话中继协议层归一化方案

> **文档性质**：架构落地方案（替换对话类 relay 协议转换内核）
> **审查日期**：2026-06-17
> **bamboo-messages**：commit `8d9632e`，`github.com/bamboo-services/bamboo-messages`
> **目标宿主**：`new-api`，`github.com/QuantumNous/new-api`，Go 1.25.1
> **目标**：用 bamboo-messages 的中间表示替换 new-api 对话类中继（4 个 Helper）的协议转换内核，达成「任意入口协议 ↔ 任意上游协议」自由格式转换

---

## 一、执行摘要

new-api 当前的对话中继是「入口 Helper + 上游 Adaptor」两段式协议转换：入口侧按 `RelayFormat` 分派到 `TextHelper` / `ClaudeHelper` / `GeminiHelper` / `ResponsesHelper`，每个 Helper 内部又调上游 Adaptor 的 `Convert*Request` / `DoResponse` 做协议转换。这套机制在「入口格式 = 上游格式」时近乎直通，但一旦「入口格式 ≠ 上游格式」（如 Claude 入口打 DeepSeek 上游），就要依赖 adaptor 内部的 `ConvertClaudeRequest` 等方法做 N×N 组合的转换逻辑，维护成本随厂商数平方增长。

bamboo-messages 提供了一个**协议无关的中间表示**（`RelayRequest` + `bamboo.Response` + `StreamEvent`），配合 `codec` 编解码层，可将 N×N 转换降为 **N+M**：

```
入口协议 ─codec.ParseRequest──→ RelayRequest（中间表示）─→ bamboo Provider ──→ StreamEvent/Response
                                                                          │
                                    出口协议 ←──codec.SerializeResponse/Serialize────┘
```

**目标替换范围**：4 个对话类 Helper 的协议转换内核（`relay/{compatible,claude,gemini,responses}_handler.go` 内部 `Convert → DoRequest → DoResponse` 三段式）。

**不在本次范围**：Embedding / Rerank / Audio / Image / Task / Realtime 的 Helper（bamboo 当前不具备这些能力，保留 new-api 原生实现）。

---

## 二、核心理念：N×N → N+M

### 2.1 现状：new-api 的转换矩阵爆炸

new-api 的 `Adaptor` 接口（`relay/channel/adapter.go:15`）要求每个上游厂商实现 4 个 Convert 方法：

```go
ConvertOpenAIRequest(...)          // OpenAI 入口 → 该厂商上游
ConvertClaudeRequest(...)          // Claude 入口 → 该厂商上游
ConvertGeminiRequest(...)          // Gemini 入口 → 该厂商上游
ConvertOpenAIResponsesRequest(...) // Responses 入口 → 该厂商上游
```

4 种入口 × 30+ 厂商 = **120+ 个转换组合**。每新增一个入口协议（如未来加 Responses），所有厂商都要补一个 Convert 方法；每新增一个厂商，要补 4 个 Convert。这是典型的 O(N×M) 组合爆炸。

### 2.2 目标：bamboo 的中间表示打破耦合

bamboo 的 `codec` 层（`bamboo/codec/`）提供 4 种协议的独立 Codec：

| 入口 Codec | 作用 |
|-----------|------|
| `codec.OpenAI` | `ParseRequest` 解析 OpenAI Chat Completions 请求体 → `RelayRequest` |
| `codec.Anthropic` | `ParseRequest` 解析 Anthropic Messages 请求体 → `RelayRequest` |
| `codec.Responses` | `ParseRequest` 解析 OpenAI Responses 请求体 → `RelayRequest` |
| `codec.Gemini` | `ParseRequest` 解析 Gemini 请求体 → `RelayRequest` |

所有 Codec 的 `ParseRequest` 输出统一的 `RelayRequest`（`bamboo/codec/types.go:9`）：

```go
type RelayRequest struct {
    Messages []bamboo.BambooMessage   // 统一消息序列
    System   string                    // 系统提示词
    Config   *bamboo.RequestConfig     // 统一请求配置
    IsStream bool                      // 是否流式
}
```

`RelayRequest` 喂给 `bamboo.NewClient(provider).Chat()`，拿到统一 `StreamEvent` 流，再由出口 Codec 的 `NewSerializer().Serialize()` 序列化为任意出口协议的 SSE 帧。

**转换复杂度从 O(N×M) 降为 O(N+M)**：新增入口协议只需加一个入口 Codec；新增上游厂商只需在 Provider 层加一个适配器。两边各自独立扩展，互不影响。

---

## 三、替换点的精确定位

new-api 对话中继的完整调用链（以 `RelayFormatClaude` 入口为例）：

```
controller.Relay(c, RelayFormatClaude)                          ← 入口路由
  └─ helper.GetAndValidateRequest(c, relayFormat)               ← 解析 HTTP body → dto.ClaudeRequest
  └─ relaycommon.GenRelayInfo(...)                              ← 构造 RelayInfo（含 RelayFormat/RelayMode/ApiType）
  └─ service.PreConsumeBilling(...)                             ← 预扣费
  └─ relay.ClaudeHelper(c, relayInfo)                           ← ★ 替换目标 1
       ├─ adaptor := GetAdaptor(info.ApiType)                   ← 选上游 adaptor
       ├─ adaptor.ConvertClaudeRequest(c, info, request)        ← ★ 入口→上游协议转换（替换为 codec.ParseRequest）
       ├─ common.Marshal(convertedRequest)                      ← 序列化请求体
       ├─ adaptor.DoRequest(c, info, requestBody)               ← 发上游 HTTP 请求（替换为 bamboo Provider.Chat）
       └─ adaptor.DoResponse(c, httpResp, info)                 ← ★ 上游→入口协议响应转换（替换为 codec.Serialize）
  └─ service.PostTextConsumeQuota(c, info, usage, nil)          ← 结算计费
```

**4 个 Helper 的替换点完全对称**（已逐一核对源码）：

| Helper | 源文件 | 入口 DTO | 替换的 Convert 方法 |
|--------|--------|---------|-------------------|
| `TextHelper` | `relay/compatible_handler.go:25` | `*dto.GeneralOpenAIRequest` | `ConvertOpenAIRequest` |
| `ClaudeHelper` | `relay/claude_handler.go:24` | `*dto.ClaudeRequest` | `ConvertClaudeRequest` |
| `GeminiHelper` | `relay/gemini_handler.go:54` | `*dto.GeminiChatRequest` | `ConvertGeminiRequest` |
| `ResponsesHelper` | `relay/responses_handler.go:25` | `*dto.OpenAIResponsesRequest` | `ConvertOpenAIResponsesRequest` |

每个 Helper 内部的三段式（`compatible_handler.go:109-222` 为典型）：

```go
// 三段式内核 —— 这是要被 bamboo 替换的部分
convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, request)   // ① 协议转换
jsonData, err := common.Marshal(convertedRequest)                          //   序列化
resp, err := adaptor.DoRequest(c, info, requestBody)                       // ② 发请求
usage, err := adaptor.DoResponse(c, httpResp, info)                        // ③ 响应转换 + 计费
```

---

## 四、改造架构

### 4.1 新增：bamboo relay bridge 包

在 new-api 内新增 `relay/bamboo/` 包，作为对话中继的统一内核：

```
relay/bamboo/
├── bridge.go           # 核心：统一的 ChatRelay 函数，替代 4 个 Helper 的三段式
├── codec_adapter.go    # bamboo codec.FormatType ↔ new-api types.RelayFormat 映射
├── provider_factory.go # RelayInfo → bamboo Provider 实例化（选上游协议）
├── dto_codec.go        # new-api dto.Request → []byte 喂给 codec.ParseRequest
└── usage.go            # bamboo Usage → dto.Usage 计费映射
```

### 4.2 改造后的调用链

以 Claude 入口打 DeepSeek（OpenAI 兼容）上游为例：

```
controller.Relay(c, RelayFormatClaude)
  └─ relay.ClaudeHelper(c, relayInfo)                         ← 入口 Helper（保留，但内部改调 bridge）
       └─ bamboo.ChatRelay(c, info, FormatAnthropic)          ← ★ 统一内核
            ├─ ① 入口侧：codec.Anthropic.ParseRequest(body) → RelayRequest
            │     （入口 Claude 协议 → bamboo 中间表示）
            ├─ ② 上游侧：provider_factory 选 openai/completions Provider
            │     bambooClient.Chat(ctx, msgs, system, cfg) → <-chan StreamEvent
            └─ ③ 出口侧：codec.Anthropic.NewSerializer().Serialize(event)
                  （bamboo 中间表示 → 出口 Claude SSE 帧）
  └─ service.PostTextConsumeQuota(c, info, usage, nil)        ← 计费（usage 由 bridge 返回）
```

**关键变化**：入口是 Claude、上游是 DeepSeek（OpenAI 协议），全程不再需要 DeepSeek adaptor 实现 `ConvertClaudeRequest`。bamboo 的中间表示承担了所有跨协议转换。

### 4.3 网关式数据流图

```
            ┌─────────────── 入口协议层（codec.ParseRequest）───────────────┐
            │                                                              │
  POST /v1/chat/completions ──→ codec.OpenAI.ParseRequest ──┐             │
  POST /v1/messages          ──→ codec.Anthropic.ParseRequest ─┤           │
  POST /v1/responses         ──→ codec.Responses.ParseRequest ─┤           │
  POST /v1beta/models/*      ──→ codec.Gemini.ParseRequest   ──┘           │
            │                                                              │
            └──────────────────────┬───────────────────────────────────────┘
                                   ▼
                    ┌──────────────────────────────┐
                    │  RelayRequest (中间表示)       │  ← 协议无关
                    │  Messages / System / Config   │
                    └──────────────┬───────────────┘
                                   ▼
                    ┌──────────────────────────────┐
                    │  bamboo Provider (上游协议层)  │  ← 4 个适配器
                    │  anthropic / completions /    │
                    │  responses / gemini           │
                    └──────────────┬───────────────┘
                                   ▼
                    ┌──────────────────────────────┐
                    │  StreamEvent / Response 流     │  ← 协议无关
                    └──────────────┬───────────────┘
                                   ▼
            ┌─────────────── 出口协议层（codec.Serialize）────────────────┐
            │  按客户端入口的 RelayFormat 选出口 codec 序列化             │
            │  NewSerializer().Serialize(event) → SSE 帧                │
            └────────────────────────────────────────────────────────────┘
```

---

## 五、关键改造点与代码骨架

### 5.1 改造点 1：统一 ChatRelay 内核

`relay/bamboo/bridge.go` —— 替代 4 个 Helper 的三段式：

```go
package bamboo

import (
    "io"
    "github.com/gin-gonic/gin"
    bamboocodec "github.com/bamboo-services/bamboo-messages/bamboo/codec"
    bamboosdk "github.com/bamboo-services/bamboo-messages/bamboo"
    "github.com/QuantumNous/new-api/dto"
    "github.com/QuantumNous/new-api/types"
    relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// ChatRelay 对话中继统一内核。
//
// 替代 TextHelper/ClaudeHelper/GeminiHelper/ResponsesHelper 内部的
// Convert→DoRequest→DoResponse 三段式，用 bamboo 的中间表示做协议归一化。
//
// entryFormat 标识客户端入口协议（决定 ParseRequest 和出口 Serialize 用哪个 codec）；
// info.ApiType 决定上游用哪个 bamboo Provider。
func ChatRelay(c *gin.Context, info *relaycommon.RelayInfo,
    entryFormat bamboocodec.FormatType, requestBody []byte) (*dto.Usage, *types.NewAPIError) {

    // ① 入口侧：解析客户端请求体为 bamboo 中间表示
    entryCodec, err := bamboocodec.Get(entryFormat)
    if err != nil {
        return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed)
    }
    relayReq, err := entryCodec.ParseRequest(requestBody)
    if err != nil {
        return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed)
    }

    // ② 上游侧：根据 ApiType 构造 bamboo Provider 并发起对话
    p, err := newProvider(info)   // 见 5.2
    if err != nil {
        return nil, types.NewError(err, types.ErrorCodeInvalidApiType)
    }
    client := bamboosdk.NewClient(p)

    if relayReq.IsStream {
        return doStreamRelay(c, client, entryCodec, relayReq, info)
    }
    return doCompleteRelay(c, client, entryCodec, relayReq, info)
}

// doStreamRelay 流式中继：消费 bamboo StreamEvent，按入口 codec 序列化为出口 SSE。
func doStreamRelay(c *gin.Context, client bamboosdk.BambooClient,
    entryCodec bamboocodec.Codec, req *bamboocodec.RelayRequest,
    info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {

    eventCh, err := client.Chat(c.Request.Context(), req.Messages, req.System, req.Config)
    if err != nil {
        return nil, types.NewError(err, types.ErrorCodeDoRequestFailed)
    }

    c.Writer.Header().Set("Content-Type", "text/event-stream")
    c.Writer.Header().Set("Cache-Control", "no-cache")
    c.Writer.Flush()

    serializer := entryCodec.NewSerializer()
    var usage dto.Usage

    for event := range eventCh {
        if event.Type == bamboosdk.EventError {
            return nil, types.NewError(event.Error, types.ErrorCodeBadResponseBody)
        }
        // 按入口协议序列化为 SSE 帧（Claude 入口 → Claude SSE；OpenAI 入口 → OpenAI SSE）
        data, err := serializer.Serialize(event)
        if err != nil {
            return nil, types.NewError(err, types.ErrorCodeConvertResponseFailed)
        }
        if _, werr := c.Writer.Write(data); werr != nil {
            break  // 客户端断开
        }
        c.Writer.Flush()

        // 从 message_delta 提取 usage
        if event.Type == bamboosdk.EventMessageDelta && event.Usage != nil {
            usage.PromptTokens = event.Usage.InputTokens
            usage.CompletionTokens = event.Usage.OutputTokens
        }
    }

    // flush 剩余缓冲
    tail, _ := serializer.Flush()
    if len(tail) > 0 { c.Writer.Write(tail); c.Writer.Flush() }

    usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
    return &usage, nil
}

// doCompleteRelay 非流式中继。
func doCompleteRelay(c *gin.Context, client bamboosdk.BambooClient,
    entryCodec bamboocodec.Codec, req *bamboocodec.RelayRequest,
    info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {

    resp, err := client.Complete(c.Request.Context(), req.Messages, req.System, req.Config)
    if err != nil {
        return nil, types.NewError(err, types.ErrorCodeDoRequestFailed)
    }

    // 按入口协议序列化响应体
    body, err := entryCodec.SerializeResponse(resp)
    if err != nil {
        return nil, types.NewError(err, types.ErrorCodeConvertResponseFailed)
    }
    c.Writer.Header().Set("Content-Type", "application/json")
    c.Writer.Write(body)

    return &dto.Usage{
        PromptTokens:     resp.Usage.InputTokens,
        CompletionTokens: resp.Usage.OutputTokens,
        TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
    }, nil
}
```

### 5.2 改造点 2：Provider 工厂

`relay/bamboo/provider_factory.go` —— 根据 `RelayInfo.ApiType` 选 bamboo Provider：

```go
package bamboo

import (
    bamboocompletions "github.com/bamboo-services/bamboo-messages/provider/openai/completions"
    bambooresponses  "github.com/bamboo-services/bamboo-messages/provider/openai/responses"
    bambooanthropic  "github.com/bamboo-services/bamboo-messages/provider/anthropic"
    bamboogemini     "github.com/bamboo-services/bamboo-messages/provider/gemini"
    "github.com/bamboo-services/bamboo-messages/provider"
    "github.com/QuantumNous/new-api/constant"
    relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// newProvider 根据 RelayInfo 构造对应的 bamboo Provider。
//
// bamboo 的 4 个适配器均支持 WithBaseURL + WithHeader，
// 可对接 OpenAI 兼容（DeepSeek/SiliconFlow/...）、Anthropic 原生、Gemini 原生等。
//
// 未覆盖的 ApiType（AWS/讯飞/腾讯等特殊签名协议）返回 ErrUnsupportedProvider，
// 由上层 fallback 回 new-api 原生 adaptor 链路。
func newProvider(info *relaycommon.RelayInfo) (provider.Provider, error) {
    apiKey := info.ApiKey
    baseURL := info.ChannelBaseUrl

    switch info.ApiType {
    case constant.APITypeAnthropic:
        return bambooanthropic.NewProviderWithOptions(
            bambooanthropic.WithAPIKey(apiKey),
            bambooanthropic.WithBaseURL(baseURL),
        ), nil

    case constant.APITypeGemini:
        return bamboogemini.NewProviderWithOptions(
            bamboogemini.WithAPIKey(apiKey),
            bamboogemini.WithBaseURL(baseURL),
        ), nil

    case constant.APITypeCodex:
        return bambooresponses.NewResponsesProviderWithOptions(
            bambooresponses.WithAPIKey(apiKey),
            bambooresponses.WithBaseURL(baseURL),
        ), nil

    case constant.APITypeOpenAI,
         constant.APITypeDeepSeek, constant.APITypeMoonshot,
         constant.APITypeSiliconFlow, constant.APITypeMistral,
         constant.APITypeXai, constant.APITypeZhipuV4,
         constant.APITypePerplexity, constant.APITypeCohere,
         constant.APITypeMiniMax, constant.APITypeBaiduV2,
         constant.APITypeOpenRouter, constant.APITypeXinference:
        // 全部 OpenAI Chat Completions 兼容渠道，统一走 completions Provider
        return bamboocompletions.NewCompletionsProviderWithOptions(
            bamboocompletions.WithAPIKey(apiKey),
            bamboocompletions.WithBaseURL(baseURL),
        ), nil

    default:
        // AWS/讯飞/腾讯/智谱v3/Coze/Dify 等特殊协议，bamboo 不覆盖
        return nil, ErrUnsupportedProvider
    }
}
```

### 5.3 改造点 3：4 个 Helper 改调 bridge

改造后 `ClaudeHelper`（其他 3 个对称）变成入口胶水代码，保留模型映射、系统提示注入、计费等 new-api 侧逻辑，只把三段式内核替换为 `bamboo.ChatRelay`：

```go
// relay/claude_handler.go（改造后）
func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
    info.InitChannelMeta(c)

    claudeReq, ok := info.Request.(*dto.ClaudeRequest)
    if !ok { /* ... 原校验逻辑保留 ... */ }

    request, err := common.DeepCopy(claudeReq)
    if err != nil { /* ... */ }

    // ── 以下 new-api 侧业务逻辑全部保留 ──
    helper.ModelMappedHelper(c, info, request)
    // 模型 thinking 后缀适配（claude_handler.go:55-108 的逻辑保留）
    applyClaudeThinkingAdapter(info, request)
    // 渠道系统提示注入（claude_handler.go:110-133 保留）
    applyChannelSystemPrompt(info, request)

    // ── 协议转换内核：原来是三段式，现在委托给 bamboo bridge ──
    bodyBytes, _ := common.Marshal(request)   // 入口请求体序列化

    usage, relayErr := bamboo.ChatRelay(c, info, bamboocodec.FormatAnthropic, bodyBytes)
    if relayErr != nil {
        // 未覆盖的上游（AWS/讯飞等）fallback 到原生 adaptor 链路
        if errors.Is(relayErr, bamboo.ErrUnsupportedProvider) {
            return originalClaudeRelay(c, info, request)   // 保留原始三段式作为兜底
        }
        return relayErr
    }

    service.PostTextConsumeQuota(c, info, usage, nil)
    return nil
}
```

### 5.4 改造点 4：codec FormatType ↔ RelayFormat 映射

```go
// relay/bamboo/codec_adapter.go
package bamboo

import (
    bamboocodec "github.com/bamboo-services/bamboo-messages/bamboo/codec"
    "github.com/QuantumNous/new-api/types"
)

func relayFormatToCodec(f types.RelayFormat) (bamboocodec.FormatType, bool) {
    switch f {
    case types.RelayFormatOpenAI:
        return bamboocodec.FormatOpenAI, true
    case types.RelayFormatClaude:
        return bamboocodec.FormatAnthropic, true
    case types.RelayFormatOpenAIResponses:
        return bamboocodec.FormatResponses, true
    case types.RelayFormatGemini:
        return bamboocodec.FormatGemini, true
    default:
        return "", false  // Audio/Image/Task 等不在范围内
    }
}
```

---

## 六、Fallback 策略：渐进式替换

bamboo 无法覆盖 new-api 的全部上游协议（AWS SigV4、讯飞 WebSocket、腾讯 TC3 签名、智谱 v3 JWT、Coze、Dify 等）。因此采用**渐进式替换 + 原生兜底**：

```
ChatRelay(c, info, format, body)
  ├─ newProvider(info) 成功 → 走 bamboo 中继（统一中间表示）
  └─ newProvider(info) 返回 ErrUnsupportedProvider
       └─ fallback → 调用 originalXxxRelay()（new-api 原生三段式，保留不动）
```

**好处**：
1. 旧代码不被删除，随时可回滚
2. AWS/讯飞/腾讯等渠道继续走原生链路，零影响
3. OpenAI 兼容渠道（62% 厂商）+ Anthropic/Gemini/Responses 原生渠道自动走 bamboo 中继
4. 可以按 ApiType 灰度开启：先只让 `APITypeOpenAI` 和 `APITypeDeepSeek` 走 bamboo，验证稳定后再扩

**灰度开关**（建议加到 `setting/model_setting`）：

```go
// setting/model_setting/bamboo_setting.go
type BambooSettings struct {
    EnableBambooRelay   bool   // 全局开关
    EnabledApiTypes     []int  // 启用的 ApiType 白名单（渐进式）
}
```

---

## 七、依赖与冲突分析

### 7.1 模块版本

| 依赖 | bamboo-messages | new-api | 处理 |
|------|-----------------|---------|------|
| Go | 1.25.0 | 1.25.1 | ✅ 兼容 |
| `gin-gonic/gin` | v1.11.0（经 bamboo-base-go 间接引入） | v1.9.1 | ⚠️ 需协调 |
| `bytedance/sonic` | v1.15.0 | — | ✅ 新增 |
| `tidwall/gjson` | v1.18.0 | v1.18.0 | ✅ 同版本 |

**gin 版本冲突**（唯一阻断性依赖问题）：bamboo-base-go 拉 gin v1.11.0，new-api 锁 v1.9.1。`v1.9→v1.11` 跨 v1.10 有 breaking changes。

处理选项：
- **A（推荐）**：new-api 升级 gin 到 v1.11.0，跑全量回归
- **B**：bamboo-base-go 解耦 gin 依赖（若仅用于 error 类型，改为接口抽象）

集成动工前先在 new-api 执行 `go mod tidy` + `go build ./...` 验证依赖树收敛。

### 7.2 错误体系对接

bamboo 的 codec 解析/序列化错误现在统一以 `pkgErrors.BambooError` 返回，通过 `Category` 区分错误来源（"下游" 表示请求格式/参数问题，"上游" 表示模型服务端问题）：

```go
import (
    "errors"

    pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

func translateBambooError(err error) *types.NewAPIError {
    var bambooErr *pkgErrors.BambooError
    if !errors.As(err, &bambooErr) {
        return types.NewError(err, types.ErrorCodeDoRequestFailed)
    }
    switch bambooErr.Category {
    case "下游":
        return types.NewError(err, types.ErrorCodeInvalidRequest)
    case "上游":
        return types.NewError(err, types.ErrorCodeDoRequestFailed)
    default:
        return types.NewError(err, types.ErrorCodeDoRequestFailed)
    }
}
```

### 7.3 Usage 计费精度

bamboo 的 `Usage` 仅 4 字段（`InputTokens` / `OutputTokens` / `CacheCreation` / `CacheRead`），new-api 的 `dto.Usage` 多 reasoning token 细分。

**缓解**：对支持 reasoning 的模型（o1/claude-thinking/deepseek-r1），在 bridge 层从 `StreamEvent` 的 thinking delta 累计 reasoning token，补齐到 `dto.Usage.CompletionTokensDetails.ReasoningTokens`。

---

## 八、覆盖率分析

### 8.1 bamboo 能覆盖的上游 ApiType（走统一中继）

| ApiType | 协议 | bamboo 适配器 | 代表厂商 |
|---------|------|-------------|---------|
| `APITypeOpenAI` | OpenAI Chat Completions | `openai/completions` | OpenAI/Azure/Custom |
| `APITypeDeepSeek` | OpenAI 兼容 | `openai/completions` | DeepSeek |
| `APITypeMoonshot` | OpenAI 兼容 | `openai/completions` | Kimi |
| `APITypeSiliconFlow` | OpenAI 兼容 | `openai/completions` | 硅基流动 |
| `APITypeMistral` | OpenAI 兼容 | `openai/completions` | Mistral |
| `APITypeXai` | OpenAI 兼容 | `openai/completions` | Grok |
| `APITypeZhipuV4` | OpenAI 兼容 | `openai/completions` | 智谱 GLM-4 |
| `APITypeAnthropic` | Anthropic Messages | `anthropic` | Claude |
| `APITypeGemini` | Google Gemini | `gemini` | Gemini |
| `APITypeCodex` | OpenAI Responses | `openai/responses` | Codex |
| + Perplexity/Cohere/MiniMax/BaiduV2/OpenRouter/Xinference 等 | OpenAI 兼容 | `openai/completions` | — |

**约 24+ 个 ApiType 走 bamboo 统一中继**，覆盖 new-api 对话类渠道的 **~70%**。

### 8.2 保留原生链路的上游 ApiType（fallback）

| ApiType | 原因 |
|---------|------|
| `APITypeAws` | AWS SigV4 签名，bamboo 无对应适配器 |
| `APITypeXunfei` | WebSocket 协议 |
| `APITypeTencent` | TC3-HMAC 自定义签名 |
| `APITypeZhipu` | 智谱 v3 JWT + 自有 URL |
| `APITypeCoze` / `APITypeDify` | 自有协议 |
| `APITypeBaidu` | 千帆 v1 OAuth |
| `APITypeAli` | DashScope 自有端点 |
| `APITypeVertexAi` | Google OAuth service-account |

这些保持 new-api 原生 adaptor 不动，通过 `ErrUnsupportedProvider` fallback。

### 8.3 不在本次范围的 RelayMode

以下 Helper **完全保留**，不受本次改造影响：
- `EmbeddingHelper` / `GeminiEmbeddingHandler` — 向量化
- `RerankHelper` — 重排序
- `AudioHelper` — TTS / Whisper
- `ImageHelper` — DALL-E / 图像编辑
- `WssHelper` — Realtime 语音
- `RelayTaskSubmit` — MJ/Suno/Kling/Sora 异步任务

---

## 九、实施路线图

### Phase 1：bridge 基础设施（1 周）

1. `relay/bamboo/` 包骨架（5 文件）
2. `provider_factory.go` 实现，覆盖 `APITypeOpenAI` + `APITypeDeepSeek`
3. `bridge.go` 的 `ChatRelay` + `doStreamRelay` + `doCompleteRelay`
4. `codec_adapter.go` 的 FormatType 映射
5. 依赖冲突解决（gin 版本）
6. 单元测试：`relay/bamboo/*_test.go`

**验收**：`ChatRelay` 能以 OpenAI 入口格式调通 DeepSeek，流式 + 非流式。

### Phase 2：接入 TextHelper（1 周）

1. `compatible_handler.go` 的三段式替换为 `bamboo.ChatRelay`
2. 保留 `ModelMappedHelper` / 系统提示注入 / `StreamOptions` 等 new-api 侧逻辑
3. 实现 `ErrUnsupportedProvider` fallback 到 `originalTextRelay`
4. 灰度开关：`BambooSettings.EnabledApiTypes` 白名单
5. 集成测试：OpenAI 入口 → DeepSeek/Moonshot/SiliconFlow 上游

**验收**：OpenAI 格式入口端到端调通，计费准确，未覆盖渠道正确 fallback。

### Phase 3：接入其余 3 个 Helper（1 周）

1. `ClaudeHelper` 接入（保留 thinking 后缀适配逻辑）
2. `GeminiHelper` 接入（保留 thinking budget 适配逻辑）
3. `ResponsesHelper` 接入（保留 Responses→Chat fallback 逻辑）
4. 扩大灰度白名单到 Anthropic/Gemini/Codex

**验收**：Claude 入口打 DeepSeek 上游、Gemini 入口打 OpenAI 上游等跨协议场景调通。

### Phase 4：稳定性与边缘补齐（持续）

1. reasoning token 计费补齐
2. `ParamOverride` / `HeadersOverride` 透传到 `ProviderExtra`
3. panic recovery + goroutine 泄漏检查
4. 压测（并发流式连接）
5. 全 ApiType 灰度开启，观察 1-2 周后删除 fallback 代码（可选）

---

## 十、风险登记册

| 风险 | 概率 | 影响 | 缓解 |
|------|:---:|:---:|------|
| gin 版本冲突编译失败 | 高 | 高 | 集成前 `go mod tidy` + `go build` 验证；必要时 bamboo-base-go 解耦 |
| 流式 bridge goroutine 泄漏 | 中 | 高 | `Chat()` channel 配合 `ctx.Done()`；pprof 泄漏测试 |
| bamboo codec 解析不完整（如 OpenAI 的 `response_format`/`logit_bias`） | 中 | 中 | Phase 2 逐字段比对 DTO，缺失字段走 `ProviderExtra` 透传 |
| usage 统计缺 reasoning token | 中 | 中 | Phase 4 补齐；短期接受计费偏差 |
| fallback 路径与 bamboo 路径行为不一致（如错误码、响应头） | 中 | 中 | 对齐测试：同请求走两条路径，比对响应 |
| bamboo Provider 的 UserAgent/Header 注入与 new-api 预期不符 | 低 | 低 | `WithHeader` 覆盖；对齐 `User-Agent: BM-SDK/{version}` 约定 |
| Realtime/Task 等 RelayMode 误入 bamboo 路径 | 低 | 高 | `relayFormatToCodec` 对非对话格式返回 false，Helper 不调 bridge |

---

## 十一、验收检查清单

- [ ] `go build ./...` 在 new-api 目录无编译错误
- [ ] `go test ./relay/bamboo/...` 单元测试通过
- [ ] OpenAI 入口 → DeepSeek 上游：流式 + 非流式 + 工具调用调通
- [ ] Claude 入口 → DeepSeek 上游（跨协议）调通
- [ ] Gemini 入口 → OpenAI 上游（跨协议）调通
- [ ] 未覆盖渠道（AWS/讯飞/腾讯）正确 fallback 到原生链路
- [ ] `usage.PromptTokens` / `CompletionTokens` / `TotalTokens` 统计正确
- [ ] 客户端断开后无 goroutine 泄漏（pprof 验证）
- [ ] 灰度开关生效：白名单外 ApiType 走 fallback
- [ ] 计费 `PostTextConsumeQuota` 金额与改造前一致（同请求对比）

---

## 十二、附录

### 12.1 bamboo-messages 关键文件索引

| 文件 | 作用 |
|------|------|
| `bamboo/codec/codec.go:26` | `Codec` 接口（ParseRequest / SerializeResponse / NewSerializer） |
| `bamboo/codec/types.go:9` | `RelayRequest` 统一中间表示 |
| `bamboo/codec/registry.go:27` | `Get(format)` Codec 工厂 |
| `bamboo/codec/{openai,anthropic,responses,gemini}/` | 4 种协议的 Codec 实现 |
| `bamboo/bamboo.go:19` | `BambooClient` 接口（Chat / Complete） |
| `bamboo/stream.go` | `StreamEvent` 流事件模型（Anthropic 风格） |
| `bamboo/convert.go:225` | `StreamConverter` 防御性 BlockStart 补发 |
| `internal/provider/provider.go:23` | `Provider` 核心接口（6 方法） |
| `internal/provider/anthropic/provider.go:77` | Anthropic 适配器构造（`WithBaseURL`） |
| `internal/provider/openai/completions/provider.go:75` | Completions 适配器构造 |
| `internal/provider/openai/responses/provider.go:78` | Responses 适配器构造 |
| `internal/provider/gemini/provider.go:80` | Gemini 适配器构造 |

### 12.2 new-api 关键文件索引

| 文件 | 作用 |
|------|------|
| `controller/relay.go:35` | `relayHandler`（对话 RelayMode 分派入口） |
| `controller/relay.go:68` | `Relay()` 主链路（计费/重试/分派） |
| `relay/compatible_handler.go:25` | `TextHelper`（OpenAI 入口）← 替换目标 |
| `relay/claude_handler.go:24` | `ClaudeHelper`（Claude 入口）← 替换目标 |
| `relay/gemini_handler.go:54` | `GeminiHelper`（Gemini 入口）← 替换目标 |
| `relay/responses_handler.go:25` | `ResponsesHelper`（Responses 入口）← 替换目标 |
| `relay/channel/adapter.go:15` | `Adaptor` 接口（4 个 Convert 方法） |
| `relay/relay_adaptor.go:53` | `GetAdaptor` 工厂 |
| `relay/common/relay_info.go:99` | `RelayInfo` 上下文 |
| `constant/api_type.go:3` | `APIType` 枚举 |
| `types/relay_format.go:5` | `RelayFormat` 枚举（入口协议） |
| `dto/openai_request.go:29` | `GeneralOpenAIRequest` 标准 DTO |
| `dto/openai_response.go:223` | `dto.Usage` 计费结构 |

---

*文档结束。本方案的核心价值：用 bamboo 的协议无关中间表示，将 new-api 对话中继的 N×M 协议转换矩阵降为 N+M，达成任意入口 ↔ 任意上游的自由格式转换。*
