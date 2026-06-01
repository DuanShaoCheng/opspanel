# OpsPanel 101 机器部署指南

## 前置准备

1. 确保 101 机器上已安装 Docker 和 Docker Compose
2. 备份当前配置文件

```bash
cd /path/to/opspanel
cp data/config.json data/config.json.backup.$(date +%Y%m%d_%H%M%S)
```

## 部署步骤

### 1. 停止当前服务

```bash
cd /path/to/opspanel
docker compose down
```

### 2. 更新代码

```bash
# 如果是 git 仓库
git pull origin master

# 或者手动上传以下文件：
# - config.go
# - handler_sentinel.go
# - llm.go
# - scheduler.go
# - CHANGELOG.md
# - deploy.sh
```

### 3. 重新构建镜像

```bash
docker compose build
```

### 4. 启动服务

```bash
docker compose up -d
```

### 5. 检查服务状态

```bash
docker compose ps
docker compose logs -f opspanel
```

## 重新配置 LLM（重要）

由于移除了 legacy LLM 配置，需要通过 Web 面板重新配置：

### 访问面板

```
http://101机器IP:9091
```

默认账号：`admin` / `admin123`

### 添加 LLM 提供商

1. 进入"配置管理"页面
2. 找到"LLM 提供商"部分
3. 点击"添加提供商"
4. 填写配置：

**示例：DeepSeek**
```json
{
  "id": "deepseek",
  "name": "DeepSeek",
  "type": "openai",
  "api_url": "https://api.deepseek.com/v1/chat/completions",
  "api_key": "sk-your-api-key-here",
  "model": "deepseek-chat"
}
```

**示例：OpenAI**
```json
{
  "id": "openai",
  "name": "OpenAI GPT-4",
  "type": "openai",
  "api_url": "https://api.openai.com/v1/chat/completions",
  "api_key": "sk-your-api-key-here",
  "model": "gpt-4"
}
```

5. 点击"保存"

### 配置日志分析模块

1. 在"配置管理"页面找到"日志分析"部分
2. 选择刚才添加的 LLM 提供商（从下拉列表选择）
3. （可选）自定义 LLM 提示词
4. 点击"保存"

### 测试 LLM 连接

1. 在配置页面找到"测试 LLM"按钮
2. 点击测试
3. 确认返回成功消息

## 验证部署

### 1. 检查状态

访问首页，确认：
- "LLM 已配置" 显示为绿色 ✓
- 其他服务状态正常

### 2. 测试日志报告

手动触发一次分析：
```bash
# 方式1：通过 Web 面板
# 进入"日志分析"页面，点击"立即分析"

# 方式2：通过 API
curl -X POST http://localhost:9091/api/trigger \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

### 3. 检查通知格式

查看飞书/企业微信/钉钉收到的通知，应该是简略格式：

```
过去 24 小时共 47 条错误日志：

[game-server][error] × 32 — Redis connection timeout after 5s
[auth-service][crash] × 8 — panic: nil pointer dereference
[gateway][error] × 7 — upstream connect error
```

而不是逐条列出的长消息。

## 回滚方案

如果出现问题需要回滚：

```bash
# 1. 停止服务
docker compose down

# 2. 恢复代码
git reset --hard HEAD~1

# 3. 恢复配置
cp data/config.json.backup.YYYYMMDD_HHMMSS data/config.json

# 4. 重新构建启动
docker compose build
docker compose up -d
```

## 常见问题

### Q: 状态仍显示"LLM 未配置"

**A:** 检查以下几点：
1. 确认已在 Web 面板添加了 LLM 提供商
2. 确认在"日志分析"配置中选择了该提供商
3. 刷新浏览器页面
4. 查看容器日志：`docker compose logs -f`

### Q: LLM 测试失败

**A:** 检查：
1. API Key 是否正确
2. API URL 是否正确（需要完整路径，如 `/v1/chat/completions`）
3. 网络是否能访问 LLM API
4. 查看详细错误信息

### Q: 日志报告格式没有变化

**A:** 
1. 确认已重新构建镜像（`docker compose build`）
2. 确认容器已重启（`docker compose restart`）
3. 手动触发一次新的分析

## 技术支持

查看详细更新日志：`cat CHANGELOG.md`

查看容器日志：
```bash
docker compose logs -f opspanel
```

进入容器调试：
```bash
docker compose exec opspanel sh
```
