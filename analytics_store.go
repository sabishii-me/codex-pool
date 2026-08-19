package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.etcd.io/bbolt"
	_ "modernc.org/sqlite"
)

type UsageEvent struct {
	RequestID        string
	StartedAt        time.Time
	CompletedAt      time.Time
	UserID           string
	OriginID         string
	ProviderID       AccountType
	ConnectionID     string
	ModelID          string
	PlanType         string
	InputTokens      int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	OutputTokens     int64
	ReasoningTokens  int64
	BillableTokens   int64
	CostUSD          float64
	WorkloadKind     WorkloadKind
	Status           string
	ImageCount       int
	ImageMIME        string
	ImageWidth       int
	ImageHeight      int
	OperationID      string
	FailureClass     string
	MediaCostUSD     *float64
	EconomicsKnown   bool
	CacheReadReported     bool
	CacheWriteReported    bool
}

func usageEventFromRequest(usage RequestUsage, costUSD float64) UsageEvent {
	usage = usage.canonicalIdentity()
	return UsageEvent{
		RequestID: usage.RequestID, StartedAt: usage.Timestamp, CompletedAt: usage.Timestamp,
		UserID: usage.UserID, OriginID: usage.OriginID, ProviderID: usage.ProviderID,
		ConnectionID: usage.ConnectionID, ModelID: usage.Model, PlanType: usage.PlanType,
		InputTokens: usage.InputTokens, CacheReadTokens: usage.CachedInputTokens,
		CacheWriteTokens: usage.CacheCreationTokens, OutputTokens: usage.OutputTokens,
		ReasoningTokens: usage.ReasoningTokens, BillableTokens: usage.BillableTokens, CostUSD: costUSD,
		CacheReadReported: usage.CacheReadReported, CacheWriteReported: usage.CacheCreationReported,
	}
}

// AnalyticsStore persists canonical usage events and compatibility projections.
type AnalyticsStore struct {
	db *sql.DB
	mu sync.Mutex // serialize writes
}

// DailyCostEntry represents one day of cost data for a provider.
type DailyCostEntry struct {
	Date         string  `json:"date"`
	AccountType  string  `json:"account_type"`
	CostUSD      float64 `json:"cost_usd"`
	RequestCount int64   `json:"request_count"`
}

