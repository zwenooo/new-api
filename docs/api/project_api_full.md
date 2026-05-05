# Transfer API `new-api` 接口总览

> 更新时间：2026-04-03
>
> 本文档基于 `router/main.go`、`router/api-router.go`、`router/relay-router.go`、`router/video-router.go`、`router/dashboard.go` 实际注册的路由整理，覆盖项目当前暴露的主要 HTTP / SSE / WebSocket 接口。
>
> 现有分项文档可继续参考：
> - `docs/api/api_auth.md`
> - `docs/api/v1_messages_codex_responses_compat.md`
> - `docs/api/v1_messages_codex_responses_ws_acceleration.md`

## 1. 接口分层

| 接口族 | 路径前缀 | 主要用途 | 典型鉴权 | 响应风格 |
|---|---|---|---|---|
| Web / Console API | `/api/**` | 控制台、站点配置、用户与后台管理 | Session 或 Access Token + `Transfer-Api-User` | 以 `{success,message,data}` 为主 |
| Dashboard API | `/dashboard/**`、`/v1/dashboard/**` | OpenAI 风格额度与用量查询 | 用户 Token | OpenAI billing 风格 JSON |
| Relay API | `/v1/**`、`/v1beta/**` | OpenAI / Anthropic / Gemini 兼容转发 | 用户 Token | 上游兼容 JSON / SSE / WS |
| Playground | `/pg/**` | 控制台内聊天调试 | 用户会话 / Access Token | OpenAI 风格 |
| 多媒体 / 任务 | `/v1/video/**`、`/kling/v1/**`、`/jimeng`、`/suno/**`、`/mj/**` | 视频、音乐、MJ 等任务代理 | 用户 Token | 任务状态 / 代理结果 |

## 2. 鉴权与通用约定

### 2.1 `/api/**` 鉴权

`UserAuth` / `AdminAuth` / `RootAuth` 支持两种方式：

1. 浏览器 Session Cookie
2. Access Token Header

Access Token 调用时必须同时提供：

```http
Authorization: Bearer <access_token>
Transfer-Api-User: <user_id>
```

兼容：

```http
Authorization: <access_token>
New-Api-User: <user_id>
```

说明：

- `AdminPageOrHeaderAuth` 额外支持仅靠浏览器 Session 打开下载页/新标签页。
- `/api/pricing` 使用 `TryUserAuth`，可匿名访问；登录后会返回更贴近当前用户分组的数据。

### 2.2 Relay / Dashboard Token 鉴权

`TokenAuth` 主要校验用户级 Token，常见写法：

```http
Authorization: Bearer sk-xxxx
```

兼容来源：

- Anthropic: `x-api-key`
- Gemini: `x-goog-api-key` 或查询串 `?key=...`
- Midjourney: `mj-api-secret`
- Realtime WebSocket: `Sec-WebSocket-Protocol: realtime, openai-insecure-api-key.sk-xxxx, openai-beta.realtime-v1`

### 2.3 其他专用鉴权

| 场景 | Header / 条件 |
|---|---|

### 2.4 通用响应

多数 `/api/**` 接口返回：

```json
{
  "success": true,
  "message": "",
  "data": {}
}
```

注意：

- 本项目很多业务失败场景仍会返回 HTTP `200`，但 `success=false`。
- Relay 接口不走这个包裹，直接返回 OpenAI / Anthropic / Gemini 兼容响应。
- `/dashboard/**` 返回 OpenAI billing 风格对象。

### 2.5 常见分页参数

多数列表接口通过 `common.GetPageQuery()` 读取分页，常用参数：

- `p`: 页码
- `page_size`: 每页大小
- 兼容 `ps`、`size`

## 3. 常用调用示例

### 3.1 读取当前用户资料

```bash
curl -X GET "https://<domain>/api/user/self" \
  -H "Authorization: Bearer <access_token>" \
  -H "New-Api-User: <user_id>"
```

### 3.2 转发 OpenAI Chat Completions

```bash
curl -X POST "https://<domain>/v1/chat/completions" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role":"user","content":"hello"}],
    "stream": false
  }'
```

## 4. 入口与公共接口

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/portable-latest.json` | 公开 | ClawBox 便携版更新描述文件 |

## 5. `/api` Web / Console 接口

### 5.1 初始化、状态与公共信息

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/setup` | 公开 | 查询系统是否已初始化、根用户是否已创建、数据库类型 |
| POST | `/api/setup` | 公开 | 首次初始化系统并创建 Root 账号 |
| GET | `/api/status` | 公开 | 站点运行状态、前端开关、OAuth 能力、公告/FAQ 等 |
| GET | `/api/uptime/status` | 公开 | Uptime-Kuma 探针状态 |
| GET | `/api/service_status/timeline.png` | 公开 | 服务状态时间线 PNG |
| GET | `/api/service_status/timeline.svg` | 公开 | 服务状态时间线 SVG |
| GET | `/api/service_status/timeline` | 用户 | 服务状态时间线明细 |
| GET | `/api/models` | 用户 | 控制台可见模型列表 |
| GET | `/api/status/test` | 管理员 | 测试数据库与 HTTP 统计状态 |
| GET | `/api/notice` | 公开 | 公告内容 |
| GET | `/api/about` | 公开 | 关于页内容 |
| GET | `/api/home_page_content` | 公开 | 首页自定义内容 |
| GET | `/api/pricing` | 公开 / 可识别当前用户 | 价格、供应商、分组倍率、支持端点、自动分组 |
| GET | `/api/verification` | 公开 | 发送邮箱验证码，受邮箱限流和 Turnstile 校验影响 |
| GET | `/api/reset_password` | 公开 | 发送重置密码邮件，受关键限流和 Turnstile 校验影响 |
| POST | `/api/user/reset` | 公开 | 提交重置密码 |
| GET | `/api/ratio_config` | 公开 | 查询倍率配置摘要 |
| POST | `/api/stripe/webhook` | 公开 | Stripe 支付回调 |

