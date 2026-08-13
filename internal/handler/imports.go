package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/starcat-app/starcat-weekly-api/internal/ingest"
	"github.com/starcat-app/starcat-weekly-api/internal/model"
)

type importService interface {
	Enqueue(model.EnqueueBatchRequest) (model.IngestBatchAcceptance, error)
}

type importRepository interface {
	GetIngestBatch(id string, includeItems bool) (*model.IngestBatch, error)
	GetSourceStatuses(manualOnly bool) ([]model.SourceStatus, error)
	CreateManualSource(input model.ManualSourceInput) (*model.SourceStatus, error)
}

type ImportsHandler struct {
	service    importService
	repository importRepository
}

func NewImportsHandler(service importService, repository importRepository) *ImportsHandler {
	return &ImportsHandler{service: service, repository: repository}
}

type importRequest struct {
	SourceCode     string                `json:"source_code"`
	IdempotencyKey string                `json:"idempotency_key"`
	Repositories   []importRepositoryDTO `json:"repositories"`
}

type importRepositoryDTO struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Title     string `json:"title,omitempty"`
	SourceURL string `json:"source_url,omitempty"`
}

var manualSourceCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

type createManualSourceRequest struct {
	Code          string `json:"code"`
	DisplayNameZH string `json:"display_name_zh"`
	DisplayNameEN string `json:"display_name_en"`
}

// HandleCreate 只持久化整批候选并返回 202；service 类型中没有 GitHub client，
// 因此 handler 不可能把外部网络等待带入请求事务。
func (h *ImportsHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request importRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", map[string]string{"error": err.Error()})
		return
	}
	candidates := make([]model.IngestCandidate, 0, len(request.Repositories))
	for _, repository := range request.Repositories {
		candidates = append(candidates, model.IngestCandidate{
			Owner: repository.Owner, Repo: repository.Repo, Title: repository.Title, SourceURL: repository.SourceURL,
		})
	}
	acceptance, err := h.service.Enqueue(model.EnqueueBatchRequest{
		SourceCode: strings.TrimSpace(request.SourceCode), Kind: model.IngestKindManualImport,
		IdempotencyKey: request.IdempotencyKey, Candidates: candidates,
	})
	if err != nil {
		var validationError *ingest.ValidationError
		if errors.As(err, &validationError) {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", validationError.Error(), nil)
			return
		}
		log.Printf("[handler] enqueue import: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to enqueue import", nil)
		return
	}
	writeJSONStatus(w, http.StatusAccepted, acceptance)
}

func (h *ImportsHandler) HandleBatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("batch_id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "batch_id is required", nil)
		return
	}
	batch, err := h.repository.GetIngestBatch(id, true)
	if err != nil {
		log.Printf("[handler] get ingest batch %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to query batch", nil)
		return
	}
	if batch == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "batch not found", map[string]string{"batch_id": id})
		return
	}
	writeJSON(w, batch)
}

func (h *ImportsHandler) HandleSources(w http.ResponseWriter, r *http.Request) {
	manualOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("manual_import")), "true")
	sources, err := h.repository.GetSourceStatuses(manualOnly)
	if err != nil {
		log.Printf("[handler] get source statuses: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to query sources", nil)
		return
	}
	writeJSONWithMeta(w, sources, &model.Meta{Total: len(sources)})
}

// HandleCreateSource 创建仅支持人工导入的来源分类。
// 能力位与图标由服务端固定，客户端只提供稳定 code 和中英文名称。
func (h *ImportsHandler) HandleCreateSource(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request createManualSourceRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", map[string]string{"error": err.Error()})
		return
	}
	request.Code = strings.TrimSpace(request.Code)
	request.DisplayNameZH = strings.TrimSpace(request.DisplayNameZH)
	request.DisplayNameEN = strings.TrimSpace(request.DisplayNameEN)
	if !manualSourceCodePattern.MatchString(request.Code) {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "code must match ^[a-z][a-z0-9_]{1,31}$", nil)
		return
	}
	if request.DisplayNameZH == "" || request.DisplayNameEN == "" || len([]rune(request.DisplayNameZH)) > 40 || len([]rune(request.DisplayNameEN)) > 80 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "display names are required and too long", nil)
		return
	}
	created, err := h.repository.CreateManualSource(model.ManualSourceInput{
		Code: request.Code, DisplayNameZH: request.DisplayNameZH, DisplayNameEN: request.DisplayNameEN,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "SOURCE_EXISTS", "source code already exists", nil)
			return
		}
		log.Printf("[handler] create manual source: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create source", nil)
		return
	}
	writeJSONStatus(w, http.StatusCreated, created)
}
