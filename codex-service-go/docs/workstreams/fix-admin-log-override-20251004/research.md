# Research（2025-10-04 03:37 UTC+8）

- 现象：`/admin` 渲染为日志页片段，触发空 ID 轮询。
- 定位：`admin.go` 登录后重定向 `/admin`；模板集 `ParseFS` 单集合；`log_full.html` 定义全局 `title/content` 覆盖首页。
- 结论：去除 `log_full.html` 的全局块定义，改为独立完整页面。