### 5.2 OAuth / 第三方登录

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/oauth/github` | 公开 | GitHub OAuth 登录入口 |
| GET | `/api/oauth/oidc` | 公开 | OIDC 登录入口 |
| GET | `/api/oauth/linuxdo` | 公开 | LinuxDo OAuth 登录入口 |
| GET | `/api/oauth/state` | 公开 | 生成 OAuth state |
| GET | `/api/oauth/wechat` | 公开 | 微信登录入口 |
| GET | `/api/oauth/wechat/bind` | 公开 | 微信绑定入口 |
| GET | `/api/oauth/email/bind` | 公开 | 邮箱绑定入口 |
| GET | `/api/oauth/telegram/login` | 公开 | Telegram 登录 |
| GET | `/api/oauth/telegram/bind` | 公开 | Telegram 绑定 |

### 5.3 ClawBox

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/clawbox/bootstrap` | 公开 | ClawBox 启动配置 |
| GET | `/api/clawbox/relay-token` | 用户 | 获取 ClawBox Relay Token |
| POST | `/api/clawbox/activation/check` | 公开 | 校验激活码，受关键限流保护 |
| POST | `/api/clawbox/register` | 公开 | 注册 ClawBox，受关键限流保护 |
| GET | `/api/clawbox/update/bundled-latest.json` | 公开 | 获取捆绑版更新描述 |
| PUT | `/api/clawbox/update/bundled-latest.json` | 管理员 | 更新捆绑版更新描述 |
| GET | `/api/clawbox/update/portable-latest.json` | 公开 | 获取便携版更新描述 |
| GET | `/api/clawbox/update/portable/releases/:id/download` | 公开 | 下载指定便携版发布包 |
| POST | `/api/clawbox/auth/verify` | 用户 | 校验设备授权 |
| POST | `/api/clawbox/auth/unregister-device` | 用户 | 解绑设备 |
| GET | `/api/clawbox/update/portable/github-token` | 管理员 | 查看 Portable GitHub Token 配置状态，不回显明文 |
| PUT | `/api/clawbox/update/portable/github-token` | 管理员 | 保存或覆盖 Portable GitHub Token |
| DELETE | `/api/clawbox/update/portable/github-token` | 管理员 | 清空后台保存的 Portable GitHub Token |
| GET | `/api/clawbox/update/portable/releases` | 管理员 | 发布列表 |
| POST | `/api/clawbox/update/portable/releases` | 管理员 | 创建发布 |
| POST | `/api/clawbox/update/portable/releases/sync/github` | 管理员 | 从 GitHub 同步发布 |
| POST | `/api/clawbox/update/portable/releases/:id/activate` | 管理员 | 激活某个发布版本 |

### 5.4 用户公共接口

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | `/api/user/register` | 公开 | 注册账号，受关键限流与 Turnstile 保护 |
| POST | `/api/user/login` | 公开 | 用户名密码登录，可能返回 `require_2fa=true` |
| POST | `/api/user/login/2fa` | 公开 | 提交二次验证码完成登录 |
| GET | `/api/user/logout` | 公开 | 清理当前 Session；有会话时才有实际效果 |
| GET | `/api/user/epay/notify` | 公开 | 易支付回调 |
| POST | `/api/user/epay/notify` | 公开 | 易支付回调 |
| GET | `/api/user/groups` | 公开 | 查询可见用户分组列表 |

