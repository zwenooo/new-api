# Transfer API – Web 界面后端接口文档

> 这份文档主要覆盖 `/api/**` 的 Web/Console 接口。
>
> 如果你需要看 **当前项目完整接口总览**（含 `/v1`、`/v1beta`、`/mj`、`/suno`、`/kling/v1` 等代理接口），优先看：`docs/api/project_api_full.md`

> 本文档汇总了 **Transfer API** 后端提供给前端 Web 界面的全部 REST 接口（不含 *Relay* 相关接口）。
>
> 接口前缀统一为 `https://<your-domain>`，以下仅列出 **路径**、**HTTP 方法**、**鉴权要求** 与 **功能简介**。
>
> 鉴权级别说明：
> * **公开** – 不需要登录即可调用
> * **用户** – 需携带用户 Token（`middleware.UserAuth`）
> * **管理员** – 需管理员 Token（`middleware.AdminAuth`）
> * **Root** – 仅限最高权限 Root 用户（`middleware.RootAuth`）

---

## 1. 初始化 / 系统状态
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET  | /api/setup | 公开 | 获取系统初始化状态 |
| POST | /api/setup | 公开 | 完成首次安装向导 |
| GET  | /api/status | 公开 | 获取运行状态摘要 |
| GET  | /api/uptime/status | 公开 | Uptime-Kuma 兼容状态探针 |
| GET  | /api/status/test | 管理员 | 测试后端与依赖组件是否正常 |

## 2. 公共信息
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/models | 用户 | 获取前端可用模型列表 |
| GET | /api/notice | 公开 | 获取公告栏内容 |
| GET | /api/about | 公开 | 关于页面信息 |
| GET | /api/home_page_content | 公开 | 首页自定义内容 |
| GET | /api/pricing | 可匿名/用户 | 价格与套餐信息 |
| GET | /api/ratio_config | 公开 | 模型倍率配置（仅公开字段） |

## 3. 邮件 / 身份验证
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/verification | 公开 (限流) | 发送邮箱验证邮件 |
| GET | /api/reset_password | 公开 (限流) | 发送重置密码邮件 |
| POST | /api/user/reset | 公开 | 提交重置密码请求 |

## 4. OAuth / 第三方登录
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/oauth/github | 公开 | GitHub OAuth 跳转 |
| GET | /api/oauth/oidc | 公开 | OIDC 通用 OAuth 跳转 |
| GET | /api/oauth/linuxdo | 公开 | LinuxDo OAuth 跳转 |
| GET | /api/oauth/wechat | 公开 | 微信扫码登录跳转 |
| GET | /api/oauth/wechat/bind | 公开 | 微信账户绑定 |
| GET | /api/oauth/email/bind | 公开 | 邮箱绑定 |
| GET | /api/oauth/telegram/login | 公开 | Telegram 登录 |
| GET | /api/oauth/telegram/bind | 公开 | Telegram 账户绑定 |
| GET | /api/oauth/state | 公开 | 获取随机 state（防 CSRF） |

## 5. 用户模块
### 5.1 账号注册/登录
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| POST | /api/user/register | 公开 | 注册新账号 |
| POST | /api/user/login | 公开 | 用户登录 |
| GET  | /api/user/logout | 用户 | 退出登录 |
| GET  | /api/user/epay/notify | 公开 | Epay 支付回调 |
| GET  | /api/user/groups | 公开 | 列出所有分组（无鉴权版） |

