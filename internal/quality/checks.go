package quality

import (
	"dialectarchive/internal/domain"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Issue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	ObjectID string `json:"object_id,omitempty"`
}
type Result struct {
	Passed bool    `json:"passed"`
	Issues []Issue `json:"issues"`
}

func CheckSegment(s domain.RecordingSegment) Result {
	issues := []Issue{}
	if s.SampleRateHz < 16000 {
		issues = append(issues, Issue{Code: "LOW_SAMPLE_RATE", Message: "采样率低于 16000 Hz", Severity: "warning"})
	}
	if s.ChannelCount != 1 {
		issues = append(issues, Issue{Code: "CHANNEL_COUNT", Message: "建议使用单声道录音", Severity: "warning"})
	}
	if s.DurationMS < 100 {
		issues = append(issues, Issue{Code: "SHORT_DURATION", Message: "片段时长过短", Severity: "error"})
	}
	if len(s.ContentSHA256) != 64 {
		issues = append(issues, Issue{Code: "INVALID_DIGEST", Message: "内容摘要长度错误", Severity: "error"})
	} else if _, e := hex.DecodeString(s.ContentSHA256); e != nil {
		issues = append(issues, Issue{Code: "INVALID_DIGEST", Message: "内容摘要不是十六进制", Severity: "error"})
	}
	if s.ConsentState == domain.ConsentPending {
		issues = append(issues, Issue{Code: "CONSENT_PENDING", Message: "说话人同意状态仍待确认", Severity: "error"})
	}
	passed := true
	for _, i := range issues {
		if i.Severity == "error" {
			passed = false
		}
	}
	return Result{passed, issues}
}

func CheckAnnotation(a domain.TranscriptAnnotation, s domain.RecordingSegment) Result {
	issues := []Issue{}
	if a.EvidenceStartMS < 0 || a.EvidenceEndMS > a.EvidenceStartMS && a.EvidenceEndMS <= s.DurationMS {
	} else {
		issues = append(issues, Issue{Code: "EVIDENCE_RANGE", Message: "证据时间范围无效", Severity: "error"})
	}
	if strings.TrimSpace(a.VariantForm) != "" && strings.ContainsAny(a.VariantForm, "\n\r") {
		issues = append(issues, Issue{Code: "VARIANT_FORMAT", Message: "异文不能包含换行", Severity: "error"})
	}
	if len([]rune(a.Text)) < 1 {
		issues = append(issues, Issue{Code: "EMPTY_TEXT", Message: "转写文本不能为空", Severity: "error"})
	}
	if a.NotationScheme != "IPA" && a.NotationScheme != "漢字" && a.NotationScheme != "拼音" {
		issues = append(issues, Issue{Code: "UNKNOWN_NOTATION", Message: "不支持的记法方案", Severity: "error"})
	}
	passed := true
	for _, i := range issues {
		if i.Severity == "error" {
			passed = false
		}
	}
	return Result{passed, issues}
}

func Summary(results ...Result) (string, []Issue) {
	all := []Issue{}
	passed := true
	for _, r := range results {
		all = append(all, r.Issues...)
		if !r.Passed {
			passed = false
		}
	}
	if passed {
		return "passed", all
	}
	for _, i := range all {
		if i.Severity == "error" {
			return "blocked", all
		}
	}
	return fmt.Sprintf("needs_fix"), all
}

func CheckTimeline(segments []domain.RecordingSegment) Result {
	issues := []Issue{}
	for i := range segments {
		for j := i + 1; j < len(segments); j++ {
			a, b := segments[i], segments[j]
			if a.SpeakerCode != b.SpeakerCode {
				continue
			}
			as, ae := a.StartedAt, a.StartedAt.Add(time.Duration(a.DurationMS)*time.Millisecond)
			bs, be := b.StartedAt, b.StartedAt.Add(time.Duration(b.DurationMS)*time.Millisecond)
			if as.Before(be) && bs.Before(ae) {
				issues = append(issues, Issue{Code: "TIMELINE_OVERLAP", Message: "同一说话人的片段时间区间重叠", Severity: "error", ObjectID: a.ID + "," + b.ID})
			}
		}
	}
	return Result{Passed: len(issues) == 0, Issues: issues}
}

func CheckConsent(s domain.RecordingSegment, policy string) Result {
	if s.ConsentState == domain.ConsentPending {
		return Result{false, []Issue{{Code: "CONSENT_PENDING", Message: "同意状态待确认，不符合批次政策", Severity: "error", ObjectID: s.ID}}}
	}
	if s.ConsentState == domain.ConsentRestricted && strings.Contains(policy, "公开") {
		return Result{false, []Issue{{Code: "CONSENT_RESTRICTED", Message: "受限同意状态不符合公开政策", Severity: "error", ObjectID: s.ID}}}
	}
	return Result{true, nil}
}
