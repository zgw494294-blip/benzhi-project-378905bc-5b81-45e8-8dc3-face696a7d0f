package application

import (
	"dialectarchive/internal/domain"
	"time"
)

// BatchRevisionCommand describes a partial batch edit after HTTP decoding.
type BatchRevisionCommand struct {
	BatchID         string
	DialectName     *string
	LocationCode    *string
	CollectorID     *string
	ConsentPolicy   *string
	ExpectedVersion int64
	IdempotencyKey  string
}

// SegmentCommand captures the immutable metadata needed to register a clip.
type SegmentCommand struct {
	BatchID, SpeakerCode, ContentSHA256 string
	StartedAt                           time.Time
	DurationMS                          int64
	SampleRateHz, ChannelCount          int
	ConsentState                        domain.ConsentState
	ExpectedVersion                     int64
	IdempotencyKey                      string
}
