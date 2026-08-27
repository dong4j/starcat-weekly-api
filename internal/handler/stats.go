// Package handler exposes read-only Weekly operations data to the Admin Console.
package handler

import (
	"net/http"
	"strconv"

	"github.com/starcat-app/starcat-weekly-api/internal/model"
	"github.com/starcat-app/starcat-weekly-api/internal/store"
)

type weeklyOperationsStore interface {
	GetWeeklyOperationalStats() (store.WeeklyOperationalStats, error)
	ListRecentIngestBatches(status string, limit int) ([]model.IngestBatch, error)
}

// HandleOperationalStats returns repository, source, batch and queue counts.
func HandleOperationalStats(s weeklyOperationsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		stats, err := s.GetWeeklyOperationalStats()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to aggregate weekly stats", nil)
			return
		}
		writeJSON(w, stats)
	}
}

// HandleRecentIngestBatches returns a bounded batch list for queue diagnostics.
func HandleRecentIngestBatches(s weeklyOperationsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		if status != "" && !validBatchStatus(status) {
			writeError(w, http.StatusBadRequest, "INVALID_STATUS", "unsupported batch status", nil)
			return
		}
		limit := 25
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 100 {
				writeError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 100", nil)
				return
			}
			limit = parsed
		}
		batches, err := s.ListRecentIngestBatches(status, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list ingest batches", nil)
			return
		}
		writeJSON(w, batches)
	}
}

func validBatchStatus(status string) bool {
	switch status {
	case model.IngestBatchPending, model.IngestBatchProcessing, model.IngestBatchSuccess,
		model.IngestBatchPartialSuccess, model.IngestBatchFailed:
		return true
	default:
		return false
	}
}