### 5.5 当前用户接口

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/user/self/groups` | 用户 | 当前用户可用分组 |
| GET | `/api/user/self` | 用户 | 当前用户资料 |
| POST | `/api/user/self/subscriptions/:subId/activate` | 用户 | 激活某个订阅 |
| POST | `/api/user/self/request_subscriptions/:subId/activate` | 用户 | 激活某个次数订阅 |
| PUT | `/api/user/avatar` | 用户 | 更新头像 |
| GET | `/api/user/models` | 用户 | 当前用户可用模型 |
| PUT | `/api/user/self` | 用户 | 修改个人资料 |
| DELETE | `/api/user/self` | 用户 | 注销当前账号 |
| GET | `/api/user/token` | 用户 | 生成 Access Token |
| GET | `/api/user/aff` | 用户 | 获取邀请码 / 推广码 |
| GET | `/api/user/aff/records` | 用户 | 邀请转化与返佣记录 |
| GET | `/api/user/balance/records` | 用户 | 余额记录 |
| GET | `/api/user/topup/info` | 用户 | 充值说明 / 可用支付信息 |
| POST | `/api/user/topup` | 用户 | 账户充值 |
| POST | `/api/user/pay` | 用户 | 发起易支付订单 |
| POST | `/api/user/amount` | 用户 | 余额支付 |
| POST | `/api/user/stripe/pay` | 用户 | 发起 Stripe 支付 |
| POST | `/api/user/stripe/amount` | 用户 | 计算 Stripe 支付金额 |
| POST | `/api/user/aff_transfer` | 用户 | 推广返利转余额 |
| PUT | `/api/user/setting` | 用户 | 更新个人设置 |
| GET | `/api/user/2fa/status` | 用户 | 查看 2FA 状态 |
| POST | `/api/user/2fa/setup` | 用户 | 初始化 2FA |
| POST | `/api/user/2fa/enable` | 用户 | 启用 2FA |
| POST | `/api/user/2fa/disable` | 用户 | 关闭 2FA |
| POST | `/api/user/2fa/backup_codes` | 用户 | 重置 2FA 备用恢复码 |

### 5.6 用户管理接口

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/user/` | 管理员 | 分页用户列表 |
| GET | `/api/user/search` | 管理员 | 搜索用户 |
| GET | `/api/user/:id` | 管理员 | 单个用户详情 |
| POST | `/api/user/` | 管理员 | 创建用户 |
| POST | `/api/user/manage` | 管理员 | 冻结、重置等管理动作 |
| PUT | `/api/user/` | 管理员 | 更新用户 |
| DELETE | `/api/user/:id` | 管理员 | 删除用户 |
| GET | `/api/user/:id/subscriptions` | 管理员 | 用户订阅列表 |
| POST | `/api/user/:id/subscriptions` | 管理员 | 创建用户订阅 |
| POST | `/api/user/:id/subscriptions/preset` | 管理员 | 按预设创建用户订阅 |
| PATCH | `/api/user/:id/subscriptions/reorder` | 管理员 | 调整用户订阅顺序 |
| PATCH | `/api/user/:id/subscriptions/:subId` | 管理员 | 更新用户订阅 |
| DELETE | `/api/user/:id/subscriptions/:subId` | 管理员 | 删除用户订阅 |
| GET | `/api/user/:id/request_subscriptions` | 管理员 | 用户次数订阅列表 |
| POST | `/api/user/:id/request_subscriptions` | 管理员 | 创建用户次数订阅 |
| POST | `/api/user/:id/request_subscriptions/preset` | 管理员 | 按预设创建次数订阅 |
| PATCH | `/api/user/:id/request_subscriptions/reorder` | 管理员 | 调整次数订阅顺序 |
| PATCH | `/api/user/:id/request_subscriptions/:subId` | 管理员 | 更新次数订阅 |
| DELETE | `/api/user/:id/request_subscriptions/:subId` | 管理员 | 删除次数订阅 |
| POST | `/api/user/:id/payg/topup` | 管理员 | 给用户增加按量余额 |
| POST | `/api/user/:id/payg/topup/group` | 管理员 | 给指定分组增加按量余额 |
| PATCH | `/api/user/:id/payg/balances/reorder` | 管理员 | 调整按量余额顺序 |
| PATCH | `/api/user/:id/payg/balances/:productId` | 管理员 | 修改余额允许分组 |
| DELETE | `/api/user/:id/payg/balances/:productId` | 管理员 | 删除某个按量余额 |
| POST | `/api/user/subscriptions/bulk/duration` | 管理员 | 批量延长订阅 |
| POST | `/api/user/subscriptions/bulk/compensation` | 管理员 | 批量补偿订阅 |
| POST | `/api/user/subscriptions/bulk/original-compensation` | 管理员 | 批量延长原始订阅 |
| GET | `/api/user/2fa/stats` | 管理员 | 2FA 统计 |
| DELETE | `/api/user/:id/2fa` | 管理员 | 管理员强制关闭某用户 2FA |

