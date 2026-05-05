# 测试与验证（2025-10-04 03:37 UTC+8）

## 手动验证
- 步骤：
  1. 打开 `/admin/login`，登录成功后应重定向至 `/admin`。
  2. 观察首页：应显示“实例管理”，网络面板不应出现 `GET /admin/instances//log/raw`。
  3. 在实例表中点击“查看日志”（或直接访问 `/admin/instances/{id}/log/full`）。
  4. 观察日志页：标题显示目标实例；每 3 秒触发 `GET /admin/instances/{id}/log/raw` 返回 200 并替换 `<pre#log-pre>` 内容。

## 遗留/风险
- 若未来新增页面再次定义全局 `title/content`，会与首页冲突。建议：
  - 页面统一改为“嵌套 define + 独立执行”模式，或将所有完整页面（含独立 `<html>`）化，避免全局块名复用。

