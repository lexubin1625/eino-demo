# Eino 跨进程短期记忆实现示例

本示例展示了如何使用 eino 框架和开源组件实现基于 sessionId 的跨进程短期记忆功能。

## 使用的开源组件

- **go-redis/v9**: 标准的 Redis Go 客户端
- **Redis List**: 使用 Redis 原生 List 数据结构存储消息
- **eino**: CloudWeGo 的 LLM 应用框架

## 功能特性

- ✅ **跨进程记忆**：基于 sessionId 在不同进程中获取和共享记忆
- ✅ **持久化存储**：使用 Redis List 实现高效存储
- ✅ **自动加载**：每次创建 Agent 时自动从存储中加载历史记忆
- ✅ **自动保存**：每次更新记忆时自动保存到存储
- ✅ **自动裁剪**：超出限制时自动删除最旧的消息
- ✅ **零依赖**：只使用标准开源组件，不重复造轮子

## 核心实现

### ConversationMemory

直接使用 **go-redis** 和 **Redis List** 数据结构，无需自定义接口：

```go
type ConversationMemory struct {
    client    *redis.Client  // go-redis 客户端
    keyPrefix string
    maxRounds int
    ttl       time.Duration
}
```

**实现方式**：
- 使用 `RPUSH` 添加消息到 List
- 使用 `LTRIM` 自动裁剪超出限制的消息
- 使用 `LRANGE` 获取历史消息
- 使用 Pipeline 保证原子性操作（RPUSH + LTRIM + EXPIRE）
- 支持 TTL 自动过期

这种方式完全基于标准 Redis 操作，符合最佳实践。

### 4. 工作流程

```
1. 创建 Agent 时，根据 sessionId 从存储加载记忆
2. 用户发送消息 → 添加到记忆 → 保存到存储
3. LLM 生成回复 → 添加到记忆 → 保存到存储
4. 下次创建相同 sessionId 的 Agent 时，自动加载历史记忆
```

## 使用方法

### 1. 配置环境变量

```bash
export DASHSCOPE_API_KEY="your-api-key"
export REDIS_ADDR="localhost:6379"  # 可选，默认 localhost:6379
```

### 2. 安装依赖

```bash
cd memory
go mod tidy
```

### 3. 运行示例

```bash
go run main.go
```

### 4. 在代码中使用

```go
// 创建 Redis 客户端（使用 go-redis）
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

// 创建记忆管理器（直接使用 go-redis）
memory := NewConversationMemory(redisClient, "eino:memory", 5, 24*time.Hour)

// 创建带记忆的 Agent
agent, err := createAgentWithMemory(ctx, "session-001", memory, chatModel)

// 使用 Agent 进行对话
response, err := agent.Invoke(ctx, &schema.Message{
    Role:    schema.User,
    Content: "我的名字是张三",
})
```

## 存储方式选择

### Redis 存储（推荐用于生产环境）

- ✅ 支持跨进程、跨服务器共享
- ✅ 支持数据持久化
- ✅ 支持过期时间（TTL）
- ✅ 适合分布式部署

```go
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})
storage := NewRedisMemoryStorage(redisClient, "eino:memory", 24*time.Hour)
```

### 内存存储（用于测试）

- ✅ 简单快速
- ✅ 无需外部依赖
- ❌ 进程重启后数据丢失
- ❌ 不支持跨进程共享

```go
storage := NewInMemoryStorage()
```

## 示例输出

```
=== 跨进程记忆演示 ===

使用存储类型: *main.RedisMemoryStorage

--- Session 1 对话 ---

[Session 1] 第 1 轮对话
用户: 我的名字是张三
助手: 好的，我记住了你的名字是张三。

[Session 1] 第 2 轮对话
用户: 我喜欢吃苹果
助手: 好的，我记住了你喜欢吃苹果。

[Session 1] 第 3 轮对话
用户: 我的名字是什么？
助手: 你的名字是张三。

--- Session 2 对话（新会话，无历史记忆）---

[Session 2] 第 1 轮对话
用户: 我的名字是什么？
助手: 抱歉，我不知道你的名字。

--- 重新加载 Session 1（模拟不同进程）---

用户: 我刚才说我喜欢吃什么？
助手: 你刚才说你喜欢吃苹果。

✅ 成功！跨进程记忆功能正常工作
```

## 实现原理

### 1. 存储接口设计

使用接口模式，支持多种存储实现，便于扩展和测试。

### 2. 自动加载和保存

- **加载**：在 `createAgentWithMemory` 中，根据 sessionId 从存储加载记忆
- **保存**：在 `StatePostHandler` 中，每次更新记忆后自动保存

### 3. SessionId 管理

- 每个会话使用唯一的 sessionId
- 相同 sessionId 的 Agent 共享同一份记忆
- 不同 sessionId 的 Agent 拥有独立的记忆

## 扩展建议

1. **数据库存储**：实现 MySQL/PostgreSQL 存储接口
2. **消息摘要**：对旧消息进行摘要压缩，节省存储空间
3. **按 Token 限制**：根据消息的 token 数量而非轮数来限制记忆大小
4. **记忆清理策略**：定期清理过期或长期未使用的记忆
5. **分布式锁**：在并发场景下使用分布式锁保证数据一致性

## 注意事项

- 确保 Redis 连接稳定，建议添加重试机制
- 根据实际需求调整 TTL（过期时间）
- 对于敏感信息，考虑添加加密存储
- 注意存储大小限制，避免超出 Redis 内存限制
- 在高并发场景下，考虑使用连接池

## 环境要求

- Go 1.23+
- Redis（可选，如果使用 Redis 存储）
- DASHSCOPE_API_KEY 环境变量
