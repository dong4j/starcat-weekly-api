package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/starcat-app/starcat-weekly-api/internal/ingest"
	"github.com/starcat-app/starcat-weekly-api/internal/middleware"
	"github.com/starcat-app/starcat-weekly-api/internal/model"
	"github.com/starcat-app/starcat-weekly-api/internal/store"
)

func TestImportsHandlerAcceptsWholeBatchWithoutGitHubCall(t *testing.T) {
	handler, repository := newImportsTestHandler(t)
	body := []byte(`{
		"source_code":"ai_intelligence",
		"idempotency_key":"article-1",
		"repositories":[
			{"owner":"Acme","repo":"Agent","title":"AI Agent"},
			{"owner":"acme","repo":"agent"}
		]
	}`)
	recorder := httptest.NewRecorder()
	handler.HandleCreate(recorder, httptest.NewRequest(http.MethodPost, "/internal/imports", bytes.NewReader(body)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope model.Envelope[model.IngestBatchAcceptance]
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Total != 1 || envelope.Data.DuplicateCount != 1 || envelope.Data.Status != model.IngestBatchPending {
		t.Fatalf("acceptance=%#v", envelope.Data)
	}
	batch, err := repository.GetIngestBatch(envelope.Data.BatchID, true)
	if err != nil {
		t.Fatal(err)
	}
	if batch == nil || batch.Status != model.IngestBatchPending || len(batch.Items) != 1 {
		t.Fatalf("batch=%#v", batch)
	}

	// 同一幂等键重放应返回原 batch，而不是创建第二批。
	replayed := httptest.NewRecorder()
	handler.HandleCreate(replayed, httptest.NewRequest(http.MethodPost, "/internal/imports", bytes.NewReader(body)))
	var replayEnvelope model.Envelope[model.IngestBatchAcceptance]
	if err := json.Unmarshal(replayed.Body.Bytes(), &replayEnvelope); err != nil {
		t.Fatal(err)
	}
	if replayEnvelope.Data.BatchID != envelope.Data.BatchID {
		t.Fatalf("replay batch=%s original=%s", replayEnvelope.Data.BatchID, envelope.Data.BatchID)
	}
}

func TestImportsHandlerRejectsCrawlerOnlySourceAndUnknownFields(t *testing.T) {
	handler, _ := newImportsTestHandler(t)
	cases := [][]byte{
		[]byte(`{"source_code":"hellogithub","idempotency_key":"x","repositories":[{"owner":"HelloGitHub-Team","repo":"geese"}]}`),
		[]byte(`{"source_code":"ai_intelligence","idempotency_key":"x","unknown":true,"repositories":[{"owner":"acme","repo":"agent"}]}`),
	}
	for _, body := range cases {
		recorder := httptest.NewRecorder()
		handler.HandleCreate(recorder, httptest.NewRequest(http.MethodPost, "/internal/imports", bytes.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestImportsHandlerListsManualSourcesAndReadsBatch(t *testing.T) {
	handler, repository := newImportsTestHandler(t)
	result, err := repository.EnqueueIngestBatch(model.EnqueueBatchRequest{
		ID: "batch-status", SourceCode: model.SourceAIIntelligence,
		Kind: model.IngestKindManualImport, IdempotencyKey: "batch-status",
		Candidates: []model.IngestCandidate{{Owner: "acme", Repo: "agent", ExternalKey: "batch-status:acme/agent"}},
	})
	if err != nil || !result.Created {
		t.Fatalf("result=%#v err=%v", result, err)
	}

	sourceRecorder := httptest.NewRecorder()
	handler.HandleSources(sourceRecorder, httptest.NewRequest(http.MethodGet, "/internal/sources?manual_import=true", nil))
	var sourceEnvelope model.Envelope[[]model.SourceStatus]
	if err := json.Unmarshal(sourceRecorder.Body.Bytes(), &sourceEnvelope); err != nil {
		t.Fatal(err)
	}
	if len(sourceEnvelope.Data) != 1 || sourceEnvelope.Data[0].Code != model.SourceAIIntelligence || !sourceEnvelope.Data[0].ManualImportEnabled {
		t.Fatalf("sources=%#v", sourceEnvelope.Data)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/imports/{batch_id}", handler.HandleBatch)
	batchRecorder := httptest.NewRecorder()
	mux.ServeHTTP(batchRecorder, httptest.NewRequest(http.MethodGet, "/internal/imports/batch-status", nil))
	if batchRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", batchRecorder.Code, batchRecorder.Body.String())
	}
}

func TestImportsHandlerCreatesManualSourceAndUsesItImmediately(t *testing.T) {
	handler, _ := newImportsTestHandler(t)
	body := []byte(`{"code":"developer_tools","display_name_zh":"开发工具","display_name_en":"Developer Tools"}`)
	created := httptest.NewRecorder()
	handler.HandleCreateSource(created, httptest.NewRequest(http.MethodPost, "/internal/sources", bytes.NewReader(body)))
	if created.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", created.Code, created.Body.String())
	}
	var sourceEnvelope model.Envelope[model.SourceStatus]
	if err := json.Unmarshal(created.Body.Bytes(), &sourceEnvelope); err != nil {
		t.Fatal(err)
	}
	if sourceEnvelope.Data.Code != "developer_tools" || !sourceEnvelope.Data.ManualImportEnabled {
		t.Fatalf("source=%#v", sourceEnvelope.Data)
	}

	importBody := []byte(`{"source_code":"developer_tools","idempotency_key":"tools-1","repositories":[{"owner":"acme","repo":"agent"}]}`)
	accepted := httptest.NewRecorder()
	handler.HandleCreate(accepted, httptest.NewRequest(http.MethodPost, "/internal/imports", bytes.NewReader(importBody)))
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", accepted.Code, accepted.Body.String())
	}

	duplicate := httptest.NewRecorder()
	handler.HandleCreateSource(duplicate, httptest.NewRequest(http.MethodPost, "/internal/sources", bytes.NewReader(body)))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
}

func TestImportsHandlerRejectsInvalidManualSource(t *testing.T) {
	handler, _ := newImportsTestHandler(t)
	for _, body := range [][]byte{
		[]byte(`{"code":"AI News","display_name_zh":"AI","display_name_en":"AI"}`),
		[]byte(`{"code":"ai_news","display_name_zh":"","display_name_en":"AI"}`),
		[]byte(`{"code":"ai_news","display_name_zh":"AI","display_name_en":"AI","unknown":true}`),
	} {
		recorder := httptest.NewRecorder()
		handler.HandleCreateSource(recorder, httptest.NewRequest(http.MethodPost, "/internal/sources", bytes.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestImportsRouteRequiresAdminKey(t *testing.T) {
	handler, _ := newImportsTestHandler(t)
	auth := middleware.NewBearerAuth([]string{"admin-secret-key-123456"})
	protected := auth.Wrap(http.HandlerFunc(handler.HandleCreate))
	body := []byte(`{"source_code":"ai_intelligence","idempotency_key":"auth","repositories":[{"owner":"acme","repo":"agent"}]}`)

	unauthorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/imports", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer ordinary-client-key")
	protected.ServeHTTP(unauthorized, request)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
}

func newImportsTestHandler(t *testing.T) (*ImportsHandler, *store.SQLiteStore) {
	t.Helper()
	repository, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "imports.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	service := ingest.NewService(repository, ingest.NewWakeSignal())
	return NewImportsHandler(service, repository), repository
}