### 5.7 订阅与支付

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/subscription/plans` | 用户 | 查询可购买订阅计划 |
| POST | `/api/subscription/order` | 用户 | 创建订阅订单 |
| GET | `/api/subscription/order/status` | 用户 | 查询订阅订单状态 |
| GET | `/api/subscription/epay/notify` | 公开 | 订阅易支付回调 |
| POST | `/api/subscription/epay/notify` | 公开 | 订阅易支付回调 |
| GET | `/api/subscription/epay/return` | 公开 | 订阅支付回跳 |
| GET | `/api/subscription/plans/all` | Root | 查询全部订阅计划 |
| POST | `/api/subscription/plans` | Root | 创建订阅计划 |
| PUT | `/api/subscription/plans/:id` | Root | 更新订阅计划 |
| DELETE | `/api/subscription/plans/:id` | Root | 删除订阅计划 |
| POST | `/api/payg/order` | 用户 | 创建按量付费订单 |
| GET | `/api/payg/order/status` | 用户 | 查询按量付费订单状态 |
| GET | `/api/payg/epay/checkout` | 公开 | 按量易支付收银台 |
| GET | `/api/payg/epay/notify` | 公开 | 按量易支付回调 |
| POST | `/api/payg/epay/notify` | 公开 | 按量易支付回调 |
| GET | `/api/payg/epay/return` | 公开 | 按量支付回跳 |
| POST | `/api/pay_request/order` | 用户 | 创建请求次数付费订单 |
| GET | `/api/pay_request/order/status` | 用户 | 查询请求次数付费订单状态 |
| GET | `/api/pay_request/epay/notify` | 公开 | 请求次数付费回调 |
| POST | `/api/pay_request/epay/notify` | 公开 | 请求次数付费回调 |
| GET | `/api/pay_request/epay/return` | 公开 | 请求次数付费回跳 |
| POST | `/api/pay_token/order` | 用户 | 创建 token 计量付费订单 |
| GET | `/api/pay_token/order/status` | 用户 | 查询 token 计量付费订单状态 |
| GET | `/api/pay_token/epay/notify` | 公开 | token 计量付费回调 |
| POST | `/api/pay_token/epay/notify` | 公开 | token 计量付费回调 |
| GET | `/api/pay_token/epay/return` | 公开 | token 计量付费回跳 |

### 5.8 订单、商品与系统级配置

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/order/subscriptions` | 管理员 + 订单模块权限 | 订阅订单列表 |
| GET | `/api/order/topups` | 管理员 + 订单模块权限 | 充值订单列表 |
| GET | `/api/order/paygs` | 管理员 + 订单模块权限 | 按量订单列表 |
| GET | `/api/order/pay_requests` | 管理员 + 订单模块权限 | 请求次数订单列表 |
| GET | `/api/order/pay_tokens` | 管理员 + 订单模块权限 | token 计量订单列表 |
| PUT | `/api/product_management/option` | 管理员 + 商品管理模块权限 | 更新商品管理配置 |
| GET | `/api/product_management/presets` | 管理员 + 商品管理模块权限 | 商品预设列表 |
| POST | `/api/product_management/presets` | 管理员 + 商品管理模块权限 | 新增或更新商品预设 |
| GET | `/api/product_management/presets/:id/revisions` | 管理员 + 商品管理模块权限 | 预设修订历史 |
| POST | `/api/product_management/presets/:id/restore` | 管理员 + 商品管理模块权限 | 回滚预设版本 |
| GET | `/api/product_management/pay_products/:type/:id/revisions` | 管理员 + 商品管理模块权限 | 支付商品修订历史 |
| POST | `/api/product_management/pay_products/:type/:id/restore` | 管理员 + 商品管理模块权限 | 回滚支付商品版本 |
| DELETE | `/api/product_management/presets/:id` | 管理员 + 商品管理模块权限 | 删除商品预设 |
| POST | `/api/product_management/presets/generate` | 管理员 + 商品管理模块权限 | 生成商品预设兑换码 |
| GET | `/api/option/` | Root | 读取站点选项 |
| PUT | `/api/option/` | Root | 更新站点选项 |
| POST | `/api/option/rest_model_ratio` | Root | 重置模型倍率 |
| POST | `/api/option/migrate_console_setting` | Root | 迁移旧控制台配置键 |

### 5.9 Root 级扩展管理

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/cx_compat/opencode/instructions` | Root | 读取 OpenCode 指令兼容配置 |
| POST | `/api/cx_compat/opencode/instructions/sync` | Root | 同步 OpenCode 指令 |
| POST | `/api/cx_compat/opencode/instructions/sync/github` | Root | 从 GitHub 同步 OpenCode 指令 |
| GET | `/api/cx_compat/opencode/instructions/github/branches` | Root | 查询 GitHub 分支 |
| GET | `/api/cx_compat/opencode/instructions/github/commits` | Root | 查询 GitHub 提交 |
| POST | `/api/cx_compat/opencode/instructions/pin_default` | Root | 固定默认指令版本 |
| POST | `/api/cx_compat/opencode/instructions/restore_default` | Root | 恢复默认指令 |
| GET | `/api/ratio_sync/channels` | Root | 获取可同步渠道 |
| POST | `/api/ratio_sync/fetch` | Root | 从上游拉取倍率配置 |

说明：
- `GET /api/ratio_sync/channels` 会返回普通渠道，以及内置的“官方倍率预设”“OpenRouter 官方模型价格”等特殊同步源。
- `POST /api/ratio_sync/fetch` 的 `upstreams[]` 支持 `source_type`、`endpoint`、`bearer_token`；当 `source_type=openrouter_models` 时，后端会优先使用请求里的 `bearer_token`，未提供时回退到系统设置 `OpenRouterPriceSyncToken`，再把 OpenRouter `/api/v1/models` 的价格结构换算为本地倍率配置结构，并包含 `create_cache_ratio`。

### 5.10 渠道管理

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/channel/` | 管理员 | 渠道列表 |
| GET | `/api/channel/search` | 管理员 | 搜索渠道 |
| GET | `/api/channel/models` | 管理员 | 渠道模型能力 |
| GET | `/api/channel/models_enabled` | 管理员 | 启用模型列表 |
| GET | `/api/channel/abnormal_consume/config` | 管理员 | 异常消耗配置 |
| PUT | `/api/channel/abnormal_consume/config` | 管理员 | 更新异常消耗配置 |
| GET | `/api/channel/abnormal_consume` | 管理员 | 异常消耗记录 |
| DELETE | `/api/channel/abnormal_consume` | 管理员 | 清空异常消耗记录 |
| GET | `/api/channel/:id` | 管理员 | 单个渠道详情 |
| GET | `/api/channel/:id/profit_stats/daily` | 管理员 | 渠道日利润统计 |
| GET | `/api/channel/:id/request_stats/daily` | 管理员 | 渠道日请求统计 |
| POST | `/api/channel/:id/key` | 管理员 | 获取渠道 key，受关键限流与禁缓存保护 |
| GET | `/api/channel/test` | 管理员 | 测试全部渠道 |
| GET | `/api/channel/test/:id` | 管理员 | 测试单渠道 |
| POST | `/api/channel/test_proxy` | 管理员 | 测试代理 |
| GET | `/api/channel/update_balance` | 管理员 | 批量刷新余额 |
| GET | `/api/channel/update_balance/:id` | 管理员 | 刷新指定渠道余额 |
| POST | `/api/channel/` | 管理员 | 新增渠道 |
| PUT | `/api/channel/` | 管理员 | 更新渠道 |
| DELETE | `/api/channel/disabled` | 管理员 | 删除已禁用渠道 |
| POST | `/api/channel/tag/disabled` | 管理员 | 批量禁用指定标签渠道 |
| POST | `/api/channel/tag/enabled` | 管理员 | 批量启用指定标签渠道 |
| PUT | `/api/channel/tag` | 管理员 | 批量编辑标签 |
| DELETE | `/api/channel/:id` | 管理员 | 删除渠道 |
| POST | `/api/channel/reset_used_quota/:id` | 管理员 | 重置渠道已用额度 |
| POST | `/api/channel/batch` | 管理员 | 批量删除渠道 |
| POST | `/api/channel/batch/reset_used_quota` | 管理员 | 批量重置已用额度 |
| POST | `/api/channel/batch/group` | 管理员 | 批量设置分组 |
| POST | `/api/channel/batch/models` | 管理员 | 批量更新模型列表 |
| POST | `/api/channel/batch/bind_users` | 管理员 | 批量绑定用户 |
| POST | `/api/channel/fix` | 管理员 | 修复渠道能力表 |
| GET | `/api/channel/fetch_models/:id` | 管理员 | 拉取单渠道模型 |
| POST | `/api/channel/fetch_models` | 管理员 | 批量拉取模型 |
| POST | `/api/channel/batch/tag` | 管理员 | 批量设置标签 |
| GET | `/api/channel/tag/models` | 管理员 | 查询标签可用模型 |
| POST | `/api/channel/copy/:id` | 管理员 | 复制渠道 |
| POST | `/api/channel/multi_key/manage` | 管理员 | 管理多 Key 渠道 |

