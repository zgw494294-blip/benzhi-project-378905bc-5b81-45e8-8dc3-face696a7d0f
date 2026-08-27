package quality

import (
	"dialectarchive/internal/domain"
	"strings"
	"testing"
)

func TestCheckSegment(t *testing.T) {
	s := domain.RecordingSegment{DurationMS: 1000, SampleRateHz: 8000, ChannelCount: 1, ContentSHA256: strings.Repeat("a", 64), ConsentState: domain.ConsentGranted}
	r := CheckSegment(s)
	if !r.Passed {
		t.Fatal("low rate should be warning only and pass")
	}
	s.ConsentState = domain.ConsentPending
	r = CheckSegment(s)
	if r.Passed {
		t.Fatal("pending consent should block")
	}
}
