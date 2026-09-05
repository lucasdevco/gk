# GK

[English](README.md) · 简体中文

GK 是一个 Go + React 全栈开发模板，内置 PostgreSQL、类型安全 SQL、OpenAPI 合约、结构化日志和 OpenTelemetry。前端构建产物嵌入 Go 二进制，应用可作为单个可执行文件部署，数据库独立运行。

## 为什么选择 GK

- **从可运行的功能开始。** 可删除的任务示例贯通 React 页面、HTTP 接口、业务逻辑、SQL 查询和数据库迁移，可以作为新增业务的参考。
- **让改动集中在业务模块。** 代码按 `internal/<domain>` 组织，在一个位置完成依赖组装。领域模型与 HTTP 数据、数据库行类型分离，便于理解和测试功能。
- **保持明确的类型合约。** OpenAPI 生成 Go HTTP 绑定和 TypeScript 客户端代码，前端直接使用生成的领域类型；sqlc 从可阅读、可调优的 SQL 生成类型安全的 Go 查询。
- **单个应用二进制交付。** Go 嵌入 React 构建产物和 Goose 迁移文件，生产环境无需 Node.js 服务托管前端；PostgreSQL 仍然是独立依赖。
- **便于定位请求问题。** 结构化日志携带请求 ID，启用追踪后关联 Trace ID 和 Span ID。OpenTelemetry 导出 HTTP Trace 与指标、Go runtime 指标和 PostgreSQL 连接池指标。
- **统一开发流程。** mise 固定工具版本，提供代码生成、开发、构建、测试和迁移命令；GitHub Actions 工作流执行验证流程。

GK 适合内部工具、管理后台，以及需要 Go 后端和 React 前端的中小型 Web 应用。模板尚未包含认证和授权；对外提供私有数据或受保护操作前，需要补齐这些能力。

## 技术栈

| 领域 | 工具 |
| --- | --- |
| 后端 | Go、标准库 `net/http` |
| 前端 | React、TypeScript、Vite、Tailwind CSS、TanStack Query |
| 数据库 | PostgreSQL、pgx、sqlc、Goose |
| 接口合约 | OpenAPI、oapi-codegen、@hey-api/openapi-ts |
| 运行基础 | slog、OpenTelemetry、健康检查、优雅停机 |
| 开发工具 | mise、pnpm、Go tests、Vitest、Docker Compose |

## 快速开始

要求：mise、支持 Compose 的 Docker。启动开发环境前，请先启动 Docker。

```bash
git clone https://github.com/lucasdevco/gk.git
cd gk
cp .env.example .env
mise trust
mise install
mise run setup
mise run dev
```

打开 <http://localhost:5173>。开发命令启动 PostgreSQL、Go 后端和 Vite，Vite 将 `/api` 和 `/health` 代理到 8080 端口的后端。前端修改通过 Vite 刷新；修改 Go 代码后需要重启后端。

## 常用命令

```bash
mise run generate                 # 生成 SQL 查询、Go 绑定和 TypeScript SDK
mise run build                    # 构建前端和 dist/bin/gkd
mise run test                     # 运行 Go 和前端单元测试
mise run lint                     # 运行 go vet 和 TypeScript 检查
mise run format                   # 格式化 Go 和前端代码
mise run ci                       # 代码生成、检查、测试和构建
mise run db:migrate:new -- add_x   # 创建 SQL 迁移
mise run docker:observability     # 启动本地可观测性栈
mise run docker:down              # 停止容器，保留数据库卷
```

## 项目结构

```text
cmd/gkd                  进程入口
internal/app             依赖组装与应用生命周期
internal/platform        配置、日志、HTTP、数据库、可观测性
internal/task            可删除的业务示例
api                      OpenAPI 合约和生成的 Go 绑定
db/migrations            编译进二进制的 Goose 迁移
db/queries               sqlc 的 SQL 源文件
db/sqlc                  生成的 Go 查询代码
web                      React 应用和生成的 TypeScript 客户端
deploy                   Docker 与本地可观测性配置
```

每个业务模块包含自己的模型、业务逻辑和 HTTP/存储适配器。精简的 Repository 接口将业务逻辑与持久化分开，测试通过内存适配器验证行为。`internal/app` 负责构造依赖并挂载路由。

## 配置与可观测性

应用通过环境变量配置。mise 在本地命令中加载 `.env`；直接运行二进制时，需要通过 Shell 或部署平台注入环境变量。默认值见 [.env.example](.env.example)。后端启动时自动执行数据库迁移。

日志写入 stdout。设置 `LOG_FORMAT=json` 使用结构化输出，通过 `LOG_LEVEL` 控制日志级别。不要记录凭证或敏感请求内容。

本地启用遥测时，先运行 `mise run docker:observability`，再将以下配置加入 `.env` 并重启后端：

```dotenv
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_EXPORTER_OTLP_INSECURE=true
OTEL_SERVICE_NAME=gk
```

内置导出器使用 OTLP over gRPC。通过 <http://localhost:16686> 的 Jaeger 查看 Trace，通过 <http://localhost:9090> 的 Prometheus 查看指标，Grafana 位于 <http://localhost:3000>。Grafana 数据源和仪表盘需要另外配置。日志保留在 stdout，不通过 OTLP 导出。

- `GET /health/live`：进程存活检查。
- `GET /health/ready`：数据库连接检查。

## 错误处理

在 `internal/<domain>/errors.go` 中使用 `errors.New` 定义领域错误，通过 `%w` 保留错误链，在 HTTP 适配器中使用 `errors.Is/As` 映射预期错误。业务逻辑不依赖 HTTP 状态码。

公共 HTTP 函数统一返回 `{ "error": { "code": "task_not_found", "message": "task not found" } }`，可附带 `requestId` 和安全的 `details`。HTTP 状态码表达协议层结果；`code` 是独立、稳定的 snake_case 业务错误标识，前端可以据此处理具体错误。多个业务错误码可以映射到同一个 HTTP 状态码。未知错误记录完整原因，对外返回 HTTP 500 和 `code: "internal_error"`，不直接暴露内部错误。

## 添加业务模块

1. 在 `api/openapi.yaml` 定义接口合约。
2. 在 `db/` 下创建迁移和 SQL 查询。
3. 运行 `mise run generate`。
4. 在 `internal/<domain>` 中实现模型、业务逻辑和 HTTP/存储适配器。
5. 在 `internal/app/app.go` 组装依赖并注册路由。
6. 添加前端功能及行为测试。

将 sqlc 行类型保留在存储适配器内部。业务调用方使用领域类型，HTTP 适配器负责将其映射为生成的接口合约类型。