### 5.11 Token 与 Token 用量

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/token/` | 用户 | Token 列表 |
| GET | `/api/token/search` | 用户 | 搜索 Token |
| GET | `/api/token/:id` | 用户 | 单个 Token |
| POST | `/api/token/` | 用户 | 创建 Token |
| PUT | `/api/token/` | 用户 | 更新 Token |
| DELETE | `/api/token/:id` | 用户 | 删除 Token |
| POST | `/api/token/batch` | 用户 | 批量删除 Token |
| GET | `/api/usage/token/` | TokenAuth | 查询当前 Bearer Token 的额度/使用信息 |

### 5.12 兑换码、日志、请求追踪与数据统计

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/redemption/` | 管理员 | 兑换码列表 |
| GET | `/api/redemption/search` | 管理员 | 搜索兑换码 |
| GET | `/api/redemption/presets` | 管理员 | 兑换码预设列表 |
| POST | `/api/redemption/presets` | 管理员 | 新增或更新兑换码预设 |
| GET | `/api/redemption/presets/:id/revisions` | 管理员 | 预设版本历史 |
| POST | `/api/redemption/presets/:id/restore` | 管理员 | 恢复预设版本 |
| DELETE | `/api/redemption/presets/:id` | 管理员 | 删除预设 |
| POST | `/api/redemption/presets/generate` | 管理员 | 通过预设批量生成兑换码 |
| POST | `/api/redemption/payg/generate` | 管理员 | 生成按量兑换码 |
| GET | `/api/redemption/:id` | 管理员 | 单个兑换码详情 |
| POST | `/api/redemption/` | 管理员 | 创建兑换码 |
| PUT | `/api/redemption/` | 管理员 | 更新兑换码 |
| POST | `/api/redemption/batch/status` | 管理员 | 批量更新状态 |
| DELETE | `/api/redemption/invalid` | 管理员 | 删除无效兑换码 |
| DELETE | `/api/redemption/:id` | 管理员 | 删除兑换码 |
| GET | `/api/log/` | 管理员 | 全量日志 |
| DELETE | `/api/log/` | 管理员 | 删除历史日志 |
| GET | `/api/log/stat` | 管理员 | 日志统计 |
| GET | `/api/log/king_rank` | 公开 | 每日 Token 榜单 |
| GET | `/api/log/cache_stat` | 管理员 | 日志缓存统计 |
| GET | `/api/log/self/stat` | 用户 | 我的日志统计 |
| GET | `/api/log/self/cache_stat` | 用户 | 我的缓存统计 |
| GET | `/api/log/global/cache_stat` | 用户 | 全局缓存统计 |
| GET | `/api/log/search` | 管理员 | 搜索全量日志 |
| GET | `/api/log/self` | 用户 | 我的日志列表 |
| GET | `/api/log/self/search` | 用户 | 搜索我的日志 |
| GET | `/api/log/token` | 公开 / CORS | 按 Token 查询日志 |
| GET | `/api/request_trace/object` | 管理员页面或 Header 鉴权 | 下载 / 打开请求追踪对象 |
| GET | `/api/request_trace/:request_id` | 管理员 | 查询请求追踪详情 |
| GET | `/api/data/` | 管理员 | 全站按日用量 |
| GET | `/api/data/self` | 用户 | 当前用户按日用量 |

#### 5.12.1 `GET /api/log/`

管理员日志列表接口，支持分页和条件筛选。控制台 `/console/log` 的管理员列表就是调用它。

鉴权：

- `AdminAuth`

请求参数：

