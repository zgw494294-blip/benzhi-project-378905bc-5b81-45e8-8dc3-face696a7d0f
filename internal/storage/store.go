package storage

import (
	"bufio"
	"crypto/sha256"
	"dialectarchive/internal/domain"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	mu                sync.Mutex
	dir               string
	seq               int64
	prevHash          string
	events            []domain.Event
	batches           map[string]domain.CorpusBatch
	segments          map[string]domain.RecordingSegment
	annotations       map[string]domain.TranscriptAnnotation
	annotationHistory map[string][]domain.TranscriptAnnotation
	reviews           map[string]domain.ReviewDecision
	credentials       map[string]domain.CitationCredential
	manifests         map[string]domain.ReleaseManifest
	qualityChecks     map[string]QualityRecord
	idempotency       map[string]any
}

type QualityRecord struct {
	BatchID   string    `json:"batch_id"`
	Version   int64     `json:"version"`
	Status    string    `json:"status"`
	Issues    []any     `json:"issues"`
	CheckedAt time.Time `json:"checked_at"`
}

func New(dir string) (*Store, error) {
	if dir == "" {
		dir = ".dialectarchive"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, batches: map[string]domain.CorpusBatch{}, segments: map[string]domain.RecordingSegment{}, annotations: map[string]domain.TranscriptAnnotation{}, annotationHistory: map[string][]domain.TranscriptAnnotation{}, reviews: map[string]domain.ReviewDecision{}, credentials: map[string]domain.CitationCredential{}, manifests: map[string]domain.ReleaseManifest{}, qualityChecks: map[string]QualityRecord{}, idempotency: map[string]any{}}
	_ = s.load()
	return s, nil
}

func (s *Store) load() error {
	if data, e := os.ReadFile(filepath.Join(s.dir, "snapshot.json")); e == nil {
		var snap struct {
			SchemaVersion     int                                      `json:"schemaVersion"`
			Sequence          int64                                    `json:"sequence"`
			Batches           map[string]domain.CorpusBatch            `json:"batches"`
			Segments          map[string]domain.RecordingSegment       `json:"segments"`
			Annotations       map[string]domain.TranscriptAnnotation   `json:"annotations"`
			AnnotationHistory map[string][]domain.TranscriptAnnotation `json:"annotationHistory"`
			Reviews           map[string]domain.ReviewDecision         `json:"reviews"`
			Credentials       map[string]domain.CitationCredential     `json:"credentials"`
			Manifests         map[string]domain.ReleaseManifest        `json:"manifests"`
			QualityChecks     map[string]QualityRecord                 `json:"qualityChecks"`
		}
		if json.Unmarshal(data, &snap) == nil && snap.SchemaVersion == 1 {
			s.seq = snap.Sequence
			s.batches = snap.Batches
			s.segments = snap.Segments
			s.annotations = snap.Annotations
			if snap.AnnotationHistory != nil {
				s.annotationHistory = snap.AnnotationHistory
			}
			s.reviews = snap.Reviews
			s.credentials = snap.Credentials
			if snap.Manifests != nil {
				s.manifests = snap.Manifests
			}
			if snap.QualityChecks != nil {
				s.qualityChecks = snap.QualityChecks
			}
		}
	}
	f, e := os.Open(filepath.Join(s.dir, "events.jsonl"))
	if e != nil {
		if errors.Is(e, os.ErrNotExist) {
			return nil
		}
		return e
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ev domain.Event
		if json.Unmarshal(sc.Bytes(), &ev) == nil {
			s.events = append(s.events, ev)
			s.seq = ev.Sequence
			s.prevHash = ev.Hash
		}
	}
	return sc.Err()
}

