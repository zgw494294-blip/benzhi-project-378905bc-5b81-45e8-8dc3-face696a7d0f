package application

import (
	"dialectarchive/internal/domain"
	"dialectarchive/internal/storage"
	"strings"
	"testing"
)

func TestFlowAndIdempotency(t *testing.T) {
	st, _ := storage.New(t.TempDir())
	a := New(st)
	b, e := a.CreateBatch("吴语", "CN-ZJ", "c", "研究", "k")
	if e != nil {
		t.Fatal(e)
	}
	b2, _ := a.CreateBatch("其他", "X", "x", "", "k")
	if b2.ID != b.ID {
		t.Fatal("idempotency failed")
	}
	seg, q, e := a.RegisterSegment(b.ID, "S", 1000, 16000, 1, strings.Repeat("a", 64), domain.ConsentGranted, b.Version, "s")
	if e != nil || !q.Passed {
		t.Fatalf("segment: %v %#v", e, q)
	}
	b, _ = a.GetBatch(b.ID)
	_, q, e = a.SubmitAnnotation(domain.TranscriptAnnotation{ID: "a", SegmentID: seg.ID, AnnotatorID: "ann", Text: "侬好", VariantForm: "侬", EvidenceStartMS: 0, EvidenceEndMS: 800, NotationScheme: "漢字", Revision: 1}, b.Version, "a")
	if e != nil || !q.Passed {
		t.Fatalf("annotation: %v %#v", e, q)
	}
	b, _ = a.GetBatch(b.ID)
	if _, e = a.RecordReview(domain.ReviewDecision{BatchID: b.ID, ReviewerID: "r", Decision: domain.DecisionApprove}, b.Version, "r"); e != nil {
		t.Fatal(e)
	}
	b, _ = a.GetBatch(b.ID)
	c, m, e := a.Freeze(b.ID, b.Version, "admin", "subject", "f")
	if e != nil || m.SHA256 == "" {
		t.Fatalf("freeze: %v", e)
	}
	if _, ok, e := a.Verify(c.ID); e != nil || !ok {
		t.Fatalf("verify: %v %v", e, ok)
	}
}
