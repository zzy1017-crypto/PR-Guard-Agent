package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"pr-guard-agent/internal/config"
	reportcache "pr-guard-agent/pkg/cache"
	"pr-guard-agent/pkg/llm"
)

func TestNewReportServiceInjectsReportCache(t *testing.T) {
	want := reportcache.NewReportCache(nil, time.Hour, true)
	service := NewReportService(nil, nil, nil, want)

	if service.reportCache != want {
		t.Fatalf("reportCache = %p, want %p", service.reportCache, want)
	}
}

func TestBuildRiskReportModel(t *testing.T) {
	report := &llm.RiskReport{
		RiskLevel:       "medium",
		Summary:         "transaction risk",
		AffectedModules: []string{"order", "stock"},
		PossibleRisks:   []string{"stock may be deducted without order creation"},
		SuggestedTests:  []string{"test rollback when order creation fails"},
		RelatedFiles:    []string{"internal/service/order.go"},
		RelatedSymbols:  []string{"OrderService.CreateOrder"},
		Confidence:      0.8,
	}

	model, err := buildRiskReportModel(1, 2, report, `{"risk_level":"medium"}`)
	if err != nil {
		t.Fatalf("buildRiskReportModel() error = %v", err)
	}

	if model.ProjectID != 1 || model.DiffID != 2 || model.RiskLevel != "medium" {
		t.Fatalf("unexpected report identity fields: %#v", model)
	}
	if model.Summary != report.Summary || model.Confidence != report.Confidence {
		t.Fatalf("unexpected summary/confidence: %#v", model)
	}
	assertJSONStringSlice(t, "affected_modules", model.AffectedModules, report.AffectedModules)
	assertJSONStringSlice(t, "possible_risks", model.PossibleRisks, report.PossibleRisks)
	assertJSONStringSlice(t, "suggested_tests", model.SuggestedTests, report.SuggestedTests)
	assertJSONStringSlice(t, "related_files", model.RelatedFiles, report.RelatedFiles)
	assertJSONStringSlice(t, "related_symbols", model.RelatedSymbols, report.RelatedSymbols)
}

func TestBuildFallbackReport(t *testing.T) {
	retrieveResult := &RetrieveResult{
		RelatedFiles: []string{"internal/service/order.go"},
		RelatedSymbols: []RelatedSymbolResult{
			{SymbolName: "OrderService.CreateOrder"},
			{SymbolName: "OrderService.CreateOrder"},
			{SymbolName: ""},
		},
	}

	result := BuildFallbackReport(3, 7, retrieveResult, "llm_timeout")
	if result.ReportID != 0 || result.ProjectID != 3 || result.DiffID != 7 {
		t.Fatalf("unexpected fallback identity: %#v", result)
	}
	if !result.Degraded || result.DegradedReason != "llm_timeout" || result.Cached {
		t.Fatalf("unexpected fallback state: %#v", result)
	}
	if result.RiskLevel != "medium" || result.Confidence != 0.2 {
		t.Fatalf("unexpected fallback risk: %#v", result)
	}
	if result.AffectedModules == nil || len(result.AffectedModules) != 0 {
		t.Fatalf("AffectedModules = %#v, want non-nil empty slice", result.AffectedModules)
	}
	if len(result.RelatedFiles) != 1 || len(result.RelatedSymbols) != 1 {
		t.Fatalf("unexpected fallback sources: files=%#v symbols=%#v", result.RelatedFiles, result.RelatedSymbols)
	}
}