// ModelDailyUsageEntry is the public, aggregated model mix used by the signal
// room. It intentionally contains no user or account identifiers.
type ModelDailyUsageEntry struct {
	Date            string  `json:"date"`
	AccountType     string  `json:"account_type"`
	Model           string  `json:"model"`
	InputTokens     int64   `json:"input_tokens"`
	CachedTokens    int64   `json:"cached_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	RequestCount    int64   `json:"request_count"`
	CostUSD         float64 `json:"cost_usd"`
}

// AccountDailyCostEntry keeps account attribution so cumulative value charts
// can compare the current pool's API-equivalent value with matching spend.
type AccountDailyCostEntry struct {
	Date         string  `json:"date"`
	AccountID    string  `json:"account_id"`
	AccountType  string  `json:"account_type"`
	CostUSD      float64 `json:"cost_usd"`
	RequestCount int64   `json:"request_count"`
}

// AccountCostSummary holds cost totals for a single account.
type AccountCostSummary struct {
	AccountID   string  `json:"account_id"`
	AccountType string  `json:"account_type"`
	CostUSD     float64 `json:"cost_usd"`
}

// AccountCostStats pairs all-time API-equivalent cost with the beginning of
// that measurement period so subscription spend uses the same time horizon.
type AccountCostStats struct {
	CostUSD   float64
	FirstSeen time.Time
}

// ProviderCostSummary holds aggregated cost data for a provider type.
type ProviderCostSummary struct {
	APICost                 float64 `json:"api_cost"`
	SubscriptionCost        float64 `json:"subscription_cost"`
	MonthlySubscriptionCost float64 `json:"monthly_subscription_cost"`
	AccountCount            int     `json:"account_count"`
	ROI                     float64 `json:"roi"`
}

func newAnalyticsStore(dbPath string) (*AnalyticsStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create analytics dir: %w", err)
	}

	// A single SQLite handle with WAL keeps model-response accounting and heavy
	// usage reads independent at the connection-pool level. The pool is sized so
	// a burst of dashboard reads cannot starve the accounting write.
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open analytics db: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)

	if err := createAnalyticsTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return &AnalyticsStore{db: db}, nil
}

func createAnalyticsTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS usage_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_id TEXT NOT NULL,
		started_at TEXT NOT NULL,
		completed_at TEXT NOT NULL,
		user_id TEXT,
		origin_id TEXT,
		provider_id TEXT NOT NULL,
		connection_id TEXT NOT NULL,
		model_id TEXT,
		plan_type TEXT,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		cache_read_tokens INTEGER NOT NULL DEFAULT 0,
		cache_write_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		reasoning_tokens INTEGER NOT NULL DEFAULT 0,
		billable_tokens INTEGER NOT NULL DEFAULT 0,
		cost_usd REAL NOT NULL DEFAULT 0,
		workload_kind TEXT NOT NULL DEFAULT 'text_generation',
		status TEXT NOT NULL DEFAULT 'success',
		image_count INTEGER NOT NULL DEFAULT 0,
		image_mime TEXT,
		image_width INTEGER NOT NULL DEFAULT 0,
		image_height INTEGER NOT NULL DEFAULT 0,
		operation_id TEXT,
		failure_class TEXT,
		media_cost_usd REAL,
		economics_known INTEGER NOT NULL DEFAULT 1,
		cache_read_reported INTEGER NOT NULL DEFAULT 0,
		cache_write_reported INTEGER NOT NULL DEFAULT 0
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_events_request ON usage_events(connection_id, request_id) WHERE request_id != '';
	CREATE INDEX IF NOT EXISTS idx_usage_events_completed ON usage_events(completed_at);
	CREATE INDEX IF NOT EXISTS idx_usage_events_provider_completed ON usage_events(provider_id, completed_at);
	CREATE INDEX IF NOT EXISTS idx_usage_events_user_completed ON usage_events(user_id, completed_at);

	CREATE TABLE IF NOT EXISTS request_costs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL,
		account_id TEXT NOT NULL,
		account_type TEXT NOT NULL,
		user_id TEXT,
		request_id TEXT NOT NULL DEFAULT '',
		model TEXT,
		input_tokens INTEGER DEFAULT 0,
		cached_tokens INTEGER DEFAULT 0,
		cache_creation_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		reasoning_tokens INTEGER DEFAULT 0,
		cost_usd REAL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_request_costs_account_ts ON request_costs(account_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_request_costs_type_ts ON request_costs(account_type, timestamp);

	CREATE TABLE IF NOT EXISTS daily_costs (
		date TEXT NOT NULL,
		account_id TEXT NOT NULL,
		account_type TEXT NOT NULL,
		model TEXT NOT NULL DEFAULT '',
		input_tokens INTEGER DEFAULT 0,
		cached_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		reasoning_tokens INTEGER DEFAULT 0,
		request_count INTEGER DEFAULT 0,
		cost_usd REAL DEFAULT 0,
		PRIMARY KEY (date, account_id, model)
	);
	`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}
	// Additive migrations for databases created before request identity and
	// cache-write accounting were introduced.
	for _, migration := range []string{
		`ALTER TABLE request_costs ADD COLUMN request_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE request_costs ADD COLUMN cache_creation_tokens INTEGER DEFAULT 0`,
		`ALTER TABLE usage_events ADD COLUMN workload_kind TEXT NOT NULL DEFAULT 'text_generation'`,
		`ALTER TABLE usage_events ADD COLUMN status TEXT NOT NULL DEFAULT 'success'`,
		`ALTER TABLE usage_events ADD COLUMN image_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE usage_events ADD COLUMN image_mime TEXT`,
		`ALTER TABLE usage_events ADD COLUMN image_width INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE usage_events ADD COLUMN image_height INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE usage_events ADD COLUMN operation_id TEXT`,
		`ALTER TABLE usage_events ADD COLUMN failure_class TEXT`,
		`ALTER TABLE usage_events ADD COLUMN media_cost_usd REAL`,
		`ALTER TABLE usage_events ADD COLUMN economics_known INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE usage_events ADD COLUMN cache_read_reported INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE usage_events ADD COLUMN cache_write_reported INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, migrationErr := db.Exec(migration); migrationErr != nil && !strings.Contains(strings.ToLower(migrationErr.Error()), "duplicate column") {
			return migrationErr
		}
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_request_costs_request ON request_costs(account_id, request_id) WHERE request_id != ''`)
	if err != nil {
		return err
	}
	return backfillLegacyUsageEvents(db)
}

