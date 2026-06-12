package ch01

// =============================================================================
// Go 与 C++/Java 的主要区别（先了解这些，写代码更顺手）
// =============================================================================
// 1. 结构体（struct）类似 Java 的类，但只是纯数据结构，没有方法（方法用函数接收者实现）
// 2. 切片（slice）类似 Java 的 ArrayList，是动态数组
// 3. map 相当于 Java 的 HashMap
// 4. defer 相当于 try-finally，函数结束前自动执行，常用于关闭资源
// 5. _ 匿名变量，类似 Java 的 var，但不占用变量名
// 6. := 表示声明并赋值（类型由编译器推断），类似 Java 的 var
// =============================================================================

import (
	"bufio"       // 缓冲区扫描器，用于逐行读取流数据（C++ 的 iostream，Java 的 BufferedReader）
	"bytes"       // 字节操作工具（C++ 的 stringstream，Java 的 ByteArrayOutputStream）
	"context"     // Go 的上下文机制，用于传递取消信号和超时（C++ 没有，Java 的 ExecutorService.submit）
	"encoding/json" // JSON 序列化/反序列化
	"fmt"         // 格式化输出（类似 C++ 的 printf，Java 的 String.format）
	"io"          // I/O 操作工具
	"log"         // 日志打印（类似 Java 的 Logger）
	"net/http"    // HTTP 客户端/服务端（类似 Java 的 HttpClient）
	"strings"     // 字符串操作工具（类似 Java 的 StringUtils）

	"babyagent/shared"  // 导入 shared 包（类似 C++ 的 #include 或 Java 的 import）
)

// =============================================================================
// 数据结构定义（类似 C++ 的 struct 或 Java 的 POJO）
// Go 的 struct 标签（json:"role"）用于 JSON 序列化/反序列化时的字段映射
// =============================================================================

// RequestMessage - 发送给 LLM 的消息结构
// 相当于 Java 的 class RequestMessage { String role; String content; }
type RequestMessage struct {
	Role    string `json:"role"`    // 角色：user/assistant/system
	Content string `json:"content"` // 消息内容
}

// ResponseMessage - LLM 返回的消息结构
type ResponseMessage struct {
	Role    string `json:"role"`    // 回复的角色
	Content string `json:"content"` // 回复的内容

	FinishReason string `json:"finish_reason"` // 结束原因：stop/tool_calls

	// reasoning_content 和 reasoning 是不同模型厂商的兼容字段
	// *string 表示指针类型，类似 Java 的 String（引用类型）
	// 不同厂商可能返回不同的字段名，所以都定义出来，JSON 反序列化时只会填充有值的字段
	ReasoningContent *string `json:"reasoning_content"` // OpenAI 等厂商使用
	Reasoning        *string `json:"reasoning"`         // 其他厂商可能使用
}

// Usage - Token 使用统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`     // 消耗的输入 token 数
	CompletionTokens int `json:"completion_tokens"` // 生成的输出 token 数
	TotalTokens      int `json:"total_tokens"`      // 总 token 数
}

// OpenAIChatCompletionResponse - 非流式响应的完整结构
// 对应 OpenAI Chat Completions API 的完整 JSON 响应
type OpenAIChatCompletionResponse struct {
	Choices []struct {    // Choices 是切片，类似 Java 的 List<Choice>
		Message ResponseMessage `json:"message"` // 每个 choice 包含一个消息
	} `json:"choices"`
	Usage *Usage `json:"usage,omitempty"` // omitempty 表示空值时不序列化
}

// OpenAIChatCompletionStreamChunk - 流式响应的单个数据块结构
// SSE 流式响应中，每行 "data: {...}" 对应一个 Chunk
type OpenAIChatCompletionStreamChunk struct {
	Choices []struct {
		Delta ResponseMessage `json:"delta"` // 流式增量内容
	} `json:"choices"`
	Usage *Usage `json:"usage,omitempty"`
}

// OpenAIChatCompletionRequest - 发送给 OpenAI API 的请求结构
type OpenAIChatCompletionRequest struct {
	Model    string           `json:"model"`    // 模型名称，如 gpt-4o
	Messages []RequestMessage `json:"messages"` // 消息列表
	Stream   bool             `json:"stream"`   // 是否开启流式响应
}

// =============================================================================
// 函数定义
// func 函数名(参数列表) 返回值类型 { }
// Go 的函数可以直接返回多个值，类似 C++ 的 pair，但更直接
// =============================================================================

