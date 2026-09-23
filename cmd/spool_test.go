package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
)

func TestSpoolEvictsOldestReportAtRecordLimit(t *testing.T) {
	spoolDir := t.TempDir()
	policy := spoolPolicy{maxBytes: 1024 * 1024, maxRecords: 2, retention: 24 * time.Hour}
	first := testSpoolRecord("11111111-1111-4111-8111-111111111111", "first")
	second := testSpoolRecord("22222222-2222-4222-8222-222222222222", "second")
	third := testSpoolRecord("33333333-3333-4333-8333-333333333333", "third")

	if err := writeSpoolRecordWithPolicy(spoolDir, first, policy); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(spoolDir, first.EventID+".json")
	oldest := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(firstPath, oldest, oldest); err != nil {
		t.Fatal(err)
	}
	if err := writeSpoolRecordWithPolicy(spoolDir, second, policy); err != nil {
		t.Fatal(err)
	}
	if err := writeSpoolRecordWithPolicy(spoolDir, third, policy); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatalf("oldest report was not evicted: %v", err)
	}
	for _, record := range []protocol.SpooledReport{second, third} {
		if _, err := os.Stat(filepath.Join(spoolDir, record.EventID+".json")); err != nil {
			t.Fatalf("newer report %s was not retained: %v", record.EventID, err)
		}
	}
}

func TestSpoolEvictsReportsPastRetention(t *testing.T) {
	spoolDir := t.TempDir()
	old := testSpoolRecord("44444444-4444-4444-8444-444444444444", "old")
	current := testSpoolRecord("55555555-5555-4555-8555-555555555555", "current")
	policy := spoolPolicy{maxBytes: 1024 * 1024, maxRecords: 10, retention: time.Hour}

	if err := writeSpoolRecordWithPolicy(spoolDir, old, policy); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(spoolDir, old.EventID+".json")
	expired := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldPath, expired, expired); err != nil {
		t.Fatal(err)
	}
	if err := writeSpoolRecordWithPolicy(spoolDir, current, policy); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expired report was not evicted: %v", err)
	}
}

func TestSpoolRejectsSingleReportOverByteLimit(t *testing.T) {
	spoolDir := t.TempDir()
	record := testSpoolRecord("66666666-6666-4666-8666-666666666666", strings.Repeat("x", 1024))
	policy := spoolPolicy{maxBytes: 100, maxRecords: 10, retention: time.Hour}

	if err := writeSpoolRecordWithPolicy(spoolDir, record, policy); err == nil {
		t.Fatal("expected oversized report to be rejected")
	}
	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("oversized report created spool files: %v", entries)
	}
}

func TestSpoolCapacityDoesNotFollowSymlinks(t *testing.T) {
	spoolDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(target, []byte("do not delete"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(spoolDir, "77777777-7777-4777-8777-777777777777.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	policy := spoolPolicy{maxBytes: 1024 * 1024, maxRecords: 1, retention: time.Hour}
	if err := writeSpoolRecordWithPolicy(
		spoolDir,
		testSpoolRecord("88888888-8888-4888-8888-888888888888", "safe"),
		policy,
	); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "do not delete" {
		t.Fatalf("symlink target was changed: data=%q err=%v", data, err)
	}
}

func TestSpoolPolicyFromConfig(t *testing.T) {
	policy, err := spoolPolicyFromConfig(&Config{
		SpoolMaxBytes:   4096,
		SpoolMaxRecords: 12,
		SpoolRetention:  "6h",
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.maxBytes != 4096 || policy.maxRecords != 12 || policy.retention != 6*time.Hour {
		t.Fatalf("unexpected policy: %#v", policy)
	}
	if _, err := spoolPolicyFromConfig(&Config{SpoolRetention: "never"}); err == nil {
		t.Fatal("invalid retention was accepted")
	}
}

func testSpoolRecord(eventID, stdout string) protocol.SpooledReport {
	return protocol.SpooledReport{
		EventID: eventID,
		Payload: protocol.ExecutionReportPayload{
			Command:   "true",
			Stdout:    stdout,
			StartTime: time.Now().UTC().Format(time.RFC3339),
		},
	}
}
