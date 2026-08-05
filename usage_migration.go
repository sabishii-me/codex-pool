package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const usageMigrationVersion = "usage-event-merge-v1"

type usageMigrationOptions struct {
	TargetPath        string
	SourcePath        string
	TargetEnvironment string
	SourceEnvironment string
	BackupDir         string
	ReportPath        string
	Apply             bool
}

type usageMigrationConflict struct {
	ConnectionID string   `json:"connection_id"`
	RequestID    string   `json:"request_id"`
	Fields       []string `json:"fields"`
}

type usageMigrationReport struct {
	Version             string                   `json:"version"`
	MigrationID         string                   `json:"migration_id"`
	Mode                string                   `json:"mode"`
	TargetPath          string                   `json:"target_path"`
	SourcePath          string                   `json:"source_path"`
	TargetEnvironment   string                   `json:"target_environment"`
	SourceEnvironment   string                   `json:"source_environment"`
	SourceSHA256        string                   `json:"source_sha256"`
	TargetBeforeSHA256  string                   `json:"target_before_sha256"`
	BackupPath          string                   `json:"backup_path,omitempty"`
	BackupSHA256        string                   `json:"backup_sha256,omitempty"`
	TargetEventsBefore  int                      `json:"target_events_before"`
	SourceEvents        int                      `json:"source_events"`
	InsertableEvents    int                      `json:"insertable_events"`
	IdenticalEvents     int                      `json:"identical_events"`
	ConflictingEvents   int                      `json:"conflicting_events"`
	SourceInvalidEvents int                      `json:"source_invalid_events"`
	TargetInvalidEvents int                      `json:"target_invalid_events"`
	InsertedEvents      int                      `json:"inserted_events"`
	TargetEventsAfter   int                      `json:"target_events_after"`
	ProjectionRebuilt   bool                     `json:"projection_rebuilt"`
	AlreadyApplied      bool                     `json:"already_applied"`
	Conflicts           []usageMigrationConflict `json:"conflicts,omitempty"`
	GeneratedAt         time.Time                `json:"generated_at"`
}

type persistedUsageEvent struct {
	RequestID, StartedAt, CompletedAt, UserID, OriginID, ProviderID, ConnectionID, ModelID, PlanType string
	WorkloadKind, Status, ImageMIME, OperationID, FailureClass                                       string
	InputTokens, CacheReadTokens, CacheWriteTokens, OutputTokens, ReasoningTokens, BillableTokens    int64
	ImageCount, ImageWidth, ImageHeight                                                              int64
	CostUSD                                                                                          float64
	MediaCostUSD                                                                                     sql.NullFloat64
	EconomicsKnown                                                                                   bool
}

var usageEventFieldNames = []string{
	"request_id", "started_at", "completed_at", "user_id", "origin_id", "provider_id", "connection_id",
	"model_id", "plan_type", "input_tokens", "cache_read_tokens", "cache_write_tokens", "output_tokens",
	"reasoning_tokens", "billable_tokens", "cost_usd", "workload_kind", "status", "image_count",
	"image_mime", "image_width", "image_height", "operation_id", "failure_class", "media_cost_usd", "economics_known",
}

func (e persistedUsageEvent) values() []any {
	return []any{e.RequestID, e.StartedAt, e.CompletedAt, e.UserID, e.OriginID, e.ProviderID, e.ConnectionID,
		e.ModelID, e.PlanType, e.InputTokens, e.CacheReadTokens, e.CacheWriteTokens, e.OutputTokens,
		e.ReasoningTokens, e.BillableTokens, e.CostUSD, e.WorkloadKind, e.Status, e.ImageCount,
		e.ImageMIME, e.ImageWidth, e.ImageHeight, e.OperationID, e.FailureClass, e.MediaCostUSD, e.EconomicsKnown}
}

func usageEventDifferences(a, b persistedUsageEvent) []string {
	av, bv := a.values(), b.values()
	var fields []string
	for i := range av {
		if fmt.Sprint(av[i]) != fmt.Sprint(bv[i]) {
			fields = append(fields, usageEventFieldNames[i])
		}
	}
	return fields
}

