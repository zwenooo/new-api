# Coding Log

- 2025-10-04 03:37 (UTC+8)
  - 现象：登录成功后首页呈现“实例日志 / 每 3 秒自动刷新 / 加载中…”，后台每 3s 打 `/admin/instances//log/raw` 并返回 400。
  - 根因：模板继承冲突。`layout.html` 使用 `block "title"/"content"`，而 `log_full.html` 与 `admin_list.html` 在同一模板集合内定义了同名块。`template.ParseFS` 一次性解析导致后解析的 `log_full.html` 覆盖首页块，使 `/admin` 实际渲染为日志页（无 Instance 导致空 ID）。
  - 变更：将 `internal/server/templates/log_full.html` 改为独立完整页面，不再定义全局 `title/content`，仅保留 `{{define "log_full"}}` 包裹的完整 HTML 与轮询片段。
  - 预期：
    - `/admin` 正常显示实例列表。
    - `/admin/instances/:id/log/full` 正常显示日志并轮询 `/:id/log/raw`。

- 2025-10-04 03:45 (UTC+8)
  - 改进：为日志页增加“暂停刷新/继续”按钮与快捷键 P，选择文本时自动暂停；新增“复制全部”按钮。
  - 新增接口：`GET /admin/instances/:id/log/plain` 返回纯文本，便于复制或另开标签查看。
  - 涉及文件：
    - `internal/server/templates/log_full.html`
    - `internal/handlers/admin.go`（新增 `LogPlain`）
    - `internal/server/server.go`（注册路由）

- 2025-10-04 03:52 (UTC+8)
  - 按用户要求，移除自动刷新与所有附加功能（暂停、复制按钮、纯文本视图、鼠标按下暂停等）。
  - 仅保留“刷新”按钮，手动触发获取日志并替换 `<pre>` 内容。
  - 回退变更：删除路由 `GET /admin/instances/:id/log/plain` 与对应处理器；模板去除 htmx 定时触发与相关 JS。

- 2025-10-04 04:57 (UTC+8)
  - 优化：进入日志页时默认自动加载一次（仅触发一次，不再定时轮询）。
  - 做法：初始 `<pre>` 加 `hx-trigger="load"` 一次性拉取 `/admin/instances/:id/log/raw` 并替换自身。
