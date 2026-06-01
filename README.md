# OpsPanel

轻量运维工具平台。核心功能：实时日志采集与监控、LLM 智能分析、多渠道告警推送。

支持本地文件监控和 Filebeat 两种日志接入方式，内置用户认证和角色权限，单个 Docker 镜像即可部署。

## 功能

- **日志采集**：本地文件增量监控 + Filebeat/ES 兼容接收，支持多服务器节点隔离
- **日志格式**：纯文本、JSON 结构化、多行堆栈
- **级别分类**：可配置关键词规则自动分类（crash/error/warn/undefined）
- **LLM 分析**：支持多 LLM 提供商（OpenAI 兼容），单条日志 AI 评分，定时批量汇总推送
- **问题追踪**：LLM 输出结构化问题列表，按优先级排序
- **通知渠道**：飞书、企业微信、钉钉、通用 Webhook、SMTP 邮件
- **定时任务**：通用 Job 调度器，支持多任务并行、手动触发、持久化
- **用户认证**：JWT 登录，admin/viewer 两种角色
- **多数据库**：SQLite（默认零配置）/ MySQL / PostgreSQL
- **Web 面板**：所有配置通过浏览器完成，单页应用内嵌于二进制
- **容器化**：单个 Docker 镜像，挂载日志目录即可运行

## 快速开始

```bash
cp .env.example .env    # 编辑 LOG_DIR
docker compose up -d
```

访问 `http://localhost:9090`，默认账号：
- 用户名：`admin`
- 密码：`admin123`

首次登录后建议修改密码。

## 角色权限

| 操作 | admin | viewer |
|------|-------|--------|
| 查看仪表盘/日志/历史/问题 | ✓ | ✓ |
| 查看配置（脱敏） | ✓ | ✓ |
| 修改配置/日志源/级别规则 | ✓ | ✗ |
| AI 分析/手动触发 | ✓ | ✗ |
| 管理用户 | ✓ | ✗ |
| 管理定时任务 | ✓ | ✗ |
| 测试通知/采集/LLM | ✓ | ✗ |
| 删除日志/历史 | ✓ | ✗ |

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DATA_DIR` | 数据存储目录 | `/data` |
| `LISTEN_ADDR` | 服务监听地址 | `:9090` |
| `LISTEN_PORT` | 宿主机映射端口（docker-compose 用） | `9090` |
| `LOG_DIR` | 宿主机日志目录（docker-compose 挂载用） | `/var/log` |
| `DB_DRIVER` | 数据库类型：sqlite / mysql / postgres | `sqlite` |
| `DB_DSN` | 数据库连接串 | `/data/sentinel.db` |
| `JWT_SECRET` | JWT 签名密钥（生产环境务必修改） | 内置默认值 |
| `GIN_MODE` | Gin 运行模式（debug/release） | `release` |
| `INGEST_API_KEY` | Filebeat 接收端点认证密钥（为空则不校验） | — |
| `LOG_SENTINEL_SCHEDULE_ENABLED` | 启用定时分析 | `false` |
| `LOG_SENTINEL_SCHEDULE_CRON` | Cron 表达式 | `0 9 * * *` |
| `LOG_SENTINEL_SCHEDULE_HOURS` | 定时任务回溯小时数 | `24` |
| `LOG_SENTINEL_SCHEDULE_MAX_BYTES` | 单次采集最大字节数 | `12000` |
| `LOG_SENTINEL_NOTIFY_TYPE` | 快速通知类型（feishu/wecom/dingtalk/webhook） | — |
| `LOG_SENTINEL_NOTIFY_WEBHOOK` | 快速通知 Webhook URL | — |
| `LOG_SENTINEL_NOTIFY_NAME` | 快速通知渠道名称 | `env-<type>` |
| `LOG_SENTINEL_NOTIFY_SECRET` | 快速通知签名密钥 | — |

> LLM 配置已移至 Web 面板管理（支持多提供商），不再通过环境变量设置。

## 数据库配置

```env
# SQLite（默认，零配置）
DB_DRIVER=sqlite
DB_DSN=/data/sentinel.db

# MySQL
DB_DRIVER=mysql
DB_DSN=user:pass@tcp(127.0.0.1:3306)/opspanel?charset=utf8mb4&parseTime=True