func (s *Store) appendEvent(typ, id string, payload any) error {
	b, _ := json.Marshal(payload)
	raw := append([]byte(s.prevHash), b...)
	h := sha256.Sum256(raw)
	seq := s.seq + 1
	ev := domain.Event{Sequence: seq, SchemaVersion: 1, Type: typ, AggregateID: id, Payload: payload, PrevHash: s.prevHash, Hash: hex.EncodeToString(h[:]), At: time.Now().UTC()}
	f, e := os.OpenFile(filepath.Join(s.dir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if e != nil {
		return e
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if e = enc.Encode(ev); e != nil {
		return e
	}
	s.seq = seq
	s.prevHash = ev.Hash
	s.events = append(s.events, ev)
	return s.snapshot()
}
func (s *Store) snapshot() error {
	data, _ := json.Marshal(struct {
		SchemaVersion     int                                      `json:"schemaVersion"`
		Sequence          int64                                    `json:"sequence"`
		Batches           map[string]domain.CorpusBatch            `json:"batches"`
		Segments          map[string]domain.RecordingSegment       `json:"segments"`
		Annotations       map[string]domain.TranscriptAnnotation   `json:"annotations"`
		AnnotationHistory map[string][]domain.TranscriptAnnotation `json:"annotationHistory"`
		Reviews           map[string]domain.ReviewDecision         `json:"reviews"`
		Credentials       map[string]domain.CitationCredential     `json:"credentials"`
		Manifests         map[string]domain.ReleaseManifest        `json:"manifests"`
		QualityChecks     map[string]QualityRecord                 `json:"qualityChecks"`
	}{1, s.seq, s.batches, s.segments, s.annotations, s.annotationHistory, s.reviews, s.credentials, s.manifests, s.qualityChecks})
	tmp := filepath.Join(s.dir, "snapshot.tmp")
	if e := os.WriteFile(tmp, data, 0644); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(s.dir, "snapshot.json"))
}

func (s *Store) CheckIdempotency(key string) (any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.idempotency[key]
	return v, ok
}
func (s *Store) Remember(key string, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != "" {
		s.idempotency[key] = v
	}
}
func (s *Store) PutBatch(b domain.CorpusBatch, event string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.appendEvent(event, b.ID, b); e != nil {
		return e
	}
	s.batches[b.ID] = b
	return nil
}
func (s *Store) GetBatch(id string) (domain.CorpusBatch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.batches[id]
	return b, ok
}
func (s *Store) ListBatches() []domain.CorpusBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.CorpusBatch, 0, len(s.batches))
	for _, b := range s.batches {
		out = append(out, b)
	}
	return out
}
func (s *Store) PutSegment(v domain.RecordingSegment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.appendEvent(domain.EventSegmentRegistered, v.ID, v); e != nil {
		return e
	}
	s.segments[v.ID] = v
	return nil
}
func (s *Store) GetSegment(id string) (domain.RecordingSegment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.segments[id]
	return v, ok
}
func (s *Store) ListSegments(batch string) []domain.RecordingSegment {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.RecordingSegment{}
	for _, v := range s.segments {
		if batch == "" || v.BatchID == batch {
			out = append(out, v)
		}
	}
	return out
}
func (s *Store) PutAnnotation(v domain.TranscriptAnnotation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.appendEvent(domain.EventAnnotationSubmitted, v.ID, v); e != nil {
		return e
	}
	s.annotations[v.ID] = v
	s.annotationHistory[v.ID] = append(s.annotationHistory[v.ID], v)
	return nil
}
func (s *Store) ListAnnotations(segment string) []domain.TranscriptAnnotation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.TranscriptAnnotation{}
	if len(s.annotationHistory) > 0 {
		for _, history := range s.annotationHistory {
			for _, v := range history {
				if segment == "" || v.SegmentID == segment {
					out = append(out, v)
				}
			}
		}
	} else {
		for _, v := range s.annotations {
			if segment == "" || v.SegmentID == segment {
				out = append(out, v)
			}
		}
	}
	return out
}

func (s *Store) AnnotationHistory(id string) []domain.TranscriptAnnotation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]domain.TranscriptAnnotation(nil), s.annotationHistory[id]...)
	return out
}
func (s *Store) GetAnnotation(id string) (domain.TranscriptAnnotation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.annotations[id]
	return a, ok
}
func (s *Store) CurrentAnnotation(segment string) (domain.TranscriptAnnotation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out domain.TranscriptAnnotation
	ok := false
	for _, a := range s.annotations {
		if a.SegmentID == segment && (!ok || a.Revision > out.Revision) {
			out, ok = a, true
		}
	}
	return out, ok
}
func (s *Store) SetAnnotationState(id string, state domain.AnnotationState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.annotations[id]
	if !ok {
		return false
	}
	a.State = state
	s.annotations[id] = a
	h := s.annotationHistory[id]
	if len(h) > 0 {
		h[len(h)-1] = a
		s.annotationHistory[id] = h
	}
	return true
}
func (s *Store) PutReview(v domain.ReviewDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.appendEvent(domain.EventReviewRecorded, v.ID, v); e != nil {
		return e
	}
	s.reviews[v.ID] = v
	return nil
}
func (s *Store) ListReviews(batch string) []domain.ReviewDecision {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.ReviewDecision{}
	for _, v := range s.reviews {
		if batch == "" || v.BatchID == batch {
			out = append(out, v)
		}
	}
	return out
}
func (s *Store) UpdateReview(v domain.ReviewDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.appendEvent(domain.EventFindingUpdated, v.ID, v); e != nil {
		return e
	}
	s.reviews[v.ID] = v
	return nil
}
func (s *Store) PutCredential(v domain.CitationCredential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.appendEvent(domain.EventCredentialIssued, v.ID, v); e != nil {
		return e
	}
	s.credentials[v.ID] = v
	return nil
}
func (s *Store) GetCredential(id string) (domain.CitationCredential, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.credentials[id]
	return v, ok
}
func (s *Store) ListCredentials(batch string) []domain.CitationCredential {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.CitationCredential{}
	for _, v := range s.credentials {
		if batch == "" || v.BatchID == batch {
			out = append(out, v)
		}
	}
	return out
}

func (s *Store) PutManifest(m domain.ReleaseManifest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.appendEvent(domain.EventManifestFrozen, m.Batch.ID, m); e != nil {
		return e
	}
	s.manifests[m.SHA256] = m
	return nil
}
func (s *Store) GetManifest(digest string) (domain.ReleaseManifest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.manifests[digest]
	return m, ok
}
func (s *Store) ListManifests(batch string) []domain.ReleaseManifest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.ReleaseManifest{}
	for _, m := range s.manifests {
		if batch == "" || m.Batch.ID == batch {
			out = append(out, m)
		}
	}
	return out
}
func (s *Store) PutQualityRecord(r QualityRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.appendEvent(domain.EventQualityChecked, r.BatchID, r); e != nil {
		return e
	}
	s.qualityChecks[r.BatchID] = r
	return nil
}
func (s *Store) GetQualityRecord(batch string) (QualityRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.qualityChecks[batch]
	return r, ok
}

// EventCount is intentionally read-only and cheap; it supports audit views
// without exposing mutable event slices to callers.
func (s *Store) EventCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func (s *Store) LastEvent() (domain.Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) == 0 {
		return domain.Event{}, false
	}
	return s.events[len(s.events)-1], true
}

func (s *Store) EventsSince(sequence int64) []domain.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []domain.Event{}
	for _, event := range s.events {
		if event.Sequence > sequence {
			result = append(result, event)
		}
	}
	return result
}
