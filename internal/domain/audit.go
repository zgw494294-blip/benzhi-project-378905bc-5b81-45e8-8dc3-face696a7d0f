package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// BatchMetadataIssue describes one field-level problem found before a command
// is persisted.  Keeping the field name separate from the message lets the
// browser highlight the exact input without parsing Chinese text.
type BatchMetadataIssue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidateBatchMetadata is shared by create and revision screens.  The
// command handlers still decide when a failed validation should change state;
// this function only reports deterministic input problems.
func ValidateBatchMetadata(dialect, location, collector, policy string) []BatchMetadataIssue {
	checks := []struct {
		field string
		value string
		code  string
		msg   string
	}{
		{"dialect_name", dialect, "DIALECT_REQUIRED", "方言名称不能为空"},
		{"location_code", location, "LOCATION_REQUIRED", "地点代码不能为空"},
		{"collector_id", collector, "COLLECTOR_REQUIRED", "采集员编号不能为空"},
	}
	issues := make([]BatchMetadataIssue, 0, len(checks)+1)
	for _, check := range checks {
		if strings.TrimSpace(check.value) == "" {
			issues = append(issues, BatchMetadataIssue{Field: check.field, Code: check.code, Message: check.msg})
		}
	}
	if strings.TrimSpace(policy) == "" {
		issues = append(issues, BatchMetadataIssue{Field: "consent_policy", Code: "POLICY_MISSING", Message: "同意政策为空，将按研究用途处理"})
	}
	return issues
}

// ValidateExpectedVersion centralizes the optimistic-concurrency diagnostic.
func ValidateExpectedVersion(expected, current int64) error {
	if expected <= 0 {
		return errors.New("expectedVersion 必须为正数")
	}
	if current != expected {
		return fmt.Errorf("版本冲突，当前版本为 %d", current)
	}
	return nil
}

// StatusLabel provides stable Chinese labels for the workbench while keeping
// API values in their protocol form.
func StatusLabel(status BatchStatus) string {
	switch status {
	case StatusDraft:
		return "草稿"
	case StatusQualityChecked:
		return "质量已检查"
	case StatusInReview:
		return "专家复核中"
	case StatusRemediation:
		return "整改中"
	case StatusFrozen:
		return "已冻结"
	case StatusReleased:
		return "已发布"
	default:
		return "未知状态"
	}
}

// ConsentLabel is used by summaries and avoids exposing an empty value as a
// valid consent state.
func ConsentLabel(state ConsentState) string {
	switch state {
	case ConsentGranted:
		return "已同意"
	case ConsentRestricted:
		return "受限"
	case ConsentPending:
		return "待确认"
	default:
		return "未知"
	}
}

// QualityLabel translates the persisted quality state for a human-facing
// summary.  Unknown is intentionally explicit so an incomplete import cannot
// look like a passing recording.
func QualityLabel(state QualityState) string {
	switch state {
	case QualityPassed:
		return "通过"
	case QualityNeedsFix:
		return "需整改"
	default:
		return "未检查"
	}
}

// AnnotationRevisionSummary captures the immutable revision chain in a form
// suitable for audit views.
type AnnotationRevisionSummary struct {
	AnnotationID string `json:"annotation_id"`
	SegmentID    string `json:"segment_id"`
	Revisions    []int  `json:"revisions"`
	Current      int    `json:"current_revision"`
	Approved     bool   `json:"approved"`
}

func BuildRevisionSummary(history []TranscriptAnnotation) AnnotationRevisionSummary {
	result := AnnotationRevisionSummary{Revisions: []int{}}
	if len(history) == 0 {
		return result
	}
	ordered := append([]TranscriptAnnotation(nil), history...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Revision < ordered[j].Revision })
	result.AnnotationID = ordered[len(ordered)-1].ID
	result.SegmentID = ordered[len(ordered)-1].SegmentID
	for _, annotation := range ordered {
		result.Revisions = append(result.Revisions, annotation.Revision)
		if annotation.Revision >= result.Current {
			result.Current = annotation.Revision
			result.Approved = annotation.State == AnnotationApproved
		}
	}
	return result
}

// RevisionChainIssues reports gaps and duplicate revisions without mutating
// the source history.  It is deliberately useful for startup audits and for
// diagnosing data imported from an older version of the service.
func RevisionChainIssues(history []TranscriptAnnotation) []string {
	if len(history) < 2 {
		return nil
	}
	ordered := append([]TranscriptAnnotation(nil), history...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Revision < ordered[j].Revision })
	issues := []string{}
	previous := ordered[0].Revision
	if previous != 1 {
		issues = append(issues, fmt.Sprintf("revision 链从 %d 开始", previous))
	}
	for _, annotation := range ordered[1:] {
		if annotation.Revision == previous {
			issues = append(issues, fmt.Sprintf("revision %d 重复", annotation.Revision))
		} else if annotation.Revision != previous+1 {
			issues = append(issues, fmt.Sprintf("revision 从 %d 跳到 %d", previous, annotation.Revision))
		}
		previous = annotation.Revision
	}
	return issues
}

// ManifestDigest computes the digest of the manifest content, excluding the
// stored digest field itself.  JSON structures use slices rather than maps,
// and callers sort those slices before invoking this function.
func ManifestDigest(manifest ReleaseManifest) (string, error) {
	manifest.SHA256 = ""
	raw, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func ValidateManifestDigest(manifest ReleaseManifest) error {
	if len(manifest.SHA256) != 64 {
		return errors.New("manifest_sha256 格式错误")
	}
	if _, err := hex.DecodeString(manifest.SHA256); err != nil {
		return errors.New("manifest_sha256 不是十六进制摘要")
	}
	expected, err := ManifestDigest(manifest)
	if err != nil {
		return err
	}
	if expected != manifest.SHA256 {
		return errors.New("冻结清单摘要不匹配")
	}
	return nil
}

func ValidateCredentialWindow(issued time.Time, expires *time.Time) error {
	if issued.IsZero() {
		return errors.New("issued_at 不能为空")
	}
	if expires != nil && !expires.After(issued) {
		return errors.New("expires_at 必须晚于 issued_at")
	}
	return nil
}

func CredentialIsExpired(credential CitationCredential, at time.Time) bool {
	return credential.ExpiresAt != nil && !at.Before(*credential.ExpiresAt)
}

// IsTerminal reports whether no further user command can change a batch.  A
// released batch remains auditable but cannot be revised or frozen again.
func IsTerminal(status BatchStatus) bool {
	return status == StatusReleased
}

func IsFrozenOrReleased(status BatchStatus) bool {
	return status == StatusFrozen || status == StatusReleased
}

// ReviewSummary makes review counters consistent across API responses.
type ReviewSummary struct {
	TotalFindings  int `json:"total_findings"`
	OpenFindings   int `json:"open_findings"`
	PendingRecheck int `json:"pending_recheck"`
	PassedFindings int `json:"passed_findings"`
}

func SummarizeReviews(reviews []ReviewDecision) ReviewSummary {
	var summary ReviewSummary
	for _, review := range reviews {
		if review.Decision != DecisionRequestChanges {
			continue
		}
		summary.TotalFindings++
		switch review.Status {
		case "passed":
			summary.PassedFindings++
		case "pending_recheck":
			summary.PendingRecheck++
		default:
			summary.OpenFindings++
		}
	}
	return summary
}
