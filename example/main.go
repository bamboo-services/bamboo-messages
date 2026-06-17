// Package main 提供 Bamboo Messages SDK 的完整使用示例。
//
// 包含四个独立演示：
//   - demoStreaming:   多轮流式对话（2 轮）
//   - demoNonStreaming: 非流式单轮请求
//   - demoThinking:    启用 ThinkingConfig 的对话
//   - demoToolCalling: 工具调用完整闭环
//
// 环境变量：
//
//	BAMBOO_API_KEY    — API 密钥（必填）
//	BAMBOO_PROVIDER   — Provider 类型: anthropic / openai / openai-responses（默认 anthropic）
//	BAMBOO_BASE_URL   — 自定义端点（可选）
//	BAMBOO_MODEL      — 模型名称（默认 claude-sonnet-4-20250514）
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/provider"
	anthropicPkg "github.com/bamboo-services/bamboo-messages/provider/anthropic"
	completionsPkg "github.com/bamboo-services/bamboo-messages/provider/openai/completions"
	responsesPkg "github.com/bamboo-services/bamboo-messages/provider/openai/responses"
)

// createProvider 根据环境变量创建对应的 Provider 实例。
func createProvider() (provider.Provider, error) {
	apiKey := os.Getenv("BAMBOO_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("环境变量 BAMBOO_API_KEY 未设置")
	}

	providerType := os.Getenv("BAMBOO_PROVIDER")
	if providerType == "" {
		providerType = "anthropic"
	}

	baseURL := os.Getenv("BAMBOO_BASE_URL")

	switch providerType {
	case "anthropic":
		opts := []anthropicPkg.Option{anthropicPkg.WithAPIKey(apiKey)}
		if baseURL != "" {
			opts = append(opts, anthropicPkg.WithBaseURL(baseURL))
		}
		return anthropicPkg.NewProviderWithOptions(opts...), nil

	case "openai":
		opts := []completionsPkg.Option{completionsPkg.WithAPIKey(apiKey)}
		if baseURL != "" {
			opts = append(opts, completionsPkg.WithBaseURL(baseURL))
		}
		return completionsPkg.NewCompletionsProviderWithOptions(opts...), nil

	case "openai-responses":
		opts := []responsesPkg.Option{responsesPkg.WithAPIKey(apiKey)}
		if baseURL != "" {
			opts = append(opts, responsesPkg.WithBaseURL(baseURL))
		}
		return responsesPkg.NewResponsesProviderWithOptions(opts...), nil

	default:
		return nil, fmt.Errorf("不支持的 BAMBOO_PROVIDER: %q（可选: anthropic / openai / openai-responses）", providerType)
	}
}

// ──────────────────────────────────────────────────────────────────────
// 演示 1: 多轮流式对话（2 轮）
// ──────────────────────────────────────────────────────────────────────

func demoStreaming(ctx context.Context, client bamboo.BambooClient, model string) error {
	fmt.Println("🔹 演示 1: 多轮流式对话")
	fmt.Println()

	systemPrompt := "你是一个有帮助的助手，请用中文回答。"

	// ── 第 1 轮 ──
	fmt.Println("📝 第 1 轮对话:")
	fmt.Println("👤 用户: 你好，请介绍一下你自己")

	messages := []bamboo.BambooMessage{
		bamboo.NewUserMessage("你好，请介绍一下你自己"),
	}

	config := &bamboo.RequestConfig{
		Model:       model,
		MaxTokens:   1024,
		Temperature: bamboo.PtrFloat64(0.7),
	}

	eventCh, err := client.Chat(ctx, messages, systemPrompt, config)
	if err != nil {
		return fmt.Errorf("第 1 轮 Chat 失败: %w", err)
	}

	var assistantText string
	fmt.Print("🤖 助手: ")
	for event := range eventCh {
		switch event.Type {
		case bamboo.EventMessageStart:
			fmt.Print("🟢 ")
		case bamboo.EventContentBlockStart:
			fmt.Print("📦")
		case bamboo.EventContentBlockDelta:
			if delta, ok := event.Delta.(*bamboo.StreamDelta); ok {
				switch delta.Type {
				case bamboo.DeltaTextDelta:
					fmt.Print(delta.Text)
					assistantText += delta.Text
				case bamboo.DeltaThinkingDelta:
					fmt.Printf("💭%s", delta.Thinking)
				}
			}
		case bamboo.EventContentBlockStop:
			// 内容块结束
		case bamboo.EventMessageDelta:
			// 消息增量
		case bamboo.EventMessageStop:
			fmt.Print(" 🔴")
		case bamboo.EventError:
			if event.Error != nil {
				return fmt.Errorf("流式错误: %s", event.Error.Error())
			}
		}
	}
	fmt.Println()
	fmt.Println()

	// ── 第 2 轮：基于第 1 轮继续对话 ──
	fmt.Println("📝 第 2 轮对话（多轮上下文）:")
	fmt.Println("👤 用户: 你能详细说说吗？")

	messages = append(messages,
		bamboo.NewAssistantMessage(assistantText),
		bamboo.NewUserMessage("你能详细说说吗？"),
	)

	eventCh, err = client.Chat(ctx, messages, systemPrompt, config)
	if err != nil {
		return fmt.Errorf("第 2 轮 Chat 失败: %w", err)
	}

	fmt.Print("🤖 助手: ")
	for event := range eventCh {
		switch event.Type {
		case bamboo.EventContentBlockDelta:
			if delta, ok := event.Delta.(*bamboo.StreamDelta); ok {
				if delta.Type == bamboo.DeltaTextDelta {
					fmt.Print(delta.Text)
				}
			}
		case bamboo.EventError:
			if event.Error != nil {
				return fmt.Errorf("流式错误: %s", event.Error.Error())
			}
		}
	}
	fmt.Println()
	return nil
}

