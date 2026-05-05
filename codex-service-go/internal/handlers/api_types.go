package handlers

type chatCompletionsRequest struct {
	Model             string             `json:"model"`
	Messages          []chatMessageInput `json:"messages"`
	Stream            *bool              `json:"stream,omitempty"`
	Reasoning         *chatReasoning     `json:"reasoning,omitempty"`
	ReasoningEffort   string             `json:"reasoning_effort,omitempty"`
	PromptCacheKey    string             `json:"prompt_cache_key,omitempty"`
	Tools             []interface{}      `json:"tools,omitempty"`
	ToolChoice        interface{}        `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool              `json:"parallel_tool_calls,omitempty"`
}

type chatMessageInput struct {
	Role       string         `json:"role"`
	Content    interface{}    `json:"content"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type responsesRequest struct {
	Model          string              `json:"model"`
	Instructions   string              `json:"instructions"`
	Input          []responsesInput    `json:"input"`
	Stream         bool                `json:"stream"`
	Store          bool                `json:"store"`
	Reasoning      *responsesReasoning `json:"reasoning,omitempty"`
	PromptCacheKey string              `json:"prompt_cache_key,omitempty"`
	// Extra fields to align with Codex CLI payload shape
	ToolChoice        interface{}   `json:"tool_choice,omitempty"`
	ParallelToolCalls bool          `json:"parallel_tool_calls,omitempty"`
	Include           []string      `json:"include,omitempty"`
	Tools             []interface{} `json:"tools,omitempty"`
}

type responsesReasoning struct {
	Effort string `json:"effort,omitempty"`
	// Codex CLI also sends summary: "auto"
	Summary string `json:"summary,omitempty"`
}

type responsesInput struct {
	Type      string             `json:"type"`
	Role      string             `json:"role,omitempty"`
	Content   []responsesContent `json:"content,omitempty"`
	CallID    string             `json:"call_id,omitempty"`
	Name      string             `json:"name,omitempty"`
	Arguments string             `json:"arguments,omitempty"`
	Output    string             `json:"output,omitempty"`
}

type responsesContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type chatCompletionsResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   *chatUsage   `json:"usage,omitempty"`
}

type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type chatMessage struct {
	Role      string         `json:"role"`
	Content   *string        `json:"content,omitempty"`
	Reasoning string         `json:"reasoning,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function *chatToolFunction `json:"function,omitempty"`
}

type chatToolCallDelta struct {
	Index    int               `json:"index"`
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function *chatToolFunction `json:"function,omitempty"`
}

type chatToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatCompletionChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []chatChunkChoice `json:"choices"`
}

type chatChunkChoice struct {
	Index        int            `json:"index"`
	Delta        chatChunkDelta `json:"delta"`
	FinishReason *string        `json:"finish_reason,omitempty"`
}

type chatChunkDelta struct {
	Role      string              `json:"role,omitempty"`
	Content   string              `json:"content,omitempty"`
	Reasoning string              `json:"reasoning,omitempty"`
	ToolCalls []chatToolCallDelta `json:"tool_calls,omitempty"`
}
