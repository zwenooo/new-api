# codex_cli_rs（Codex CLI Rust）/v1/responses：请求与响应（SSE）拆解

本文只基于仓库内 `codex/codex-rs` 源码对 **codex_cli_rs** 发起的 **`POST /v1/responses`** 进行拆解，覆盖：

- 客户端如何组装请求（URL、headers、body）
- 服务端如何返回响应（HTTP headers + SSE 事件流）
- codex_cli_rs 如何解析 SSE 并转成内部事件（ResponseEvent）

> 关键结论先说：codex_cli_rs 在 `/v1/responses` 链路里 **不会发送 `conversation_id` header**；它把“线程 id（ThreadId）”同时放在：
>
> - 请求 header：`session_id: <thread_id>`
> - 请求 body：`prompt_cache_key: "<thread_id>"`
>
> 相关实现见：`codex/codex-rs/codex-api/src/requests/headers.rs`、`codex/codex-rs/core/src/client.rs`。

---

## 1. 端到端调用链（客户端侧）

以 HTTP SSE 的 Responses API（`WireApi::Responses`）为例：

1. `ModelClientSession::stream` 选择走 Responses API：`codex/codex-rs/core/src/client.rs`
2. `stream_responses_api`：
   - 组装 `ApiPrompt`（instructions、input、tools、parallel_tool_calls）
   - 组装 `ApiResponsesOptions`（reasoning/include/text、prompt_cache_key、conversation_id、extra_headers、compression、turn_state）
3. `codex-api` 层发请求：
   - `ResponsesClient::stream_prompt`：`codex/codex-rs/codex-api/src/endpoint/responses.rs`
   - `ResponsesRequestBuilder::build`：`codex/codex-rs/codex-api/src/requests/responses.rs`
   - `StreamingClient::stream` 统一加 `Accept: text/event-stream` + auth headers：`codex/codex-rs/codex-api/src/endpoint/streaming.rs`
4. `codex-client` 通过 reqwest 发起 streaming 请求：
   - `ReqwestTransport::stream`：`codex/codex-rs/codex-client/src/transport.rs`
5. 收到响应后，SSE 解析：
   - `spawn_response_stream` 读取响应 headers（如 `x-codex-turn-state`）并启动 SSE 解析协程：`codex/codex-rs/codex-api/src/sse/responses.rs`

---

## 2. 请求（HTTP）

### 2.1 URL 与 Path

`codex-api` 使用 `Provider.base_url` 作为基础地址，并拼接 path：

- `Provider::url_for_path`：`codex/codex-rs/codex-api/src/provider.rs`
- `/v1/responses` 实际由 `ResponsesClient::path()` 决定：`codex/codex-rs/codex-api/src/endpoint/responses.rs`
  - `WireApi::Responses` / `WireApi::Compact` → `"responses"`
  - `WireApi::Chat` → `"chat/completions"`（非本文重点）

因此，若 provider base_url 配成 `https://api.openai.com/v1`，最终就是：

```
POST https://api.openai.com/v1/responses
```

若 provider base_url 配成某个代理前缀（例如 ChatGPT/codex 后端），同理拼出对应路径。

### 2.2 headers（codex_cli_rs 会加什么）

#### 2.2.1 streaming 层强制加入

每次 streaming 请求都会加：

- `Accept: text/event-stream`

代码位置：`codex/codex-rs/codex-api/src/endpoint/streaming.rs`

#### 2.2.2 鉴权相关

`codex-api` 会把 auth 提供的 bearer token 写入：

- `Authorization: Bearer <token>`

如有 account_id，还会加：

- `ChatGPT-Account-ID: <account_id>`

代码位置：`codex/codex-rs/codex-api/src/auth.rs`

#### 2.2.3 “会话/线程”相关（重点）

**codex_cli_rs 的“线程 id（ThreadId）”在 wire 上以 `session_id` 这个 header 名称出现。**

- `build_conversation_headers(conversation_id)` 会插入 `session_id: <id>`
- 注意：这里函数入参叫 `conversation_id`，但真正写入的是 **`session_id` header**

代码位置：`codex/codex-rs/codex-api/src/requests/headers.rs`

该 `conversation_id` 值来自 core 层的 thread id：

- `let conversation_id = self.state.conversation_id.to_string();`
- 并传入 `ApiResponsesOptions.conversation_id`

代码位置：`codex/codex-rs/core/src/client.rs`

#### 2.2.4 Codex 客户端标识（originator / UA）

默认会有：

- `originator: codex_cli_rs`
- `User-Agent: codex_cli_rs/<ver> (<os> <os_ver>; <arch>) <terminal_ua>(...)`

由默认 reqwest client 的 default headers + dedicated user_agent 设置实现：

- `DEFAULT_ORIGINATOR = "codex_cli_rs"`
- `build_reqwest_client()`：插入 `originator` header，并设置 `user_agent`

代码位置：`codex/codex-rs/core/src/default_client.rs`

#### 2.2.5 codex CLI 自己加的业务 headers