| 参数 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `p` | int | 否 | 页码，从 `1` 开始，默认 `1` |
| `page_size` | int | 否 | 每页数量，默认项目分页大小，最大 `100` |
| `ps` | int | 否 | `page_size` 兼容别名 |
| `size` | int | 否 | `page_size` 兼容别名 |
| `type` | int | 否 | 日志类型，`0` 表示全部 |
| `start_timestamp` | int64 | 否 | 起始时间，Unix 秒 |
| `end_timestamp` | int64 | 否 | 结束时间，Unix 秒 |
| `username` | string | 否 | 用户名，精确匹配 |
| `token_name` | string | 否 | Token 名称，精确匹配 |
| `model_name` | string | 否 | 模型名，SQL `LIKE` 查询；如需模糊匹配可自行传 `%gpt%` |
| `request_id` | string | 否 | 请求 ID，精确匹配 |
| `channel` | int | 否 | 渠道 ID |
| `group_id` | int | 否 | 分组 ID |
| `group` | int | 否 | `group_id` 兼容别名 |

`type` 枚举：

| 值 | 含义 |
|---|---|
| `0` | 全部，仅查询参数使用 |
| `1` | 充值 |
| `2` | 消费 |
| `3` | 管理 |
| `4` | 系统 |
| `5` | 错误 |
| `6` | 消费进行中 |

响应结构：

```json
{
  "success": true,
  "message": "",
  "data": {
    "page": 1,
    "page_size": 20,
    "total": 1234,
    "items": [
      {
        "id": 1001,
        "user_id": 12,
        "created_at": 1743650000,
        "type": 2,
        "content": "",
        "username": "admin",
        "token_name": "default",
        "model_name": "gpt-4o-mini",
        "quota": 12345,
        "prompt_tokens": 100,
        "completion_tokens": 20,
        "use_time": 3,
        "is_stream": false,
        "channel": 7,
        "channel_name": "OpenAI-A",
        "token_id": 88,
        "group": "1",
        "ip": "1.2.3.4",
        "request_id": "req_xxx",
        "other": "{\"request_method\":\"POST\",\"request_path\":\"/v1/chat/completions\"}"
      }
    ]
  }
}
```

`items[]` 字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | int | 日志主键 |
| `user_id` | int | 用户 ID |
| `created_at` | int64 | 记录时间，Unix 秒 |
| `type` | int | 日志类型 |
| `content` | string | 日志文本 |
| `username` | string | 用户名 |
| `token_name` | string | Token 名 |
| `model_name` | string | 模型名 |
| `quota` | int | 扣费额度 |
| `prompt_tokens` | int | 输入 tokens |
| `completion_tokens` | int | 输出 tokens |
| `use_time` | int | 用时，秒 |
| `is_stream` | bool | 是否流式 |
| `channel` | int | 渠道 ID |
| `channel_name` | string | 渠道名称 |
| `token_id` | int | Token ID |
| `group` | string | 分组 ID 字符串 |
| `ip` | string | 请求 IP |
| `request_id` | string | 请求 ID |
| `other` | string | JSON 字符串，页面中的很多“常规项”都从这里解析 |

`other` 中常见的可解析字段：

- `prompt_cache_key`
- `session_id`
- `conversation_id`
- `quota_bucket`
- `request_method`
- `request_path`
- `request_ua`
- `cache_tokens`
- `cache_creation_tokens`
- `cache_creation_tokens_5m`
- `cache_creation_tokens_1h`
- `stream_exit_reason`
- `stream_exit_error`
- `reasoning_effort`
- `service_tier`
- `service_tier_multiplier`
- `is_model_mapped`
- `upstream_model_name`
- `admin_info.request_headers`
- `admin_info.request_content_length`
- `admin_info.use_channel`

说明：

- 返回结果按 `logs.id desc` 倒序。
- `channel_name` 会在查询后按 `channel` 批量回填。
- 这是外部分析推荐使用的日志列表接口，因为字段最完整。

#### 5.12.2 `GET /api/log/self`

当前用户自己的分页日志列表。

和 `GET /api/log/` 的主要差异：

- 鉴权为 `UserAuth`
- 不支持 `username` 和 `channel` 过滤
- 默认会排除管理日志
- 返回前会清理 `other.admin_info`、价格/倍率等字段，并对 `id` 做掩码处理

因此如果你要做与管理员 `/console/log` 尽量一致的外部分析，优先使用 `GET /api/log/`。

