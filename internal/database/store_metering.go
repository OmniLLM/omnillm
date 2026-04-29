package database

import (
	"fmt"
	"strings"
	"time"
)

// InsertMeteringRecord writes one request_logs row.
func (db *Database) InsertMeteringRecord(r MeteringRecord) error {
	_, err := db.db.Exec(
		`INSERT INTO request_logs
				(request_id, model_id, model_used, provider_id, client, api_shape,
				 input_tokens, output_tokens, total_tokens,
				 latency_ms, is_stream, status_code, error_message, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RequestID, r.ModelID, r.ModelUsed, r.ProviderID, r.Client, r.APIShape,
		r.InputTokens, r.OutputTokens, r.TotalTokens,
		r.LatencyMS, boolToInt(r.IsStream), r.StatusCode, r.ErrorMessage,
		r.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// MeteringFilter holds optional filters for metering queries.
type MeteringFilter struct {
	ModelID    string
	ProviderID string
	APIShape   string
	Since      time.Time
	Until      time.Time
}

// ListMeteringRecords returns paginated request_logs rows newest-first.
func (db *Database) ListMeteringRecords(f MeteringFilter, limit, offset int) ([]MeteringRecord, int64, error) {
	where, args := meteringWhere(f)

	// count
	var total int64
	countSQL := "SELECT COUNT(*) FROM request_logs" + where
	if err := db.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("metering count: %w", err)
	}

	// rows
	querySQL := `SELECT id, request_id, model_id, model_used, provider_id, client, api_shape,
		input_tokens, output_tokens, total_tokens, latency_ms, is_stream,
		status_code, error_message, created_at
		FROM request_logs` + where +
		` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("metering list: %w", err)
	}
	defer rows.Close()

	var records []MeteringRecord
	for rows.Next() {
		var r MeteringRecord
		var isStream int
		var createdAt string
		if err := rows.Scan(
			&r.ID, &r.RequestID, &r.ModelID, &r.ModelUsed, &r.ProviderID, &r.Client, &r.APIShape,
			&r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.LatencyMS, &isStream,
			&r.StatusCode, &r.ErrorMessage, &createdAt,
		); err != nil {
			return nil, 0, fmt.Errorf("metering scan: %w", err)
		}
		r.IsStream = isStream == 1
		r.CreatedAt = parseTime(createdAt)
		records = append(records, r)
	}
	return records, total, rows.Err()
}

// MeteringStats holds aggregate usage numbers.
type MeteringStats struct {
	TotalRequests int64   `json:"total_requests"`
	TotalInput    int64   `json:"total_input_tokens"`
	TotalOutput   int64   `json:"total_output_tokens"`
	TotalTokens   int64   `json:"total_tokens"`
	AvgLatencyMS  float64 `json:"avg_latency_ms"`
	ErrorCount    int64   `json:"error_count"`
}

// GetMeteringStats returns aggregate stats for the given filter window.
func (db *Database) GetMeteringStats(f MeteringFilter) (MeteringStats, error) {
	where, args := meteringWhere(f)
	sql := `SELECT
		COUNT(*),
		COALESCE(SUM(input_tokens),  0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens),  0),
		COALESCE(AVG(latency_ms),    0),
		COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0)
		FROM request_logs` + where
	var s MeteringStats
	err := db.db.QueryRow(sql, args...).Scan(
		&s.TotalRequests, &s.TotalInput, &s.TotalOutput,
		&s.TotalTokens, &s.AvgLatencyMS, &s.ErrorCount,
	)
	return s, err
}

// ModelBreakdown holds per-model aggregate usage.
type ModelBreakdown struct {
	ModelID      string  `json:"model_id"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
}

// GetMeteringByModel returns per-model breakdown for the given filter window.
func (db *Database) GetMeteringByModel(f MeteringFilter) ([]ModelBreakdown, error) {
	where, args := meteringWhere(f)
	sql := `SELECT model_id,
		COUNT(*),
		COALESCE(SUM(input_tokens),  0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens),  0),
		COALESCE(AVG(latency_ms),    0)
		FROM request_logs` + where +
		` GROUP BY model_id ORDER BY SUM(total_tokens) DESC`
	rows, err := db.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("metering by-model: %w", err)
	}
	defer rows.Close()
	var result []ModelBreakdown
	for rows.Next() {
		var b ModelBreakdown
		if err := rows.Scan(&b.ModelID, &b.Requests, &b.InputTokens, &b.OutputTokens, &b.TotalTokens, &b.AvgLatencyMS); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

// ProviderBreakdown holds per-provider aggregate usage.
type ProviderBreakdown struct {
	ProviderID   string  `json:"provider_id"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
}

