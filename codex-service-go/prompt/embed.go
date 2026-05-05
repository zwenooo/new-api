package prompt

import _ "embed"

// GPT5Codex 是与 Codex CLI 保持一致的内置 instructions 文本。
//
//go:embed gpt_5_codex_prompt.md
var GPT5Codex string

// OpenCodeCodexHeader 是 OpenCode 官方的 Codex 指令头（instructions）。
//
//go:embed opencode_codex_header.txt
var OpenCodeCodexHeader string