func TestReportServiceCacheUsesActualTopK(t *testing.T) {
	db, sqlMock := newReportServiceTestDB(t)
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	rag := &fakeReportRAGService{}
	service := NewReportService(
		db,
		nil,
		llm.NewClient(config.LLMConfig{Provider: "mock", MockMode: "normal", TimeoutSeconds: 1}),
		reportcache.NewReportCache(redisClient, time.Hour, true),
	)
	service.ragService = rag

	expectReportIdentityLookup(sqlMock)
	expectRiskReportInsert(sqlMock, 101)
	firstTop5, err := service.AnalyzeDiff(context.Background(), 6, 5, 5)
	if err != nil {
		t.Fatalf("first top_k=5 AnalyzeDiff() error = %v", err)
	}
	if firstTop5.ReportID != 101 || firstTop5.Cached || firstTop5.Degraded {
		t.Fatalf("first top_k=5 result = %#v", firstTop5)
	}

	top5Key := reportcache.BuildReportCacheKey(6, "version-a", "diff-a", 5)
	if !redisServer.Exists(top5Key) {
		t.Fatalf("top_k=5 cache key %q was not written", top5Key)
	}

	expectReportIdentityLookup(sqlMock)
	secondTop5, err := service.AnalyzeDiff(context.Background(), 6, 5, 5)
	if err != nil {
		t.Fatalf("second top_k=5 AnalyzeDiff() error = %v", err)
	}
	if secondTop5.ReportID != firstTop5.ReportID || !secondTop5.Cached {
		t.Fatalf("second top_k=5 result = %#v, first = %#v", secondTop5, firstTop5)
	}
	if len(rag.topKs) != 1 {
		t.Fatalf("RAG calls after top_k=5 cache hit = %v, want [5]", rag.topKs)
	}

	expectReportIdentityLookup(sqlMock)
	expectRiskReportInsert(sqlMock, 102)
	firstTop8, err := service.AnalyzeDiff(context.Background(), 6, 5, 8)
	if err != nil {
		t.Fatalf("first top_k=8 AnalyzeDiff() error = %v", err)
	}
	if firstTop8.ReportID != 102 || firstTop8.Cached || firstTop8.Degraded {
		t.Fatalf("first top_k=8 result = %#v", firstTop8)
	}
	if firstTop8.ReportID == firstTop5.ReportID {
		t.Fatalf("top_k=8 reused top_k=5 report_id %d", firstTop8.ReportID)
	}

	top8Key := reportcache.BuildReportCacheKey(6, "version-a", "diff-a", 8)
	if top8Key == top5Key || !redisServer.Exists(top8Key) {
		t.Fatalf("top_k cache keys are not isolated: top5=%q top8=%q", top5Key, top8Key)
	}
	if redisServer.Exists("prguard:report:6:version-a:diff-a") {
		t.Fatal("legacy cache key must not be read or written")
	}

	expectReportIdentityLookup(sqlMock)
	secondTop8, err := service.AnalyzeDiff(context.Background(), 6, 5, 8)
	if err != nil {
		t.Fatalf("second top_k=8 AnalyzeDiff() error = %v", err)
	}
	if secondTop8.ReportID != firstTop8.ReportID || !secondTop8.Cached {
		t.Fatalf("second top_k=8 result = %#v, first = %#v", secondTop8, firstTop8)
	}
	if len(rag.topKs) != 2 || rag.topKs[0] != 5 || rag.topKs[1] != 8 {
		t.Fatalf("RAG top_k calls = %v, want [5 8]", rag.topKs)
	}
	assertReportServiceSQLExpectations(t, sqlMock)
}

func TestReportServiceFallbackIsNotCached(t *testing.T) {
	db, sqlMock := newReportServiceTestDB(t)
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	rag := &fakeReportRAGService{}
	service := NewReportService(
		db,
		nil,
		llm.NewClient(config.LLMConfig{Provider: "mock", MockMode: "invalid_json", TimeoutSeconds: 1}),
		reportcache.NewReportCache(redisClient, time.Hour, true),
	)
	service.ragService = rag

	expectReportIdentityLookup(sqlMock)
	result, err := service.AnalyzeDiff(context.Background(), 6, 5, 5)
	if err != nil {
		t.Fatalf("AnalyzeDiff() error = %v", err)
	}
	if !result.Degraded || result.Cached || result.ReportID != 0 {
		t.Fatalf("fallback result = %#v", result)
	}
	key := reportcache.BuildReportCacheKey(6, "version-a", "diff-a", 5)
	if redisServer.Exists(key) {
		t.Fatalf("fallback result was cached at %q", key)
	}
	assertReportServiceSQLExpectations(t, sqlMock)
}

