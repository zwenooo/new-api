# codex-service-go

基于 Go 实现的多实例 Codex 代理服务，单端口、多协程运行，通过路径前缀（例如 `/1/v1/responses`）区分实例。项目当前提供：

- 基本的实例管理界面（Gin + html/template + HTMX）。
- SQLite 持久化，并自动迁移。
- `/v1/responses` 透传以及 `/v1/chat/completions`（基于 Responses API 的自动转换）。
- Docker 镜像脚手架，挂载 `auth/`、`log/`、`db/` 目录。

## 快速开始

```bash
# 进入项目目录
cd codex-service-go

# 按需修改 .env（可参考 .env.example）
cp .env.example .env

# 本地启动（需要 Go 1.21）
go run ./cmd/server
```

默认会在 `./db/codex.db` 创建 SQLite 数据库，在 `./auth`、`./log` 下生成实例文件。

访问 `http://localhost:8080/admin` 打开管理界面，可增删实例、查看内部令牌、执行登录或手动上传凭据。

- 点击 “Login” 启动本地 OAuth 登录流程，页面会给出授权链接，授权完成后凭据会自动写入 `auth/<实例ID>.json`。
- 若已有 `auth.json`，点击 “Auth” 可直接粘贴内容并保存。

## 与 Transfer API 主从同步

当你在本项目中批量创建/维护多个实例后，可让 Transfer API 作为“主”，自动拉取实例列表并在 Transfer API 中生成对应“渠道”。

- 从机（本项目）提供接口：`GET /internal/instances`
  - Header：`Authorization: Bearer <PROXY_INTERNAL_TOKEN>`
  - 返回：`revision` + 实例列表（`id`、`name`、`group`、`base_path`、`enabled`、`available`、`state`、`auth_mode`、`internal_token`）
  - `state` 可能为：`normal` / `cooldown` / `channel_backoff` / `transport_quarantine` / `expired` / `member_expired` / `stopped`
  - `cooldown` 仅表示上游明确返回的休眠/限流；`channel_backoff` 表示主机侧渠道退避；`transport_quarantine` 表示传输链路隔离
  - 当上游返回 `402 Payment Required`（例如 `detail.code=deactivated_workspace`）时会进入 `member_expired`，并在主机侧自动禁用对应渠道
- 从机（本项目）提供变更监听：`GET /internal/instances/watch?since=<revision>&timeout=<seconds>`
  - Header：`Authorization: Bearer <PROXY_INTERNAL_TOKEN>`
  - 返回：`revision` + `changed/reset`（长轮询，用于近实时同步）
  - `revision` 会在实例配置变更或运行态变更（例如冷却/会员过期）时更新
- 主机（Transfer API）开启环境变量：
  - `CODEX_SERVICE_GO_SYNC_ENABLED=true`
  - `CODEX_SERVICE_GO_SYNC_BASE_URL=http://<codex-service-go-host>:<port>`
  - `CODEX_SERVICE_GO_SYNC_TOKEN=<PROXY_INTERNAL_TOKEN>`

## Docker

```bash
# 进入项目目录（建议在 VPS 上放到独立目录，例如 /opt/codex-service-go）
cd codex-service-go

# 准备配置（至少设置 TOKEN / WEB_ADMIN_USER / WEB_ADMIN_PASS）
cp .env.example .env

# 构建本地镜像
IMAGE=codex-service-go:local
docker build -t "$IMAGE" .

# 启动（持久化 auth/log/db）
docker run -d --name codex-service-go --restart unless-stopped \
  --env-file .env \
  -p 8080:8080 \
  -v $(pwd)/auth:/app/auth \
  -v $(pwd)/log:/app/log \
  -v $(pwd)/db:/app/db \
  "$IMAGE"

# 更新时重新构建镜像并重建容器。
```

访问 `http://<VPS-IP>:8080/admin`。

注意：
- `TOKEN/PROXY_INTERNAL_TOKEN` 为空会导致鉴权可被 `Authorization: Bearer `（空 token）绕过（不要留空）。
- `WEB_ADMIN_USER/WEB_ADMIN_PASS` 为空会禁用后台登录保护（不建议在公网暴露）。

## 目录结构

- `cmd/server`: 服务入口。
- `internal/config`: `.env` 配置读取。
- `internal/db`: SQLite 打开与迁移。
- `internal/services`: 业务逻辑（实例、代理等）。
- `internal/handlers`: Gin Handler（管理界面、API）。
- `internal/server/templates`: HTML 模板。
- `internal/db/migrations`: 数据库迁移脚本。

## 下一步计划

- 在管理界面中接入自动化 ChatGPT OAuth 登录流程（当前需手动粘贴 `auth.json`）。
- 为实例提供更丰富的运维信息（日志查看、请求统计等）。
- 编写端到端测试与 CI 工作流。
