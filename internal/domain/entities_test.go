package domain

import (
	"testing"
	"time"
)

func TestBatchTransitions(t *testing.T) {
	b, e := NewBatch("b", "方言", "CN", "c", "研究", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	for _, st := range []BatchStatus{StatusQualityChecked, StatusInReview, StatusRemediation, StatusQualityChecked, StatusInReview, StatusFrozen, StatusReleased} {
		if e = b.Transition(st, time.Now()); e != nil {
			t.Fatalf("to %s: %v", st, e)
		}
	}
	if b.Version != 8 {
		t.Fatalf("version=%d", b.Version)
	}
}
func TestAnnotationRange(t *testing.T) {
	s := RecordingSegment{ID: "s", BatchID: "b", DurationMS: 1000}
	a := TranscriptAnnotation{ID: "a", SegmentID: "s", AnnotatorID: "x", Text: "字", EvidenceStartMS: 0, EvidenceEndMS: 1001, NotationScheme: "漢字", Revision: 1}
	if ValidateAnnotation(a, s) == nil {
		t.Fatal("expected range error")
	}
}
