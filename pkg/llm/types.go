package llm

// 定义了与LLM交互的请求和响应结构体，包括消息、请求和响应的字段。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// 定义了发送给LLM的请求结构体，包括模型、提示、消息、最大令牌数和温度等字段。
type Request struct {
	Model       string    `json:"model"`
	Prompt      string    `json:"prompt,omitempty"`
	Messages    []Message `json:"messages,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
}

// 定义了LLM的响应结构体，包括输出、内容、文本和选择等字段。
type Response struct {
	Output  string   `json:"output"`
	Content string   `json:"content"`
	Text    string   `json:"text"`
	Choices []Choice `json:"choices"`
}

// Choice 定义了LLM响应中的选择结构体，包括文本和消息等字段。
type Choice struct {
	Text    string   `json:"text"`
	Message *Message `json:"message"`
}
