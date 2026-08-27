package application

import (
	"crypto/sha256"
	"dialectarchive/internal/domain"
	"dialectarchive/internal/quality"
	"dialectarchive/internal/storage"
	"encoding/hex"
	"errors"
	"sort"
	"time"
)

// SegmentAudit is the read model used by the browser's batch detail view.
// It intentionally contains only metadata and summaries; audio bytes never
// enter the application or persistence layers.
type SegmentAudit struct {
	Segment         domain.RecordingSegment `json:"segment"`
	SpeakerLabel    string                  `json:"speaker_label"`
	ConsentLabel    string                  `json:"consent_label"`
	QualityLabel    string                  `json:"quality_label"`
	CurrentRevision int                     `json:"current_revision"`
	RevisionCount   int                     `json:"revision_count"`
	AnnotationState domain.AnnotationState  `json:"annotation_state,omitempty"`
	Issues          []quality.Issue         `json:"issues"`
}

type BatchAudit struct {
	Batch           domain.CorpusBatch      `json:"batch"`
	StatusLabel     string                  `json:"status_label"`
	SegmentCount    int                     `json:"segment_count"`
	AnnotationCount int                     `json:"annotation_count"`
	RevisionCount   int                     `json:"revision_count"`
	EventCount      int                     `json:"event_count"`
	Segments        []SegmentAudit          `json:"segments"`
	Reviews         domain.ReviewSummary    `json:"reviews"`
	Quality         *storage.QualityRecord  `json:"quality,omitempty"`
	QualitySummary  quality.SeveritySummary `json:"quality_summary"`
	Issues          []quality.Issue         `json:"issues"`
	ReadyForFreeze  bool                    `json:"ready_for_freeze"`
	CredentialCount int                     `json:"credential_count"`
	GeneratedAt     time.Time               `json:"generated_at"`
}

// AuditBatch builds a deterministic, read-only projection from the current
// store. It is safe to call repeatedly while users are editing a batch.
func (s *Service) AuditBatch(id string) (BatchAudit, error) {
	batch, err := s.GetBatch(id)
	if err != nil {
		return BatchAudit{}, err
	}
	segments := s.Store.ListSegments(id)
	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].StartedAt.Equal(segments[j].StartedAt) {
			return segments[i].ID < segments[j].ID
		}
		return segments[i].StartedAt.Before(segments[j].StartedAt)
	})
	current := make(map[string]domain.TranscriptAnnotation, len(segments))
	histories := make(map[string][]domain.TranscriptAnnotation, len(segments))
	allAnnotations := s.Store.ListAnnotations("")
	for _, annotation := range allAnnotations {
		segment, ok := s.Store.GetSegment(annotation.SegmentID)
		if !ok || segment.BatchID != id {
			continue
		}
		histories[annotation.SegmentID] = append(histories[annotation.SegmentID], annotation)
		if old, exists := current[annotation.SegmentID]; !exists || annotation.Revision > old.Revision {
			current[annotation.SegmentID] = annotation
		}
	}
	input := quality.BatchCheckInput{Batch: batch, Segments: segments, Annotations: current}
	qualityReport := quality.EvaluateBatch(input)
	audit := BatchAudit{
		Batch:           batch,
		StatusLabel:     domain.StatusLabel(batch.Status),
		SegmentCount:    len(segments),
		AnnotationCount: len(current),
		EventCount:      s.Store.EventCount(),
		Reviews:         domain.SummarizeReviews(s.Store.ListReviews(id)),
		Issues:          qualityReport.Issues,
		QualitySummary:  qualityReport.Severity,
		ReadyForFreeze:  qualityReport.Status == "passed" && batch.Status == domain.StatusInReview,
		CredentialCount: len(s.Store.ListCredentials(id)),
		GeneratedAt:     time.Now().UTC(),
	}
	if record, ok := s.Store.GetQualityRecord(id); ok {
		audit.Quality = &record
	}
	for _, segment := range segments {
		history := histories[segment.ID]
		summary := domain.BuildRevisionSummary(history)
		segmentAudit := SegmentAudit{
			Segment:         segment,
			SpeakerLabel:    segment.SpeakerCode,
			ConsentLabel:    domain.ConsentLabel(segment.ConsentState),
			QualityLabel:    domain.QualityLabel(segment.QualityState),
			CurrentRevision: summary.Current,
			RevisionCount:   len(summary.Revisions),
			Issues:          issuesForObject(qualityReport.Issues, segment.ID),
		}
		if annotation, ok := current[segment.ID]; ok {
			segmentAudit.AnnotationState = annotation.State
			segmentAudit.Issues = append(segmentAudit.Issues, issuesForObject(qualityReport.Issues, annotation.ID)...)
		}
		audit.RevisionCount += segmentAudit.RevisionCount
		audit.Segments = append(audit.Segments, segmentAudit)
	}
	audit.Issues = quality.SortedIssues(audit.Issues)
	return audit, nil
}

