package quality

import "dialectarchive/internal/domain"

// CheckEvidence is the focused annotation evidence rule used by diagnostics
// and by clients that only need to validate a proposed time range.
func CheckEvidence(annotation domain.TranscriptAnnotation, segment domain.RecordingSegment) Result {
	if domain.EvidenceWithinSegment(annotation, segment) {
		return Result{Passed: true, Issues: []Issue{}}
	}
	return Result{Passed: false, Issues: []Issue{{Code: "EVIDENCE_RANGE", Message: "证据时间范围无效", Severity: SeverityError, ObjectID: annotation.ID}}}
}
