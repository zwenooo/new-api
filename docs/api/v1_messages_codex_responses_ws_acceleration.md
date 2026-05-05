# `/v1/messages -> /v1/responses` 上游 WebSocket 加速

## 背景

当前 `cx2cc` 链路是：

- 客户端 -> `new-api`：`/v1/messages`（Anthropic Messages）
- `new-api` -> 上游：`/v1/responses`

下游对客户端仍然是 Claude SSE，但上游这段链路在满足条件时可以直接走 Responses WebSocket，以降低流式首包和中途事件时延。

## 生效条件

同时满足以下条件时启用：

1. 外部入口是 `POST /v1/messages`
2. 命中开启 `messages_to_responses_compat` 的渠道
3. 请求是 `stream=true`
4. 上游侧选择的是 Responses 模式
5. 下游不是 Responses WebSocket 客户端

这是一条**普通渠道级**能力。

## 处理方式

### 1. 下游协议不变

- 客户端看到的仍然是 `Claude Messages` + SSE
- 现有的 Claude SSE 翻译器继续负责对外输出

### 2. 上游协议改为 Responses WebSocket

- `new-api` 主动连接上游 Responses WebSocket
- 首帧发送归一化后的 Responses JSON
- 上游返回的每条 WebSocket JSON 消息会被桥接成本地 SSE 数据流
- 然后交给现有的 Claude SSE 翻译器继续处理

这样做的边界很清楚：只替换上游传输层，不改下游协议，不改业务语义。

## 渠道选择

`/v1/messages` 在选渠道时会优先尝试两类能力：

1. 直接支持该模型的普通渠道
2. 开启 `messages_to_responses_compat`，且能通过渠道模型声明或 `model_mapping` 承接该模型的兼容渠道

只有实际选中了第二类渠道，这个 WebSocket 加速分支才会进入。

## 失败处理

如果上游 WebSocket 握手失败：

- 优先返回握手阶段拿到的上游错误响应
- 如果连握手响应都没有，就按连接错误处理

如果桥接过程中上游断开：

- 按现有流式错误聚合逻辑结束或报错
- 不做偷偷回退到 HTTP/SSE 的隐藏兜底

## 说明

- 该能力只影响 `new-api -> 上游` 这一段传输方式。
- 外部客户端不需要支持 WebSocket。
- `/v1/messages` 的请求翻译与必要约束，见 [v1_messages_codex_responses_compat.md](./v1_messages_codex_responses_compat.md)。
