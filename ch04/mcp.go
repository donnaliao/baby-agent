package ch04

import (
	"context"     // 上下文，用于传递取消信号和超时
	"encoding/json" // JSON 处理
	"fmt"         // 格式化字符串
	"log"         // 日志
	"os/exec"     // 执行外部命令
	"strings"     // 字符串操作，类似 Java 的 StringBuilder

	"github.com/modelcontextprotocol/go-sdk/mcp" // MCP SDK，类似 Java 的第三方库
	"github.com/openai/openai-go/v3"
	shared2 "github.com/openai/openai-go/v3/shared"

	"babyagent/ch04/tool"
	"babyagent/shared"
)

// ========== MCP 客户端结构体（类似 Java 的 class）==========

// McpClient - MCP（Model Context Protocol）客户端
// 用于连接外部 MCP 服务器，让 Agent 能够调用 MCP 服务器提供的工具
//
// 类似于 Java 的：
// class McpClient {
//     private String name;
//     private MCPClient client;  // 第三方库提供的客户端
//     private McpServerConfig serverConfig;  // 服务器配置
//     private ClientSession session;  // 连接会话
//     private List<Tool> tools;  // 暴露给 Agent 的工具列表
// }
type McpClient struct {
	name         string                        // MCP 服务器名称
	client       *mcp.Client                   // MCP SDK 的客户端实例（第三方库提供）
	serverConfig shared.McpServerConfig        // 服务器配置（命令/URL、环境变量等）

	session *mcp.ClientSession // 当前会话（连接成功后才有）
	tools   []tool.Tool        // 从 MCP 服务器发现的所有工具
}

// initRunningVars - 初始化运行时变量
// 这些变量会在连接 MCP 服务器前被替换到配置中
//
// 相当于 Java 的：
// private Map<String, String> initRunningVars() {
//     Map<String, String> vars = new HashMap<>();
//     vars.put("${workspaceFolder}", getWorkspaceDir());
//     return vars;
// }
func initRunningVars() map[string]string {
	runningVars := map[string]string{
		"${workspaceFolder}": shared.GetWorkspaceDir(), // 替换为当前工作目录
	}
	return runningVars
}

// NewMcpToolProvider - 工厂方法，创建 MCP 客户端
// 相当于 Java 的：public static McpClient newMcpToolProvider(String name, McpServerConfig config)
func NewMcpToolProvider(name string, server shared.McpServerConfig) *McpClient {

	return &McpClient{
		name: name,
		// 创建 MCP 客户端，Implementation 是客户端的标识信息
		// 相当于 Java 的：new MCPClient(new Implementation("babyagent-mcp-client", "BabyAgent", "v1.0.0"), null)
		client: mcp.NewClient(&mcp.Implementation{
			Name:    "babyagent-mcp-client",
			Title:   "BabyAgent",
			Version: "v1.0.0",
		}, nil),
		// ReplacePlaceholders 会把配置里的 ${workspaceFolder} 替换成实际路径
		serverConfig: server.ReplacePlaceholders(initRunningVars()),
		tools:        make([]tool.Tool, 0),
	}
}

// Name - 返回 MCP 服务器名称
func (e *McpClient) Name() string {
	return e.name
}

// connect - 建立与 MCP 服务器的连接
// 如果已经连接过且连接有效（ping 得通），就直接返回，不重复连接
//
// 相当于 Java 的：
// public void connect() throws Exception {
//     if (session != null && session.ping() == null) return;
//     if (serverConfig.isStdio()) {
//         // 通过 stdio 连接（子进程方式）
//         ProcessBuilder pb = new ProcessBuilder(serverConfig.command, serverConfig.args);
//         for (Map.Entry<String, String> e : serverConfig.env.entrySet()) {
//             pb.environment().put(e.getKey(), e.getValue());
//         }
//         session = client.connect(new CommandTransport(pb.start()));
//     } else {
//         // 通过 HTTP 连接
//         session = client.connect(new StreamableClientTransport(serverConfig.url));
//     }
// }
func (e *McpClient) connect(ctx context.Context) error {
	// 如果已经有有效会话，不需要重新连接
	// Ping 是心跳检查，返回 nil 表示连接正常
	if e.session != nil && e.session.Ping(ctx, &mcp.PingParams{}) == nil {
		return nil
	}
	var err error

	// 判断是 stdio 模式还是 HTTP 模式
	if e.serverConfig.IsStdio() {
		// stdio 模式：通过子进程启动 MCP 服务器，通过标准输入输出通信
		// 类似 Java 的 ProcessBuilder：启动一个子进程，然后通过它的 stdin/stdout 通信
		cmd := exec.Command(e.serverConfig.Command, e.serverConfig.Args...)
		// 设置环境变量
		for k, v := range e.serverConfig.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		// 通过命令通道连接
		e.session, err = e.client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	} else {
		// HTTP 模式：通过 HTTP/SSE 连接（远程 MCP 服务器）
		e.session, err = e.client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: e.serverConfig.Url}, nil)
	}
	if err != nil {
		return err
	}

	return nil
}

