package persistence_error_state_leak_test

import (
	"dialectarchive/internal/application"
	"dialectarchive/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestFailedPersistenceDoesNotPublishBatchInMemory(t *testing.T) {
	dir := t.TempDir()
	st, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "events.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = application.New(st).CreateBatch("吴语", "CN-ZJ", "collector", "研究用途", "write-failure-key")
	if err == nil {
		t.Fatal("test setup did not trigger a persistence error")
	}
	if got := len(st.ListBatches()); got != 0 {
		t.Fatalf("failed persistence left batch visible in memory: count=%d", got)
	}
}
