// Package cstheoryse is the library behind the cst2 command: the HTTP client,
// request shaping, and the typed data models for Computer Science Theory Stack
// Exchange.
//
// The Stack Exchange API is open and requires no key for read operations. All
// responses are gzip-compressed; this package decompresses them transparently.
package cstheoryse

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to the Stack Exchange API.
const DefaultUserAgent = "cst2/dev (+https://github.com/tamnd/cstheoryse-cli)"

// ErrNotFound is returned when the API reports no results.
var ErrNotFound = errors.New("not found")

// Config holds constructor parameters.
type Config struct {
	BaseURL   string
	Site      string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.stackexchange.com",
		Site:      "cstheory",
		Rate:      200 * time.Millisecond,
		Retries:   5,
		Timeout:   30 * time.Second,
		UserAgent: DefaultUserAgent,
	}
}

// Client talks to the Stack Exchange API.
type Client struct {
	http      *http.Client
	cfg       Config
	mu        sync.Mutex
	last      time.Time
}

// NewClient returns a Client configured by cfg.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultConfig().BaseURL
	}
	if cfg.Site == "" {
		cfg.Site = DefaultConfig().Site
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	return &Client{
		http: &http.Client{Timeout: cfg.Timeout},
		cfg:  cfg,
	}
}

// Question is the record emitted for each SE question.
type Question struct {
	QuestionID  int    `json:"question_id"`
	Title       string `json:"title"`
	Tags        string `json:"tags"`
	Score       int    `json:"score"`
	AnswerCount int    `json:"answer_count"`
	ViewCount   int    `json:"view_count"`
	AskedAt     string `json:"asked_at"`
	URL         string `json:"url"`
}

// seResponse is the envelope the Stack Exchange API wraps items in.
type seResponse struct {
	Items []seItem `json:"items"`
}

// seItem is the raw wire shape from the API.
type seItem struct {
	QuestionID  int      `json:"question_id"`
	Title       string   `json:"title"`
	Tags        []string `json:"tags"`
	Score       int      `json:"score"`
	AnswerCount int      `json:"answer_count"`
	ViewCount   int      `json:"view_count"`
	CreationDate int64   `json:"creation_date"`
	Link        string   `json:"link"`
}

func wireToQuestion(it seItem) Question {
	return Question{
		QuestionID:  it.QuestionID,
		Title:       htmlDecode(it.Title),
		Tags:        strings.Join(it.Tags, ";"),
		Score:       it.Score,
		AnswerCount: it.AnswerCount,
		ViewCount:   it.ViewCount,
		AskedAt:     time.Unix(it.CreationDate, 0).UTC().Format(time.RFC3339),
		URL:         it.Link,
	}
}

// Questions returns questions sorted by sort with at most limit results.
// sort is one of: "votes", "activity", "creation".
func (c *Client) Questions(ctx context.Context, sort string, limit int) ([]Question, error) {
	if sort == "" {
		sort = "votes"
	}
	if limit <= 0 {
		limit = 10
	}
	params := url.Values{}
	params.Set("site", c.cfg.Site)
	params.Set("order", "desc")
	params.Set("sort", sort)
	params.Set("pagesize", strconv.Itoa(limit))
	params.Set("filter", "default")

	rawURL := c.cfg.BaseURL + "/2.3/questions?" + params.Encode()
	var resp seResponse
	if err := c.getJSON(ctx, rawURL, &resp); err != nil {
		return nil, err
	}
	out := make([]Question, 0, len(resp.Items))
	for _, it := range resp.Items {
		out = append(out, wireToQuestion(it))
	}
	return out, nil
}

// Search searches for questions with title matching query.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Question, error) {
	if limit <= 0 {
		limit = 10
	}
	params := url.Values{}
	params.Set("site", c.cfg.Site)
	params.Set("intitle", query)
	params.Set("order", "desc")
	params.Set("sort", "relevance")
	params.Set("pagesize", strconv.Itoa(limit))
	params.Set("filter", "default")

	rawURL := c.cfg.BaseURL + "/2.3/search?" + params.Encode()
	var resp seResponse
	if err := c.getJSON(ctx, rawURL, &resp); err != nil {
		return nil, err
	}
	out := make([]Question, 0, len(resp.Items))
	for _, it := range resp.Items {
		out = append(out, wireToQuestion(it))
	}
	return out, nil
}

func (c *Client) getJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, true, fmt.Errorf("gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}

	b, err := io.ReadAll(io.LimitReader(reader, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// htmlDecode replaces common HTML entities in SE titles.
func htmlDecode(s string) string {
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&apos;", "'")
	return s
}