// NonStreamingRequestRawHTTP - 非流式调用（直接使用原生 HTTP，不依赖 SDK）
// 流程：C++/Java 发送 HTTP POST → 等待完整响应 → 返回全部内容
//
// @param ctx 上下文，用于控制请求超时和取消（类似 Java 的 ExecutorService 的 Context）
// @param modelConf 模型配置（URL、API Key、模型名）
// @param query 用户查询内容
func NonStreamingRequestRawHTTP(ctx context.Context, modelConf shared.ModelConfig, query string) {
	client := http.Client{} // 创建 HTTP 客户端，类似 Java 的 HttpClient.newHttpClient()

	// 构建请求体 - 类似于 Java 构建 JSON 请求体
	requestBody := OpenAIChatCompletionRequest{
		Messages: []RequestMessage{
			{Role: "user", Content: query}, // 用户消息
		},
		Model:  modelConf.Model, // 模型名
		Stream: false,           // 非流式
	}

	// json.Marshal 将结构体序列化为 JSON 字节数组
	// 相当于 Java 的 ObjectMapper.writeValueAsBytes(requestBody)
	bodyBytes, _ := json.Marshal(requestBody)

	// 创建 HTTP 请求
	// http.NewRequestWithContext 类似 Java 的 HttpRequest.newBuilder()
	// bytes.NewReader(bodyBytes) 将字节数组转为 Reader，类似 Java 的 ByteArrayInputStream
	httpReq, _ := http.NewRequestWithContext(
		ctx,                                        // 上下文（可取消/超时）
		"POST",                                     // HTTP 方法
		fmt.Sprintf("%s/chat/completions", modelConf.BaseURL), // URL
		bytes.NewReader(bodyBytes),                 // 请求体
	)

	// 设置请求头
	// 相当于 Java 的 HttpRequest.header("Content-Type", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+modelConf.ApiKey)

	// 发送请求并获取响应
	// 相当于 Java 的 client.send(request, HttpResponse.BodyHandlers.ofString())
	httpResp, err := client.Do(httpReq)
	if err != nil {
		log.Fatalf("failed to send http request: %v", err) // 致命错误，终止程序
		return
	}

	// defer 确保函数返回前执行，常用于关闭资源
	// 相当于 Java 的 try-with-resources 或 finally { body.close(); }
	defer httpResp.Body.Close()

	// 检查 HTTP 状态码
	if httpResp.StatusCode != 200 {
		log.Fatalf("failed to send http request: %v", httpResp.StatusCode)
		return
	}

	// 读取响应体
	// io.ReadAll 类似 Java 的 response.body().readAllBytes()
	respBodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		log.Fatalf("failed to read http response: %v", err)
		return
	}

	// 反序列化 JSON 到结构体
	// 相当于 Java 的 ObjectMapper.readValue(respBodyBytes, OpenAIChatCompletionResponse.class)
	resp := OpenAIChatCompletionResponse{}
	if err := json.Unmarshal(respBodyBytes, &resp); err != nil {
		log.Fatalf("failed to unmarshal http response: %v", err)
		return
	}

	// 检查返回内容
	if len(resp.Choices) == 0 {
		log.Printf("no choices returned, resp: %v", resp)
		return
	}

	// 打印结果
	log.Printf("resp content: %s", resp.Choices[0].Message.Content)
	log.Printf("token usage: %+v", resp.Usage)
}

// StreamingRequestRawHTTP - 流式调用（使用原生 HTTP）
// 流程：C++/Java 发送 HTTP POST → 持续接收数据块 → 逐步处理
// 适用于需要实时显示 LLM 回复的场景（如打字机效果）
//
// SSE（Server-Sent Events）格式：
//   data: {"choices":[{"delta":{"content":"Hello"}}]}
//   data: {"choices":[{"delta":{"content":" world"}}]}
//   data: [DONE]
func StreamingRequestRawHTTP(ctx context.Context, modelConf shared.ModelConfig, query string) {
	client := http.Client{}

	// 构建请求体，Stream 设置为 true 表示流式响应
	requestBody := OpenAIChatCompletionRequest{
		Messages: []RequestMessage{
			{Role: "user", Content: query},
		},
		Model:  modelConf.Model,
		Stream: true, // 关键：开启流式
	}
	bodyBytes, _ := json.Marshal(requestBody)

	// 创建 HTTP POST 请求
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", modelConf.BaseURL), bytes.NewReader(bodyBytes))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+modelConf.ApiKey)

	// 发送请求
	httpResp, err := client.Do(httpReq)
	if err != nil {
		log.Fatalf("failed to send http request: %v", err)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		log.Fatalf("failed to send http request: %v", httpResp.StatusCode)
		return
	}

	// bufio.Scanner 用于逐行读取流数据
	// 类似于 Java 的 BufferedReader.readLine()，但更安全（自动处理缓冲区）
	scanner := bufio.NewScanner(httpResp.Body)

	// 循环读取每一行
	// scanner.Scan() 返回 true 表示还有数据，false 表示结束
	for scanner.Scan() {
		line := scanner.Text() // 获取当前行的文本

		// 跳过空行
		if line == "" {
			continue
		}

		// SSE 格式中，每行数据以 "data:" 开头
		// strings.HasPrefix 类似 Java 的 String.startsWith()
		if strings.HasPrefix(line, "data:") {
			// strings.TrimPrefix 去掉 "data:" 前缀，获取实际 JSON
			// "data: {\"choices\":[...]}" → " {\"choices\":[...]}"
			v := strings.TrimPrefix(line, "data:")

			// SSE 结束标记
			// strings.TrimSpace 去掉首尾空白，类似 Java 的 String.trim()
			if strings.TrimSpace(v) == "[DONE]" {
				break // 跳出循环
			}

			// 解析单个数据块
			chunk := OpenAIChatCompletionStreamChunk{}
			if err := json.Unmarshal([]byte(v), &chunk); err != nil {
				log.Fatalf("failed to unmarshal chunk: %v", err)
				return
			}

			log.Printf("stream chunk: %s", v)
			if chunk.Usage != nil {
				log.Printf("token usage: %+v", chunk.Usage)
			}
		}
	}

	// 检查扫描错误
	// 相当于 Java 的 while 循环后的 try-catch
	if scanner.Err() != nil {
		log.Fatalf("failed to read http response: %v", err)
		return
	}
}