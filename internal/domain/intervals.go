package domain

import "time"

// SegmentEnd returns the exclusive end of a recording interval.
func SegmentEnd(segment RecordingSegment) time.Time {
	return segment.StartedAt.Add(time.Duration(segment.DurationMS) * time.Millisecond)
}

// IntervalsOverlap identifies a real overlap while allowing adjacent clips.
func IntervalsOverlap(a, b RecordingSegment) bool {
	if a.SpeakerCode != b.SpeakerCode {
		return false
	}
	return a.StartedAt.Before(SegmentEnd(b)) && b.StartedAt.Before(SegmentEnd(a))
}

// EvidenceWithinSegment validates an annotation's half-open evidence range.
func EvidenceWithinSegment(annotation TranscriptAnnotation, segment RecordingSegment) bool {
	return annotation.EvidenceStartMS >= 0 && annotation.EvidenceEndMS > annotation.EvidenceStartMS && annotation.EvidenceEndMS <= segment.DurationMS
}
