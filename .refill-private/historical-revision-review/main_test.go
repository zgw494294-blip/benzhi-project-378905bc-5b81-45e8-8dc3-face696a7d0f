package historical_revision_review_test

import (
	"dialectarchive/internal/application"
	"dialectarchive/internal/domain"
	"dialectarchive/internal/storage"
	"strings"
	"testing"
)

func TestReviewCanTargetRetainedHistoricalRevision(t *testing.T) {
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	b, err := app.CreateBatch("吴语", "CN-ZJ", "collector", "研究用途", "batch-key")
	if err != nil {
		t.Fatal(err)
	}
	segment, _, err := app.RegisterSegment(b.ID, "S01", 1000, 16000, 1, strings.Repeat("a", 64), domain.ConsentGranted, b.Version, "segment-key")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = app.GetBatch(b.ID)
	first := domain.TranscriptAnnotation{ID: "annotation-one", SegmentID: segment.ID, AnnotatorID: "annotator", Text: "侬好", EvidenceStartMS: 0, EvidenceEndMS: 800, NotationScheme: "漢字", Revision: 1}
	if _, _, err := app.SubmitAnnotation(first, b.Version, "annotation-one-key"); err != nil {
		t.Fatal(err)
	}
	b, _ = app.GetBatch(b.ID)
	second := first
	second.Text = "侬好呀"
	second.Revision = 2
	second.PreviousRevision = 1
	if _, _, err := app.SubmitAnnotation(second, b.Version, "annotation-two-key"); err != nil {
		t.Fatal(err)
	}
	b, _ = app.GetBatch(b.ID)
	review := domain.ReviewDecision{BatchID: b.ID, ReviewerID: "reviewer", FindingCode: "TEXT_EVIDENCE", Comment: "第一版证据不足", RequiredAction: "补充第一版的证据说明", Decision: domain.DecisionRequestChanges, TargetRevision: 1, TargetAnnotationID: first.ID}
	if _, err := app.RecordReview(review, b.Version, "review-key"); err != nil {
		t.Fatalf("historical annotation revision could not be reviewed: %v", err)
	}
}