### 5.13 分组、预填组、任务、供应商、模型元数据

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/api/group/resolve` | 用户 | 解析当前用户可用分组 |
| GET | `/api/group/no_billing/product_options` | 管理员 | 无计费分组的商品选项 |
| GET | `/api/group/` | 管理员 | 分组列表 |
| POST | `/api/group/` | 管理员 | 创建分组 |
| PUT | `/api/group/` | 管理员 | 更新分组 |
| DELETE | `/api/group/:id` | 管理员 | 删除分组 |
| GET | `/api/prefill_group/` | 管理员 | 预填组列表 |
| POST | `/api/prefill_group/` | 管理员 | 创建预填组 |
| PUT | `/api/prefill_group/` | 管理员 | 更新预填组 |
| DELETE | `/api/prefill_group/:id` | 管理员 | 删除预填组 |
| GET | `/api/mj/self` | 用户 | 当前用户 Midjourney 任务 |
| GET | `/api/mj/` | 管理员 | 全量 Midjourney 任务 |
| GET | `/api/task/self` | 用户 | 当前用户任务列表 |
| GET | `/api/task/` | 管理员 | 全量任务列表 |
| GET | `/api/vendors/` | 管理员 | 供应商列表 |
| GET | `/api/vendors/search` | 管理员 | 搜索供应商 |
| GET | `/api/vendors/:id` | 管理员 | 单个供应商元数据 |
| POST | `/api/vendors/` | 管理员 | 创建供应商元数据 |
| PUT | `/api/vendors/` | 管理员 | 更新供应商元数据 |
| DELETE | `/api/vendors/:id` | 管理员 | 删除供应商元数据 |
| GET | `/api/models/sync_upstream/preview` | 管理员 | 预览上游模型同步结果 |
| POST | `/api/models/sync_upstream` | 管理员 | 执行上游模型同步 |
| GET | `/api/models/missing` | 管理员 | 缺失模型列表 |
| GET | `/api/models/` | 管理员 | 模型元数据列表 |
| GET | `/api/models/search` | 管理员 | 搜索模型元数据 |
| GET | `/api/models/:id` | 管理员 | 单个模型元数据 |
| POST | `/api/models/` | 管理员 | 创建模型元数据 |
| PUT | `/api/models/` | 管理员 | 更新模型元数据 |
| DELETE | `/api/models/:id` | 管理员 | 删除模型元数据 |

## 6. Dashboard API

这些接口走 `TokenAuth`，返回 OpenAI billing 风格数据：

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/dashboard/billing/subscription` | TokenAuth | 当前用户 / Token 的额度摘要 |
| GET | `/v1/dashboard/billing/subscription` | TokenAuth | 同上，兼容 `v1` 路径 |
| GET | `/dashboard/billing/usage` | TokenAuth | 当前用户 / Token 的已用额度 |
| GET | `/v1/dashboard/billing/usage` | TokenAuth | 同上，兼容 `v1` 路径 |

## 7. Relay / 兼容代理接口

### 7.1 模型发现与 Playground

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/v1/models` | TokenAuth | 模型列表；会按 Header 自动判定 OpenAI / Anthropic / Gemini 风格 |
| GET | `/v1/models/:model` | TokenAuth | 单模型信息 |
| POST | `/v1/messages/count_tokens` | TokenAuth | Anthropic Messages token 统计工具接口 |
| GET | `/v1beta/models` | TokenAuth | Gemini 模型列表 |
| GET | `/v1beta/openai/models` | TokenAuth | OpenAI 兼容模型列表 |
| POST | `/pg/chat/completions` | 用户 | 控制台 Playground 聊天调试，支持 `group_id` 指定分组 |

### 7.2 `/v1` 主 Relay

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/v1/realtime` | TokenAuth | OpenAI Realtime WebSocket 代理 |
| GET | `/v1/responses` | TokenAuth | OpenAI Responses WebSocket 代理 |
| POST | `/v1/messages` | TokenAuth | Anthropic Messages 兼容接口；命中启用 `messages_to_responses_compat` 的渠道时会在内部转到 `/v1/responses` |
| POST | `/v1/completions` | TokenAuth | OpenAI Completions |
| POST | `/v1/chat/completions` | TokenAuth | OpenAI Chat Completions；受 `chat_completions_enabled` 开关控制 |
| POST | `/v1/responses` | TokenAuth | OpenAI Responses |
| POST | `/v1/responses/compact` | TokenAuth | 紧凑版 Responses 入口 |
| POST | `/v1/edits` | TokenAuth | 图像 / 编辑类代理入口 |
| POST | `/v1/images/generations` | TokenAuth | 图片生成 |
| POST | `/v1/images/edits` | TokenAuth | 图片编辑 |
| POST | `/v1/embeddings` | TokenAuth | 向量嵌入 |
| POST | `/v1/audio/transcriptions` | TokenAuth | 音频转写 |
| POST | `/v1/audio/translations` | TokenAuth | 音频翻译 |
| POST | `/v1/audio/speech` | TokenAuth | 文本转语音 |
| POST | `/v1/rerank` | TokenAuth | Rerank |
| POST | `/v1/engines/:model/embeddings` | TokenAuth | Gemini 风格 embeddings 兼容入口 |
| POST | `/v1/models/*path` | TokenAuth | Gemini 风格模型动作代理 |
| POST | `/v1/moderations` | TokenAuth | 内容审核 |

附加说明：

- `/v1/**` 默认叠加 `Distribute()`，会结合用户分组、Token 限制、模型映射选择实际渠道。
- 流式请求会额外受 `ModelRequestConcurrencyLimit()` 限制。

### 7.3 当前未实现的 OpenAI 兼容占位接口

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | `/v1/images/variations` | TokenAuth | 当前返回未实现 |
| GET | `/v1/files` | TokenAuth | 当前返回未实现 |
| POST | `/v1/files` | TokenAuth | 当前返回未实现 |
| DELETE | `/v1/files/:id` | TokenAuth | 当前返回未实现 |
| GET | `/v1/files/:id` | TokenAuth | 当前返回未实现 |
| GET | `/v1/files/:id/content` | TokenAuth | 当前返回未实现 |
| POST | `/v1/fine-tunes` | TokenAuth | 当前返回未实现 |
| GET | `/v1/fine-tunes` | TokenAuth | 当前返回未实现 |
| GET | `/v1/fine-tunes/:id` | TokenAuth | 当前返回未实现 |
| POST | `/v1/fine-tunes/:id/cancel` | TokenAuth | 当前返回未实现 |
| GET | `/v1/fine-tunes/:id/events` | TokenAuth | 当前返回未实现 |
| DELETE | `/v1/models/:model` | TokenAuth | 当前返回未实现 |