// RefreshTools - 从 MCP 服务器刷新工具列表
// 调用 ListTools API 获取 MCP 服务器提供的所有工具，并转换为 Agent 能调用的格式
//
// 相当于 Java 的：
// public List<Tool> refreshTools() throws Exception {
//     connect();  // 确保已连接
//     List<McpTool> mcpTools = session.listTools();  // 调用 MCP 的 list_tools
//     tools = new ArrayList<>();
//     for (McpTool mcpTool : mcpTools) {
//         tools.add(new McpTool(this, mcpTool.name, session, mcpTool));
//     }
//     return tools;
// }
func (e *McpClient) RefreshTools(ctx context.Context) error {
	// 确保已连接
	if err := e.connect(ctx); err != nil {
		return err
	}

	// 调用 MCP 服务器的 ListTools 方法获取可用工具列表
	mcpToolResult, err := e.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return err
	}

	// 清空旧工具列表，重新填充
	e.tools = make([]tool.Tool, 0)
	for _, mcpTool := range mcpToolResult.Tools {
		// 为每个 MCP 工具创建一个包装器（McpTool）
		// 这样可以把 MCP 工具适配成 Agent 的 Tool 接口
		agentTool := &McpTool{
			client:   e,           // 引用父客户端，用于调用 callTool
			toolName: mcpTool.Name, // MCP 服务器看到的工具名
			session:  e.session,    // 会话引用
			mcpTool:  mcpTool,      // 原始 MCP 工具定义
		}

		e.tools = append(e.tools, agentTool)
	}
	return nil
}

// GetTools - 返回当前已发现的所有工具
func (e *McpClient) GetTools() []tool.Tool {
	return e.tools
}

// callTool - 调用 MCP 服务器上的工具
// 这是实际执行 MCP 工具的地方
//
// 相当于 Java 的：
// public String callTool(String toolName, String arguments) throws Exception {
//     connect();  // 确保已连接
//     CallToolResult result = session.callTool(new CallToolParams(toolName, arguments));
//     StringBuilder sb = new StringBuilder();
//     for (Content c : result.content) {
//         if (c instanceof TextContent) {
//             sb.append(((TextContent) c).text);
//         }
//     }
//     return sb.toString();
// }
func (e *McpClient) callTool(ctx context.Context, toolName string, argumentsInJSON string) (string, error) {
	// 确保已连接
	if err := e.connect(ctx); err != nil {
		return "", err
	}

	// 调用 MCP 服务器的 CallTool 方法
	mcpResult, err := e.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,                          // 工具名称
		Arguments: json.RawMessage(argumentsInJSON), // JSON 格式的参数
	})
	if err != nil {
		log.Printf("failed to call tool: %v", err)
		return "", err
	}

	// MCP 返回的结果可能包含多个 content block，拼接成字符串
	// 类似 Java 的：StringBuilder sb; for (Content c : result.content) { if (c instanceof TextContent) sb.append(((TextContent) c).text); }
	var builder strings.Builder
	for _, content := range mcpResult.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			builder.WriteString(textContent.Text)
		}
	}
	return builder.String(), nil
}

// ========== MCP 工具适配器（实现 Tool 接口）==========

// McpTool - 包装 MCP 工具，使其符合 Agent 的 Tool 接口
// 类似于 Java 的：class McpTool implements Tool { ... }
type McpTool struct {
	toolName string          // MCP 服务器看到的工具名（内部名）
	client   *McpClient      // 引用父客户端，用于调用 callTool
	session  *mcp.ClientSession // 会话引用（未使用，可能是预留）
	mcpTool  *mcp.Tool       // 原始 MCP 工具定义（包含 description、inputSchema 等）
}

// ToolName - 返回给大模型看的工具名
// 格式：babyagent_mcp__<服务器名>__<工具名>
// 例如：babyagent_mcp__filesystem__read_file
//
// 为什么要加前缀？因为可能有多个 MCP 服务器，每个服务器可能有同名的工具
// 所以用前缀来区分，相当于 Java 的：return client.getName() + "_" + toolName;
func (t *McpTool) ToolName() string {
	return fmt.Sprintf("babyagent_mcp__%s__%s", t.client.Name(), t.toolName)
}

// Info - 返回工具的函数定义，供大模型了解如何调用
// 直接使用 MCP 服务器提供的 description 和 inputSchema
//
// 相当于 Java 的：
// public FunctionDefinition info() {
//     return new FunctionDefinition(mcpTool.description, toolName(), mcpTool.inputSchema);
// }
func (t *McpTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared2.FunctionDefinitionParam{
		Description: openai.String(t.mcpTool.Description),         // MCP 服务器提供的描述
		Name:        t.ToolName(),                                  // 带前缀的工具名
		Parameters:  t.mcpTool.InputSchema.(map[string]any),        // 参数 schema
	})
}

// Execute - 执行 MCP 工具
// 委托给父客户端的 callTool 方法
//
// 相当于 Java 的：
// public String execute(Context ctx, String jsonParams) {
//     return client.callTool(ctx, toolName, jsonParams);
// }
func (t *McpTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	return t.client.callTool(ctx, t.toolName, argumentsInJSON)
}