package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
)

const (
	defaultSpoolMaxBytes   int64 = 256 * 1024 * 1024
	defaultSpoolMaxRecords       = 10_000
	defaultSpoolRetention        = 30 * 24 * time.Hour
)

type spoolPolicy struct {
	maxBytes   int64
	maxRecords int
	retention  time.Duration
}

type spoolFile struct {
	name    string
	path    string
	size    int64
	modTime time.Time
}

func defaultSpoolPolicy() spoolPolicy {
	return spoolPolicy{
		maxBytes:   defaultSpoolMaxBytes,
		maxRecords: defaultSpoolMaxRecords,
		retention:  defaultSpoolRetention,
	}
}

func spoolPolicyFromConfig(config *Config) (spoolPolicy, error) {
	policy := defaultSpoolPolicy()
	if config == nil {
		return policy, nil
	}
	if config.SpoolMaxBytes < 0 {
		return spoolPolicy{}, errors.New("spool_max_bytes must be positive")
	}
	if config.SpoolMaxBytes > 0 {
		policy.maxBytes = config.SpoolMaxBytes
	}
	if config.SpoolMaxRecords < 0 {
		return spoolPolicy{}, errors.New("spool_max_records must be positive")
	}
	if config.SpoolMaxRecords > 0 {
		policy.maxRecords = config.SpoolMaxRecords
	}
	if config.SpoolRetention != "" {
		retention, err := time.ParseDuration(config.SpoolRetention)
		if err != nil || retention <= 0 {
			return spoolPolicy{}, errors.New("spool_retention must be a positive duration")
		}
		policy.retention = retention
	}
	return policy, nil
}

func (policy spoolPolicy) normalized() spoolPolicy {
	defaults := defaultSpoolPolicy()
	if policy.maxBytes == 0 {
		policy.maxBytes = defaults.maxBytes
	}
	if policy.maxRecords == 0 {
		policy.maxRecords = defaults.maxRecords
	}
	if policy.retention == 0 {
		policy.retention = defaults.retention
	}
	return policy
}

func (policy spoolPolicy) validate() error {
	if policy.maxBytes <= 0 {
		return errors.New("spool maximum bytes must be positive")
	}
	if policy.maxRecords <= 0 {
		return errors.New("spool maximum records must be positive")
	}
	if policy.retention <= 0 {
		return errors.New("spool retention must be positive")
	}
	return nil
}

func writeSpoolRecordWithPolicy(
	spoolDir string,
	record protocol.SpooledReport,
	policy spoolPolicy,
) error {
	if !validEventID(record.EventID) {
		return errors.New("execution report event ID is not a UUID")
	}
	policy = policy.normalized()
	if err := policy.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if int64(len(data)) > policy.maxBytes {
		return fmt.Errorf("execution report exceeds spool byte limit (%d > %d)", len(data), policy.maxBytes)
	}
	if err := ensurePrivateDir(spoolDir); err != nil {
		return err
	}
	name := record.EventID + ".json"
	if err := makeSpoolCapacity(spoolDir, name, int64(len(data)), policy, time.Now()); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(spoolDir, name), record)
}

func makeSpoolCapacity(
	spoolDir string,
	targetName string,
	incomingBytes int64,
	policy spoolPolicy,
	now time.Time,
) error {
	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		return fmt.Errorf("read report spool: %w", err)
	}

	files := make([]spoolFile, 0, len(entries))
	var totalBytes int64
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 ||
			!strings.HasSuffix(entry.Name(), ".json") || entry.Name() == targetName {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect spooled report: %w", err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		file := spoolFile{
			name:    entry.Name(),
			path:    filepath.Join(spoolDir, entry.Name()),
			size:    info.Size(),
			modTime: info.ModTime(),
		}
		if now.Sub(file.modTime) > policy.retention {
			if err := evictSpoolFile(file, "retention"); err != nil {
				return err
			}
			continue
		}
		files = append(files, file)
		totalBytes += file.size
	}

	sort.Slice(files, func(left, right int) bool {
		if files[left].modTime.Equal(files[right].modTime) {
			return files[left].name < files[right].name
		}
		return files[left].modTime.Before(files[right].modTime)
	})

	for len(files)+1 > policy.maxRecords || totalBytes+incomingBytes > policy.maxBytes {
		if len(files) == 0 {
			return errors.New("cannot make capacity for execution report spool")
		}
		oldest := files[0]
		files = files[1:]
		if err := evictSpoolFile(oldest, "capacity"); err != nil {
			return err
		}
		totalBytes -= oldest.size
	}
	return nil
}

func evictSpoolFile(file spoolFile, reason string) error {
	if err := os.Remove(file.path); err != nil {
		return fmt.Errorf("evict spooled report %s: %w", file.name, err)
	}
	log.Printf("Spool eviction: reason=%s file=%s bytes=%d", reason, file.name, file.size)
	return nil
}