func readUsageEvents(db *sql.DB) (map[string]persistedUsageEvent, int, error) {
	rows, err := db.Query(`SELECT request_id, started_at, completed_at, COALESCE(user_id,''), COALESCE(origin_id,''),
		provider_id, connection_id, COALESCE(model_id,''), COALESCE(plan_type,''), input_tokens,
		cache_read_tokens, cache_write_tokens, output_tokens, reasoning_tokens, billable_tokens, cost_usd,
		COALESCE(workload_kind,'text_generation'), COALESCE(status,'success'), image_count, COALESCE(image_mime,''),
		image_width, image_height, COALESCE(operation_id,''), COALESCE(failure_class,''), media_cost_usd, economics_known
		FROM usage_events ORDER BY connection_id, request_id`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	events := make(map[string]persistedUsageEvent)
	invalid := 0
	for rows.Next() {
		var e persistedUsageEvent
		if err := rows.Scan(&e.RequestID, &e.StartedAt, &e.CompletedAt, &e.UserID, &e.OriginID,
			&e.ProviderID, &e.ConnectionID, &e.ModelID, &e.PlanType, &e.InputTokens, &e.CacheReadTokens,
			&e.CacheWriteTokens, &e.OutputTokens, &e.ReasoningTokens, &e.BillableTokens, &e.CostUSD,
			&e.WorkloadKind, &e.Status, &e.ImageCount, &e.ImageMIME, &e.ImageWidth, &e.ImageHeight,
			&e.OperationID, &e.FailureClass, &e.MediaCostUSD, &e.EconomicsKnown); err != nil {
			return nil, invalid, err
		}
		if strings.TrimSpace(e.RequestID) == "" || strings.TrimSpace(e.ConnectionID) == "" {
			invalid++
			continue
		}
		events[e.ConnectionID+"\x00"+e.RequestID] = e
	}
	return events, invalid, rows.Err()
}

func ensureUsageMigrationSchema(db *sql.DB) error {
	var found bool
	rows, err := db.Query(`PRAGMA table_info(usage_events)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		found = found || name == "source_environment"
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !found {
		if _, err := db.Exec(`ALTER TABLE usage_events ADD COLUMN source_environment TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS usage_migrations (
		migration_id TEXT PRIMARY KEY,
		version TEXT NOT NULL,
		source_environment TEXT NOT NULL,
		source_sha256 TEXT NOT NULL,
		applied_at TEXT NOT NULL,
		inserted_events INTEGER NOT NULL,
		manifest_json TEXT NOT NULL
	)`)
	return err
}

func usageMigrationAlreadyApplied(db *sql.DB, migrationID string) (bool, error) {
	var tableCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'usage_migrations'`).Scan(&tableCount); err != nil {
		return false, err
	}
	if tableCount == 0 {
		return false, nil
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_migrations WHERE migration_id = ?`, migrationID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func sqliteLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func snapshotSQLite(db *sql.DB, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("backup already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	_, err := db.Exec(`VACUUM INTO ` + sqliteLiteral(filepath.Clean(path)))
	return err
}

func rebuildUsageProjections(tx *sql.Tx) error {
	if _, err := tx.Exec(`DELETE FROM request_costs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM daily_costs`); err != nil {
		return err
	}
	today := time.Now().UTC().Format("2006-01-02")
	if _, err := tx.Exec(`INSERT INTO request_costs (
		timestamp, account_id, account_type, user_id, request_id, model, input_tokens, cached_tokens,
		cache_creation_tokens, output_tokens, reasoning_tokens, cost_usd)
		SELECT completed_at, connection_id, provider_id, user_id, request_id, model_id, input_tokens,
		cache_read_tokens, cache_write_tokens, output_tokens, reasoning_tokens, cost_usd
		FROM usage_events WHERE completed_at >= ?`, today+"T00:00:00Z"); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO daily_costs (date, account_id, account_type, model,
		input_tokens, cached_tokens, output_tokens, reasoning_tokens, request_count, cost_usd)
		SELECT substr(completed_at,1,10), connection_id, provider_id, COALESCE(model_id,''),
		SUM(input_tokens), SUM(cache_read_tokens), SUM(output_tokens), SUM(reasoning_tokens), COUNT(*), SUM(cost_usd)
		FROM usage_events WHERE completed_at < ?
		GROUP BY substr(completed_at,1,10), connection_id, provider_id, COALESCE(model_id,'')`, today+"T00:00:00Z")
	return err
}

func migrateUsageEvents(opts usageMigrationOptions) (usageMigrationReport, error) {
	report := usageMigrationReport{
		Version: usageMigrationVersion, Mode: "dry-run", TargetPath: filepath.Clean(opts.TargetPath),
		SourcePath: filepath.Clean(opts.SourcePath), TargetEnvironment: opts.TargetEnvironment,
		SourceEnvironment: opts.SourceEnvironment, GeneratedAt: time.Now().UTC(),
	}
	if opts.Apply {
		report.Mode = "apply"
	}
	if opts.TargetPath == "" || opts.SourcePath == "" || opts.TargetEnvironment == "" || opts.SourceEnvironment == "" {
		return report, errors.New("target, source, target-environment, and source-environment are required")
	}
	targetAbs, _ := filepath.Abs(opts.TargetPath)
	sourceAbs, _ := filepath.Abs(opts.SourcePath)
	if strings.EqualFold(targetAbs, sourceAbs) {
		return report, errors.New("source and target databases must be different")
	}
	var err error
	report.SourceSHA256, err = fileSHA256(opts.SourcePath)
	if err != nil {
		return report, fmt.Errorf("hash source: %w", err)
	}
	report.TargetBeforeSHA256, err = fileSHA256(opts.TargetPath)
	if err != nil {
		return report, fmt.Errorf("hash target: %w", err)
	}
	idSum := sha256.Sum256([]byte(usageMigrationVersion + "\x00" + opts.SourceEnvironment + "\x00" + report.SourceSHA256))
	report.MigrationID = hex.EncodeToString(idSum[:16])

	target, err := sql.Open("sqlite", opts.TargetPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return report, err
	}
	defer target.Close()
	target.SetMaxOpenConns(1)
	if opts.Apply {
		// Keep exclusive locking mode on the sole target connection. The probe
		// fails if a gateway still has the database open for writes.
		if _, err := target.Exec(`PRAGMA locking_mode=EXCLUSIVE`); err != nil {
			return report, fmt.Errorf("set exclusive target lock: %w", err)
		}
		if _, err := target.Exec(`BEGIN EXCLUSIVE; COMMIT`); err != nil {
			return report, fmt.Errorf("target is not offline: %w", err)
		}
	}
	source, err := sql.Open("sqlite", "file:"+filepath.ToSlash(sourceAbs)+"?mode=ro")
	if err != nil {
		return report, err
	}
	defer source.Close()

	targetEvents, targetInvalid, err := readUsageEvents(target)
	if err != nil {
		return report, fmt.Errorf("read target events: %w", err)
	}
	sourceEvents, sourceInvalid, err := readUsageEvents(source)
	if err != nil {
		return report, fmt.Errorf("read source events: %w", err)
	}
	report.TargetEventsBefore, report.SourceEvents = len(targetEvents), len(sourceEvents)
	report.TargetInvalidEvents, report.SourceInvalidEvents = targetInvalid, sourceInvalid
	keys := make([]string, 0, len(sourceEvents))
	for key := range sourceEvents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var insertable []persistedUsageEvent
	for _, key := range keys {
		sourceEvent := sourceEvents[key]
		targetEvent, exists := targetEvents[key]
		if !exists {
			insertable = append(insertable, sourceEvent)
			continue
		}
		fields := usageEventDifferences(targetEvent, sourceEvent)
		if len(fields) == 0 {
			report.IdenticalEvents++
			continue
		}
		report.Conflicts = append(report.Conflicts, usageMigrationConflict{
			ConnectionID: sourceEvent.ConnectionID, RequestID: sourceEvent.RequestID, Fields: fields,
		})
	}
	report.InsertableEvents = len(insertable)
	report.ConflictingEvents = len(report.Conflicts)
	report.TargetEventsAfter = report.TargetEventsBefore
	if report.SourceInvalidEvents > 0 {
		return report, fmt.Errorf("source contains %d events without canonical identity", report.SourceInvalidEvents)
	}
	if report.ConflictingEvents > 0 {
		return report, fmt.Errorf("migration has %d payload conflicts; refusing to apply", report.ConflictingEvents)
	}
	if !opts.Apply {
		return report, nil
	}
	if opts.BackupDir == "" {
		return report, errors.New("backup-dir is required with --apply")
	}
	alreadyApplied, err := usageMigrationAlreadyApplied(target, report.MigrationID)
	if err != nil {
		return report, err
	}
	if alreadyApplied {
		report.AlreadyApplied = true
		report.TargetEventsAfter = report.TargetEventsBefore
		return report, nil
	}
	backupName := fmt.Sprintf("analytics-before-%s-%s.db", time.Now().UTC().Format("20060102T150405Z"), report.MigrationID)
	report.BackupPath = filepath.Join(opts.BackupDir, backupName)
	if err := snapshotSQLite(target, report.BackupPath); err != nil {
		return report, fmt.Errorf("snapshot target: %w", err)
	}
	report.BackupSHA256, err = fileSHA256(report.BackupPath)
	if err != nil {
		return report, err
	}
	if err := ensureUsageMigrationSchema(target); err != nil {
		return report, err
	}
	tx, err := target.Begin()
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE usage_events SET source_environment = ? WHERE source_environment = ''`, opts.TargetEnvironment); err != nil {
		return report, err
	}
	stmt, err := tx.Prepare(`INSERT INTO usage_events (
		request_id, started_at, completed_at, user_id, origin_id, provider_id, connection_id, model_id,
		plan_type, input_tokens, cache_read_tokens, cache_write_tokens, output_tokens, reasoning_tokens,
		billable_tokens, cost_usd, workload_kind, status, image_count, image_mime, image_width, image_height,
		operation_id, failure_class, media_cost_usd, economics_known, source_environment)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return report, err
	}
	for _, event := range insertable {
		values := append(event.values(), opts.SourceEnvironment)
		if _, err := stmt.Exec(values...); err != nil {
			stmt.Close()
			return report, err
		}
		report.InsertedEvents++
	}
	if err := stmt.Close(); err != nil {
		return report, err
	}
	if err := rebuildUsageProjections(tx); err != nil {
		return report, fmt.Errorf("rebuild projections: %w", err)
	}
	report.ProjectionRebuilt = true
	report.TargetEventsAfter = report.TargetEventsBefore + report.InsertedEvents
	manifest, _ := json.Marshal(report)
	if _, err := tx.Exec(`INSERT INTO usage_migrations
		(migration_id, version, source_environment, source_sha256, applied_at, inserted_events, manifest_json)
		VALUES (?,?,?,?,?,?,?)`, report.MigrationID, report.Version, opts.SourceEnvironment, report.SourceSHA256,
		time.Now().UTC().Format(time.RFC3339Nano), report.InsertedEvents, string(manifest)); err != nil {
		return report, err
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

func writeUsageMigrationReport(report usageMigrationReport, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func runUsageMigration(args []string) error {
	flags := flag.NewFlagSet("usage-migrate", flag.ContinueOnError)
	var opts usageMigrationOptions
	flags.StringVar(&opts.TargetPath, "target", "", "destination analytics.db")
	flags.StringVar(&opts.SourcePath, "source", "", "source analytics.db snapshot")
	flags.StringVar(&opts.TargetEnvironment, "target-environment", "", "destination environment name")
	flags.StringVar(&opts.SourceEnvironment, "source-environment", "", "source environment name")
	flags.StringVar(&opts.BackupDir, "backup-dir", "", "immutable rollback backup directory")
	flags.StringVar(&opts.ReportPath, "report", "", "JSON report path (stdout when empty)")
	flags.BoolVar(&opts.Apply, "apply", false, "apply after conflict-free dry-run; default is read-only")
	if err := flags.Parse(args); err != nil {
		return err
	}
	report, migrationErr := migrateUsageEvents(opts)
	if reportErr := writeUsageMigrationReport(report, opts.ReportPath); reportErr != nil {
		return reportErr
	}
	return migrationErr
}
