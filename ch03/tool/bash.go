package tool

import (
	"context"     // 上下文包，类似 Java 的 CancellationToken
	"encoding/json" // JSON 序列化/反序列化，类似 Java 的 ObjectMapper
	"os/exec"      // 执行外部命令，类似 Java 的 ProcessBuilder
	"runtime"      // 运行时信息，用来判断操作系统

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// ========== 结构体定义（类似 Java 的 class）==========

// BashTool - Bash 工具，用于执行 shell 命令
// 对应 Java 的：class BashTool implements Tool
type BashTool struct{}

// NewBashTool - 工厂方法，创建 BashTool 实例
// 相当于 Java 的：public static BashTool newBashTool() { return new BashTool(); }
func NewBashTool() *BashTool {
	return &BashTool{}
}

// BashToolParam - 工具参数结构体
// LLM 调用 bash 时传递的参数，例如：{"command": "ls -la"}
// 对应 Java 的 class BashToolParam { String command; }
type BashToolParam struct {
	Command string `json:"command"` // JSON 字段名是 "command"
}

// ========== Tool 接口实现（类似 Java 的 implements）==========

// ToolName - 返回工具名称，Agent 用这个识别要调哪个工具
// 相当于 Java 的：public AgentTool getName() { return AgentTool.Bash; }
func (t *BashTool) ToolName() AgentTool {
	return AgentToolBash
}

// Info - 返回工具的函数定义（Function Definition）
// 大模型看到的是：name="bash", description="execute bash command", parameters={command}
// 相当于 Java 的：public FunctionDefinition getInfo() { ... }
func (t *BashTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        string(AgentToolBash),
		Description: openai.String("execute bash command"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "the bash command to execute",
				},
			},
			"required": []string{"command"}, // command 是必填参数
		},
	})
}

// Execute - 真正执行 bash 命令的地方
// 参数：
//   ctx - 上下文，用于接收取消信号，类似 Java 的 CancellationToken
//   argumentsInJSON - LLM 传来的 JSON 参数，例如：`{"command": "ls -la"}`
//
// 返回：命令执行结果（stdout+stderr），或者错误
//
// 相当于 Java 的：public String execute(CancellationToken ctx, String jsonParams)
func (t *BashTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	// ========== 第一步：解析 JSON 参数 ==========
	// json.Unmarshal 把 JSON 字符串转成结构体
	// 相当于 Java 的：BashToolParam p = objectMapper.readValue(jsonParams, BashToolParam.class)
	p := BashToolParam{}
	err := json.Unmarshal([]byte(argumentsInJSON), &p)
	if err != nil {
		return "", err // 解析失败，返回空字符串和错误
	}

	// ========== 第二步：构造命令 ==========
	// runtime.GOOS 判断当前操作系统
	// Windows 用 cmd.exe，Linux/macOS 用 sh（POSIX 标准 shell，比 bash 更通用）
	// 相当于 Java 的：
	//   ProcessBuilder pb;
	//   if (System.getProperty("os.name").contains("Windows")) {
	//       pb = new ProcessBuilder("cmd", "/C", command);
	//   } else {
	//       pb = new ProcessBuilder("sh", "-c", command);
	//   }
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Windows: 用 cmd.exe 执行命令（/C 表示执行完后退出）
		cmd = exec.CommandContext(ctx, "cmd", "/C", p.Command)
	} else {
		// Linux/macOS: 用 POSIX sh（更通用，不依赖 bash）
		cmd = exec.CommandContext(ctx, "sh", "-c", p.Command)
	}

	// ========== 第三步：执行命令 ==========
	// CombinedOutput() 同时捕获 stdout 和 stderr
	// 相当于 Java 的：Process process = pb.start(); String output = readString(process.getInputStream());
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err // 命令执行失败，返回空字符串和错误
	}

	// ========== 第四步：返回结果 ==========
	// string(output) 把字节数组转成字符串
	// 相当于 Java 的：return new String(output, StandardCharsets.UTF_8);
	return string(output), nil
}