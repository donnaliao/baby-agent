package ch01

// =============================================================================
// SDK 方式调用 OpenAI API
// 对比 raw.go：raw.go 用原生 http.Client 直接调 API，
// sdk.go 用 openai-go 库（官方 SDK），代码更简洁，但需要额外依赖
// 相当于：自己写 JDBC vs 用 Spring Data JPA
// =============================================================================

import (
	"context"  // 上下文，用于传递取消信号和超时
	"log"      // 日志打印

	"github.com/openai/openai-go/v3"          // OpenAI 官方 SDK（相当于 Java 的 OpenAI SDK）
	"github.com/openai/openai-go/v3/option"   // SDK 的选项模式（Builder 模式）

	"babyagent/shared" // 共享配置
)

// NonStreamingRequestSDK - 非流式调用（使用官方 SDK）
// 对比 raw.go 的 NonStreamingRequestRawHTTP：
//   raw.go: 自己构建 JSON、手动处理 HTTP、手动反序列化
//   sdk.go: 用 SDK 提供的结构体和方法，代码更简洁
//
// Go 的优点：代码简洁，类型安全
// 缺点：引入外部依赖
func NonStreamingRequestSDK(ctx context.Context, modelConf shared.ModelConfig, query string) {
	// 创建 OpenAI 客户端
	// option.WithBaseURL 和 option.WithAPIKey 是函数式选项模式（类似 Builder）
	// 相当于 Java 的 OpenAI client = OpenAI.builder().baseUrl(url).apiKey(key).build()
	client := openai.NewClient(
		option.WithBaseURL(modelConf.BaseURL), // API 基础 URL
		option.WithAPIKey(modelConf.ApiKey),   // API Key
	)

	// 构建请求参数
	// Go 的结构体初始化可以只填需要的字段，其他字段用零值
	// 相当于 Java 的 ChatCompletionRequest request = ChatCompletionRequest.builder()
	//     .messages(List.of(new UserMessage(query)))
	//     .model(model)
	//     .build()
	req := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(query), // 创建用户消息，类似 Java 的 UserMessage.of(query)
		},
		Model: modelConf.Model,
	}

	// 发送请求
	// client.Chat.Completions.New 相当于 Java 的 client.chat().completions().create(request)
	// Go 的方法返回 (result, error)，Java 是 throws Exception
	resp, err := client.Chat.Completions.New(ctx, req)
	if err != nil {
		log.Fatalf("failed to send a new completion request: %v", err)
		return
	}

	// 检查返回结果
	if len(resp.Choices) == 0 {
		// resp.RawJSON() 返回原始 JSON 字符串，用于调试
		log.Printf("no choices returned, resp: %s", resp.RawJSON())
		return
	}

	// 打印回复内容
	// resp.Choices[0].Message.Content 相当于 Java 的 resp.getChoices().get(0).getMessage().getContent()
	log.Printf("resp content: %s", resp.Choices[0].Message.Content)
	log.Printf("token usage: %s", resp.Usage.RawJSON())
}

// StreamingRequestSDK - 流式调用（使用官方 SDK）
// 对比 raw.go 的 StreamingRequestRawHTTP：
//   raw.go: 用 bufio.Scanner 逐行读取，手动解析 SSE 格式
//   sdk.go: 用 SDK 的 NewStreaming()，自动处理流，直接遍历即可
//
// SDK 的 Streaming 模式封装了 SSE 解析，开发者只需要调用 .Next() 遍历即可
func StreamingRequestSDK(ctx context.Context, modelConf shared.ModelConfig, query string) {
	// 创建客户端（同上）
	client := openai.NewClient(
		option.WithBaseURL(modelConf.BaseURL),
		option.WithAPIKey(modelConf.ApiKey),
	)

	// 构建请求参数
	req := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(query),
		},
		Model: modelConf.Model,
	}

	// 创建流式响应对象
	// .NewStreaming() 返回一个流迭代器，类似 Java 的 StreamingResponse
	stream := client.Chat.Completions.NewStreaming(ctx, req)

	// 遍历流数据
	// for stream.Next() {} 类似 Java 的 while (stream.hasNext())
	// Go 的 for 是唯一循环语句，没有 while
	for stream.Next() {
		// 获取当前的数据块
		chunk := stream.Current()
		log.Printf("stream chunk: %s", chunk.RawJSON())

		// 检查 token 使用量（最后一个 chunk 才有完整统计）
		if chunk.Usage.TotalTokens != 0 {
			log.Printf("token usage: %s", chunk.Usage.RawJSON())
		}
	}

	// 检查流式请求是否有错误
	// 相当于 Java 的 while 循环后的 if (stream.hasError())
	if stream.Err() != nil {
		log.Fatalf("stream error: %v", stream.Err())
		return
	}
}

// =============================================================================
// 对比总结
// =============================================================================
//                    raw.go (原生 HTTP)          sdk.go (官方 SDK)
// -----------------------------------------------------------------------------
// 代码量            多（手写 JSON 序列化）        少（用 SDK 结构体）
// 依赖              无外部依赖                    需要 go mod 引入
// 灵活性            高（完全可控）                中（受 SDK 限制）
// 流式处理          手动解析 SSE                  自动封装
// 类型安全          低（手写字段可能错）           高（编译期检查）
// -----------------------------------------------------------------------------
// 建议：学习时先看 raw.go 理解原理，生产环境用 sdk.go
// =============================================================================