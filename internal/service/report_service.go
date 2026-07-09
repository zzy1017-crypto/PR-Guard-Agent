package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/pkg/embedding"
	"pr-guard-agent/pkg/llm"

	"gorm.io/gorm"
)

type ReportService struct {
	db             *gorm.DB
	projectRepo    *repository.ProjectRepository
	diffRepo       *repository.DiffRepository
	riskReportRepo *repository.RiskReportRepository
	ragService     *RAGService
	llmClient      *llm.Client
}

type AnalyzeResult struct {
	ReportID        uint     `json:"report_id"`
	ProjectID       uint     `json:"project_id"`
	DiffID          uint     `json:"diff_id"`
	RiskLevel       string   `json:"risk_level"`
	Summary         string   `json:"summary"`
	AffectedModules []string `json:"affected_modules"`
	PossibleRisks   []string `json:"possible_risks"`
	SuggestedTests  []string `json:"suggested_tests"`
	RelatedFiles    []string `json:"related_files"`
	RelatedSymbols  []string `json:"related_symbols"`
	Confidence      float64  `json:"confidence"`
	Cached          bool     `json:"cached"`
}

func NewReportService(db *gorm.DB, ragService *RAGService, llmClient *llm.Client) *ReportService {
	return &ReportService{
		db:             db,
		projectRepo:    repository.NewProjectRepository(db),
		diffRepo:       repository.NewDiffRepository(db),
		riskReportRepo: repository.NewRiskReportRepository(db),
		ragService:     ragService,
		llmClient:      llmClient,
	}
}

func AnalyzeDiff(projectID uint, diffID uint, topK int) (*AnalyzeResult, error) {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		return nil, fmt.Errorf("load config failed: %w", err)
	}

	embeddingClient := embedding.NewClient(cfg.Embedding)
	ragService := NewRAGService(database.DB, cfg.Qdrant, embeddingClient)
	reportService := NewReportService(database.DB, ragService, llm.NewClient(cfg.LLM))
	return reportService.AnalyzeDiff(projectID, diffID, topK)
}

func (s *ReportService) AnalyzeDiff(projectID uint, diffID uint, topK int) (*AnalyzeResult, error) {
	return s.AnalyzeDiffWithContext(context.Background(), projectID, diffID, topK)
}

func (s *ReportService) AnalyzeDiffWithContext(ctx context.Context, projectID uint, diffID uint, topK int) (*AnalyzeResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("report service is not initialized")
	}
	if s.projectRepo == nil || s.diffRepo == nil || s.riskReportRepo == nil {
		return nil, errors.New("report repository is not initialized")
	}
	if s.ragService == nil {
		return nil, errors.New("rag service is not initialized")
	}
	if s.llmClient == nil {
		return nil, errors.New("llm client is not initialized")
	}

	project, err := s.projectRepo.GetByID(projectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("query project failed: %w", err)
	}

	diff, err := s.diffRepo.GetByID(diffID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDiffNotFound
		}
		return nil, fmt.Errorf("query diff failed: %w", err)
	}
	if diff.ProjectID != project.ID {
		return nil, ErrDiffProjectMismatch
	}

	diffText := strings.TrimPrefix(diff.DiffText, "\uFEFF")
	if strings.TrimSpace(diffText) == "" {
		return nil, ErrDiffTextEmpty
	}

	retrieveResult, err := s.ragService.RetrieveRelatedChunksWithContext(ctx, project.ID, diff.ID, topK)
	if err != nil {
		return nil, fmt.Errorf("retrieve related chunks failed: %w", err)
	}

	prompt, err := llm.BuildRiskAnalysisPrompt(llm.RiskPromptInput{
		DiffText:      diffText,
		ContextChunks: toLLMContextChunks(retrieveResult.ContextChunks),
	})
	if err != nil {
		return nil, fmt.Errorf("build risk analysis prompt failed: %w", err)
	}

	rawOutput, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("generate risk report failed: %w", err)
	}

	report, err := llm.ParseRiskReport(rawOutput)
	if err != nil {
		return nil, fmt.Errorf("llm returned invalid risk report JSON: %w", err)
	}

	riskReport, err := buildRiskReportModel(project.ID, diff.ID, report, rawOutput)
	if err != nil {
		return nil, err
	}
	if err := s.riskReportRepo.Create(riskReport); err != nil {
		return nil, fmt.Errorf("save risk report failed: %w", err)
	}

	return &AnalyzeResult{
		ReportID:        riskReport.ID,
		ProjectID:       project.ID,
		DiffID:          diff.ID,
		RiskLevel:       report.RiskLevel,
		Summary:         report.Summary,
		AffectedModules: report.AffectedModules,
		PossibleRisks:   report.PossibleRisks,
		SuggestedTests:  report.SuggestedTests,
		RelatedFiles:    report.RelatedFiles,
		RelatedSymbols:  report.RelatedSymbols,
		Confidence:      report.Confidence,
		Cached:          false,
	}, nil
}

func toLLMContextChunks(chunks []ContextChunkResult) []llm.ContextChunk {
	result := make([]llm.ContextChunk, 0, len(chunks))
	for _, chunk := range chunks {
		result = append(result, llm.ContextChunk{
			ChunkID:    chunk.ChunkID,
			FilePath:   chunk.FilePath,
			SymbolName: chunk.SymbolName,
			SymbolType: chunk.SymbolType,
			StartLine:  chunk.StartLine,
			EndLine:    chunk.EndLine,
			Score:      chunk.Score,
			ChunkText:  chunk.ChunkText,
		})
	}
	return result
}

func buildRiskReportModel(projectID uint, diffID uint, report *llm.RiskReport, rawOutput string) (*model.RiskReport, error) {
	if report == nil {
		return nil, errors.New("risk report is nil")
	}

	affectedModules, err := marshalStringSlice("affected_modules", report.AffectedModules)
	if err != nil {
		return nil, err
	}
	possibleRisks, err := marshalStringSlice("possible_risks", report.PossibleRisks)
	if err != nil {
		return nil, err
	}
	suggestedTests, err := marshalStringSlice("suggested_tests", report.SuggestedTests)
	if err != nil {
		return nil, err
	}
	relatedFiles, err := marshalStringSlice("related_files", report.RelatedFiles)
	if err != nil {
		return nil, err
	}
	relatedSymbols, err := marshalStringSlice("related_symbols", report.RelatedSymbols)
	if err != nil {
		return nil, err
	}

	return &model.RiskReport{
		ProjectID:       projectID,
		DiffID:          diffID,
		RiskLevel:       report.RiskLevel,
		Summary:         report.Summary,
		AffectedModules: affectedModules,
		PossibleRisks:   possibleRisks,
		SuggestedTests:  suggestedTests,
		RelatedFiles:    relatedFiles,
		RelatedSymbols:  relatedSymbols,
		Confidence:      report.Confidence,
		RawJSON:         rawOutput,
	}, nil
}

func marshalStringSlice(field string, values []string) (string, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("marshal %s failed: %w", field, err)
	}
	return string(raw), nil
}
