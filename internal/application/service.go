package application

import (
	"crypto/rand"
	"crypto/sha256"
	"dialectarchive/internal/domain"
	"dialectarchive/internal/quality"
	"dialectarchive/internal/storage"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Service struct{ Store *storage.Store }

func New(s *storage.Store) *Service { return &Service{Store: s} }
func now() time.Time                { return time.Now().UTC() }
func newID() string                 { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func idem(s *Service, k string) (any, bool) {
	if k == "" {
		return nil, false
	}
	return s.Store.CheckIdempotency(k)
}
func (s *Service) CreateBatch(d, l, c, p, k string) (domain.CorpusBatch, error) {
	if v, ok := idem(s, k); ok {
		return v.(domain.CorpusBatch), nil
	}
	b, e := domain.NewBatch(newID(), d, l, c, p, now())
	if e != nil {
		return b, e
	}
	e = s.Store.PutBatch(b, domain.EventBatchCreated)
	if e == nil {
		s.Store.Remember(k, b)
	}
	return b, e
}
func (s *Service) GetBatch(id string) (domain.CorpusBatch, error) {
	b, ok := s.Store.GetBatch(id)
	if !ok {
		return b, errors.New("批次不存在")
	}
	return b, nil
}
func (s *Service) ListBatches() []domain.CorpusBatch { return s.Store.ListBatches() }
func (s *Service) ReviseBatch(id, d, l, c, p string, expected int64, k string) (domain.CorpusBatch, error) {
	if v, ok := idem(s, k); ok {
		return v.(domain.CorpusBatch), nil
	}
	b, e := s.GetBatch(id)
	if e != nil {
		return b, e
	}
	if e = b.Revise(d, l, c, p, expected, now()); e != nil {
		return b, e
	}
	e = s.Store.PutBatch(b, domain.EventBatchRevised)
	if e == nil {
		s.Store.Remember(k, b)
	}
	return b, e
}

func (s *Service) ReviseBatchFields(id string, d, l, c, p *string, expected int64, k string) (domain.CorpusBatch, error) {
	b, err := s.GetBatch(id)
	if err != nil {
		return b, err
	}
	if d == nil {
		d = &b.DialectName
	}
	if l == nil {
		l = &b.LocationCode
	}
	if c == nil {
		c = &b.CollectorID
	}
	if p == nil {
		p = &b.ConsentPolicy
	}
	return s.ReviseBatch(id, *d, *l, *c, *p, expected, k)
}

type segmentResult struct {
	S domain.RecordingSegment
	Q quality.Result
}

func (s *Service) RegisterSegment(batchID, speaker string, duration int64, rate, ch int, digest string, consent domain.ConsentState, expected int64, key string) (domain.RecordingSegment, quality.Result, error) {
	return s.RegisterSegmentAt(batchID, speaker, time.Time{}, duration, rate, ch, digest, consent, expected, key)
}
func (s *Service) RegisterSegmentAt(batchID, speaker string, started time.Time, duration int64, rate, ch int, digest string, consent domain.ConsentState, expected int64, key string) (domain.RecordingSegment, quality.Result, error) {
	if v, ok := idem(s, key); ok {
		r := v.(segmentResult)
		return r.S, r.Q, nil
	}
	b, e := s.GetBatch(batchID)
	if e != nil {
		return domain.RecordingSegment{}, quality.Result{}, e
	}
	if expected <= 0 || b.Version != expected {
		return domain.RecordingSegment{}, quality.Result{}, fmt.Errorf("版本冲突，当前版本为 %d", b.Version)
	}
	if started.IsZero() {
		started = now()
	}
	seg := domain.RecordingSegment{ID: newID(), BatchID: batchID, SpeakerCode: speaker, StartedAt: started, DurationMS: duration, SampleRateHz: rate, ChannelCount: ch, ContentSHA256: digest, ConsentState: consent, QualityState: domain.QualityUnknown}
	if e = domain.ValidateSegment(seg); e != nil {
		return seg, quality.Result{}, e
	}
	for _, old := range s.Store.ListSegments(batchID) {
		if strings.EqualFold(old.ContentSHA256, digest) {
			q := quality.Result{Passed: false, Issues: []quality.Issue{{Code: "DUPLICATE_CONTENT", Message: "批次内存在相同内容摘要", Severity: "error", ObjectID: old.ID}}}
			return seg, q, fmt.Errorf("重复内容，已有片段 %s", old.ID)
		}
	}
	q := quality.CheckSegment(seg)
	tq := quality.CheckTimeline(append(s.Store.ListSegments(batchID), seg))
	q.Issues = append(q.Issues, tq.Issues...)
	q.Passed = q.Passed && tq.Passed
	if q.Passed {
		seg.QualityState = domain.QualityPassed
	} else {
		seg.QualityState = domain.QualityNeedsFix
	}
	if !q.Passed {
		for _, i := range q.Issues {
			if i.Severity == "error" {
				return seg, q, fmt.Errorf("片段质量检查未通过: %s", i.Code)
			}
		}
	}
	if e = s.Store.PutSegment(seg); e != nil {
		return seg, q, e
	}
	b.Version++
	b.UpdatedAt = now()
	if b.Status == domain.StatusDraft {
		b.Status = domain.StatusQualityChecked
	}
	_ = s.Store.PutBatch(b, domain.EventSegmentRegistered)
	r := segmentResult{seg, q}
	s.Store.Remember(key, r)
	return seg, q, nil
}

type annotationResult struct {
	A domain.TranscriptAnnotation
	Q quality.Result
}

func (s *Service) SubmitAnnotation(a domain.TranscriptAnnotation, expected int64, key string) (domain.TranscriptAnnotation, quality.Result, error) {
	if v, ok := idem(s, key); ok {
		r := v.(annotationResult)
		return r.A, r.Q, nil
	}
	seg, ok := s.Store.GetSegment(a.SegmentID)
	if !ok {
		return a, quality.Result{}, errors.New("片段不存在")
	}
	b, e := s.GetBatch(seg.BatchID)
	if e != nil {
		return a, quality.Result{}, e
	}
	if expected > 0 && b.Version != expected {
		return a, quality.Result{}, fmt.Errorf("版本冲突，当前版本为 %d", b.Version)
	}
	if hist := s.Store.AnnotationHistory(a.ID); len(hist) > 0 && a.Revision != hist[len(hist)-1].Revision+1 {
		return a, quality.Result{}, errors.New("revision 必须严格递增且引用上一版本")
	}
	if current, ok := s.Store.CurrentAnnotation(a.SegmentID); ok && current.ID != a.ID && a.Revision != current.Revision+1 {
		return a, quality.Result{}, errors.New("revision 必须严格递增且引用上一版本")
	}
	if a.PreviousRevision > 0 && a.PreviousRevision != a.Revision-1 {
		return a, quality.Result{}, errors.New("previous_revision 必须引用上一版本")
	}
	if e = domain.ValidateAnnotation(a, seg); e != nil {
		return a, quality.Result{}, e
	}
	q := quality.CheckAnnotation(a, seg)
	if !q.Passed {
		return a, q, errors.New("转写未通过质量检查")
	}
	a.State = domain.AnnotationSubmitted
	if e = s.Store.PutAnnotation(a); e != nil {
		return a, q, e
	}
	for _, finding := range s.Store.ListReviews(seg.BatchID) {
		if finding.Decision == domain.DecisionRequestChanges && finding.Status != "passed" && a.Revision > finding.TargetRevision && (finding.TargetAnnotationID == "" || finding.TargetAnnotationID == a.ID) {
			finding.Status = "pending_recheck"
			finding.ResolvedRevision = a.Revision
			_ = s.Store.UpdateReview(finding)
		}
	}
	b.Version++
	b.UpdatedAt = now()
	if b.Status == domain.StatusDraft || b.Status == domain.StatusQualityChecked {
		b.Status = domain.StatusInReview
	}
	_ = s.Store.PutBatch(b, domain.EventAnnotationRevised)
	r := annotationResult{a, q}
	s.Store.Remember(key, r)
	return a, q, nil
}

func (s *Service) RecordReview(r domain.ReviewDecision, expected int64, key string) (domain.ReviewDecision, error) {
	if v, ok := idem(s, key); ok {
		return v.(domain.ReviewDecision), nil
	}
	b, e := s.GetBatch(r.BatchID)
	if e != nil {
		return r, e
	}
	if expected > 0 && b.Version != expected {
		return r, fmt.Errorf("版本冲突，当前版本为 %d", b.Version)
	}
	if r.Decision != domain.DecisionApprove && r.Decision != domain.DecisionRequestChanges {
		return r, errors.New("无效的复核决定")
	}
	if r.Decision == domain.DecisionRequestChanges {
		if strings.TrimSpace(r.FindingCode) == "" || strings.TrimSpace(r.Comment) == "" || strings.TrimSpace(r.RequiredAction) == "" || r.TargetRevision < 1 {
			return r, errors.New("退回决定必须包含发现码、意见、整改要求和目标 revision")
		}
		if r.TargetAnnotationID != "" {
			ann, ok := s.Store.GetAnnotation(r.TargetAnnotationID)
			seg, segOK := s.Store.GetSegment(ann.SegmentID)
			if !ok || !segOK || seg.BatchID != r.BatchID {
				return r, errors.New("发现项目标注不存在或不属于该批次")
			}
			if ann.Revision != r.TargetRevision {
				return r, errors.New("发现项目标 revision 不存在")
			}
		}
	}
	r.ID = newID()
	r.ReviewedAt = now()
	if r.Status == "" {
		if r.Decision == domain.DecisionRequestChanges {
			r.Status = "open"
		} else {
			r.Status = "passed"
		}
	}
	if r.Decision == domain.DecisionApprove {
		for _, seg := range s.Store.ListSegments(r.BatchID) {
			if a, ok := s.Store.CurrentAnnotation(seg.ID); ok {
				s.Store.SetAnnotationState(a.ID, domain.AnnotationApproved)
			}
		}
		for _, finding := range s.Store.ListReviews(r.BatchID) {
			if finding.Decision == domain.DecisionRequestChanges && finding.Status == "pending_recheck" && (r.FindingCode == "" || finding.FindingCode == r.FindingCode) && r.TargetRevision >= finding.ResolvedRevision {
				finding.Status = "passed"
				_ = s.Store.UpdateReview(finding)
			}
		}
		allPassed := true
		hasFinding := false
		for _, finding := range s.Store.ListReviews(r.BatchID) {
			if finding.Decision == domain.DecisionRequestChanges {
				hasFinding = true
				if finding.Status != "passed" {
					allPassed = false
				}
			}
		}
		if hasFinding && allPassed && b.Status == domain.StatusRemediation {
			b.Status = domain.StatusInReview
		}
	}
	if e = s.Store.PutReview(r); e != nil {
		return r, e
	}
	b.Version++
	b.UpdatedAt = now()
	if r.Decision == domain.DecisionRequestChanges {
		b.Status = domain.StatusRemediation
	} else if b.Status == domain.StatusQualityChecked {
		b.Status = domain.StatusInReview
	}
	_ = s.Store.PutBatch(b, domain.EventFindingUpdated)
	s.Store.Remember(key, r)
	return r, nil
}

type QualityReport struct {
	BatchID      string          `json:"batch_id"`
	Version      int64           `json:"version"`
	Status       string          `json:"status"`
	ErrorCount   int             `json:"error_count"`
	WarningCount int             `json:"warning_count"`
	Issues       []quality.Issue `json:"issues"`
	CheckedAt    time.Time       `json:"checked_at"`
}

func (s *Service) RecheckBatch(id string, expected int64, key string) (QualityReport, error) {
	if v, ok := idem(s, key); ok {
		return v.(QualityReport), nil
	}
	b, e := s.GetBatch(id)
	if e != nil {
		return QualityReport{}, e
	}
	if expected <= 0 || b.Version != expected {
		return QualityReport{}, fmt.Errorf("版本冲突，当前版本为 %d", b.Version)
	}
	segs := s.Store.ListSegments(id)
	var issues []quality.Issue
	for _, seg := range segs {
		q := quality.CheckSegment(seg)
		for i := range q.Issues {
			q.Issues[i].ObjectID = seg.ID
		}
		issues = append(issues, q.Issues...)
		issues = append(issues, quality.CheckConsent(seg, b.ConsentPolicy).Issues...)
	}
	issues = append(issues, quality.CheckTimeline(segs).Issues...)
	for _, seg := range segs {
		a, ok := s.Store.CurrentAnnotation(seg.ID)
		if !ok {
			issues = append(issues, quality.Issue{Code: "MISSING_TRANSCRIPT", Message: "片段缺少当前转写", Severity: "error", ObjectID: seg.ID})
		} else {
			q := quality.CheckAnnotation(a, seg)
			for i := range q.Issues {
				q.Issues[i].ObjectID = a.ID
			}
			issues = append(issues, q.Issues...)
		}
	}
	r := QualityReport{BatchID: id, Version: b.Version, Status: "passed", Issues: issues, CheckedAt: now()}
	for _, i := range issues {
		if i.Severity == "error" {
			r.Status = "blocked"
			r.ErrorCount++
		} else {
			r.WarningCount++
		}
	}
	if r.Status == "passed" && r.WarningCount > 0 {
		r.Status = "needs_fix"
	}
	if r.Status == "blocked" {
		b.Status = domain.StatusRemediation
	} else {
		b.Status = domain.StatusQualityChecked
	}
	b.Version++
	b.UpdatedAt = now()
	_ = s.Store.PutBatch(b, domain.EventQualityChecked)
	r.Version = b.Version
	issueValues := make([]any, len(r.Issues))
	for i := range r.Issues {
		issueValues[i] = r.Issues[i]
	}
	e = s.Store.PutQualityRecord(storage.QualityRecord{BatchID: id, Version: r.Version, Status: r.Status, Issues: issueValues, CheckedAt: r.CheckedAt})
	if e == nil {
		s.Store.Remember(key, r)
	}
	return r, e
}

func (s *Service) Preflight(id string, expected int64) (map[string]any, error) {
	b, e := s.GetBatch(id)
	if e != nil {
		return nil, e
	}
	if expected <= 0 || b.Version != expected {
		return nil, fmt.Errorf("版本冲突，当前版本为 %d", b.Version)
	}
	var blockers []quality.Issue
	segs := s.Store.ListSegments(id)
	if len(segs) == 0 {
		blockers = append(blockers, quality.Issue{Code: "NO_SEGMENTS", Message: "批次至少需要一个片段", Severity: "error"})
	}
	for _, seg := range segs {
		if seg.QualityState != domain.QualityPassed {
			blockers = append(blockers, quality.Issue{Code: "SEGMENT_NOT_PASSED", Message: "片段质量未通过", Severity: "error", ObjectID: seg.ID})
		}
		if q := quality.CheckConsent(seg, b.ConsentPolicy); !q.Passed {
			blockers = append(blockers, q.Issues...)
		}
		a, ok := s.Store.CurrentAnnotation(seg.ID)
		if !ok {
			blockers = append(blockers, quality.Issue{Code: "MISSING_TRANSCRIPT", Message: "缺少当前转写", Severity: "error", ObjectID: seg.ID})
		} else if a.State != domain.AnnotationApproved {
			blockers = append(blockers, quality.Issue{Code: "ANNOTATION_NOT_APPROVED", Message: "当前转写尚未获专家批准", Severity: "error", ObjectID: a.ID})
		}
	}
	for _, r := range s.Store.ListReviews(id) {
		if r.Decision == domain.DecisionRequestChanges && r.Status != "passed" {
			blockers = append(blockers, quality.Issue{Code: "OPEN_FINDING", Message: "存在未解决复核发现项", Severity: "error", ObjectID: r.ID})
		}
	}
	return map[string]any{"ready": len(blockers) == 0, "blockers": blockers, "warnings": []quality.Issue{}}, nil
}

type freezeResult struct {
	C domain.CitationCredential
	M domain.ReleaseManifest
}

func (s *Service) Freeze(id string, expected int64, issuer, subject, key string) (domain.CitationCredential, domain.ReleaseManifest, error) {
	return s.FreezeWithExpiry(id, expected, issuer, subject, nil, key)
}

func (s *Service) FreezeWithExpiry(id string, expected int64, issuer, subject string, expires *time.Time, key string) (domain.CitationCredential, domain.ReleaseManifest, error) {
	if v, ok := idem(s, key); ok {
		r := v.(freezeResult)
		return r.C, r.M, nil
	}
	b, e := s.GetBatch(id)
	if e != nil {
		return domain.CitationCredential{}, domain.ReleaseManifest{}, e
	}
	if expected <= 0 || b.Version != expected {
		return domain.CitationCredential{}, domain.ReleaseManifest{}, fmt.Errorf("版本冲突，当前版本为 %d", b.Version)
	}
	if b.Status != domain.StatusInReview {
		return domain.CitationCredential{}, domain.ReleaseManifest{}, errors.New("批次必须处于 in_review 才能冻结")
	}
	if expires != nil && !expires.After(now()) {
		return domain.CitationCredential{}, domain.ReleaseManifest{}, errors.New("expires_at 必须晚于 issued_at")
	}
	pre, e := s.Preflight(id, expected)
	if e != nil {
		return domain.CitationCredential{}, domain.ReleaseManifest{}, e
	}
	if !pre["ready"].(bool) {
		return domain.CitationCredential{}, domain.ReleaseManifest{}, errors.New("批次未达到冻结就绪条件")
	}
	if b.Status == domain.StatusInReview {
		_ = b.Transition(domain.StatusFrozen, now())
	}
	segs := s.Store.ListSegments(id)
	sort.Slice(segs, func(i, j int) bool { return segs[i].ID < segs[j].ID })
	anns := []domain.TranscriptAnnotation{}
	for _, seg := range segs {
		a, ok := s.Store.CurrentAnnotation(seg.ID)
		if ok {
			anns = append(anns, a)
		}
	}
	sort.Slice(anns, func(i, j int) bool {
		if anns[i].SegmentID == anns[j].SegmentID {
			return anns[i].Revision < anns[j].Revision
		}
		return anns[i].SegmentID < anns[j].SegmentID
	})
	m := domain.ReleaseManifest{Batch: b, Segments: segs, Annotations: anns}
	raw, _ := json.Marshal(m)
	h := sha256.Sum256(raw)
	m.SHA256 = hex.EncodeToString(h[:])
	if e = s.Store.PutManifest(m); e != nil {
		return domain.CitationCredential{}, m, e
	}
	issued := now()
	sh := sha256.Sum256([]byte(m.SHA256 + issuer))
	c := domain.CitationCredential{ID: newID(), BatchID: id, ManifestSHA256: m.SHA256, SubjectLabel: subject, IssuedBy: issuer, IssuedAt: issued, Signature: hex.EncodeToString(sh[:]), VerificationState: "valid", FrozenBatchVersion: b.Version}
	c.ExpiresAt = expires
	b.Status = domain.StatusReleased
	b.Version++
	_ = s.Store.PutBatch(b, domain.EventBatchFrozen)
	e = s.Store.PutCredential(c)
	if e == nil {
		s.Store.Remember(key, freezeResult{c, m})
	}
	return c, m, e
}
func (s *Service) GetManifest(d string) (domain.ReleaseManifest, error) {
	if len(d) != 64 {
		return domain.ReleaseManifest{}, errors.New("manifest_sha256 格式错误")
	}
	m, ok := s.Store.GetManifest(d)
	if !ok {
		return m, errors.New("冻结清单不存在")
	}
	return m, nil
}
func (s *Service) Verify(id string) (domain.CitationCredential, bool, error) {
	c, ok := s.Store.GetCredential(id)
	if !ok {
		return c, false, errors.New("凭据不存在")
	}
	m, ok := s.Store.GetManifest(c.ManifestSHA256)
	if !ok {
		return c, false, errors.New("冻结清单不存在")
	}
	expectedDigest := m.SHA256
	m.SHA256 = ""
	raw, _ := json.Marshal(m)
	h := sha256.Sum256(raw)
	b, e := s.GetBatch(c.BatchID)
	if e != nil {
		return c, false, e
	}
	reason := "VALID"
	valid := true
	if c.ExpiresAt != nil && now().After(*c.ExpiresAt) {
		valid = false
		reason = "EXPIRED"
	} else if hex.EncodeToString(h[:]) != expectedDigest || expectedDigest != c.ManifestSHA256 {
		valid = false
		reason = "DIGEST_MISMATCH"
	} else {
		sh := sha256.Sum256([]byte(c.ManifestSHA256 + c.IssuedBy))
		if hex.EncodeToString(sh[:]) != c.Signature {
			valid = false
			reason = "SIGNATURE_MISMATCH"
		} else if b.Status != domain.StatusReleased {
			valid = false
			reason = "BATCH_NOT_RELEASED"
		}
	}
	c.VerificationState = reason
	return c, valid, nil
}