func TestReportServiceRedisSetFailureDoesNotFailAnalysis(t *testing.T) {
	db, sqlMock := newReportServiceTestDB(t)
	redisClient := newReportServiceHookedRedisClient(func(cmd redis.Cmder) error {
		switch cmd.Name() {
		case "get":
			return redis.Nil
		case "set":
			return errors.New("redis set unavailable")
		default:
			return nil
		}
	})
	t.Cleanup(func() { _ = redisClient.Close() })

	rag := &fakeReportRAGService{}
	service := NewReportService(
		db,
		nil,
		llm.NewClient(config.LLMConfig{Provider: "mock", MockMode: "normal", TimeoutSeconds: 1}),
		reportcache.NewReportCache(redisClient, time.Hour, true),
	)
	service.ragService = rag

	expectReportIdentityLookup(sqlMock)
	expectRiskReportInsert(sqlMock, 103)
	result, err := service.AnalyzeDiff(context.Background(), 6, 5, 5)
	if err != nil {
		t.Fatalf("AnalyzeDiff() error = %v", err)
	}
	if result.ReportID != 103 || result.Cached || result.Degraded {
		t.Fatalf("result after Redis SET failure = %#v", result)
	}
	assertReportServiceSQLExpectations(t, sqlMock)
}

type fakeReportRAGService struct {
	topKs []int
}

func (f *fakeReportRAGService) RetrieveRelatedChunks(
	_ context.Context,
	projectID uint,
	diffID uint,
	topK int,
) (*RetrieveResult, error) {
	f.topKs = append(f.topKs, topK)
	return &RetrieveResult{
		ProjectID:      projectID,
		DiffID:         diffID,
		TopK:           topK,
		RelatedFiles:   make([]string, 0),
		RelatedSymbols: make([]RelatedSymbolResult, 0),
		ContextChunks:  make([]ContextChunkResult, 0),
	}, nil
}

func newReportServiceTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func expectReportIdentityLookup(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `projects`").WillReturnRows(
		sqlmock.NewRows([]string{"id", "name", "code_version_hash", "create_at", "update_at"}).
			AddRow(6, "test-project", "version-a", now, now),
	)
	mock.ExpectQuery("SELECT \\* FROM `diff_records`").WillReturnRows(
		sqlmock.NewRows([]string{"id", "project_id", "diff_hash", "diff_text", "create_at", "update_at"}).
			AddRow(5, 6, "diff-a", "diff --git a/a.go b/a.go\n+changed", now, now),
	)
}

func expectRiskReportInsert(mock sqlmock.Sqlmock, reportID int64) {
	mock.ExpectExec("INSERT INTO `risk_reports`").
		WillReturnResult(sqlmock.NewResult(reportID, 1))
}

func assertReportServiceSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type reportServiceRedisHook struct {
	process func(redis.Cmder) error
}

func newReportServiceHookedRedisClient(process func(redis.Cmder) error) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:            "unused",
		Protocol:        2,
		DisableIdentity: true,
		DialTimeout:     time.Millisecond,
		MaxRetries:      -1,
	})
	client.AddHook(reportServiceRedisHook{process: process})
	return client
}

func (h reportServiceRedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h reportServiceRedisHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		return h.process(cmd)
	}
}

func (h reportServiceRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func assertJSONStringSlice(t *testing.T, field string, raw string, want []string) {
	t.Helper()

	var got []string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("%s is not valid JSON: %v", field, err)
	}
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", field, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q", field, i, got[i], want[i])
		}
	}
}
