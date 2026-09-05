# GK

GK 是一个可直接开始写业务的 Go + React 全栈模板。它保留了大型 Go 项目里真正有价值的结构：单一组合根、按领域组织、类型安全 SQL、OpenAPI 合约、统一生命周期和可观测性，但不预装与业务无关的框架。

## 快速开始

要求：`mise`、Docker。

```bash
cp .env.example .env
mise run setup
mise run dev
```

打开 <http://localhost:5173>。Vite 会把 `/api` 和 `/health` 代理到 `gkd`；生产构建则把 React 静态资源嵌入同一个 Go 二进制。

## 常用命令

```bash
mise run generate                  # sqlc + Go HTTP bindings + TypeScript SDK
mise run build                     # dist/bin/gkd
mise run test                      # Go + React tests
mise run lint
mise run format
mise run db:migrate:new -- add_x   # 新建迁移
mise run docker:observability      # PostgreSQL + OTel + Jaeger + Prometheus + Grafana
mise run docker:down
```

## 结构

```text
cmd/gkd                  进程入口
internal/app             唯一组合根与生命周期
internal/platform        配置、日志、HTTP、数据库、可观测性
internal/task            可删除的完整业务示例
api                      OpenAPI 合约和生成的 Go 绑定
db/migrations            Goose 迁移（编译进二进制）
db/queries               sqlc 查询
web                      React 应用和生成的 TypeScript 客户端
deploy                   Docker 与本地可观测性栈
```

业务代码按领域放在 `internal/<domain>` 中。HTTP 和 PostgreSQL 是业务模块的 adapter；不要把整个项目横向拆成 `controllers/services/repositories`。

## 配置

应用完全通过环境变量配置。开发环境复制 `.env.example`；生产环境由运行平台注入。不要提交 `.env`。

设置 `OTEL_EXPORTER_OTLP_ENDPOINT` 后，服务会导出 traces 和 metrics。日志始终写到 stdout，并自动关联请求的 `trace_id` 和 `span_id`。

健康检查：

- `GET /health/live`：进程存活。
- `GET /health/ready`：数据库连接可用。

## 添加一个业务模块

1. 先在 `api/openapi.yaml` 定义接口。
2. 创建数据库迁移，并在 `db/queries` 写查询。
3. 运行 `mise run generate`。
4. 在 `internal/<domain>` 中实现模型、业务逻辑和 transport/storage adapter。
5. 在 `internal/app/app.go` 组合依赖并挂载路由。

业务模块应返回自己的领域类型，不要向 HTTP 层泄露 sqlc 生成的数据库行类型。
