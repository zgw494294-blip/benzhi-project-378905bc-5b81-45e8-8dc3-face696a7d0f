package web

import (
	"dialectarchive/internal/application"
	"dialectarchive/internal/domain"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	App *application.Service
	Mux *http.ServeMux
}

func New(app *application.Service) *Server {
	s := &Server{App: app, Mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) routes() {
	s.Mux.HandleFunc("/", s.handleIndex)
	s.Mux.HandleFunc("/api/batches", s.handleBatches)
	s.Mux.HandleFunc("/api/batches/", s.handleBatchAction)
	s.Mux.HandleFunc("/api/segments", s.handleSegments)
	s.Mux.HandleFunc("/api/annotations", s.handleAnnotations)
	s.Mux.HandleFunc("/api/reviews", s.handleReviews)
	s.Mux.HandleFunc("/api/releases", s.handleReleases)
	s.Mux.HandleFunc("/api/credentials/verify", s.handleVerify)
	s.Mux.HandleFunc("/api/manifests", s.handleManifest)
	s.Mux.HandleFunc("/api/quality-checks", s.handleQualityChecks)
	s.Mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
}
func (s *Server) handleQualityChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		BatchID         string `json:"batch_id"`
		ExpectedVersion int64  `json:"expectedVersion"`
		IdempotencyKey  string `json:"idempotencyKey"`
	}
	if e := decode(r, &in); e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 400)
		return
	}
	q, e := s.App.RecheckBatch(in.BatchID, in.ExpectedVersion, in.IdempotencyKey)
	if e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 409)
		return
	}
	writeJSON(w, q, 200)
}
func writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decode(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "web/index.html")
}
func (s *Server) handleBatches(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, s.App.ListBatches(), 200)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		DialectName    string `json:"dialect_name"`
		LocationCode   string `json:"location_code"`
		CollectorID    string `json:"collector_id"`
		ConsentPolicy  string `json:"consent_policy"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if e := decode(r, &in); e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 400)
		return
	}
	b, e := s.App.CreateBatch(in.DialectName, in.LocationCode, in.CollectorID, in.ConsentPolicy, in.IdempotencyKey)
	if e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 400)
		return
	}
	writeJSON(w, b, 201)
}
func (s *Server) handleBatchAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/batches/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodPatch {
		var in struct {
			DialectName     *string `json:"dialect_name"`
			LocationCode    *string `json:"location_code"`
			CollectorID     *string `json:"collector_id"`
			ConsentPolicy   *string `json:"consent_policy"`
			ExpectedVersion int64   `json:"expectedVersion"`
			IdempotencyKey  string  `json:"idempotencyKey"`
		}
		if e := decode(r, &in); e != nil {
			writeJSON(w, map[string]string{"error": e.Error()}, 400)
			return
		}
		b, e := s.App.ReviseBatchFields(id, in.DialectName, in.LocationCode, in.CollectorID, in.ConsentPolicy, in.ExpectedVersion, in.IdempotencyKey)
		if e != nil {
			current, _ := s.App.GetBatch(id)
			writeJSON(w, map[string]any{"error": e.Error(), "status": current.Status, "version": current.Version}, 409)
			return
		}
		writeJSON(w, b, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "quality-check" && r.Method == http.MethodGet {
		q, ok := s.App.Store.GetQualityRecord(id)
		if !ok {
			writeJSON(w, map[string]string{"error": "质量检查记录不存在"}, 404)
		} else {
			writeJSON(w, q, 200)
		}
		return
	}
	if len(parts) == 2 && parts[1] == "quality-check" && r.Method == http.MethodPost {
		var in struct {
			ExpectedVersion int64  `json:"expectedVersion"`
			IdempotencyKey  string `json:"idempotencyKey"`
		}
		_ = decode(r, &in)
		q, e := s.App.RecheckBatch(id, in.ExpectedVersion, in.IdempotencyKey)
		if e != nil {
			writeJSON(w, map[string]string{"error": e.Error()}, 409)
			return
		}
		writeJSON(w, q, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "preflight" && r.Method == http.MethodPost {
		var in struct {
			ExpectedVersion int64 `json:"expectedVersion"`
		}
		_ = decode(r, &in)
		v, e := s.App.Preflight(id, in.ExpectedVersion)
		if e != nil {
			writeJSON(w, map[string]string{"error": e.Error()}, 409)
			return
		}
		writeJSON(w, v, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "audit" && r.Method == http.MethodGet {
		audit, e := s.App.AuditBatch(id)
		if e != nil {
			writeJSON(w, map[string]string{"error": e.Error()}, 404)
			return
		}
		writeJSON(w, audit, 200)
		return
	}
	http.NotFound(w, r)
}
func (s *Server) handleSegments(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, s.App.Store.ListSegments(r.URL.Query().Get("batch_id")), 200)
		return
	}
	var in struct {
		BatchID         string              `json:"batch_id"`
		SpeakerCode     string              `json:"speaker_code"`
		DurationMS      int64               `json:"duration_ms"`
		SampleRateHz    int                 `json:"sample_rate_hz"`
		ChannelCount    int                 `json:"channel_count"`
		ContentSHA256   string              `json:"content_sha256"`
		ConsentState    domain.ConsentState `json:"consent_state"`
		ExpectedVersion int64               `json:"expectedVersion"`
		IdempotencyKey  string              `json:"idempotencyKey"`
		StartedAt       time.Time           `json:"started_at"`
	}
	if e := decode(r, &in); e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 400)
		return
	}
	v, q, e := s.App.RegisterSegmentAt(in.BatchID, in.SpeakerCode, in.StartedAt, in.DurationMS, in.SampleRateHz, in.ChannelCount, in.ContentSHA256, in.ConsentState, in.ExpectedVersion, in.IdempotencyKey)
	if e != nil {
		resp := map[string]any{"error": e.Error(), "quality": q}
		for _, issue := range q.Issues {
			if issue.Code == "DUPLICATE_CONTENT" {
				resp["existing_segment_id"] = issue.ObjectID
			}
		}
		writeJSON(w, resp, 400)
		return
	}
	writeJSON(w, map[string]any{"segment": v, "quality": q}, 201)
}
func (s *Server) handleAnnotations(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		segmentID := r.URL.Query().Get("segment_id")
		items := s.App.Store.ListAnnotations(segmentID)
		if segmentID != "" {
			current := 0
			for _, a := range items {
				if a.Revision > current {
					current = a.Revision
				}
			}
			writeJSON(w, map[string]any{"annotations": items, "current_revision": current}, 200)
		} else {
			writeJSON(w, items, 200)
		}
		return
	}
	var in struct {
		domain.TranscriptAnnotation
		ExpectedVersion int64  `json:"expectedVersion"`
		IdempotencyKey  string `json:"idempotencyKey"`
	}
	if e := decode(r, &in); e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 400)
		return
	}
	expected, _ := strconv.ParseInt(r.Header.Get("X-Expected-Version"), 10, 64)
	if expected == 0 {
		expected = in.ExpectedVersion
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		key = in.IdempotencyKey
	}
	v, q, e := s.App.SubmitAnnotation(in.TranscriptAnnotation, expected, key)
	if e != nil {
		writeJSON(w, map[string]any{"error": e.Error(), "quality": q}, 400)
		return
	}
	writeJSON(w, map[string]any{"annotation": v, "quality": q}, 201)
}
func (s *Server) handleReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, s.App.Store.ListReviews(r.URL.Query().Get("batch_id")), 200)
		return
	}
	var in struct {
		domain.ReviewDecision
		ExpectedVersion int64  `json:"expectedVersion"`
		IdempotencyKey  string `json:"idempotencyKey"`
	}
	if e := decode(r, &in); e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 400)
		return
	}
	expected, _ := strconv.ParseInt(r.Header.Get("X-Expected-Version"), 10, 64)
	if expected == 0 {
		expected = in.ExpectedVersion
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		key = in.IdempotencyKey
	}
	v, e := s.App.RecordReview(in.ReviewDecision, expected, key)
	if e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 400)
		return
	}
	writeJSON(w, v, 201)
}
func (s *Server) handleReleases(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if d := r.URL.Query().Get("manifest_sha256"); d != "" {
			m, e := s.App.GetManifest(d)
			if e != nil {
				writeJSON(w, map[string]string{"error": e.Error()}, 404)
			} else {
				writeJSON(w, m, 200)
			}
			return
		}
		writeJSON(w, s.App.Store.ListCredentials(r.URL.Query().Get("batch_id")), 200)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		BatchID         string     `json:"batch_id"`
		Issuer          string     `json:"issuer"`
		Subject         string     `json:"subject_label"`
		ExpectedVersion int64      `json:"expectedVersion"`
		IdempotencyKey  string     `json:"idempotencyKey"`
		ExpiresAt       *time.Time `json:"expires_at"`
	}
	if e := decode(r, &in); e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 400)
		return
	}
	c, m, e := s.App.FreezeWithExpiry(in.BatchID, in.ExpectedVersion, in.Issuer, in.Subject, in.ExpiresAt, in.IdempotencyKey)
	if e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 400)
		return
	}
	writeJSON(w, map[string]any{"credential": c, "manifest": m}, 201)
}
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	d := r.URL.Query().Get("manifest_sha256")
	if d == "" {
		d = r.URL.Query().Get("sha256")
	}
	if d == "" {
		ms := s.App.Store.ListManifests(r.URL.Query().Get("batch_id"))
		if len(ms) == 0 {
			writeJSON(w, map[string]string{"error": "冻结清单不存在"}, 404)
			return
		}
		writeJSON(w, ms[len(ms)-1], 200)
		return
	}
	m, e := s.App.GetManifest(d)
	if e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 404)
		return
	}
	audit, auditErr := s.App.AuditManifest(d)
	if auditErr != nil {
		writeJSON(w, map[string]string{"error": auditErr.Error()}, 500)
		return
	}
	writeJSON(w, map[string]any{"manifest": m, "batch_version": m.Batch.Version, "segment_count": len(m.Segments), "annotation_count": len(m.Annotations), "audit": audit}, 200)
}
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		id = r.URL.Query().Get("credential_id")
	}
	if id == "" {
		manifest := r.URL.Query().Get("manifest_sha256")
		for _, c := range s.App.Store.ListCredentials("") {
			if c.ManifestSHA256 == manifest {
				id = c.ID
				break
			}
		}
	}
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/api/credentials/verify/")
	}
	c, ok, e := s.App.Verify(id)
	if e != nil {
		writeJSON(w, map[string]string{"error": e.Error()}, 404)
		return
	}
	response := map[string]any{"credential": c, "valid": ok}
	if diagnostic, diagnosticErr := s.App.DiagnoseCredential(id); diagnosticErr == nil {
		response["diagnostics"] = diagnostic
	}
	writeJSON(w, response, 200)
}
