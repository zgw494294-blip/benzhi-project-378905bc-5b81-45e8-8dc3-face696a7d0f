package storage

import "path/filepath"

const (
	eventsFilename   = "events.jsonl"
	snapshotFilename = "snapshot.json"
)

// EventPath and SnapshotPath keep persistence filenames in one place for
// startup checks and operational diagnostics.
func EventPath(dir string) string    { return filepath.Join(dir, eventsFilename) }
func SnapshotPath(dir string) string { return filepath.Join(dir, snapshotFilename) }
