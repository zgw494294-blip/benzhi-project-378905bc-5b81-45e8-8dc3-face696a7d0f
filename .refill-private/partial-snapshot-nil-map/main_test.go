package partial_snapshot_nil_map_test

import (
	"dialectarchive/internal/application"
	"dialectarchive/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestPartialSnapshotCannotCauseNilMapPanic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), []byte(`{"schemaVersion":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := storage.New(dir)
	if err != nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("partial snapshot caused nil-map panic: %v", recovered)
		}
	}()
	_, _ = application.New(st).CreateBatch("吴语", "CN-ZJ", "collector", "研究用途", "snapshot-key")
}
