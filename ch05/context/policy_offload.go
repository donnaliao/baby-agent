package context

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"

	"babyagent/ch05/storage"
	"babyagent/shared"
)

// OffloadPolicy - 卸载策略
// 当上下文使用率超过阈值时，把长 tool 消息的内容存到外部存储（如文件系统），
// 在上下文中只保留一段预览文本 + 一个 key，agent 需要时可以用 load_storage(key) 取回全文。
//
// 类比：
// 你在看一本很长的文档，但桌面空间有限，于是你把文档放进抽屉（存储），
// 桌面上只留一张便签写着"前100字...全文在抽屉A1"，需要时再从抽屉拿出来。
//
// 对应 Java 的：
// class OffloadPolicy implements Policy {
//     Storage storage;              // 外部存储
//     double usageThreshold;        // 触发阈值，如 0.8 表示 80%
//     int keepRecentMessages;       // 保留最近 N 条消息不卸载
//     int previewCharLimit;         // 预览字符数
// }
type OffloadPolicy struct {
	Storage            storage.Storage // 外部存储，用于保存被卸载的长文本内容
	UsageThreshold     float64        // 上下文使用率阈值（0.8 = 80%），超过才触发卸载
	KeepRecentMessages int            // 跳过最后 N 条消息，不卸载最近的对话（保证当前上下文完整）
	PreviewCharLimit   int            // 卸载后在上下文里保留的前 N 个字符（摘要）
}

// NewOffloadPolicy - 工厂方法，创建 OffloadPolicy 实例
// 参数：
//   storage - 存储后端（文件系统、内存等）
//   usageThreshold - 触发阈值，如 0.8
//   keepRecentMessages - 保留最近几条消息不卸载
//   previewCharLimit - 预览保留多少字符
func NewOffloadPolicy(storage storage.Storage, usageThreshold float64, keepRecentMessages, previewCharLimit int) *OffloadPolicy {
	return &OffloadPolicy{
		Storage:            storage,
		UsageThreshold:     usageThreshold,
		KeepRecentMessages: keepRecentMessages,
		PreviewCharLimit:   previewCharLimit,
	}
}

// Name - 策略名称
func (p *OffloadPolicy) Name() string {
	return "offload"
}

// makeStorageKey - 生成存储的 key
// 格式：/offload/日期时间_消息索引，例如 /offload/20260629_143052_3
// 用时间+索引确保 key 不重复
func (p *OffloadPolicy) makeStorageKey(offloadIndex int) string {
	return fmt.Sprintf("/offload/%s_%d", time.Now().Format("20060102_150405"), offloadIndex)
}

// Apply - 执行卸载策略
// 遍历消息链，找出长 tool 消息，把全文存到外部存储，替换为"预览+key"的缩略版
//
// 举例：
//   原始 tool 消息：10000 字的命令输出
//   卸载后：前100字...（更多内容已卸载，如需查看全文请使用 load_storage(key="/offload/20260629_143052_3") 工具）
//
// 对应 Java 的：
// public PolicyResult apply(Engine engine) {
//     List<MessageWrap> messages = new ArrayList<>(engine.messages); // 复制
//     for (int i = 0; i < messages.size() - keepRecentMessages; i++) {
//         if (messages.get(i).role != "tool") continue;  // 只处理 tool 消息
//         if (messages.get(i).content.length() <= previewCharLimit) continue; // 太短不需要卸载
//         String key = makeStorageKey(i);
//         storage.store(key, messages.get(i).content);  // 存全文
//         messages.get(i).content = content.substring(0, previewCharLimit) + "...全文key=" + key; // 替换为缩略版
//     }
//     return new PolicyResult(messages, newTokenCount);
// }
func (p *OffloadPolicy) Apply(ctx context.Context, engine *Engine) (PolicyResult, error) {
	// 如果消息总数都不够保留数，无需卸载，直接返回
	if len(engine.messages) <= p.KeepRecentMessages {
		return PolicyResult{
			Messages:      engine.messages,
			ContextTokens: engine.contextTokens,
		}, nil
	}l

	// ========== 第一步：复制消息列表 ==========
	// 不直接修改 engine.messages，而是创建副本，类似 Java 的 new ArrayList<>(list)
	messages := make([]messageWrap, len(engine.messages))
	copy(messages, engine.messages)
	contextTokens := engine.contextTokens

	// 需要检查的消息范围：排除最后 KeepRecentMessages 条
	// 例如总共 10 条消息，KeepRecentMessages=3，只检查前 7 条
	offloadCount := len(messages) - p.KeepRecentMessages

	for i := 0; i < offloadCount; i++ {
		// ========== 第二步：只卸载 tool 类型的消息 ==========
		// user 和 assistant 消息很重要，不能卸载
		// 类比：只把"附录"放进抽屉，"正文"留在桌面上
		if shared.GetRoleName(messages[i].Message) != "tool" {
			continue
		}

		// ========== 第三步：获取 tool 消息的文本内容 ==========
		contentAny := messages[i].Message.GetContent().AsAny()
		contentStr, ok := contentAny.(*string) // 类型断言：把 any 类型转成 *string，类似 Java 的 instanceof + 强转
		if !ok {
			continue // 不是字符串类型，跳过
		}

		// ========== 第四步：短消息不需要卸载 ==========
		// 如果内容本来就比预览限制短，卸载没意义
		if len(*contentStr) <= p.PreviewCharLimit {
			continue
		}

		// ========== 第五步：保存全文到外部存储 ==========
		oldTokens := messages[i].Tokens // 记录原始 token 数，后面要算差值
		key := p.makeStorageKey(i)

		// storage.Store 相当于 Java 的：Files.writeString(Path.of(key), content)
		if err := p.Storage.Store(ctx, key, *contentStr); err != nil {
			log.Printf("failed to store offload message: %v", err)
			continue // 存储失败也不影响其他消息，继续处理
		}

		// ========== 第六步：构造缩略版消息 ==========
		// 截取前 PreviewCharLimit 个字符作为预览
		abstract := (*contentStr)[0:p.PreviewCharLimit]
		var b strings.Builder // 类似 Java 的 StringBuilder
		b.WriteString(abstract)
		b.WriteString("...")
		// 告诉大模型：全文已经卸载了，你可以用 load_storage 工具取回
		b.WriteString(fmt.Sprintf("（更多内容已卸载，如需查看全文请使用 load_storage(key=\"%s\") 工具）\n", key))
		newContent := b.String()

		// ========== 第七步：用缩略版替换原消息 ==========
		// 用 openai.ToolMessage 构造一条新的 tool 消息，内容是缩略版
		// ToolCallID 必须和原来一致，否则 API 会报错
		newMessage := openai.ToolMessage(newContent, *engine.messages[i].Message.GetToolCallID())

		// 计算新消息的 token 数，更新总计数
		// 新总 token = 原总 token - (原消息 token - 新消息 token)
		// 即：减去被卸载掉的那部分 token
		newTokens := CountTokens(newMessage)
		messages[i] = messageWrap{Message: newMessage, Tokens: newTokens}
		contextTokens -= oldTokens - newTokens
	}

	return PolicyResult{
		Messages:      messages,
		ContextTokens: contextTokens,
	}, nil
}

// ShouldApply - 判断是否需要执行卸载策略
// 当上下文使用率 > 阈值时触发
// 例如：用了 170000/200000 = 85% > 80% 阈值 → 触发卸载
func (p *OffloadPolicy) ShouldApply(ctx context.Context, engine *Engine) bool {
	return engine.GetContextUsage() > p.UsageThreshold
}