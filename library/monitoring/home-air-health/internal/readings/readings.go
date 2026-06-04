// Package readings is the unified time-series store every Home Health source
// writes into. It lives alongside the generator-owned store package (which is
// DO NOT EDIT) and manages its own `readings` table on the same SQLite DB, so
// regenerating the scaffold never clobbers it. The dashboard reads exclusively
// from here, which is what lets one command answer mold/allergy questions
// across AirThings, IQAir and MOCREO at once.
package readings

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-air-health/internal/source"
)

// EnsureSchema creates the readings table and indexes if absent. Safe to call
// on every command; CREATE/INDEX IF NOT EXISTS are no-ops once present. The
// UNIQUE (source, device_id, metric, ts) key makes re-sync idempotent — the
// same vendor sample inserted twice is ignored rather than duplicated.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS readings (
			ts        INTEGER NOT NULL,   -- unix epoch seconds (UTC)
			source    TEXT    NOT NULL,
			device_id TEXT    NOT NULL,
			room      TEXT    NOT NULL,
			metric    TEXT    NOT NULL,
			value     REAL    NOT NULL,
			unit      TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS readings_uniq
			ON readings (source, device_id, metric, ts)`,
		`CREATE INDEX IF NOT EXISTS readings_q
			ON readings (metric, room, ts)`,
		`CREATE INDEX IF NOT EXISTS readings_ts ON readings (ts)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("readings: ensure schema: %w", err)
		}
	}
	return nil
}

