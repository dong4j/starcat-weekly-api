package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/starcat-app/starcat-weekly-api/internal/model"
)

func TestWeeklyOperationalStatsAndRecentBatches(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "weekly.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.EnqueueIngestBatch(model.EnqueueBatchRequest{
		ID: "batch-1", SourceCode: model.SourceWeekly, Kind: model.IngestKindCollector,
		IdempotencyKey: "secret-execution-key", Cursor: map[string]any{"page": 3},
		Candidates: []model.IngestCandidate{{Owner: "starcat", Repo: "app", ExternalKey: "one", OccurredAt: time.Now()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := s.GetWeeklyOperationalStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.BatchStates[model.IngestBatchPending] != 1 || stats.QueueStates[model.IngestItemPending] != 1 {
		t.Fatalf("unexpected queue stats: %#v", stats)
	}
	batches, err := s.ListRecentIngestBatches(model.IngestBatchPending, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || batches[0].ID != "batch-1" || batches[0].IdempotencyKey != "" || batches[0].Cursor != nil {
		t.Fatalf("unexpected recent batches: %#v", batches)
	}
}
