## 1. 安装 Node.js 18+（通用）

**方法一：使用官方仓库（推荐）**
```bash
# 添加 NodeSource 仓库
curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -

# 安装 Node.js
sudo apt-get install -y nodejs
```

**方法二：使用系统包管理器**
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install nodejs npm

# CentOS/RHEL/Fedora
sudo dnf install nodejs npm
```

**注意事项**
- 如果遇到权限问题，使用 `sudo`
- 确保你的用户在 npm 的全局目录有写权限

**验证安装是否成功**
```bash
node --version
npm --version
```

## 2. 安装 CodeX CLI

```bash
npm install -g @openai/codex

# 如遇权限问题
sudo npm install -g @openai/codex
```

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
base_url = "https://your-new-api.example.com/v1"
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