// InsertBatch writes readings idempotently (INSERT OR IGNORE on the unique
// key). Returns the number of new rows actually inserted. Skips zero-value
// timestamps and empty metrics defensively.
func InsertBatch(ctx context.Context, db *sql.DB, rs []source.Reading) (int, error) {
	if len(rs) == 0 {
		return 0, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO readings
		(ts, source, device_id, room, metric, value, unit) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var inserted int
	for _, r := range rs {
		if r.TS.IsZero() || r.Metric == "" {
			continue
		}
		// Normalize room/device labels at the single write choke point so
		// vendor quirks (MOCREO node names ship with trailing spaces) don't
		// fragment a room across "Kitchen" and "Kitchen " or break lookups.
		room := strings.TrimSpace(r.Room)
		res, err := stmt.ExecContext(ctx, r.TS.UTC().Unix(), r.Source, strings.TrimSpace(r.DeviceID), room, r.Metric, r.Value, r.Unit)
		if err != nil {
			return inserted, fmt.Errorf("readings: insert: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}
	if err := tx.Commit(); err != nil {
		return inserted, err
	}
	return inserted, nil
}

// Filter scopes a query. Empty slices mean "all"; zero times mean unbounded.
type Filter struct {
	Since   time.Time
	Until   time.Time
	Rooms   []string
	Metrics []string
	Sources []string
}

func (f Filter) where() (string, []any) {
	var clauses []string
	var args []any
	if !f.Since.IsZero() {
		clauses = append(clauses, "ts >= ?")
		args = append(args, f.Since.UTC().Unix())
	}
	if !f.Until.IsZero() {
		clauses = append(clauses, "ts <= ?")
		args = append(args, f.Until.UTC().Unix())
	}
	add := func(col string, vals []string) {
		if len(vals) == 0 {
			return
		}
		ph := strings.TrimRight(strings.Repeat("?,", len(vals)), ",")
		clauses = append(clauses, fmt.Sprintf("%s IN (%s)", col, ph))
		for _, v := range vals {
			args = append(args, v)
		}
	}
	add("room", f.Rooms)
	add("metric", f.Metrics)
	add("source", f.Sources)
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// AggRow is one (source, room, metric) summary over the filter window.
type AggRow struct {
	Source string    `json:"source"`
	Room   string    `json:"room"`
	Metric string    `json:"metric"`
	Unit   string    `json:"unit"`
	Count  int       `json:"count"`
	Avg    float64   `json:"avg"`
	Min    float64   `json:"min"`
	Max    float64   `json:"max"`
	Last   float64   `json:"last"`
	LastTS time.Time `json:"last_ts"`
}

// Aggregate returns per (source, room, metric) summary stats over the window.
func Aggregate(ctx context.Context, db *sql.DB, f Filter) ([]AggRow, error) {
	where, args := f.where()
	q := `SELECT source, room, metric, COALESCE(unit,''),
		COUNT(*), AVG(value), MIN(value), MAX(value),
		(SELECT value FROM readings r2 WHERE r2.source=r.source AND r2.room=r.room AND r2.metric=r.metric` + whereSub(where) + ` ORDER BY ts DESC LIMIT 1),
		MAX(ts)
		FROM readings r` + where + `
		GROUP BY source, room, metric, unit
		ORDER BY room, metric`
	// The correlated subquery reuses the same window args, so duplicate them.
	allArgs := append(append([]any{}, args...), args...)
	rows, err := db.QueryContext(ctx, q, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("readings: aggregate: %w", err)
	}
	defer rows.Close()
	var out []AggRow
	for rows.Next() {
		var a AggRow
		var lastTS int64
		var last sql.NullFloat64
		if err := rows.Scan(&a.Source, &a.Room, &a.Metric, &a.Unit, &a.Count, &a.Avg, &a.Min, &a.Max, &last, &lastTS); err != nil {
			return nil, err
		}
		a.Last = last.Float64
		a.LastTS = time.Unix(lastTS, 0).UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}

// whereSub adapts the outer WHERE (which references column names valid in the
// subquery too) for the correlated latest-value subquery. The subquery already
// constrains source/room/metric, so we only carry the ts bounds across; reusing
// the full predicate is harmless because the columns exist in r2 as well.
func whereSub(where string) string {
	if where == "" {
		return ""
	}
	return " AND " + strings.TrimPrefix(strings.TrimSpace(where), "WHERE ")
}

// ExceedRow reports how often a metric crossed a threshold per room — the basis
// for "mold-hours" (humidity over the mold threshold) and similar risk tallies.
type ExceedRow struct {
	Room     string  `json:"room"`
	Total    int     `json:"total"`     // total samples in window
	Over     int     `json:"over"`      // samples at/over threshold
	Fraction float64 `json:"fraction"`  // Over/Total
	MaxValue float64 `json:"max_value"` // worst reading in window
}

// Exceedance tallies, per room, how many samples of `metric` were >= threshold.
func Exceedance(ctx context.Context, db *sql.DB, metric string, threshold float64, f Filter) ([]ExceedRow, error) {
	f.Metrics = []string{metric}
	where, args := f.where()
	q := `SELECT room, COUNT(*),
		SUM(CASE WHEN value >= ? THEN 1 ELSE 0 END), MAX(value)
		FROM readings` + where + ` GROUP BY room ORDER BY 3 DESC, 4 DESC`
	// threshold placeholder comes before the WHERE args in the SELECT list.
	allArgs := append([]any{threshold}, args...)
	rows, err := db.QueryContext(ctx, q, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("readings: exceedance: %w", err)
	}
	defer rows.Close()
	var out []ExceedRow
	for rows.Next() {
		var e ExceedRow
		if err := rows.Scan(&e.Room, &e.Total, &e.Over, &e.MaxValue); err != nil {
			return nil, err
		}
		if e.Total > 0 {
			e.Fraction = float64(e.Over) / float64(e.Total)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Distinct returns the distinct values of one whitelisted column.
func Distinct(ctx context.Context, db *sql.DB, col string, f Filter) ([]string, error) {
	switch col {
	case "room", "metric", "source", "device_id":
	default:
		return nil, fmt.Errorf("readings: distinct: column %q not allowed", col)
	}
	where, args := f.where()
	rows, err := db.QueryContext(ctx, "SELECT DISTINCT "+col+" FROM readings"+where+" ORDER BY 1", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Span returns the earliest and latest reading timestamps matching the filter.
func Span(ctx context.Context, db *sql.DB, f Filter) (min, max time.Time, count int, err error) {
	where, args := f.where()
	var lo, hi sql.NullInt64
	row := db.QueryRowContext(ctx, "SELECT MIN(ts), MAX(ts), COUNT(*) FROM readings"+where, args...)
	if err = row.Scan(&lo, &hi, &count); err != nil {
		return
	}
	if lo.Valid {
		min = time.Unix(lo.Int64, 0).UTC()
	}
	if hi.Valid {
		max = time.Unix(hi.Int64, 0).UTC()
	}
	return
}