func backfillLegacyUsageEvents(db *sql.DB) error {
	// One-time compatibility backfill. Historical request_costs rows predate
	// request IDs and cache-write persistence, so they receive stable synthetic
	// identities while preserving every available token dimension.
	_, err := db.Exec(`
		INSERT OR IGNORE INTO usage_events (
			request_id, started_at, completed_at, user_id, origin_id, provider_id,
			connection_id, model_id, plan_type, input_tokens, cache_read_tokens,
			cache_write_tokens, output_tokens, reasoning_tokens, billable_tokens, cost_usd)
		SELECT CASE WHEN request_id != '' THEN request_id ELSE 'legacy-' || id END,
			timestamp, timestamp, COALESCE(user_id,''), '', account_type,
			account_id, COALESCE(model,''), '', input_tokens, cached_tokens,
			COALESCE(cache_creation_tokens,0), output_tokens, reasoning_tokens,
			MAX(0, input_tokens - cached_tokens - COALESCE(cache_creation_tokens,0) + output_tokens), cost_usd
		FROM request_costs`)
	return err
}

func (s *AnalyticsStore) loadConnectionTotals() (map[string]AccountUsage, error) {
	rows, err := s.db.Query(`
		SELECT connection_id, COALESCE(SUM(input_tokens),0), COALESCE(SUM(cache_read_tokens),0),
			COALESCE(SUM(output_tokens),0), COALESCE(SUM(reasoning_tokens),0),
			COALESCE(SUM(billable_tokens),0), COUNT(*), COALESCE(SUM(cost_usd),0), MAX(completed_at)
		FROM usage_events GROUP BY connection_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]AccountUsage)
	for rows.Next() {
		var connectionID, lastUpdated string
		var usage AccountUsage
		if err := rows.Scan(&connectionID, &usage.TotalInputTokens, &usage.TotalCachedTokens,
			&usage.TotalOutputTokens, &usage.TotalReasoningTokens, &usage.TotalBillableTokens,
			&usage.RequestCount, &usage.TotalCostEstimate, &lastUpdated); err != nil {
			return nil, err
		}
		usage.LastUpdated, _ = time.Parse(time.RFC3339Nano, lastUpdated)
		result[connectionID] = usage
	}
	return result, rows.Err()
}

// recordRequest retains the legacy API while writing through the canonical
// usage event transaction.
func (s *AnalyticsStore) recordRequest(ru RequestUsage, costUSD float64) error {
	_, err := s.recordUsageEvent(usageEventFromRequest(ru, costUSD))
	return err
}

// recordUsageEvent inserts one immutable canonical event and updates the
// request_costs compatibility projection in the same SQLite transaction.
// It returns false for an already-persisted connection/request identity.
func (s *AnalyticsStore) recordUsageEvent(event UsageEvent) (bool, error) {
	if event.WorkloadKind == "" {
		event.WorkloadKind = WorkloadTextGeneration
	}
	if event.Status == "" {
		event.Status = "success"
	}
	if event.WorkloadKind == WorkloadTextGeneration && !event.EconomicsKnown {
		// Existing text accounting has authoritative pricing semantics; preserve
		// compatibility. Native media callers must set unknown economics explicitly.
		event.EconomicsKnown = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`
		INSERT OR IGNORE INTO usage_events (
			request_id, started_at, completed_at, user_id, origin_id, provider_id,
			connection_id, model_id, plan_type, input_tokens, cache_read_tokens,
			cache_write_tokens, output_tokens, reasoning_tokens, billable_tokens, cost_usd,
			workload_kind, status, image_count, image_mime, image_width, image_height,
			operation_id, failure_class, media_cost_usd, economics_known,
			cache_read_reported, cache_write_reported)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.RequestID, event.StartedAt.UTC().Format(time.RFC3339Nano), event.CompletedAt.UTC().Format(time.RFC3339Nano),
		event.UserID, event.OriginID, string(event.ProviderID), event.ConnectionID, event.ModelID, event.PlanType,
		event.InputTokens, event.CacheReadTokens, event.CacheWriteTokens, event.OutputTokens,
		event.ReasoningTokens, event.BillableTokens, event.CostUSD, string(event.WorkloadKind), event.Status,
		event.ImageCount, event.ImageMIME, event.ImageWidth, event.ImageHeight, event.OperationID, event.FailureClass, event.MediaCostUSD, event.EconomicsKnown,
		event.CacheReadReported, event.CacheWriteReported,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	_, err = tx.Exec(`
		INSERT OR IGNORE INTO request_costs (timestamp, account_id, account_type, user_id, request_id, model,
			input_tokens, cached_tokens, cache_creation_tokens, output_tokens, reasoning_tokens, cost_usd)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.CompletedAt.UTC().Format(time.RFC3339), event.ConnectionID, string(event.ProviderID), event.UserID,
		event.RequestID, event.ModelID, event.InputTokens, event.CacheReadTokens, event.CacheWriteTokens,
		event.OutputTokens, event.ReasoningTokens, event.CostUSD,
	)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// getCostByAccount returns total cost per account for the last N days.
func (s *AnalyticsStore) getCostByAccount(days int) (map[string]float64, error) {
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.db.Query(`
		SELECT account_id, SUM(cost_usd)
		FROM daily_costs
		WHERE date >= ?
		GROUP BY account_id`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var id string
		var cost float64
		if err := rows.Scan(&id, &cost); err != nil {
			continue
		}
		result[id] = cost
	}

	// Also include today's un-rolled-up request_costs
	todayStart := time.Now().UTC().Format("2006-01-02")
	rows2, err := s.db.Query(`
		SELECT account_id, SUM(cost_usd)
		FROM request_costs
		WHERE timestamp >= ?
		GROUP BY account_id`, todayStart+"T00:00:00Z")
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var id string
			var cost float64
			if err := rows2.Scan(&id, &cost); err != nil {
				continue
			}
			result[id] += cost
		}
	}

	return result, nil
}

// getCostByProvider returns total cost per provider type for the last N days.
func (s *AnalyticsStore) getCostByProvider(days int) (map[string]float64, error) {
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.db.Query(`
		SELECT account_type, SUM(cost_usd)
		FROM daily_costs
		WHERE date >= ?
		GROUP BY account_type`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var accType string
		var cost float64
		if err := rows.Scan(&accType, &cost); err != nil {
			continue
		}
		result[accType] = cost
	}

	// Include today's un-rolled-up data
	todayStart := time.Now().UTC().Format("2006-01-02")
	rows2, err := s.db.Query(`
		SELECT account_type, SUM(cost_usd)
		FROM request_costs
		WHERE timestamp >= ?
		GROUP BY account_type`, todayStart+"T00:00:00Z")
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var accType string
			var cost float64
			if err := rows2.Scan(&accType, &cost); err != nil {
				continue
			}
			result[accType] += cost
		}
	}

	return result, nil
}

// getDailyCosts returns daily cost totals by provider for the last N days (for charting).
// getUserBillableByRange returns per-user billable tokens for the requested
// range (ISO since). Used by the member usage pie chart so the slices track
// the selected 24h/7d/30d window instead of the all-time leaderboard totals.
func (s *AnalyticsStore) getUserBillableByRange(since string) (map[string]UserUsage, error) {
	result := map[string]UserUsage{}
	if s == nil || s.db == nil {
		return result, nil
	}
	rows, err := s.db.Query(`
		SELECT user_id,
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(billable_tokens), 0),
		       COUNT(*)
		FROM usage_events
		WHERE started_at >= ? AND user_id != ''
		GROUP BY user_id`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u UserUsage
		var reqs int
		if err := rows.Scan(&u.UserID, &u.TotalInputTokens, &u.TotalOutputTokens, &u.TotalBillableTokens, &reqs); err != nil {
			continue
		}
		u.RequestCount = int64(reqs)
		result[u.UserID] = u
	}
	return result, rows.Err()
}

func (s *AnalyticsStore) getDailyCosts(days int) ([]DailyCostEntry, error) {
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.db.Query(`
		SELECT date, account_type, SUM(cost_usd), SUM(request_count)
		FROM daily_costs
		WHERE date >= ?
		GROUP BY date, account_type
		ORDER BY date`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DailyCostEntry
	for rows.Next() {
		var e DailyCostEntry
		if err := rows.Scan(&e.Date, &e.AccountType, &e.CostUSD, &e.RequestCount); err != nil {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}

// getModelDailyUsage returns rolled-up historical rows plus today's live
// requests. Keeping the date/model/provider grain makes demand anatomy useful
// without exposing individual accounts or users.
func (s *AnalyticsStore) getModelDailyUsage(days int) ([]ModelDailyUsageEntry, error) {
	if days <= 0 {
		days = 30
	}
	today := time.Now().UTC().Format("2006-01-02")
	since := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.db.Query(`
		SELECT date, account_type, COALESCE(model, ''),
			SUM(input_tokens), SUM(cached_tokens), SUM(output_tokens),
			SUM(reasoning_tokens), SUM(request_count), SUM(cost_usd)
		FROM daily_costs
		WHERE date >= ? AND date < ?
		GROUP BY date, account_type, COALESCE(model, '')
		ORDER BY date, account_type, model`, since, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ModelDailyUsageEntry, 0)
	for rows.Next() {
		var entry ModelDailyUsageEntry
		if err := rows.Scan(&entry.Date, &entry.AccountType, &entry.Model,
			&entry.InputTokens, &entry.CachedTokens, &entry.OutputTokens,
			&entry.ReasoningTokens, &entry.RequestCount, &entry.CostUSD); err != nil {
			continue
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	live, err := s.db.Query(`
		SELECT account_type, COALESCE(model, ''),
			SUM(input_tokens), SUM(cached_tokens), SUM(output_tokens),
			SUM(reasoning_tokens), COUNT(*), SUM(cost_usd)
		FROM request_costs
		WHERE timestamp >= ?
		GROUP BY account_type, COALESCE(model, '')
		ORDER BY account_type, model`, today+"T00:00:00Z")
	if err != nil {
		return nil, err
	}
	defer live.Close()
	for live.Next() {
		entry := ModelDailyUsageEntry{Date: today}
		if err := live.Scan(&entry.AccountType, &entry.Model,
			&entry.InputTokens, &entry.CachedTokens, &entry.OutputTokens,
			&entry.ReasoningTokens, &entry.RequestCount, &entry.CostUSD); err == nil {
			result = append(result, entry)
		}
	}
	if err := live.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// getAllAccountDailyCosts returns all rolled-up history plus today's live rows.
// Callers filter to the current account set before calculating pool ROI.
func (s *AnalyticsStore) getAllAccountDailyCosts() ([]AccountDailyCostEntry, error) {
	rows, err := s.db.Query(`
		SELECT date, account_id, account_type, SUM(cost_usd), SUM(request_count)
		FROM daily_costs
		GROUP BY date, account_id, account_type
		ORDER BY date, account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AccountDailyCostEntry
	for rows.Next() {
		var entry AccountDailyCostEntry
		if err := rows.Scan(&entry.Date, &entry.AccountID, &entry.AccountType, &entry.CostUSD, &entry.RequestCount); err == nil {
			result = append(result, entry)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	today := time.Now().UTC().Format("2006-01-02")
	liveRows, err := s.db.Query(`
		SELECT account_id, account_type, SUM(cost_usd), COUNT(*)
		FROM request_costs
		WHERE timestamp >= ?
		GROUP BY account_id, account_type`, today+"T00:00:00Z")
	if err != nil {
		return nil, err
	}
	defer liveRows.Close()
	for liveRows.Next() {
		entry := AccountDailyCostEntry{Date: today}
		if err := liveRows.Scan(&entry.AccountID, &entry.AccountType, &entry.CostUSD, &entry.RequestCount); err == nil {
			result = append(result, entry)
		}
	}
	if err := liveRows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// getAllTimeAccountCostStats returns all-time cost and the first date covered
// by that cost for each account.
func (s *AnalyticsStore) getAllTimeAccountCostStats() (map[string]AccountCostStats, error) {
	rows, err := s.db.Query(`
		SELECT account_id, SUM(cost_usd), MIN(date)
		FROM daily_costs
		GROUP BY account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]AccountCostStats)
	for rows.Next() {
		var id, firstDate string
		var cost float64
		if err := rows.Scan(&id, &cost, &firstDate); err != nil {
			continue
		}
		firstSeen, _ := time.Parse("2006-01-02", firstDate)
		result[id] = AccountCostStats{CostUSD: cost, FirstSeen: firstSeen}
	}

	// Add today's un-rolled-up requests without double-counting older rows that
	// have already been copied into daily_costs.
	todayStart := time.Now().UTC().Format("2006-01-02")
	rows2, err := s.db.Query(`
		SELECT account_id, SUM(cost_usd), MIN(timestamp)
		FROM request_costs
		WHERE timestamp >= ?
		GROUP BY account_id`, todayStart+"T00:00:00Z")
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var id, firstTimestamp string
			var cost float64
			if err := rows2.Scan(&id, &cost, &firstTimestamp); err != nil {
				continue
			}
			stats := result[id]
			stats.CostUSD += cost
			if firstSeen, err := time.Parse(time.RFC3339, firstTimestamp); err == nil &&
				(stats.FirstSeen.IsZero() || firstSeen.Before(stats.FirstSeen)) {
				stats.FirstSeen = firstSeen
			}
			result[id] = stats
		}
	}

	return result, nil
}

// runDailyRollup aggregates yesterday's request_costs into daily_costs and prunes old data.
func (s *AnalyticsStore) runDailyRollup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// Roll up request_costs for yesterday into daily_costs
	_, err := s.db.Exec(`
		INSERT INTO daily_costs (date, account_id, account_type, model,
			input_tokens, cached_tokens, output_tokens, reasoning_tokens, request_count, cost_usd)
		SELECT
			? as date,
			account_id,
			account_type,
			COALESCE(model, '') as model,
			SUM(input_tokens),
			SUM(cached_tokens),
			SUM(output_tokens),
			SUM(reasoning_tokens),
			COUNT(*),
			SUM(cost_usd)
		FROM request_costs
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY account_id, account_type, COALESCE(model, '')
		ON CONFLICT(date, account_id, model) DO UPDATE SET
			account_type = excluded.account_type,
			input_tokens = excluded.input_tokens,
			cached_tokens = excluded.cached_tokens,
			output_tokens = excluded.output_tokens,
			reasoning_tokens = excluded.reasoning_tokens,
			request_count = excluded.request_count,
			cost_usd = excluded.cost_usd`,
		yesterday, yesterday+"T00:00:00Z", now.Format("2006-01-02")+"T00:00:00Z")
	if err != nil {
		log.Printf("analytics: daily rollup failed: %v", err)
		return
	}

	// Prune old request_costs (keep last 30 days)
	cutoff := now.AddDate(0, 0, -30).Format("2006-01-02") + "T00:00:00Z"
	result, err := s.db.Exec(`DELETE FROM request_costs WHERE timestamp < ?`, cutoff)
	if err != nil {
		log.Printf("analytics: prune failed: %v", err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		log.Printf("analytics: pruned %d old request_costs rows", n)
	}
}

// startDailyRollup runs the rollup once at startup and then daily at midnight UTC.
func (s *AnalyticsStore) startDailyRollup(ctx context.Context, jobs *backgroundJobs) {
	// Run once after a brief startup delay to catch up.
	jobs.Go(ctx, func(ctx context.Context) {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.runDailyRollup()
		}
	})

	jobs.Go(ctx, func(ctx context.Context) {
		for {
			now := time.Now().UTC()
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 5, 0, 0, time.UTC)
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				s.runDailyRollup()
			}
		}
	})
}

// seedFromBoltDB backfills daily_costs from historical BoltDB request data.
// Only runs if daily_costs is empty (first time setup).
func (s *AnalyticsStore) seedFromBoltDB(store *usageStore, pricing *PricingData) {
	if s == nil || store == nil || store.db == nil || pricing == nil {
		return
	}

	// Check if we already have data
	var count int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM daily_costs`).Scan(&count); err != nil || count > 0 {
		return
	}

	log.Printf("analytics: seeding from BoltDB historical data...")

	// Aggregate: date -> accountID -> model -> {tokens, cost}
	type aggKey struct {
		date, accountID, accountType, model string
	}
	type aggVal struct {
		input, cached, output, reasoning, count int64
		cost                                    float64
	}
	agg := make(map[aggKey]*aggVal)
	var totalRequests int64

	err := store.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketUsageRequests))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var ru RequestUsage
			if err := json.Unmarshal(v, &ru); err != nil {
				return nil // skip bad records
			}
			ru = ru.canonicalIdentity()
			if ru.InputTokens == 0 && ru.OutputTokens == 0 {
				return nil
			}
			totalRequests++

			date := ru.Timestamp.UTC().Format("2006-01-02")
			model := ru.Model
			accType := string(ru.AccountType)

			costUSD := pricing.calculateCost(ru)

			key := aggKey{date, ru.AccountID, accType, model}
			if v, ok := agg[key]; ok {
				v.input += ru.InputTokens
				v.cached += ru.CachedInputTokens
				v.output += ru.OutputTokens
				v.reasoning += ru.ReasoningTokens
				v.count++
				v.cost += costUSD
			} else {
				agg[key] = &aggVal{
					input:     ru.InputTokens,
					cached:    ru.CachedInputTokens,
					output:    ru.OutputTokens,
					reasoning: ru.ReasoningTokens,
					count:     1,
					cost:      costUSD,
				}
			}
			return nil
		})
	})
	if err != nil {
		log.Printf("analytics: seed scan failed: %v", err)
		return
	}

	if len(agg) == 0 {
		log.Printf("analytics: no historical data to seed")
		return
	}

	// Batch insert into daily_costs
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("analytics: seed transaction failed: %v", err)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT INTO daily_costs (date, account_id, account_type, model,
			input_tokens, cached_tokens, output_tokens, reasoning_tokens, request_count, cost_usd)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date, account_id, model) DO UPDATE SET
			input_tokens = input_tokens + excluded.input_tokens,
			cached_tokens = cached_tokens + excluded.cached_tokens,
			output_tokens = output_tokens + excluded.output_tokens,
			reasoning_tokens = reasoning_tokens + excluded.reasoning_tokens,
			request_count = request_count + excluded.request_count,
			cost_usd = cost_usd + excluded.cost_usd`)
	if err != nil {
		tx.Rollback()
		log.Printf("analytics: seed prepare failed: %v", err)
		return
	}
	defer stmt.Close()

	var totalCost float64
	for key, val := range agg {
		_, err := stmt.Exec(key.date, key.accountID, key.accountType, key.model,
			val.input, val.cached, val.output, val.reasoning, val.count, val.cost)
		if err != nil {
			log.Printf("analytics: seed insert failed for %s/%s: %v", key.date, key.accountID, err)
		}
		totalCost += val.cost
	}

	if err := tx.Commit(); err != nil {
		log.Printf("analytics: seed commit failed: %v", err)
		return
	}

	log.Printf("analytics: seeded %d daily aggregates from %d historical requests, total cost $%.2f", len(agg), totalRequests, totalCost)
}

// Close closes the underlying database connection.
func (s *AnalyticsStore) Close() error {
	return s.db.Close()
}