### 5.2 用户自身操作 (需登录)
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/user/self/groups | 用户 | 获取自己所在分组 |
| GET | /api/user/self | 用户 | 获取个人资料 |
| GET | /api/user/models | 用户 | 获取模型可见性 |
| PUT | /api/user/self | 用户 | 修改个人资料 |
| DELETE | /api/user/self | 用户 | 注销账号 |
| GET | /api/user/token | 用户 | 生成用户级别 Access Token |
| GET | /api/user/aff | 用户 | 获取推广码信息 |
| GET | /api/user/aff/records | 用户 | 获取邀请转化与返利记录（在线支付订阅/按量商品/付费兑换码，分页：p/page_size） |
| POST | /api/user/topup | 用户 | 余额直充 |
| POST | /api/user/pay | 用户 | 提交支付订单 |
| POST | /api/user/amount | 用户 | 余额支付 |
| POST | /api/user/aff_transfer | 用户 | 推广返利划转到余额（金额单位：分，字段：amount_fen） |
| PUT | /api/user/setting | 用户 | 更新用户设置 |

## 5.4 订阅购买
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/subscription/plans | 用户 | 获取已启用订阅套餐 |
| POST | /api/subscription/order | 用户 | 创建订阅订单（pay_method=balance/epay，apply_mode=stack/defer） |
| GET | /api/subscription/epay/notify | 公开 | 订阅易支付回调 |

> 说明（套餐 mode）：
> - `mode=subscription`：按额度（quota）计费的订阅（现有模式）。
> - `mode=tokens`：按 tokens 计费的订阅（扣减 `usage.total_tokens`，用完为止）。
> - `mode=request`：按请求次数计费的订阅。
> - 字段含义：
>   - `quota`：`subscription/tokens` 为总额度（0=无限）；`request` 为总次数（0=无限）。
>   - `daily_quota_limit`：`subscription/tokens` 为日限额（0=无限）；`request` 使用 `daily_request_limit`。

### 5.3 管理员用户管理
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/user/ | 管理员 | 获取全部用户列表 |
| GET | /api/user/search | 管理员 | 搜索用户 |
| GET | /api/user/:id | 管理员 | 获取单个用户信息 |
| POST | /api/user/ | 管理员 | 创建用户 |
| POST | /api/user/manage | 管理员 | 冻结/重置等管理操作 |
| PUT | /api/user/ | 管理员 | 更新用户 |
| DELETE | /api/user/:id | 管理员 | 删除用户 |
| POST | /api/user/subscriptions/bulk/duration | 管理员 | 批量调整订阅到期时间（支持额度订阅/小团订阅，按剩余天数筛选，可统一增加天数） |

## 6. 站点选项 (Root)
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/option/ | Root | 获取全局配置 |
| PUT | /api/option/ | Root | 更新全局配置 |
| POST | /api/option/rest_model_ratio | Root | 重置模型倍率 |
| POST | /api/option/migrate_console_setting | Root | 迁移旧版控制台配置 |

## 6.1 订阅套餐管理 (Root)
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/subscription/plans/all | Root | 获取全部订阅套餐（含禁用） |
| POST | /api/subscription/plans | Root | 新增订阅套餐 |
| PUT | /api/subscription/plans/:id | Root | 更新订阅套餐 |
| DELETE | /api/subscription/plans/:id | Root | 删除订阅套餐 |

## 7. 模型倍率同步 (Root)
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/ratio_sync/channels | Root | 获取可同步渠道列表 |
| POST | /api/ratio_sync/fetch | Root | 从上游拉取倍率 |

说明：
- `GET /api/ratio_sync/channels` 会返回普通渠道，以及内置的“官方倍率预设”“OpenRouter 官方模型价格”等特殊同步源。
- `POST /api/ratio_sync/fetch` 支持在 `upstreams[]` 中携带：
  - `source_type`
  - `endpoint`
  - `bearer_token`
- 当 `source_type=openrouter_models` 时，后端会优先使用 `upstreams[].bearer_token`，未提供时回退到系统设置中的 `OpenRouterPriceSyncToken`；随后读取 OpenRouter `/api/v1/models`，并将 `pricing.prompt/completion/input_cache_read/input_cache_write/request` 转换为本地的 `model_ratio` / `completion_ratio` / `cache_ratio` / `create_cache_ratio` / `model_price` 供管理员筛选后应用。