# PostgreSQL
DB_DRIVER=postgres
DB_DSN=host=127.0.0.1 user=admin password=pass dbname=opspanel port=5432 sslmode=disable
```

## 日志接入

### 方式一：本地文件监控

在 Web 面板「日志源」中添加本地类型的日志源，配置目录路径、文件名 glob 和过滤正则。系统每 10 秒扫描新增内容。

### 方式二：Filebeat 接收

OpsPanel 兼容 Elasticsearch Bulk API，可直接作为 Filebeat 的 output 目标：

```yaml
# filebeat.yml
output.elasticsearch:
  hosts: ["http://opspanel-host:9090"]
  # 如果设置了 INGEST_API_KEY，需要配置认证
  # username: "elastic"
  # password: "your-ingest-api-key"

fields:
  server_id: "node1"    # 用于区分不同服务器节点
fields_under_root: false
```

在 Web 面板「日志源」中添加 filebeat 类型，配置「服务器标识字段」为 `fields.server_id`。

## API

所有 `/api/*` 接口需要 `Authorization: Bearer <token>` header。

### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/login` | 登录 |
| GET | `/api/auth/me` | 当前用户 |
| POST | `/api/auth/password` | 修改密码 |

### 用户管理（admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/users` | 用户列表 |
| POST | `/api/users` | 创建用户 |
| PUT | `/api/users/:id` | 修改用户 |
| DELETE | `/api/users/:id` | 删除用户 |

### 日志采集

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/logs` | viewer | 日志列表（分页、按级别/来源/服务器筛选） |
| GET | `/api/logs/:id` | viewer | 单条日志详情 |
| GET | `/api/logs/stats` | viewer | 采集统计 |
| POST | `/api/logs/:id/analyze` | admin | 单条日志 AI 分析 |
| POST | `/api/logs/reclassify` | admin | 重新分类所有日志 |
| DELETE | `/api/logs` | admin | 批量清除日志 |
| DELETE | `/api/logs/:id` | admin | 删除单条日志 |

### 配置与分析

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/config` | viewer | 获取配置 |
| POST | `/api/config` | admin | 保存配置 |
| GET | `/api/status` | viewer | 仪表盘状态 |
| POST | `/api/trigger` | admin | 手动触发分析 |
| GET | `/api/history` | viewer | 执行历史 |
| DELETE | `/api/history` | admin | 清空历史 |
| DELETE | `/api/history/:index` | admin | 删除单条历史 |
| GET | `/api/issues` | viewer | 问题列表 |
| POST | `/api/issues` | admin | 更新问题状态 |
| POST | `/api/test-notify` | admin | 测试通知 |
| POST | `/api/test-collect` | admin | 测试采集 |
| POST | `/api/test-llm` | admin | 测试 LLM |

### 定时任务

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/jobs` | viewer | 任务列表 |
| GET | `/api/jobs/handlers` | viewer | 可用 handler 列表 |
| GET | `/api/jobs/modules` | viewer | 可用模块列表 |
| POST | `/api/jobs` | admin | 创建任务 |
| PUT | `/api/jobs/:id` | admin | 更新任务 |
| DELETE | `/api/jobs/:id` | admin | 删除任务 |
| POST | `/api/jobs/:id/run` | admin | 手动执行任务 |

### Ingest（Filebeat 兼容）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/_bulk` | ES Bulk API |
| POST/PUT | `/:index/_bulk` | ES 索引 Bulk API |
| POST | `/api/ingest` | 自定义日志接收（需 Bearer token） |
| GET | `/healthz` | 健康检查 |

## 目录结构

```
opspanel/
├── main.go              # 入口、路由注册、graceful shutdown
├── config.go            # 配置结构、JSON 持久化、环境变量覆盖
├── database.go          # 数据库初始化（SQLite/MySQL/PostgreSQL）
├── model.go             # 数据模型（User、LogEntry）
├── auth.go              # JWT 生成/验证、密码哈希
├── middleware.go        # Gin 中间件（认证、角色、Ingest Key）
├── handler_auth.go      # 认证和用户管理路由
├── handler_sentinel.go  # 配置管理、手动触发、问题追踪
├── handler_logs.go      # 日志查看、筛选、AI 分析
├── handler_scheduler.go # 定时任务管理
├── ingest.go            # Filebeat/ES 兼容接收、上下文缓冲
├── watcher.go           # 本地文件持续监控采集
├── collector.go         # 日志收集、级别分类
├── scheduler.go         # 通用定时调度器
├── llm.go               # LLM API 调用（多提供商）
├── notify.go            # 通知分发（飞书/企微/钉钉/Webhook）
├── templates/
│   └── index.html       # Web 管理面板（SPA）
├── Dockerfile
├── docker-compose.yml
├── .env.example
└── go.mod
```
