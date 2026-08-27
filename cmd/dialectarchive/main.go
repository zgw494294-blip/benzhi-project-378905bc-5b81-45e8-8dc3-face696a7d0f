package main

import (
	"bytes"
	"dialectarchive/internal/application"
	"dialectarchive/internal/domain"
	"dialectarchive/internal/storage"
	"dialectarchive/internal/web"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19081", "监听地址")
	self := flag.Bool("selfcheck", false, "运行自检")
	flag.Parse()
	if env := os.Getenv("PORT"); env != "" && flag.Lookup("addr").Value.String() == "127.0.0.1:19081" {
		*addr = "127.0.0.1:" + env
	}
	if !strings.HasPrefix(*addr, "127.0.0.1:") {
		fmt.Fprintln(os.Stderr, "监听地址必须是回环地址")
		os.Exit(2)
	}
	dir := os.Getenv("DIALECTARCHIVE_DATA")
	st, e := storage.New(dir)
	if e != nil {
		panic(e)
	}
	srv := web.New(application.New(st))
	if *self {
		if e := runSelfcheck(srv); e != nil {
			panic(e)
		}
		return
	}
	httpSrv := &http.Server{Addr: *addr, Handler: srv.Mux}
	go func() {
		if e := httpSrv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			panic(e)
		}
	}()
	fmt.Println("方言语料工作台已启动: " + *addr)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	httpSrv.Close()
}

func runSelfcheck(s *web.Server) error {
	ts := httptest.NewServer(s.Mux)
	defer ts.Close()
	post := func(path string, v any, out any) error {
		b, _ := json.Marshal(v)
		r, e := http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
		if e != nil {
			return e
		}
		defer r.Body.Close()
		if r.StatusCode >= 300 {
			x, _ := io.ReadAll(r.Body)
			return fmt.Errorf("%s: %s", path, string(x))
		}
		return json.NewDecoder(r.Body).Decode(out)
	}
	var b domain.CorpusBatch
	if e := post("/api/batches", map[string]any{"dialect_name": "吴语", "location_code": "CN-ZJ", "collector_id": "collector-1", "consent_policy": "研究用途", "idempotencyKey": "self-batch"}, &b); e != nil {
		return e
	}
	var seg struct{ Segment domain.RecordingSegment }
	digest := strings.Repeat("a", 64)
	if e := post("/api/segments", map[string]any{"batch_id": b.ID, "speaker_code": "S01", "duration_ms": 1200, "sample_rate_hz": 16000, "channel_count": 1, "content_sha256": digest, "consent_state": "granted", "expectedVersion": b.Version, "idempotencyKey": "self-seg"}, &seg); e != nil {
		return e
	}
	var ann struct{ Annotation domain.TranscriptAnnotation }
	if e := post("/api/annotations", map[string]any{"id": "ann-" + b.ID, "segment_id": seg.Segment.ID, "annotator_id": "a1", "text": "侬好", "variant_form": "侬", "evidence_start_ms": 0, "evidence_end_ms": 800, "notation_scheme": "漢字", "revision": 1}, &ann); e != nil {
		return e
	}
	b, _ = s.App.GetBatch(b.ID)
	var rev domain.ReviewDecision
	if e := post("/api/reviews", map[string]any{"batch_id": b.ID, "reviewer_id": "expert-1", "finding_code": "OK", "comment": "通过", "required_action": "", "decision": "approve", "target_revision": 1}, &rev); e != nil {
		return e
	}
	b, _ = s.App.GetBatch(b.ID)
	var rel struct{ Credential domain.CitationCredential }
	if e := post("/api/releases", map[string]any{"batch_id": b.ID, "issuer": "admin", "subject_label": "吴语田野批次", "expectedVersion": b.Version, "idempotencyKey": "self-release"}, &rel); e != nil {
		return e
	}
	r, e := http.Get(ts.URL + "/api/credentials/verify?id=" + rel.Credential.ID)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("验证失败: %s", r.Status)
	}
	fmt.Println("selfcheck passed", time.Now().Format(time.RFC3339))
	return nil
}
