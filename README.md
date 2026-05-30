# OpsPanel

通用运维工具平台。内置日志分析告警模块：定时收集日志，调用 LLM 分析错误，推送结果到飞书/企业微信/钉钉/自定义 Webhook。

内置用户认证和角色权限，支持多数据库后端，后续可扩展更多运维功能模块。

## 功能

- **用户认证**：JWT 登录，支持 admin/viewer 两种角色
- **多数据库**：默认 SQLite（零配置），可切换 MySQL/PostgreSQL
- **多日志源**：配置任意数量的日志目录，支持文件名 glob 和行过滤正则
- **多日志格式**：纯文本、JSON 结构化日志、多行堆栈日志
- **多通知渠道**：飞书、企业微信、钉钉、通用 Webhook
- **LLM 分析**：兼容 OpenAI chat/completions 格式的任意 API
- **问题追踪**：LLM 输出结构化问题列表，按优先级排序
- **Web 管理面板**：所有配置通过浏览器完成
- **定时调度**：内置 Cron 调度器
- **容器化部署**：单个 Docker 镜像，挂载日志目录即可运行

## 快速开始

```bash
cp .env.example .env    # 编辑 LOG_DIR 和 JWT_SECRET
docker compose up -d
```

访问 `http://localhost:9090`，使用默认账号登录：
- 用户名：`admin`
- 密码：`admin123`

首次登录后建议修改密码。

## 角色权限

| 操作 | admin | viewer |
|------|-------|--------|
| 查看仪表盘/历史/问题 | ✓ | ✓ |
| 查看配置（脱敏） | ✓ | ✓ |
| 修改配置 | ✓ | ✗ |
| 手动触发分析 | ✓ | ✗ |
| 管理用户 | ✓ | ✗ |
| 测试通知/采集/LLM | ✓ | ✗ |

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `LOG_DIR` | 宿主机日志目录（必填） | — |
| `LISTEN_PORT` | 监听端口 | `9090` |
| `DB_DRIVER` | 数据库类型 | `sqlite` |
| `DB_DSN` | 数据库连接串 | `/data/sentinel.db` |
| `JWT_SECRET` | JWT 签名密钥 | 内置默认值 |
| `LOG_SENTINEL_LLM_URL` | LLM API 地址 | — |
| `LOG_SENTINEL_LLM_KEY` | LLM API Key | — |
| `LOG_SENTINEL_LLM_MODEL` | 模型名称 | — |
| `LOG_SENTINEL_SCHEDULE_ENABLED` | 启用定时 | `false` |
| `LOG_SENTINEL_SCHEDULE_CRON` | Cron 表达式 | `0 9 * * *` |
| `LOG_SENTINEL_NOTIFY_TYPE` | 快速通知类型 | — |
| `LOG_SENTINEL_NOTIFY_WEBHOOK` | 快速通知 URL | — |

## 数据库配置

```env
# SQLite（默认，零配置）
DB_DRIVER=sqlite
DB_DSN=/data/sentinel.db

# MySQL
DB_DRIVER=mysql
DB_DSN=user:pass@tcp(127.0.0.1:3306)/log_sentinel?charset=utf8mb4&parseTime=True

# PostgreSQL
DB_DRIVER=postgres
DB_DSN=host=127.0.0.1 user=admin password=pass dbname=log_sentinel port=5432 sslmode=disable
```

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

### 日志分析

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/config` | viewer | 获取配置 |
| POST | `/api/config` | admin | 保存配置 |
| GET | `/api/status` | viewer | 仪表盘状态 |
| POST | `/api/trigger` | admin | 手动触发 |
| GET | `/api/history` | viewer | 执行历史 |
| GET | `/api/issues` | viewer | 问题列表 |
| POST | `/api/issues` | admin | 更新问题状态 |
| POST | `/api/test-notify` | admin | 测试通知 |
| POST | `/api/test-collect` | admin | 测试采集 |
| POST | `/api/test-llm` | admin | 测试 LLM |

## 目录结构

```
log-sentinel/
├── main.go              # 入口、Gin 路由、调度
├── config.go            # 应用配置、JSON 持久化
├── database.go          # 数据库初始化、多驱动支持
├── model.go             # GORM 模型（User）
├── auth.go              # JWT 生成/验证、密码哈希
├── middleware.go        # Gin 中间件（认证、角色）
├── handler_auth.go      # 认证和用户管理路由
├── handler_sentinel.go  # 日志分析相关路由
├── collector.go         # 日志收集
├── llm.go              # LLM API 调用
├── notify.go           # 通知分发
├── templates/
│   └── index.html      # Web 管理面板（SPA）
├── Dockerfile
├── docker-compose.yml
├── .env.example
└── go.mod
```
