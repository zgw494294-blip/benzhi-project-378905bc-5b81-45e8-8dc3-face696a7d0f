package idempotency_type_confusion_test

import (
	"dialectarchive/internal/application"
	"dialectarchive/internal/domain"
	"dialectarchive/internal/storage"
	"strings"
	"testing"
)

func TestIdempotencyKeyCrossOperationDoesNotPanic(t *testing.T) {
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	b, err := app.CreateBatch("吴语", "CN-ZJ", "collector", "研究用途", "shared-key")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("cross-operation idempotency key panic: %v", recovered)
		}
	}()
	_, _, _ = app.RegisterSegment(b.ID, "S01", 1000, 16000, 1, strings.Repeat("a", 64), domain.ConsentGranted, b.Version, "shared-key")
}