func issuesForObject(issues []quality.Issue, objectID string) []quality.Issue {
	result := []quality.Issue{}
	for _, issue := range issues {
		if issue.ObjectID == objectID {
			result = append(result, issue)
		}
	}
	return result
}

// ManifestAudit describes a frozen manifest without exposing credential
// signatures. The digest is recalculated from the persisted immutable copy.
type ManifestAudit struct {
	ManifestSHA256  string `json:"manifest_sha256"`
	BatchID         string `json:"batch_id"`
	BatchVersion    int64  `json:"batch_version"`
	SegmentCount    int    `json:"segment_count"`
	AnnotationCount int    `json:"annotation_count"`
	DigestMatches   bool   `json:"digest_matches"`
	Status          string `json:"status"`
}

func (s *Service) AuditManifest(digest string) (ManifestAudit, error) {
	manifest, err := s.GetManifest(digest)
	if err != nil {
		return ManifestAudit{}, err
	}
	calculated, err := domain.ManifestDigest(manifest)
	if err != nil {
		return ManifestAudit{}, err
	}
	status := "match"
	if calculated != manifest.SHA256 {
		status = "mismatch"
	}
	return ManifestAudit{
		ManifestSHA256:  manifest.SHA256,
		BatchID:         manifest.Batch.ID,
		BatchVersion:    manifest.Batch.Version,
		SegmentCount:    len(manifest.Segments),
		AnnotationCount: len(manifest.Annotations),
		DigestMatches:   calculated == manifest.SHA256,
		Status:          status,
	}, nil
}

// CredentialDiagnostics gives the verification page stable, explainable
// checks while Verify remains the compact compatibility API.
type CredentialDiagnostics struct {
	CredentialID   string    `json:"credential_id"`
	ManifestSHA256 string    `json:"manifest_sha256"`
	Exists         bool      `json:"exists"`
	DigestMatches  bool      `json:"digest_matches"`
	SignatureValid bool      `json:"signature_valid"`
	BatchReleased  bool      `json:"batch_released"`
	Expired        bool      `json:"expired"`
	ReasonCode     string    `json:"reason_code"`
	Message        string    `json:"message"`
	CheckedAt      time.Time `json:"checked_at"`
}

func (s *Service) DiagnoseCredential(id string) (CredentialDiagnostics, error) {
	credential, ok := s.Store.GetCredential(id)
	if !ok {
		return CredentialDiagnostics{CredentialID: id, Exists: false, ReasonCode: "NOT_FOUND", Message: "凭据不存在", CheckedAt: time.Now().UTC()}, errors.New("凭据不存在")
	}
	diagnostic := CredentialDiagnostics{CredentialID: id, ManifestSHA256: credential.ManifestSHA256, Exists: true, CheckedAt: time.Now().UTC()}
	manifest, manifestOK := s.Store.GetManifest(credential.ManifestSHA256)
	if manifestOK {
		calculated, digestErr := domain.ManifestDigest(manifest)
		diagnostic.DigestMatches = digestErr == nil && calculated == credential.ManifestSHA256
	}
	diagnostic.Expired = domain.CredentialIsExpired(credential, diagnostic.CheckedAt)
	batch, batchOK := s.Store.GetBatch(credential.BatchID)
	diagnostic.BatchReleased = batchOK && batch.Status == domain.StatusReleased
	signatureHash := sha256.Sum256([]byte(credential.ManifestSHA256 + credential.IssuedBy))
	diagnostic.SignatureValid = hex.EncodeToString(signatureHash[:]) == credential.Signature
	switch {
	case !manifestOK:
		diagnostic.ReasonCode, diagnostic.Message = "MANIFEST_NOT_FOUND", "冻结清单不存在"
	case diagnostic.Expired:
		diagnostic.ReasonCode, diagnostic.Message = "EXPIRED", "凭据已过期"
	case !diagnostic.DigestMatches:
		diagnostic.ReasonCode, diagnostic.Message = "DIGEST_MISMATCH", "清单摘要不匹配"
	case !diagnostic.SignatureValid:
		diagnostic.ReasonCode, diagnostic.Message = "SIGNATURE_MISMATCH", "凭据签名不匹配"
	case !diagnostic.BatchReleased:
		diagnostic.ReasonCode, diagnostic.Message = "BATCH_NOT_RELEASED", "批次尚未发布"
	default:
		diagnostic.ReasonCode, diagnostic.Message = "VALID", "凭据有效"
	}
	return diagnostic, nil
}
