package annotation_cross_segment_revision_test

import (
	"dialectarchive/internal/application"
	"dialectarchive/internal/domain"
	"dialectarchive/internal/storage"
	"strings"
	"testing"
)

func TestAnnotationRevisionCannotMoveBetweenSegments(t *testing.T) {
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	b, err := app.CreateBatch("吴语", "CN-ZJ", "collector", "研究用途", "batch-key")
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := app.RegisterSegment(b.ID, "S01", 1000, 16000, 1, strings.Repeat("a", 64), domain.ConsentGranted, b.Version, "segment-one-key")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = app.GetBatch(b.ID)
	second, _, err := app.RegisterSegment(b.ID, "S02", 1000, 16000, 1, strings.Repeat("b", 64), domain.ConsentGranted, b.Version, "segment-two-key")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = app.GetBatch(b.ID)
	base := domain.TranscriptAnnotation{ID: "annotation-one", SegmentID: first.ID, AnnotatorID: "annotator", Text: "侬好", EvidenceStartMS: 0, EvidenceEndMS: 800, NotationScheme: "漢字", Revision: 1}
	if _, _, err := app.SubmitAnnotation(base, b.Version, "annotation-one-key"); err != nil {
		t.Fatal(err)
	}
	b, _ = app.GetBatch(b.ID)
	moved := base
	moved.SegmentID = second.ID
	moved.Revision = 2
	moved.PreviousRevision = 1
	if _, _, err := app.SubmitAnnotation(moved, b.Version, "annotation-two-key"); err == nil {
		t.Fatal("annotation revision was accepted for a different segment")
	}
}