core 层对 `/responses` 还会加一些“能力/开关” headers（放在 `ApiResponsesOptions.extra_headers`）：

- `x-oai-web-search-eligible: true|false`（由 config.web_search_mode 决定）
- `x-codex-beta-features: <comma-separated>`（若启用 beta 功能）
- `x-codex-turn-state: <value>`（如果本 turn 已从服务端拿到 turn-state，则回传，做 sticky routing）

代码位置：`codex/codex-rs/core/src/client.rs`（`build_responses_headers`、`beta_feature_headers`）

#### 2.2.6 x-openai-subagent（可选）

如果 session_source 是 SubAgent，会追加：

- `x-openai-subagent: review|compact|collab_spawn|<custom>`

代码位置：

- `subagent_header`：`codex/codex-rs/codex-api/src/requests/headers.rs`
- 注入点：`codex/codex-rs/codex-api/src/requests/responses.rs`

### 2.3 body（JSON）

#### 2.3.1 顶层字段（精确来自结构体）

`/v1/responses` 的请求 body 结构由 `ResponsesApiRequest` 定义（序列化为 JSON）：

- `model`（string）
- `instructions`（string）
- `input`（ResponseItem[]）
- `tools`（Tool[]）
- `tool_choice`（固定 `"auto"`）
- `parallel_tool_calls`（bool）
- `reasoning`（可选）
- `store`（bool；Azure responses 默认可能为 true）
- `stream`（bool；codex_cli_rs 这里固定为 true）
- `include`（string[]；例如包含 `"reasoning.encrypted_content"`）
- `prompt_cache_key`（可选 string）
- `text`（可选；verbosity / json_schema 输出控制）

代码位置：`codex/codex-rs/codex-api/src/common.rs`（`ResponsesApiRequest`）

#### 2.3.2 prompt_cache_key 的来源（重点）

core 层会把 thread id 同时写到：

- `ApiResponsesOptions.prompt_cache_key = Some(thread_id)`

代码位置：`codex/codex-rs/core/src/client.rs`

随后在 `ResponsesRequestBuilder` 写入 body 的 `prompt_cache_key` 字段：

代码位置：`codex/codex-rs/codex-api/src/requests/responses.rs`

> 这也是为什么你在某些下游（例如某些代理/日志系统）里看到的 “conversation_id” 实际上可能等于 `prompt_cache_key`：它们会把 `prompt_cache_key` 作为会话追踪 key 来用。

#### 2.3.3 示例 body（示意）

```jsonc
{
  "model": "gpt-5-codex",
  "instructions": "…",
  "input": [
    {
      "type": "message",
      "role": "user",
      "content": [{ "type": "input_text", "text": "hello" }]
    }
  ],
  "tools": [
    { "type": "function", "name": "shell", "description": "…", "strict": true, "parameters": { "type": "object", "properties": {} } }
  ],
  "tool_choice": "auto",
  "parallel_tool_calls": false,
  "reasoning": { "effort": "medium", "summary": "auto" },
  "store": false,
  "stream": true,
  "include": ["reasoning.encrypted_content"],
  "prompt_cache_key": "67e55044-10b1-426f-9247-bb680e5fe0c8",
  "text": {
    "verbosity": "medium",
    "format": {
      "type": "json_schema",
      "strict": true,
      "name": "codex_output_schema",
      "schema": { "type": "object", "properties": {} }
    }
  }
}
```

### 2.4 请求体压缩（可选：zstd）

当满足以下条件时，codex_cli_rs 会对 streaming 请求体进行 zstd 压缩，并设置：

- `Content-Encoding: zstd`

触发条件（简化）：feature `EnableRequestCompression` 开启 + auth 模式为 ChatGPT + provider 为 OpenAI（见代码条件判断）。

代码位置：

- 条件判断：`codex/codex-rs/core/src/client.rs`（`responses_request_compression`）
- 实际压缩与 header 注入：`codex/codex-rs/codex-client/src/transport.rs`

---

## 3. 响应（HTTP + SSE）

### 3.1 HTTP 响应 headers：codex_cli_rs 会读取哪些

`spawn_response_stream` 会在开始解析 SSE 前读取并处理部分响应 headers：

- `x-codex-turn-state`：如果存在，会写入 `turn_state: OnceLock<String>`，后续同一 turn 的请求会回传 `x-codex-turn-state`
- `X-Models-Etag`：如存在，会先发出 `ResponseEvent::ModelsEtag(etag)`
- `x-reasoning-included`：如存在，会先发出 `ResponseEvent::ServerReasoningIncluded(true)`
- rate-limit 相关 headers：解析后会先发出 `ResponseEvent::RateLimits(snapshot)`

代码位置：`codex/codex-rs/codex-api/src/sse/responses.rs`（`spawn_response_stream`）

### 3.2 SSE 数据流格式：codex_cli_rs 期望的形态

codex_cli_rs 解析 SSE 时 **只使用每个 SSE message 的 `data` 字段**，并要求其中是 JSON：

