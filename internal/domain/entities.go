package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type BatchStatus string

const (
	StatusDraft          BatchStatus = "draft"
	StatusQualityChecked BatchStatus = "quality_checked"
	StatusInReview       BatchStatus = "in_review"
	StatusRemediation    BatchStatus = "remediation"
	StatusFrozen         BatchStatus = "frozen"
	StatusReleased       BatchStatus = "released"
)

type ConsentState string

const (
	ConsentPending    ConsentState = "pending"
	ConsentGranted    ConsentState = "granted"
	ConsentRestricted ConsentState = "restricted"
)

type QualityState string

const (
	QualityUnknown  QualityState = "unknown"
	QualityPassed   QualityState = "passed"
	QualityNeedsFix QualityState = "needs_fix"
)

type AnnotationState string

const (
	AnnotationSubmitted AnnotationState = "submitted"
	AnnotationApproved  AnnotationState = "approved"
	AnnotationChanges   AnnotationState = "changes_requested"
)

type ReviewDecisionType string

const (
	DecisionApprove        ReviewDecisionType = "approve"
	DecisionRequestChanges ReviewDecisionType = "request_changes"
)

type CorpusBatch struct {
	ID            string      `json:"id"`
	DialectName   string      `json:"dialect_name"`
	LocationCode  string      `json:"location_code"`
	CollectorID   string      `json:"collector_id"`
	ConsentPolicy string      `json:"consent_policy"`
	Status        BatchStatus `json:"status"`
	Version       int64       `json:"version"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

type RecordingSegment struct {
	ID            string       `json:"id"`
	BatchID       string       `json:"batch_id"`
	SpeakerCode   string       `json:"speaker_code"`
	StartedAt     time.Time    `json:"started_at"`
	DurationMS    int64        `json:"duration_ms"`
	SampleRateHz  int          `json:"sample_rate_hz"`
	ChannelCount  int          `json:"channel_count"`
	ContentSHA256 string       `json:"content_sha256"`
	ConsentState  ConsentState `json:"consent_state"`
	QualityState  QualityState `json:"quality_state"`
}

type TranscriptAnnotation struct {
	ID               string          `json:"id"`
	SegmentID        string          `json:"segment_id"`
	AnnotatorID      string          `json:"annotator_id"`
	Text             string          `json:"text"`
	VariantForm      string          `json:"variant_form"`
	EvidenceStartMS  int64           `json:"evidence_start_ms"`
	EvidenceEndMS    int64           `json:"evidence_end_ms"`
	NotationScheme   string          `json:"notation_scheme"`
	Revision         int             `json:"revision"`
	PreviousRevision int             `json:"previous_revision,omitempty"`
	State            AnnotationState `json:"state"`
}

type ReviewDecision struct {
	ID                 string             `json:"id"`
	BatchID            string             `json:"batch_id"`
	ReviewerID         string             `json:"reviewer_id"`
	FindingCode        string             `json:"finding_code"`
	Comment            string             `json:"comment"`
	RequiredAction     string             `json:"required_action"`
	Decision           ReviewDecisionType `json:"decision"`
	TargetRevision     int                `json:"target_revision"`
	TargetAnnotationID string             `json:"target_annotation_id,omitempty"`
	Status             string             `json:"status,omitempty"`
	ResolvedRevision   int                `json:"resolved_revision,omitempty"`
	ReviewedAt         time.Time          `json:"reviewed_at"`
}

type CitationCredential struct {
	ID                 string     `json:"id"`
	BatchID            string     `json:"batch_id"`
	ManifestSHA256     string     `json:"manifest_sha256"`
	SubjectLabel       string     `json:"subject_label"`
	IssuedBy           string     `json:"issued_by"`
	IssuedAt           time.Time  `json:"issued_at"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	Signature          string     `json:"signature"`
	VerificationState  string     `json:"verification_state"`
	FrozenBatchVersion int64      `json:"frozen_batch_version,omitempty"`
}

type ReleaseManifest struct {
	Batch       CorpusBatch            `json:"batch"`
	Segments    []RecordingSegment     `json:"segments"`
	Annotations []TranscriptAnnotation `json:"annotations"`
	SHA256      string                 `json:"sha256"`
}