## 8. 渠道管理 (管理员)
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/channel/ | 获取渠道列表 |
| GET | /api/channel/search | 搜索渠道 |
| GET | /api/channel/models | 查询渠道模型能力 |
| GET | /api/channel/models_enabled | 查询启用模型能力 |
| GET | /api/channel/:id | 获取单个渠道 |
| GET | /api/channel/test | 批量测试渠道连通性 |
| GET | /api/channel/test/:id | 单个渠道测试 |
| GET | /api/channel/update_balance | 批量刷新余额 |
| GET | /api/channel/update_balance/:id | 单个刷新余额 |
| POST | /api/channel/ | 新增渠道 |
| PUT | /api/channel/ | 更新渠道 |
| DELETE | /api/channel/disabled | 删除已禁用渠道 |
| POST | /api/channel/tag/disabled | 批量禁用标签渠道 |
| POST | /api/channel/tag/enabled | 批量启用标签渠道 |
| PUT | /api/channel/tag | 编辑渠道标签 |
| DELETE | /api/channel/:id | 删除渠道 |
| POST | /api/channel/batch | 批量删除渠道 |
| POST | /api/channel/fix | 修复渠道能力表 |
| GET | /api/channel/fetch_models/:id | 拉取单渠道模型 |
| POST | /api/channel/fetch_models | 拉取全部渠道模型 |
| POST | /api/channel/batch/tag | 批量设置渠道标签 |
| GET | /api/channel/tag/models | 根据标签获取模型 |
| POST | /api/channel/copy/:id | 复制渠道 |

## 9. Token 管理
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/token/ | 用户 | 获取全部 Token |
| GET | /api/token/search | 用户 | 搜索 Token |
| GET | /api/token/:id | 用户 | 获取单个 Token |
| POST | /api/token/ | 用户 | 创建 Token |
| PUT | /api/token/ | 用户 | 更新 Token |
| DELETE | /api/token/:id | 用户 | 删除 Token |
| POST | /api/token/batch | 用户 | 批量删除 Token |

## 10. 兑换码管理 (管理员)
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/redemption/ | 获取兑换码列表 |
| GET | /api/redemption/search | 搜索兑换码 |
| GET | /api/redemption/:id | 获取单个兑换码 |
| POST | /api/redemption/ | 创建兑换码 |
| PUT | /api/redemption/ | 更新兑换码 |
| GET | /api/redemption/presets | 获取预置商品列表 |
| POST | /api/redemption/presets | 新建/更新预置商品（按 name Upsert） |
| DELETE | /api/redemption/presets/:id | 删除预置商品 |
| POST | /api/redemption/presets/generate | 按预置商品 name 生成兑换码 |
| POST | /api/redemption/payg/generate | 生成按量付费兑换码（美元/tokens/次数） |
| DELETE | /api/redemption/invalid | 删除无效兑换码 |
| DELETE | /api/redemption/:id | 删除兑换码 |

> 说明：
> - `POST /api/redemption/` 与 `PUT /api/redemption/` 支持 `allowed_group_ids`（数组，分组 ID），用于配置订阅额度/按量付费(美元)/按量付费(tokens)/按量付费(次数)/次数订阅兑换码的可用分组。
> - 仍兼容 `allowed_groups`（数组，分组 code）；当两者同时提供时，以 `allowed_group_ids` 为准。
> - 订阅类兑换码支持 `mode=subscription/tokens/request`：
>   - `subscription`：额度订阅(美元)
>   - `tokens`：额度订阅(tokens)
>   - `request`：额度订阅(次数)
> - 按量付费（永久额度）支持 `mode=payg/pay_token/pay_request`：
>   - `payg`：按量付费(美元)
>   - `pay_token`：按量付费(token)
>   - `pay_request`：按量付费(次数)

