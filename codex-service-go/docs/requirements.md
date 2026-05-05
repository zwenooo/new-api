# 需求梳理（2025-10-04 03:37 UTC+8）

- 登录成功后应进入实例列表 `/admin`，不可显示日志页；首页不应产生对 `admin/instances//log/raw` 的轮询请求。
- 日志完整页 `/admin/instances/:id/log/full` 需保持自动刷新功能。

