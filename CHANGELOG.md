# OpsPanel 更新日志

## 2026-06-01 - 重要更新

### 修复问题

#### 1. LLM 配置检测逻辑修复

**问题描述：**
- 状态接口显示"LLM 未配置"，即使在后台已经配置了多个 LLM 提供商
- 原因：代码只检查 legacy 的单一 `LLM` 字段，忽略了 `LLMProviders[]` 数组

**修复内容：**
- 移除了 legacy 的 `Config.LLM` 字段和 `LLMConfig` 类型
- 统一使用 `LLMProviders[]` 多提供商配置
- 修复了 3 处检测逻辑：
  - `handleGetStatus` - 状态检查改用 `GetLLMProvider("")`
  - `runAnalysis` - 分析流程改用 `GetLLMProvider(c.LogAnalysis.LLMProvider)`
  - `handleTestLLM` - 测试接口同样使用新逻辑
- 移除了环境变量 `LOG_SENTINEL_LLM_*` 的支持（请通过 Web 面板配置）

**迁移说明：**
- 如果之前通过 `.env` 配置了 LLM，需要在 Web 面板重新配置
- 在"配置管理"页面添加 LLM 提供商，填写：
  - ID: 自定义标识（如 `deepseek`）
  - 名称: 显示名称（如 `DeepSeek`）
  - 类型: `openai`（兼容 OpenAI API 的都选这个）
  - API URL: `https://api.deepseek.com/v1/chat/completions`
  - API Key: 你的密钥
  - Model: `deepseek-chat`
- 在"日志分析"配置中选择要使用的 LLM 提供商

#### 2. 日志报告格式优化

**问题描述：**
- 日志报告逐条列出所有错误，导致消息过长被拆成多条发送
- 用户希望只看简略摘要，而不是完整日志内容

**修复内容：**
- 改为按 `source + level` 分组统计
- 每组只显示：来源、级别、计数、最近一条错误（截断到 80 字符）
- 所有错误汇总在一条消息中发送

**效果对比：**

旧格式（多条消息）：
```
过去 24 小时共 47 条错误日志：

[10:23:45][error][game-server] Redis connection timeout after 5s
[10:24:12][error][game-server] Redis connection timeout after 5s
[10:25:33][error][game-server] Redis connection timeout after 5s
...（还有 44 条）
```

新格式（一条消息）：
```
过去 24 小时共 47 条错误日志：

[game-server][error] × 32 — Redis connection timeout after 5s
[auth-service][crash] × 8 — panic: nil pointer dereference
[gateway][error] × 7 — upstream connect error
```

### 技术细节

**修改文件：**
- `config.go` - 移除 `LLM` 字段和 `LLMConfig` 类型，清理环境变量加载
- `handler_sentinel.go` - 修复状态检查、配置保存、测试接口、分析流程
- `scheduler.go` - 重写日志报告格式化逻辑
- `llm.go` - 修改 `CallLLM` 函数签名，直接接受 `LLMProvider`

**向后兼容性：**
- ⚠️ 不兼容：移除了 legacy `LLM` 配置和环境变量支持
- 需要重新配置 LLM 提供商

### 部署步骤

1. 备份当前配置：
```bash
cp data/config.json data/config.json.backup
```

2. 更新代码并重新构建：
```bash
git pull
docker compose down
docker compose build
docker compose up -d
```

3. 访问 Web 面板重新配置 LLM 提供商

4. 测试 LLM 连接（配置页面有"测试"按钮）

### 验证

- 访问首页，状态卡片应显示"LLM 已配置"
- 手动触发一次日志分析，检查通知格式是否为简略摘要
- 查看日志：`docker compose logs -f`
