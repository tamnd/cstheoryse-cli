package cstheoryse_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/cstheoryse-cli/cstheoryse"
)

// gzipJSON encodes v as JSON and gzip-compresses it, as the SE API does.
func gzipJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func fakeResponse(items []map[string]any) map[string]any {
	return map[string]any{"items": items}
}

func TestQuestionsReturnsItems(t *testing.T) {
	item := map[string]any{
		"question_id":   42,
		"title":         "What is P vs NP?",
		"tags":          []string{"cc.complexity-theory", "p-vs-np"},
		"score":         100,
		"answer_count":  5,
		"view_count":    2000,
		"creation_date": 1284252219,
		"link":          "https://cstheory.stackexchange.com/questions/42/what-is-p-vs-np",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(gzipJSON(t, fakeResponse([]map[string]any{item})))
	}))
	defer srv.Close()

	cfg := cstheoryse.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := cstheoryse.NewClient(cfg)

	questions, err := c.Questions(context.Background(), "votes", 5)
	if err != nil {
		t.Fatalf("Questions: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("got %d questions, want 1", len(questions))
	}
	q := questions[0]
	if q.QuestionID != 42 {
		t.Errorf("QuestionID = %d, want 42", q.QuestionID)
	}
	if q.Title != "What is P vs NP?" {
		t.Errorf("Title = %q, want 'What is P vs NP?'", q.Title)
	}
	if q.Score != 100 {
		t.Errorf("Score = %d, want 100", q.Score)
	}
	if q.Tags != "cc.complexity-theory;p-vs-np" {
		t.Errorf("Tags = %q", q.Tags)
	}
}

func TestSearchReturnsItems(t *testing.T) {
	item := map[string]any{
		"question_id":   99,
		"title":         "Reducing P vs. NP to SAT",
		"tags":          []string{"p-vs-np"},
		"score":         12,
		"answer_count":  2,
		"view_count":    1439,
		"creation_date": 1287310149,
		"link":          "https://cstheory.stackexchange.com/questions/99/reducing-p-vs-np",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("intitle"); got != "P vs NP" {
			t.Errorf("intitle = %q, want 'P vs NP'", got)
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(gzipJSON(t, fakeResponse([]map[string]any{item})))
	}))
	defer srv.Close()

	cfg := cstheoryse.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := cstheoryse.NewClient(cfg)

	results, err := c.Search(context.Background(), "P vs NP", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].QuestionID != 99 {
		t.Errorf("QuestionID = %d, want 99", results[0].QuestionID)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	item := map[string]any{
		"question_id":   1,
		"title":         "recovered",
		"tags":          []string{},
		"score":         0,
		"answer_count":  0,
		"view_count":    0,
		"creation_date": 0,
		"link":          "https://cstheory.stackexchange.com/questions/1",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(gzipJSON(t, fakeResponse([]map[string]any{item})))
	}))
	defer srv.Close()

	cfg := cstheoryse.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := cstheoryse.NewClient(cfg)

	start := time.Now()
	_, err := c.Questions(context.Background(), "votes", 1)
	if err != nil {
		t.Fatalf("Questions after retries: %v", err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGzipDecompression(t *testing.T) {
	item := map[string]any{
		"question_id":   7,
		"title":         "Gzip works",
		"tags":          []string{"test"},
		"score":         1,
		"answer_count":  0,
		"view_count":    0,
		"creation_date": 1000000,
		"link":          "https://cstheory.stackexchange.com/questions/7",
	}
	// Server sends gzip-compressed response (as SE API does).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(gzipJSON(t, fakeResponse([]map[string]any{item})))
	}))
	defer srv.Close()

	cfg := cstheoryse.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := cstheoryse.NewClient(cfg)

	qs, err := c.Questions(context.Background(), "votes", 1)
	if err != nil {
		t.Fatalf("gzip decompression failed: %v", err)
	}
	if len(qs) == 0 || qs[0].Title != "Gzip works" {
		t.Errorf("unexpected result: %+v", qs)
	}
}