type Event struct {
	Sequence      int64     `json:"sequence"`
	SchemaVersion int       `json:"schemaVersion"`
	Type          string    `json:"type"`
	AggregateID   string    `json:"aggregate_id"`
	Payload       any       `json:"payload"`
	PrevHash      string    `json:"prev_hash"`
	Hash          string    `json:"hash"`
	At            time.Time `json:"at"`
}

func NewBatch(id, dialect, location, collector, policy string, now time.Time) (CorpusBatch, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(dialect) == "" || strings.TrimSpace(location) == "" || strings.TrimSpace(collector) == "" {
		return CorpusBatch{}, errors.New("批次标识、方言、地点和采集员不能为空")
	}
	return CorpusBatch{ID: id, DialectName: dialect, LocationCode: location, CollectorID: collector, ConsentPolicy: policy, Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (b *CorpusBatch) Transition(to BatchStatus, now time.Time) error {
	allowed := map[BatchStatus]map[BatchStatus]bool{StatusDraft: {StatusQualityChecked: true, StatusInReview: true}, StatusQualityChecked: {StatusInReview: true, StatusRemediation: true}, StatusInReview: {StatusRemediation: true, StatusFrozen: true}, StatusRemediation: {StatusQualityChecked: true, StatusInReview: true}, StatusFrozen: {StatusReleased: true}}
	if !allowed[b.Status][to] {
		return fmt.Errorf("批次状态不能从 %s 转为 %s", b.Status, to)
	}
	b.Status = to
	b.Version++
	b.UpdatedAt = now
	return nil
}

func ValidateSegment(s RecordingSegment) error {
	if s.ID == "" || s.BatchID == "" || s.SpeakerCode == "" {
		return errors.New("片段标识、批次和说话人不能为空")
	}
	if s.DurationMS <= 0 || s.DurationMS > 24*60*60*1000 {
		return errors.New("片段时长必须在 1 毫秒到 24 小时之间")
	}
	if s.SampleRateHz < 8000 || s.SampleRateHz > 192000 {
		return errors.New("采样率不在支持范围")
	}
	if s.ChannelCount < 1 || s.ChannelCount > 8 {
		return errors.New("声道数不在支持范围")
	}
	if len(s.ContentSHA256) != 64 {
		return errors.New("content_sha256 必须是 64 位十六进制摘要")
	}
	if _, err := hex.DecodeString(s.ContentSHA256); err != nil {
		return errors.New("content_sha256 格式错误")
	}
	if s.ConsentState == "" {
		return errors.New("必须提供同意状态")
	}
	return nil
}

func ValidateAnnotation(a TranscriptAnnotation, seg RecordingSegment) error {
	if a.ID == "" || a.SegmentID != seg.ID || a.AnnotatorID == "" || strings.TrimSpace(a.Text) == "" {
		return errors.New("转写标识、片段、标注员和文本不能为空")
	}
	if a.EvidenceStartMS < 0 || a.EvidenceEndMS <= a.EvidenceStartMS || a.EvidenceEndMS > seg.DurationMS {
		return errors.New("证据时间范围超出片段时长")
	}
	if a.NotationScheme == "" {
		return errors.New("必须提供记法方案")
	}
	if a.Revision < 1 {
		return errors.New("revision 必须为正数")
	}
	return nil
}

func (b CorpusBatch) Editable() bool { return b.Status != StatusFrozen && b.Status != StatusReleased }

func (b *CorpusBatch) Revise(dialect, location, collector, policy string, expected int64, now time.Time) error {
	if !b.Editable() {
		return errors.New("已冻结或已发布批次禁止修订")
	}
	if expected <= 0 || b.Version != expected {
		return fmt.Errorf("版本冲突，当前状态为 %s，当前版本为 %d", b.Status, b.Version)
	}
	if strings.TrimSpace(dialect) == "" || strings.TrimSpace(location) == "" || strings.TrimSpace(collector) == "" {
		return errors.New("方言名称、地点代码和采集员编号不能为空")
	}
	b.DialectName, b.LocationCode, b.CollectorID, b.ConsentPolicy = dialect, location, collector, policy
	b.Version++
	b.UpdatedAt = now
	return nil
}

func Digest(v any) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%v", v)))
	return hex.EncodeToString(h[:])
}
