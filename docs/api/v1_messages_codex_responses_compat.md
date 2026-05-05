# `/v1/messages` 兼容 OpenAI Responses 上游

## 目标

`new-api` 对外继续提供 `POST /v1/messages`（Anthropic Messages 协议），但在**普通渠道级**命中 `messages_to_responses_compat` 时，内部会把请求改走上游 `POST /v1/responses`，再把上游 Responses 的 JSON / SSE 翻译回 Anthropic Messages。

这条链路现在只依赖**渠道配置**，不依赖旧的托管渠道标记或嵌入代理方案。

## 触发条件

同时满足以下条件时生效：

1. 外部入口是 `POST /v1/messages`。
2. 最终选中的渠道类型属于 `OpenAI / Azure / Custom / OpenRouter / Xinference`。
3. 该渠道开启了 `messages_to_responses_compat=true`。
4. 该渠道对当前请求模型可用：
   - 要么渠道 `model_mapping` 明确映射了这个模型。
   - 要么渠道本身声明了这个模型。

如果不满足上述条件，请求仍走原本的 `/v1/messages` 处理路径，不会强行改成 `/v1/responses`。

## 行为矩阵

- 命中原生 Claude 渠道：保持原生 `/v1/messages`。
- 命中普通 OpenAI 渠道但未开启 `messages_to_responses_compat`：保持现有非 cx2cc 兼容逻辑。
- 命中开启 `messages_to_responses_compat` 的渠道：内部翻译为 `/v1/responses`。

## 请求侧行为

命中兼容渠道后，内部会做这些事：

1. 固定把上游请求路径改为 `/v1/responses`。
2. 将 Claude Messages 请求翻译为 OpenAI Responses 请求。
3. 剥离 `system` 中动态的 `x-anthropic-billing-header: ...` 行，避免影响缓存稳定性。
4. 根据消息里的会话信息补齐线程语义：
   - 请求头写入 `session_id`
   - 请求体写入 `prompt_cache_key`
5. 使用渠道自己的 `model_mapping` 解析上游模型名。
6. 为上游补齐 Responses 兼容所需约束：
   - `instructions` 必须是非空字符串
   - `stream=true`
   - `store=false`
   - `parallel_tool_calls=true`
   - `include` 至少包含 `reasoning.encrypted_content`
   - `reasoning.effort=minimal` 会被改成 `none`
7. 删除不应继续发给这类上游的字段：
   - `max_output_tokens`
   - `max_completion_tokens`
   - `temperature`
   - `top_p`
   - `frequency_penalty`
   - `presence_penalty`
   - `truncation`
   - `user`
   - `context_management`
   - `prompt_cache_retention`
   - `safety_identifier`
8. 非工具续传场景下，会移除 `previous_response_id`，避免生成“只有输出、没有对应 tool call”的非法输入。

同时会补齐一组最小必要的上游头：

- `Accept: text/event-stream` 或 `application/json`
- `Content-Type: application/json`
- `OpenAI-Beta: responses=experimental`
- `originator: codex_cli_rs`

并透传这类与会话/能力相关的请求头（如客户端有带）：

- `ChatGPT-Account-ID`
- `session_id`
- `conversation_id`
- `x-codex-beta-features`
- `x-codex-turn-state`
- `x-codex-turn-metadata`
- `x-oai-web-search-eligible`
- `x-openai-subagent`

## 响应侧行为

上游 `/v1/responses` 返回后：

1. JSON 响应会翻译回 Anthropic Messages JSON。
2. SSE 事件流会翻译回 Anthropic Messages SSE。
3. 工具名映射会在请求阶段记录，响应阶段按原始工具名回填。
4. `usage` 会补齐 Claude 侧需要的 prompt cache / message_start 相关信息。

## 配置方式

渠道侧只需要两类配置：

1. 在渠道设置里开启 `messages_to_responses_compat`。
2. 按需配置该渠道自己的 `model_mapping`。

建议：

- 如果客户端传的是 Claude 模型名，而上游实际吃的是 OpenAI Responses 模型名，就在该渠道的 `model_mapping` 里显式写清楚。
- 如果客户端已经直接传上游模型名，且该模型已在渠道模型列表中声明，可以不额外做映射。

## 备注

- 这条兼容链路是**渠道级能力**，不是全局托管渠道能力。
- `/v1/messages -> /v1/responses` 的流式 WebSocket 加速见 [v1_messages_codex_responses_ws_acceleration.md](./v1_messages_codex_responses_ws_acceleration.md)。