### 7.4 Gemini、Suno、Midjourney、多媒体任务

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | `/v1beta/models/*path` | TokenAuth | Gemini API 兼容代理 |
| GET | `/mj/image/:id` | 公开 | Midjourney 图片获取 |
| GET | `/:mode/mj/image/:id` | 公开 | 带 `mode` 前缀的 Midjourney 图片获取 |
| POST | `/mj/submit/action` | TokenAuth | MJ 动作提交 |
| POST | `/mj/submit/shorten` | TokenAuth | MJ Shorten |
| POST | `/mj/submit/modal` | TokenAuth | MJ Modal |
| POST | `/mj/submit/imagine` | TokenAuth | MJ Imagine |
| POST | `/mj/submit/change` | TokenAuth | MJ Change |
| POST | `/mj/submit/simple-change` | TokenAuth | MJ Simple Change |
| POST | `/mj/submit/describe` | TokenAuth | MJ Describe |
| POST | `/mj/submit/blend` | TokenAuth | MJ Blend |
| POST | `/mj/submit/edits` | TokenAuth | MJ Edits |
| POST | `/mj/submit/video` | TokenAuth | MJ Video |
| POST | `/mj/notify` | TokenAuth | MJ 回调代理 |
| GET | `/mj/task/:id/fetch` | TokenAuth | 查询 MJ 任务 |
| GET | `/mj/task/:id/image-seed` | TokenAuth | 查询 MJ image seed |
| POST | `/mj/task/list-by-condition` | TokenAuth | 条件查询 MJ 任务 |
| POST | `/mj/insight-face/swap` | TokenAuth | InsightFace 换脸 |
| POST | `/mj/submit/upload-discord-images` | TokenAuth | 上传 Discord 图片 |
| POST | `/:mode/mj/submit/action` | TokenAuth | 带 `mode` 前缀的 MJ 动作提交 |
| POST | `/:mode/mj/submit/shorten` | TokenAuth | 同 `/mj/submit/shorten` |
| POST | `/:mode/mj/submit/modal` | TokenAuth | 同 `/mj/submit/modal` |
| POST | `/:mode/mj/submit/imagine` | TokenAuth | 同 `/mj/submit/imagine` |
| POST | `/:mode/mj/submit/change` | TokenAuth | 同 `/mj/submit/change` |
| POST | `/:mode/mj/submit/simple-change` | TokenAuth | 同 `/mj/submit/simple-change` |
| POST | `/:mode/mj/submit/describe` | TokenAuth | 同 `/mj/submit/describe` |
| POST | `/:mode/mj/submit/blend` | TokenAuth | 同 `/mj/submit/blend` |
| POST | `/:mode/mj/submit/edits` | TokenAuth | 同 `/mj/submit/edits` |
| POST | `/:mode/mj/submit/video` | TokenAuth | 同 `/mj/submit/video` |
| POST | `/:mode/mj/notify` | TokenAuth | 同 `/mj/notify` |
| GET | `/:mode/mj/task/:id/fetch` | TokenAuth | 同 `/mj/task/:id/fetch` |
| GET | `/:mode/mj/task/:id/image-seed` | TokenAuth | 同 `/mj/task/:id/image-seed` |
| POST | `/:mode/mj/task/list-by-condition` | TokenAuth | 同 `/mj/task/list-by-condition` |
| POST | `/:mode/mj/insight-face/swap` | TokenAuth | 同 `/mj/insight-face/swap` |
| POST | `/:mode/mj/submit/upload-discord-images` | TokenAuth | 同 `/mj/submit/upload-discord-images` |
| POST | `/suno/submit/:action` | TokenAuth | Suno 任务提交 |
| POST | `/suno/fetch` | TokenAuth | 批量查询 Suno 任务 |
| GET | `/suno/fetch/:id` | TokenAuth | 单任务查询 Suno 任务 |
| POST | `/v1/video/generations` | TokenAuth | 通用视频生成任务 |
| GET | `/v1/video/generations/:task_id` | TokenAuth | 通用视频任务查询 |
| POST | `/kling/v1/videos/text2video` | TokenAuth | Kling 文生视频，请求体会做官方格式转换 |
| POST | `/kling/v1/videos/image2video` | TokenAuth | Kling 图生视频，请求体会做官方格式转换 |
| GET | `/kling/v1/videos/text2video/:task_id` | TokenAuth | Kling 文生视频任务查询 |
| GET | `/kling/v1/videos/image2video/:task_id` | TokenAuth | Kling 图生视频任务查询 |
| POST | `/jimeng/` | TokenAuth | 即梦官方 API 兼容入口，请求体/查询串会做官方格式转换 |

## 8. 相关补充文档

| 文档 | 用途 |
|---|---|
| `docs/api/api_auth.md` | `/api/**` Access Token 鉴权细节 |
| `docs/api/v1_messages_codex_responses_compat.md` | `/v1/messages` 命中渠道级 `messages_to_responses_compat` 时的内部兼容行为 |
| `docs/api/v1_messages_codex_responses_ws_acceleration.md` | `/v1/messages -> /v1/responses` 的 WS 加速方案说明 |
| `docs/api/web_api.md` | 旧版 Web API 文档，现可优先参考本文档 |