// ──────────────────────────────────────────────────────────────────────
// 演示 2: 非流式单轮请求
// ──────────────────────────────────────────────────────────────────────

func demoNonStreaming(ctx context.Context, client bamboo.BambooClient, model string) error {
	fmt.Println("🔹 演示 2: 非流式单轮请求")
	fmt.Println()

	messages := []bamboo.BambooMessage{
		bamboo.NewUserMessage("什么是量子计算？用一句话概括。"),
	}

	config := &bamboo.RequestConfig{
		Model:       model,
		MaxTokens:   512,
		Temperature: bamboo.PtrFloat64(0.7),
	}

	resp, err := client.Complete(ctx, messages, "你是一个有帮助的助手。", config)
	if err != nil {
		return fmt.Errorf("Complete 失败: %w", err)
	}

	fmt.Println("📋 响应详情:")
	fmt.Printf("  ID:           %s\n", resp.ID)
	fmt.Printf("  StopReason:   %s\n", resp.StopReason)
	fmt.Printf("  Usage:        input=%d, output=%d\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	fmt.Printf("  ProviderType: %s\n", resp.ProviderType)
	fmt.Printf("  RequestID:    %s\n", resp.RequestID)
	fmt.Println()

	fmt.Println("📦 内容块:")
	for i, block := range resp.Content {
		switch b := block.(type) {
		case *bamboo.TextBlock:
			fmt.Printf("  [%d] 📝 text: %s\n", i, b.Text)
		case *bamboo.ThinkingBlock:
			fmt.Printf("  [%d] 💭 thinking: %s\n", i, b.Thinking)
		case *bamboo.ToolUseBlock:
			fmt.Printf("  [%d] 🔧 tool_use: name=%s, id=%s\n", i, b.Name, b.ID)
		}
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────
// 演示 3: 启用 ThinkingConfig
// ──────────────────────────────────────────────────────────────────────

func demoThinking(ctx context.Context, client bamboo.BambooClient, model string) error {
	fmt.Println("🔹 演示 3: ThinkingConfig 思考模式")
	fmt.Println()

	messages := []bamboo.BambooMessage{
		bamboo.NewUserMessage("请分析一下人工智能的未来发展趋势。"),
	}

	thinkingCfg := &bamboo.ThinkingConfig{
		Effort: "high",
	}

	config := &bamboo.RequestConfig{
		Model:          model,
		MaxTokens:      4096,
		Temperature:    bamboo.PtrFloat64(0.7),
		ThinkingConfig: thinkingCfg,
	}

	resp, err := client.Complete(ctx, messages, "你是一个有帮助的助手，请用中文回答。", config)
	if err != nil {
		return fmt.Errorf("Complete (Thinking) 失败: %w", err)
	}

	fmt.Println("🧠 思考模式响应:")
	fmt.Printf("  StopReason: %s\n", resp.StopReason)
	fmt.Printf("  Usage:      input=%d, output=%d\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	fmt.Println()

	for i, block := range resp.Content {
		switch b := block.(type) {
		case *bamboo.ThinkingBlock:
			fmt.Printf("  [%d] 💭 思考过程:\n%s\n\n", i, b.Thinking)
		case *bamboo.TextBlock:
			fmt.Printf("  [%d] 📝 回复:\n%s\n", i, b.Text)
		}
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────
// 演示 4: 工具调用完整闭环
// ──────────────────────────────────────────────────────────────────────

func demoToolCalling(ctx context.Context, client bamboo.BambooClient, model string) error {
	fmt.Println("🔹 演示 4: 工具调用完整闭环")
	fmt.Println()

	// 定义 get_weather 工具
	weatherTool := bamboo.Tool{
		Name:        "get_weather",
		Description: "获取指定城市的天气信息",
		InputSchema: bamboo.InputSchema{
			Type: "object",
			Properties: map[string]bamboo.PropertyDef{
				"city": {
					Type:        "string",
					Description: "城市名称",
				},
			},
			Required: []string{"city"},
		},
	}

	messages := []bamboo.BambooMessage{
		bamboo.NewUserMessage("今天北京天气怎么样？"),
	}

	config := &bamboo.RequestConfig{
		Model:       model,
		MaxTokens:   1024,
		Temperature: bamboo.PtrFloat64(0.7),
		Tools:       []bamboo.Tool{weatherTool},
	}

	systemPrompt := "你是一个有帮助的助手，可以使用提供的工具来回答问题。"

	// ── Step 1: 发送请求 ──
	fmt.Println("📤 Step 1: 发送带工具的请求")
	resp, err := client.Complete(ctx, messages, systemPrompt, config)
	if err != nil {
		return fmt.Errorf("Complete 失败: %w", err)
	}

	fmt.Printf("  StopReason: %s\n", resp.StopReason)

	// ── Step 2: 检查是否触发工具调用 ──
	if resp.StopReason != bamboo.FinishReasonToolUse {
		fmt.Println("  ⚠️ AI 未请求工具调用，直接回复:")
		for _, block := range resp.Content {
			if b, ok := block.(*bamboo.TextBlock); ok {
				fmt.Printf("  📝 %s\n", b.Text)
			}
		}
		return nil
	}

	// 提取 tool_use 内容块
	var toolUseBlocks []bamboo.ContentBlock
	for _, block := range resp.Content {
		if _, ok := block.(*bamboo.ToolUseBlock); ok {
			toolUseBlocks = append(toolUseBlocks, block)
		}
	}

	fmt.Printf("  🔧 AI 请求调用 %d 个工具\n", len(toolUseBlocks))

	// ── Step 3: 模拟工具执行 ──
	fmt.Println()
	fmt.Println("⚙️  Step 2: 模拟工具执行")
	var toolResults []bamboo.ContentBlock
	for _, block := range toolUseBlocks {
		toolUseBlock, ok := block.(*bamboo.ToolUseBlock)
		if !ok {
			continue
		}
		var input struct {
			City string `json:"city"`
		}
		if err := json.Unmarshal(toolUseBlock.Input, &input); err != nil {
			return fmt.Errorf("解析工具参数失败: %w", err)
		}
		fmt.Printf("  🔧 调用 %s(%s)\n", toolUseBlock.Name, input.City)

		// 硬编码模拟结果
		result := "北京: 晴, 25°C"
		fmt.Printf("  📊 返回: %s\n", result)
		toolResults = append(toolResults, bamboo.NewToolResultBlock(toolUseBlock.ID, result, false))
	}

	// ── Step 4: 发送工具结果，获取 AI 最终回复 ──
	fmt.Println()
	fmt.Println("📤 Step 3: 发送工具结果，获取 AI 最终回复")
	messages = append(messages,
		bamboo.NewAssistantMessageBlocks(toolUseBlocks...),
		bamboo.NewUserMessageBlocks(toolResults...),
	)

	resp, err = client.Complete(ctx, messages, systemPrompt, config)
	if err != nil {
		return fmt.Errorf("最终 Complete 失败: %w", err)
	}

	fmt.Println("✅ 工具调用闭环完成!")
	for _, block := range resp.Content {
		if b, ok := block.(*bamboo.TextBlock); ok {
			fmt.Printf("  📝 AI: %s\n", b.Text)
		}
	}
	fmt.Printf("  📊 Token: input=%d, output=%d\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	return nil
}

// ──────────────────────────────────────────────────────────────────────
// main
// ──────────────────────────────────────────────────────────────────────

func main() {
	fmt.Println("🌿 Bamboo Messages SDK 示例演示")
	fmt.Println()

	// 读取模型配置
	model := os.Getenv("BAMBOO_MODEL")
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	providerName := os.Getenv("BAMBOO_PROVIDER")
	if providerName == "" {
		providerName = "anthropic"
	}

	fmt.Printf("📋 配置: provider=%s, model=%s\n", providerName, model)
	fmt.Println()

	// 创建 Provider
	p, err := createProvider()
	if err != nil {
		log.Fatalf("❌ 创建 Provider 失败: %v", err)
	}

	// 创建 SDK 客户端
	client := bamboo.NewClient(p)
	ctx := context.Background()

	separator := "════════════════════════════════════════"

	// 演示 1: 流式对话
	fmt.Println(separator)
	if err := demoStreaming(ctx, client, model); err != nil {
		log.Fatalf("❌ demoStreaming 失败: %v", err)
	}
	fmt.Println()

	// 演示 2: 非流式
	fmt.Println(separator)
	if err := demoNonStreaming(ctx, client, model); err != nil {
		log.Fatalf("❌ demoNonStreaming 失败: %v", err)
	}
	fmt.Println()

	// 演示 3: ThinkingConfig
	fmt.Println(separator)
	if err := demoThinking(ctx, client, model); err != nil {
		log.Fatalf("❌ demoThinking 失败: %v", err)
	}
	fmt.Println()

	// 演示 4: 工具调用
	fmt.Println(separator)
	if err := demoToolCalling(ctx, client, model); err != nil {
		log.Fatalf("❌ demoToolCalling 失败: %v", err)
	}

	fmt.Println()
	fmt.Println(separator)
	fmt.Println("✅ 所有演示完成!")
}
