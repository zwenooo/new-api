# 设计（2025-10-04 03:37 UTC+8）

## 根因
- `layout.html` 使用 `block "title"/"content"` 提供插槽；`admin_list.html`、`log_full.html` 在同一模板集合内重复定义同名块，`ParseFS` 单次解析导致后者覆盖前者，首页被错误替换为日志页。

## 方案
- 将 `log_full.html` 改为独立完整页面（参考 `login.html` 做法），避免定义全局块名，从而消除冲突；其余模板保持不变。

## 权衡
- 最小改动、风险低；不改变路由与数据结构；渲染链清晰。