- `sse.data` → `serde_json::from_str::<ResponsesStreamEvent>(&sse.data)`
- 事件类型来自 JSON 的 `type` 字段（不是 SSE 的 `event:` 行）

代码位置：`codex/codex-rs/codex-api/src/sse/responses.rs`（`process_sse` / `ResponsesStreamEvent`）

因此服务端最稳妥的输出是形如：

```
event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"Hi"}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp-1","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}
```

### 3.3 SSE 事件到 ResponseEvent 的映射（按代码精确列出）

映射逻辑在 `process_responses_event`：

代码位置：`codex/codex-rs/codex-api/src/sse/responses.rs`

| SSE JSON `type` | 关键字段 | 转换结果 |
|---|---|---|
| `response.created` | `response` 存在即可 | `ResponseEvent::Created` |
| `response.output_item.done` | `item` | 解析为 `ResponseItem` → `ResponseEvent::OutputItemDone(item)` |
| `response.output_item.added` | `item` | `ResponseEvent::OutputItemAdded(item)` |
| `response.output_text.delta` | `delta` | `ResponseEvent::OutputTextDelta(delta)` |
| `response.reasoning_summary_text.delta` | `delta`,`summary_index` | `ResponseEvent::ReasoningSummaryDelta{…}` |
| `response.reasoning_text.delta` | `delta`,`content_index` | `ResponseEvent::ReasoningContentDelta{…}` |
| `response.reasoning_summary_part.added` | `summary_index` | `ResponseEvent::ReasoningSummaryPartAdded{…}` |
| `response.completed` | `response.id`, `response.usage` | `ResponseEvent::Completed{response_id, token_usage}` 并结束流 |
| `response.done` | `response.id?`, `response.usage?` | 也会转换成 `Completed` 并结束流 |
| `response.failed` | `response.error` | 解析 error，转成 `ApiError::*`（见下节），并作为“待返回错误”缓存 |

### 3.4 错误处理（`response.failed` / 断流 / idle timeout）

#### 3.4.1 response.failed 的处理策略

`response.failed` 会被解析成一个 `ApiError`，但 **不会立刻向上游 channel 发送 Err**；
而是把它缓存到 `response_error`，继续等待流结束。

当 SSE stream 在未收到 `response.completed` 的情况下关闭时（`Ok(None)`），最终会发送：

- `Err(response_error)`（如果之前收到过 `response.failed`）
- 否则 `Err("stream closed before response.completed")`

代码位置：`codex/codex-rs/codex-api/src/sse/responses.rs`（`process_sse`）

#### 3.4.2 response.failed → ApiError 的分类

当 `response.error` 能解析为结构体 `Error{code,message,...}` 时：

- `code == "context_length_exceeded"` → `ApiError::ContextWindowExceeded`
- `code == "insufficient_quota"` → `ApiError::QuotaExceeded`
- `code == "usage_not_included"` → `ApiError::UsageNotIncluded`
- `code == "invalid_prompt"` → `ApiError::InvalidRequest{message}`
- 其他 → `ApiError::Retryable{message, delay}`（其中 delay 会尝试从 rate_limit_exceeded 的 message 里解析 retry-after）

代码位置：`codex/codex-rs/codex-api/src/sse/responses.rs`

#### 3.4.3 idle timeout

若在 `provider.stream_idle_timeout` 内没有拉到下一个 SSE message：

- 直接发送 `Err("idle timeout waiting for SSE")`

代码位置：`codex/codex-rs/codex-api/src/sse/responses.rs`

---

## 4. 常见误区：conversation_id / session_id / prompt_cache_key

在 codex_cli_rs 的 `/v1/responses` 里：

- **thread id** 在代码里变量名常叫 `conversation_id`
- 但 wire 上：
  - header 名叫 **`session_id`**
  - body 字段叫 **`prompt_cache_key`**
- codex_cli_rs 不会主动发送 `conversation_id` header

对应实现与数据流：

- `core/src/client.rs`：生成 `conversation_id = ThreadId.to_string()` 并放入 options：
  - `prompt_cache_key: Some(conversation_id.clone())`
  - `conversation_id: Some(conversation_id)`
- `codex-api/src/requests/headers.rs`：把 `conversation_id` 写成 header `session_id`
- `codex-api/src/common.rs`：body 里只有 `prompt_cache_key`，没有 `conversation_id`

---

## 5.（附录）与 Transfer API 日志视图的观测差异

如果你看的不是 codex_cli_rs 自己抓包，而是 `new-api` 的消费日志或请求追踪，需要注意：

- 日志归档时会从请求体提取 `prompt_cache_key`
- 若请求头里没有 `conversation_id` / `session_id`，日志层会用 `prompt_cache_key` 回填这两个字段
- 因此日志视图里可能同时看到 `conversation_id`、`session_id`、`prompt_cache_key`

对应代码位置：`new-api/service/log_info_generate.go`

所以日志里出现的 `conversation_id`，不一定表示客户端原始请求真的发了 `conversation_id` header；它可能只是日志层基于 `prompt_cache_key` 补出的会话视图。
