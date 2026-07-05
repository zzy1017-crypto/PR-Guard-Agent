package vector

type ChunkPoint struct {
	ID              uint64    `json:"id"`
	Vector          []float32 `json:"vector"`
	ProjectID       uint      `json:"project_id"`
	CodeVersionHash string    `json:"code_version_hash"`
	ChunkID         uint      `json:"chunk_id"`
	FilePath        string    `json:"file_path"`
	SymbolName      string    `json:"symbol_name"`
	SymbolType      string    `json:"symbol_type"`
	StartLine       int       `json:"start_line"`
	EndLine         int       `json:"end_line"`
	ContentHash     string    `json:"content_hash"`
}

type SearchFilter struct {
	ProjectID       uint   `json:"project_id"`
	CodeVersionHash string `json:"code_version_hash"`
}

type SearchResult struct {
	ID         uint    `json:"id"`
	Score      float32 `json:"score"`
	ChunkID    uint    `json:"chunk_id"`
	FilePath   string  `json:"file_path"`
	SymbolName string  `json:"symbol_name"`
	SymbolType string  `json:"symbol_type"`
	StartLine  int     `json:"start_line"`
	EndLine    int     `json:"end_line"`
}
