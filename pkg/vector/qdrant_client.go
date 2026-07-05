package vector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	qdrantapi "github.com/qdrant/go-client/qdrant"

	"pr-guard-agent/internal/config"
)

const (
	defaultHost           = "localhost"
	defaultPort           = 6334
	defaultCollectionName = "pr_guard_code_chunks"
	defaultVectorSize     = 1536
	defaultTimeout        = 10 * time.Second

	payloadProjectID       = "project_id"
	payloadCodeVersionHash = "code_version_hash"
	payloadChunkID         = "chunk_id"
	payloadFilePath        = "file_path"
	payloadSymbolName      = "symbol_name"
	payloadSymbolType      = "symbol_type"
	payloadStartLine       = "start_line"
	payloadEndLine         = "end_line"
	payloadContentHash     = "content_hash"
)

type Client struct {
	client         *qdrantapi.Client
	collectionName string
	vectorSize     int
	distance       qdrantapi.Distance
	timeout        time.Duration
}

func NewClient(cfg config.QdrantConfig) (*Client, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = defaultHost
	}

	port := cfg.Port
	if port <= 0 {
		port = defaultPort
	}

	collectionName := strings.TrimSpace(cfg.CollectionName)
	if collectionName == "" {
		collectionName = defaultCollectionName
	}

	vectorSize := cfg.VectorSize
	if vectorSize <= 0 {
		vectorSize = defaultVectorSize
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	qdrantClient, err := qdrantapi.NewClient(&qdrantapi.Config{
		Host:                   host,
		Port:                   port,
		PoolSize:               1,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create qdrant client failed: %w", err)
	}

	return &Client{
		client:         qdrantClient,
		collectionName: collectionName,
		vectorSize:     vectorSize,
		distance:       parseDistance(cfg.Distance),
		timeout:        timeout,
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *Client) EnsureCollection(ctx context.Context) error {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	exists, err := c.client.CollectionExists(requestCtx, c.collectionName)
	if err != nil {
		return fmt.Errorf("check qdrant collection failed: %w", err)
	}
	if exists {
		return nil
	}

	if err := c.client.CreateCollection(requestCtx, &qdrantapi.CreateCollection{
		CollectionName: c.collectionName,
		VectorsConfig: qdrantapi.NewVectorsConfig(&qdrantapi.VectorParams{
			Size:     uint64(c.vectorSize),
			Distance: c.distance,
		}),
	}); err != nil {
		return fmt.Errorf("create qdrant collection failed: %w", err)
	}

	return nil
}

func (c *Client) UpsertChunks(ctx context.Context, points []ChunkPoint) error {
	if len(points) == 0 {
		return errors.New("points is empty")
	}

	qdrantPoints := make([]*qdrantapi.PointStruct, 0, len(points))
	for i, point := range points {
		if len(point.Vector) != c.vectorSize {
			return fmt.Errorf("point at index %d vector dimension mismatch: got %d, want %d", i, len(point.Vector), c.vectorSize)
		}

		payload, err := chunkPointPayload(point)
		if err != nil {
			return fmt.Errorf("build point payload failed at index %d: %w", i, err)
		}

		qdrantPoints = append(qdrantPoints, &qdrantapi.PointStruct{
			Id:      qdrantapi.NewIDNum(point.ID),
			Vectors: qdrantapi.NewVectorsDense(point.Vector),
			Payload: payload,
		})
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	wait := true
	requestTimeout := uint64(c.timeout.Seconds())
	if _, err := c.client.Upsert(requestCtx, &qdrantapi.UpsertPoints{
		CollectionName: c.collectionName,
		Points:         qdrantPoints,
		Wait:           &wait,
		Timeout:        &requestTimeout,
	}); err != nil {
		return fmt.Errorf("upsert qdrant points failed: %w", err)
	}

	return nil
}

func (c *Client) SearchTopK(ctx context.Context, vector []float32, filter SearchFilter, topK uint64) ([]SearchResult, error) {
	if len(vector) != c.vectorSize {
		return nil, fmt.Errorf("query vector dimension mismatch: got %d, want %d", len(vector), c.vectorSize)
	}
	if filter.ProjectID == 0 {
		return nil, errors.New("project_id is required")
	}
	codeVersionHash := strings.TrimSpace(filter.CodeVersionHash)
	if codeVersionHash == "" {
		return nil, errors.New("code_version_hash is required")
	}
	if topK == 0 {
		topK = 5
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	requestTimeout := uint64(c.timeout.Seconds())
	scoredPoints, err := c.client.Query(requestCtx, &qdrantapi.QueryPoints{
		CollectionName: c.collectionName,
		Query:          qdrantapi.NewQueryDense(vector),
		Filter: &qdrantapi.Filter{
			Must: []*qdrantapi.Condition{
				qdrantapi.NewMatchInt(payloadProjectID, int64(filter.ProjectID)),
				qdrantapi.NewMatch(payloadCodeVersionHash, codeVersionHash),
			},
		},
		Limit:       &topK,
		WithPayload: qdrantapi.NewWithPayload(true),
		Timeout:     &requestTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("search qdrant points failed: %w", err)
	}

	results := make([]SearchResult, 0, len(scoredPoints))
	for _, point := range scoredPoints {
		payload := point.GetPayload()
		results = append(results, SearchResult{
			ID:         pointIDNum(point.GetId()),
			Score:      point.GetScore(),
			ChunkID:    uint(payloadInt(payload, payloadChunkID)),
			FilePath:   payloadString(payload, payloadFilePath),
			SymbolName: payloadString(payload, payloadSymbolName),
			SymbolType: payloadString(payload, payloadSymbolType),
			StartLine:  int(payloadInt(payload, payloadStartLine)),
			EndLine:    int(payloadInt(payload, payloadEndLine)),
		})
	}

	return results, nil
}

func parseDistance(distance string) qdrantapi.Distance {
	switch strings.ToLower(strings.TrimSpace(distance)) {
	case "dot":
		return qdrantapi.Distance_Dot
	case "euclid", "euclidean":
		return qdrantapi.Distance_Euclid
	case "manhattan":
		return qdrantapi.Distance_Manhattan
	default:
		return qdrantapi.Distance_Cosine
	}
}

func chunkPointPayload(point ChunkPoint) (map[string]*qdrantapi.Value, error) {
	return qdrantapi.TryValueMap(map[string]any{
		payloadProjectID:       point.ProjectID,
		payloadCodeVersionHash: point.CodeVersionHash,
		payloadChunkID:         point.ChunkID,
		payloadFilePath:        point.FilePath,
		payloadSymbolName:      point.SymbolName,
		payloadSymbolType:      point.SymbolType,
		payloadStartLine:       point.StartLine,
		payloadEndLine:         point.EndLine,
		payloadContentHash:     point.ContentHash,
	})
}

func payloadString(payload map[string]*qdrantapi.Value, key string) string {
	if payload == nil || payload[key] == nil {
		return ""
	}
	return payload[key].GetStringValue()
}

func payloadInt(payload map[string]*qdrantapi.Value, key string) int64 {
	if payload == nil || payload[key] == nil {
		return 0
	}
	return payload[key].GetIntegerValue()
}

func pointIDNum(id *qdrantapi.PointId) uint {
	if id == nil {
		return 0
	}
	return uint(id.GetNum())
}