// GetMeteringByProvider returns per-provider breakdown for the given filter window.
func (db *Database) GetMeteringByProvider(f MeteringFilter) ([]ProviderBreakdown, error) {
	where, args := meteringWhere(f)
	sql := `SELECT provider_id,
		COUNT(*),
		COALESCE(SUM(input_tokens),  0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens),  0),
		COALESCE(AVG(latency_ms),    0)
		FROM request_logs` + where +
		` GROUP BY provider_id ORDER BY SUM(total_tokens) DESC`
	rows, err := db.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("metering by-provider: %w", err)
	}
	defer rows.Close()
	var result []ProviderBreakdown
	for rows.Next() {
		var b ProviderBreakdown
		if err := rows.Scan(&b.ProviderID, &b.Requests, &b.InputTokens, &b.OutputTokens, &b.TotalTokens, &b.AvgLatencyMS); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

// ClientBreakdown holds per-client aggregate usage.
type ClientBreakdown struct {
	Client       string  `json:"client"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
}

// GetMeteringByClient returns per-client breakdown for the given filter window.
func (db *Database) GetMeteringByClient(f MeteringFilter) ([]ClientBreakdown, error) {
	where, args := meteringWhere(f)
	sql := `SELECT client,
		COUNT(*),
		COALESCE(SUM(input_tokens),  0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens),  0),
		COALESCE(AVG(latency_ms),    0)
		FROM request_logs` + where +
		` GROUP BY client ORDER BY SUM(total_tokens) DESC`
	rows, err := db.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("metering by-client: %w", err)
	}
	defer rows.Close()
	var result []ClientBreakdown
	for rows.Next() {
		var b ClientBreakdown
		if err := rows.Scan(&b.Client, &b.Requests, &b.InputTokens, &b.OutputTokens, &b.TotalTokens, &b.AvgLatencyMS); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

// meteringWhere builds a WHERE clause and args slice from a MeteringFilter.
func meteringWhere(f MeteringFilter) (string, []any) {
	var clauses []string
	var args []any

	if f.ModelID != "" {
		clauses = append(clauses, "model_id = ?")
		args = append(args, f.ModelID)
	}
	if f.ProviderID != "" {
		clauses = append(clauses, "provider_id = ?")
		args = append(args, f.ProviderID)
	}
	if f.APIShape != "" {
		clauses = append(clauses, "api_shape = ?")
		args = append(args, f.APIShape)
	}
	if !f.Since.IsZero() {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}

	if len(clauses) == 0 {
		return "", args
	}
	var sb strings.Builder
	sb.WriteString(" WHERE ")
	sb.WriteString(clauses[0])
	for _, c := range clauses[1:] {
		sb.WriteString(" AND ")
		sb.WriteString(c)
	}
	return sb.String(), args
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// GetDistinctMeteringModels returns distinct model_ids from request_logs within the filter window.
func (db *Database) GetDistinctMeteringModels(f MeteringFilter) ([]string, error) {
	where, args := meteringWhere(f)
	sql := `SELECT DISTINCT model_id FROM request_logs` + where + ` ORDER BY model_id`
	rows, err := db.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("distinct models: %w", err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// GetDistinctMeteringProviders returns distinct provider_ids from request_logs within the filter window.
func (db *Database) GetDistinctMeteringProviders(f MeteringFilter) ([]string, error) {
	where, args := meteringWhere(f)
	sql := `SELECT DISTINCT provider_id FROM request_logs` + where + ` ORDER BY provider_id`
	rows, err := db.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("distinct providers: %w", err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
