package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func migrationTestStore(t *testing.T, path string, events ...UsageEvent) *AnalyticsStore {
	t.Helper()
	store, err := newAnalyticsStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if inserted, err := store.recordUsageEvent(event); err != nil || !inserted {
			t.Fatalf("record event inserted=%v err=%v", inserted, err)
		}
	}
	return store
}

func migrationEvent(requestID, userID string, input int64, completed time.Time) UsageEvent {
	return UsageEvent{
		RequestID: requestID, StartedAt: completed.Add(-time.Second), CompletedAt: completed,
		UserID: userID, OriginID: "origin", ProviderID: AccountTypeCodex,
		ConnectionID: "connection", ModelID: "gpt-5.6-sol", PlanType: "pro",
		InputTokens: input, CacheReadTokens: 2, CacheWriteTokens: 1,
		OutputTokens: 4, ReasoningTokens: 3, BillableTokens: input + 2, CostUSD: 1.25,
	}
}

func TestUsageMigrationDryRunIsReadOnlyAndReportsPlan(t *testing.T) {
	dir := t.TempDir()
	targetPath, sourcePath := filepath.Join(dir, "target.db"), filepath.Join(dir, "source.db")
	now := time.Now().UTC()
	target := migrationTestStore(t, targetPath, migrationEvent("existing", "member", 10, now))
	target.Close()
	source := migrationTestStore(t, sourcePath,
		migrationEvent("existing", "member", 10, now), migrationEvent("new", "member", 20, now))
	source.Close()
	before, err := fileSHA256(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	report, err := migrateUsageEvents(usageMigrationOptions{
		TargetPath: targetPath, SourcePath: sourcePath, TargetEnvironment: "staging", SourceEnvironment: "staging-archive",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.InsertableEvents != 1 || report.IdenticalEvents != 1 || report.InsertedEvents != 0 || report.Mode != "dry-run" {
		t.Fatalf("unexpected report: %+v", report)
	}
	after, _ := fileSHA256(targetPath)
	if before != after {
		t.Fatal("dry-run changed target database")
	}
}

func TestUsageMigrationRejectsPayloadConflictsWithoutBackupOrWrite(t *testing.T) {
	dir := t.TempDir()
	targetPath, sourcePath := filepath.Join(dir, "target.db"), filepath.Join(dir, "source.db")
	now := time.Now().UTC()
	target := migrationTestStore(t, targetPath, migrationEvent("same", "real-member", 10, now))
	target.Close()
	source := migrationTestStore(t, sourcePath, migrationEvent("same", "local-development", 10, now))
	source.Close()
	before, _ := fileSHA256(targetPath)
	backupDir := filepath.Join(dir, "backups")
	report, err := migrateUsageEvents(usageMigrationOptions{
		TargetPath: targetPath, SourcePath: sourcePath, TargetEnvironment: "staging", SourceEnvironment: "staging-archive",
		BackupDir: backupDir, Apply: true,
	})
	if err == nil || report.ConflictingEvents != 1 || len(report.Conflicts) != 1 || report.Conflicts[0].Fields[0] != "user_id" {
		t.Fatalf("expected user conflict, report=%+v err=%v", report, err)
	}
	after, _ := fileSHA256(targetPath)
	if before != after {
		t.Fatal("conflict changed target database")
	}
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Fatalf("conflict should not create backup, stat err=%v", err)
	}
}

func TestUsageMigrationApplyBacksUpRebuildsProjectionsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	targetPath, sourcePath := filepath.Join(dir, "target.db"), filepath.Join(dir, "source.db")
	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1)
	target := migrationTestStore(t, targetPath, migrationEvent("existing", "member", 10, yesterday))
	target.Close()
	source := migrationTestStore(t, sourcePath, migrationEvent("new", "member", 20, now))
	source.Close()
	opts := usageMigrationOptions{
		TargetPath: targetPath, SourcePath: sourcePath, TargetEnvironment: "staging", SourceEnvironment: "staging-archive",
		BackupDir: filepath.Join(dir, "backups"), Apply: true,
	}
	report, err := migrateUsageEvents(opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.InsertedEvents != 1 || report.TargetEventsAfter != 2 || !report.ProjectionRebuilt || report.BackupPath == "" || report.BackupSHA256 == "" {
		t.Fatalf("unexpected applied report: %+v", report)
	}
	if _, err := os.Stat(report.BackupPath); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var events, requests, daily, migrations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM request_costs`).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM daily_costs`).Scan(&daily); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_migrations`).Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	if events != 2 || requests != 1 || daily != 1 || migrations != 1 {
		t.Fatalf("events=%d requests=%d daily=%d migrations=%d", events, requests, daily, migrations)
	}
	var existingSource, newSource string
	if err := db.QueryRow(`SELECT source_environment FROM usage_events WHERE request_id='existing'`).Scan(&existingSource); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT source_environment FROM usage_events WHERE request_id='new'`).Scan(&newSource); err != nil {
		t.Fatal(err)
	}
	if existingSource != "staging" || newSource != "staging-archive" {
		t.Fatalf("provenance existing=%q new=%q", existingSource, newSource)
	}
	db.Close()

	second, err := migrateUsageEvents(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !second.AlreadyApplied || second.InsertedEvents != 0 {
		t.Fatalf("second apply not idempotent: %+v", second)
	}
	matches, err := filepath.Glob(filepath.Join(opts.BackupDir, "*.db"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("idempotent apply created backup: matches=%v err=%v", matches, err)
	}
}
