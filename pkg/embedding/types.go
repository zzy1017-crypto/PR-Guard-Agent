package embedding

type Request struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type Response struct {
	Data []ResponseData `json:"data"`
}

type ResponseData struct {
	Index     *int      `json:"index,omitempty"`
	Embedding []float32 `json:"embedding"`
}