## 11. 日志
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/log/ | 管理员 | 获取全部日志 |
| DELETE | /api/log/ | 管理员 | 删除历史日志 |
| GET | /api/log/stat | 管理员 | 日志统计 |
| GET | /api/log/self/stat | 用户 | 我的日志统计 |
| GET | /api/log/search | 管理员 | 搜索全部日志 |
| GET | /api/log/self | 用户 | 获取我的日志 |
| GET | /api/log/self/search | 用户 | 搜索我的日志 |
| GET | /api/log/token | 公开 | 根据 Token 查询日志（支持 CORS） |

### 11.1 `GET /api/log/` 详细说明

这是管理员使用的分页日志列表接口，`/console/log` 页面管理员视角的主表格就是调用它。

请求参数：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `p` | int | 否 | 页码，从 `1` 开始，默认 `1` |
| `page_size` | int | 否 | 每页数量，最大 `100` |
| `ps` | int | 否 | `page_size` 兼容别名 |
| `size` | int | 否 | `page_size` 兼容别名 |
| `type` | int | 否 | 日志类型；`0` 表示全部 |
| `start_timestamp` | int64 | 否 | 起始时间，Unix 秒 |
| `end_timestamp` | int64 | 否 | 结束时间，Unix 秒 |
| `username` | string | 否 | 用户名，精确匹配 |
| `token_name` | string | 否 | Token 名称，精确匹配 |
| `model_name` | string | 否 | 模型名，SQL `LIKE` 查询 |
| `request_id` | string | 否 | 请求 ID，精确匹配 |
| `channel` | int | 否 | 渠道 ID |
| `group_id` | int | 否 | 分组 ID |
| `group` | int | 否 | `group_id` 兼容别名 |

`type` 枚举：

| 值 | 说明 |
|------|------|
| `0` | 全部 |
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

日志项核心字段：

- `id`
- `user_id`
- `created_at`
- `type`
- `content`
- `username`
- `token_name`
- `model_name`
- `quota`
- `prompt_tokens`
- `completion_tokens`
- `use_time`
- `is_stream`
- `channel`
- `channel_name`
- `token_id`
- `group`
- `ip`
- `request_id`
- `other`

其中 `other` 是 JSON 字符串，`/console/log` 中很多常规显示项都从这里解析，例如：

- `prompt_cache_key`
- `session_id`
- `conversation_id`
- `quota_bucket`
- `request_method`
- `request_path`
- `request_ua`
- `cache_tokens`
- `cache_creation_tokens`
- `stream_exit_reason`
- `stream_exit_error`
- `service_tier`
- `reasoning_effort`
- `admin_info.request_headers`
- `admin_info.request_content_length`
- `admin_info.use_channel`

### 11.2 `GET /api/log/self` 与管理员接口的差异

- `GET /api/log/self` 也是分页列表，但会裁掉 `other.admin_info`、价格/倍率等字段。
- `GET /api/log/self` 默认不返回管理日志。
- 如果你要做和管理员 `/console/log` 尽量一致的外部分析，优先使用 `GET /api/log/`。

## 12. 数据统计
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/data/ | 管理员 | 全站用量按日期统计 |
| GET | /api/data/self | 用户 | 我的用量按日期统计 |

## 13. 分组
| GET | /api/group/ | 管理员 | 获取全部分组列表 |

## 14. Midjourney 任务
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/mj/self | 用户 | 获取自己的 MJ 任务 |
| GET | /api/mj/ | 管理员 | 获取全部 MJ 任务 |

## 15. 任务中心
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/task/self | 用户 | 获取我的任务 |
| GET | /api/task/ | 管理员 | 获取全部任务 |

## 16. 账户计费面板 (Dashboard)
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /dashboard/billing/subscription | 用户 Token | 获取订阅额度信息 |
| GET | /v1/dashboard/billing/subscription | 同上 | 兼容 OpenAI SDK 路径 |
| GET | /dashboard/billing/usage | 用户 Token | 获取使用量信息 |
| GET | /v1/dashboard/billing/usage | 同上 | 兼容 OpenAI SDK 路径 |

---

> **更新日期**：2026.04.03
