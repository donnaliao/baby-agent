package tool

import (
	"context"
	"encoding/json" // JSON 序列化/反序列化包，类似 Java 的 ObjectMapper
	"os"            // 文件操作包，类似 Java 的 Files/Path
	"strings"       // 字符串操作包，类似 Java 的 StringUtils

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// EditTool - 文件编辑工具，支持对文件内容进行替换
// 原理：读取文件 → 备份 → 替换内容 → 写回文件
type EditTool struct{}

// NewEditTool - 创建 EditTool 实例
func NewEditTool() *EditTool {
	return &EditTool{}
}

// EditToolParam - edit 工具的参数结构
// LLM 调用 edit 工具时会传递 JSON 格式的参数，例如：
//   {"path": "a.txt", "before": "hello", "after": "world"}
// 这个结构体就是用来解析那个 JSON 的
type EditToolParam struct {
	Path   string `json:"path"`   // 文件路径
	Before string `json:"before"` // 要替换的原内容（精确匹配）
	After  string `json:"after"`  // 替换后的新内容
}

// ToolName - 返回工具名称，用于 Agent 识别
func (t *EditTool) ToolName() AgentTool {
	return AgentToolEdit
}

// Info - 返回工具的函数定义，供 LLM 了解如何调用
// LLM 会看到：name="edit", description="edit content in file", parameters={path, before, after}
func (t *EditTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        string(AgentToolEdit),
		Description: openai.String("edit content in file"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "the file path to edit",
				},
				"before": map[string]any{
					"type":        "string",
					"description": "the content to search for",
				},
				"after": map[string]any{
					"type":        "string",
					"description": "the content to replace with",
				},
			},
			"required": []string{"path", "before", "after"},
		},
	})
}

// Execute - 执行文件编辑
// 流程：读取文件 → 备份(.bak) → 替换内容 → 写回文件
// 如果写回失败，自动从备份恢复
func (t *EditTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	// ========== 第一步：解析 JSON 参数 ==========
	// argumentsInJSON 是 LLM 传递的 JSON 字符串，例如：
	//   `{"path": "a.txt", "before": "hello", "after": "world"}`
	//
	// json.Unmarshal 是"反序列化"：把 JSON 字符串转成 Go 的结构体
	// 类似 Java 的：ObjectMapper.readValue(jsonString, EditToolParam.class)
	// - 第一个参数是 JSON 字符串（[]byte 是字符串的另一种表示）
	// - 第二个参数是目标结构体的指针（&p 表示把解析结果存到 p 变量里）
	//
	// 如果 JSON 格式错误（比如少了引号、多了逗号），err 就不为 nil
	p := EditToolParam{}
	err := json.Unmarshal([]byte(argumentsInJSON), &p)
	if err != nil {
		// 返回空字符串和错误，Agent 会看到这个错误
		return "", err
	}

	// ========== 第二步：读取原文件内容 ==========
	// os.ReadFile 读取整个文件内容，类似 Java 的 Files.readString(Path)
	// 返回的是 []byte（字节数组），需要转成 string 才能用 strings.ReplaceAll
	raw, err := os.ReadFile(p.Path)
	if err != nil {
		return "", err
	}

	// ========== 第三步：备份原文件 ==========
	// 为了防止替换失败导致文件损坏，先备份一份
	// 备份文件名 = 原文件名 + ".bak"，例如 a.txt.bak
	backupPath := p.Path + ".bak"
	// os.WriteFile 写文件，类似 Java 的 Files.writeString(Path, content)
	// 0644 是文件权限，类似 Unix 的 -rw-r--r--
	err = os.WriteFile(backupPath, raw, 0644)
	if err != nil {
		return "", err
	}

	// ========== 第四步：执行字符串替换 ==========
	// strings.ReplaceAll(source, old, new) 相当于 Java 的 String.replace(old, new)
	// 把文件内容中所有 "before" 替换成 "after"
	// 如果找不到 "before"，原样返回（不会报错）
	replaced := strings.ReplaceAll(string(raw), p.Before, p.After)

	// ========== 第五步：写回文件 ==========
	err = os.WriteFile(p.Path, []byte(replaced), 0644)
	if err != nil {
		// 写文件失败（比如磁盘满了、权限不足），从备份恢复
		os.Rename(backupPath, p.Path) // 把备份重命名回原文件，覆盖损坏的文件
		return "", err
	}

	// ========== 第六步：删除备份 ==========
	// 替换成功，备份就没用了，删掉它
	os.Remove(backupPath)

	// 返回空字符串表示成功（没有要返回的内容）
	// 如果返回有内容的字符串，Agent 会把它展示给用户
	return "", nil
}