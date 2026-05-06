## 1. 安装 Node.js 18+（通用）

**官网下载（推荐）**
1. 打开浏览器访问 https://nodejs.org/
2. 推荐下载 LTS 版本
3. 下载完成后双击 `.msi` 文件
4. 按照安装向导完成安装，保持默认设置即可

**注意事项**
- 如果遇到权限问题，尝试以管理员身份运行
- 某些杀毒软件可能会误报，需要添加白名单

**验证安装是否成功**
```bash
node --version
npm --version
```

## 2. 安装 CodeX CLI

```bash
npm install -g @openai/codex
```

**提示**
- 可在 CMD 或 PowerShell 中执行

**验证 CodeX CLI 安装**
```bash
codex --version
```

## 3. 配置环境（CodeX）

**配置路径与打开方式**
1. 切换到配置目录：终端执行 `cd ~/.codex`（如果提示不存在，可先运行一下 codex，然后新开终端即可）
2. 打开目录：
   - VS Code：执行 `code .`（注意空格）
   - Cursor：执行 `cursor .`（会自动打开该文件夹）
3. 编辑 `config.toml` 与 `auth.json`：复制下方内容（文件不存在就新建）

**config.toml 文件**
```toml
model_provider = "codez"
model = "gpt-5.2-codex"
model_reasoning_effort = "high"

[model_providers.codez]
name = "openai"
base_url = "https://api.example.com/v1"
wire_api = "responses"
requires_openai_auth = true
```

**auth.json 文件**
```json
{
  "OPENAI_API_KEY": "粘贴你的 API 密钥"
}
```

**配置解释**
- `model` 为使用的模型，可在 CLI 或插件中再自定义
- `model_reasoning_effort` 表示推理强度，可选 `Extra high`/`high` / `medium` / `low`
- `model_provider` 与 `[model_providers.*]` 需与中转服务一致
