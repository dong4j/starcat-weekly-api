package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/starcat-app/starcat-weekly-api/internal/model"
)

// WeeklyOperationalStats is a bounded snapshot for the Admin Console.
type WeeklyOperationalStats struct {
	Repositories    int            `json:"repositories"`
	AvailableRepos  int            `json:"available_repos"`
	EnrichedRepos   int            `json:"enriched_repos"`
	WeeklyIssues    int            `json:"weekly_issues"`
	WeeklyEvents    int            `json:"weekly_events"`
	ZreadEvents     int            `json:"zread_events"`
	DiscoveryEvents int            `json:"discovery_events"`
	SourceEvents    int            `json:"source_events"`
	Pins            int            `json:"pins"`
	LatestEventAt   string         `json:"latest_event_at,omitempty"`
	BatchStates     map[string]int `json:"batch_states"`
	QueueStates     map[string]int `json:"queue_states"`
}

// GetWeeklyOperationalStats reads only counts and timestamps; candidate payloads remain private.
func (s *SQLiteStore) GetWeeklyOperationalStats() (WeeklyOperationalStats, error) {
	var result WeeklyOperationalStats
	var latest sql.NullString
	err := s.db.QueryRow(`
SELECT
  (SELECT COUNT(*) FROM github_repos),
  (SELECT COUNT(*) FROM github_repos WHERE is_available=1),
  (SELECT COUNT(*) FROM github_repos WHERE enriched_at IS NOT NULL),
  (SELECT COUNT(*) FROM weekly_issues),
  (SELECT COUNT(*) FROM weekly_extras),
  (SELECT COUNT(*) FROM zread_events),
  (SELECT COUNT(*) FROM discovery_submissions),
  (SELECT COUNT(*) FROM repo_source_events),
  (SELECT COUNT(*) FROM weekly_pins),
  (SELECT MAX(latest_event_at) FROM github_repos)
`).Scan(&result.Repositories, &result.AvailableRepos, &result.EnrichedRepos, &result.WeeklyIssues,
		&result.WeeklyEvents, &result.ZreadEvents, &result.DiscoveryEvents, &result.SourceEvents,
		&result.Pins, &latest)
	if err != nil {
		return WeeklyOperationalStats{}, err
	}
	result.LatestEventAt = latest.String
	result.BatchStates, err = s.countStates("ingest_batches")
	if err != nil {
		return WeeklyOperationalStats{}, err
	}
	result.QueueStates, err = s.countStates("ingest_items")
	if err != nil {
		return WeeklyOperationalStats{}, err
	}
	return result, nil
}

func (s *SQLiteStore) countStates(table string) (map[string]int, error) {
	// table is selected from code, never from request input.
	rows, err := s.db.Query("SELECT status, COUNT(*) FROM " + table + " GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
	}
	return result, rows.Err()
}

// ListRecentIngestBatches returns recent batch summaries without expanding item payloads.
func (s *SQLiteStore) ListRecentIngestBatches(status string, limit int) ([]model.IngestBatch, error) {
	query := `SELECT id, source_code, kind, idempotency_key, status, cursor_json,
       total, success, discarded, created_at, started_at, finished_at, updated_at
FROM ingest_batches`
	args := make([]any, 0, 2)
	if strings.TrimSpace(status) != "" {
		query += " WHERE status=?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.IngestBatch, 0, limit)
	for rows.Next() {
		batch, err := scanIngestBatch(rows)
		if err != nil {
			return nil, fmt.Errorf("scan recent ingest batch: %w", err)
		}
		// Idempotency keys and cursors are execution details, not required by the overview table.
		batch.IdempotencyKey = ""
		batch.Cursor = nil
		result = append(result, *batch)
	}
	return result, rows.Err()
}
