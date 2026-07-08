package llm

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string    `json:"model"`
	Prompt      string    `json:"prompt,omitempty"`
	Messages    []Message `json:"messages,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
}

type Response struct {
	Output  string   `json:"output"`
	Content string   `json:"content"`
	Text    string   `json:"text"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Text    string   `json:"text"`
	Message *Message `json:"message"`
}
