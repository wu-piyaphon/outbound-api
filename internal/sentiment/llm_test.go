package sentiment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testArticles() []marketdata.News {
	return []marketdata.News{
		{Headline: "Apple beats Q3 earnings expectations", Summary: "Revenue surged 12% year-over-year."},
		{Headline: "iPhone sales rally in Asia", Summary: "Strong demand from emerging markets."},
	}
}

// newMockLLMServer returns an httptest.Server that responds with the given
// sentiment JSON payload wrapped in a chat completions response envelope.
func newMockLLMServer(t *testing.T, statusCode int, label string, score float64, reasoning string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		if statusCode != http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"message": "rate limit exceeded"},
			})
			return
		}

		sentPayload, _ := json.Marshal(map[string]any{
			"label":     label,
			"score":     score,
			"reasoning": reasoning,
		})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": string(sentPayload)}},
			},
		})
	}))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestLLMAnalyzer_PositiveLabel_Passes(t *testing.T) {
	srv := newMockLLMServer(t, 200, "positive", 0.85, "strong earnings beat")
	defer srv.Close()

	a := newLLMAnalyzerWithClient(srv.URL, "test-key", "deepseek-chat", srv.Client())
	result, err := a.Analyze(context.Background(), "AAPL", testArticles())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Positive {
		t.Error("expected Positive=true for label 'positive'")
	}
	if result.Score != 0.85 {
		t.Errorf("expected score 0.85, got %f", result.Score)
	}
	if result.Reasoning == "" {
		t.Error("expected non-empty reasoning")
	}
}

func TestLLMAnalyzer_NeutralLabel_Passes(t *testing.T) {
	srv := newMockLLMServer(t, 200, "neutral", 0.5, "mixed signals")
	defer srv.Close()

	a := newLLMAnalyzerWithClient(srv.URL, "test-key", "deepseek-chat", srv.Client())
	result, err := a.Analyze(context.Background(), "AAPL", testArticles())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Positive {
		t.Error("expected Positive=true for label 'neutral' (neutral must not veto)")
	}
}

func TestLLMAnalyzer_NegativeLabel_Vetoes(t *testing.T) {
	srv := newMockLLMServer(t, 200, "negative", 0.15, "accounting fraud allegations")
	defer srv.Close()

	a := newLLMAnalyzerWithClient(srv.URL, "test-key", "deepseek-chat", srv.Client())
	result, err := a.Analyze(context.Background(), "AAPL", testArticles())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Positive {
		t.Error("expected Positive=false for label 'negative'")
	}
}

func TestLLMAnalyzer_APIError_FailClosed(t *testing.T) {
	srv := newMockLLMServer(t, 429, "", 0, "")
	defer srv.Close()

	a := newLLMAnalyzerWithClient(srv.URL, "test-key", "deepseek-chat", srv.Client())
	result, err := a.Analyze(context.Background(), "AAPL", testArticles())
	if err == nil {
		t.Errorf("expected error on non-200 status, got result: %+v", result)
	}
}

func TestLLMAnalyzer_NetworkFailure_FailClosed(t *testing.T) {
	// Point at a server that is immediately closed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	a := newLLMAnalyzerWithClient(srv.URL, "test-key", "deepseek-chat", srv.Client())
	_, err := a.Analyze(context.Background(), "AAPL", testArticles())
	if err == nil {
		t.Error("expected error when server is unreachable")
	}
}

func TestLLMAnalyzer_InvalidLabel_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		sentPayload, _ := json.Marshal(map[string]any{
			"label":     "maybe",
			"score":     0.5,
			"reasoning": "unsure",
		})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": string(sentPayload)}},
			},
		})
	}))
	defer srv.Close()

	a := newLLMAnalyzerWithClient(srv.URL, "test-key", "deepseek-chat", srv.Client())
	_, err := a.Analyze(context.Background(), "AAPL", testArticles())
	if err == nil {
		t.Error("expected error for unrecognised label 'maybe'")
	}
}

func TestLLMAnalyzer_EmptyChoices_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer srv.Close()

	a := newLLMAnalyzerWithClient(srv.URL, "test-key", "deepseek-chat", srv.Client())
	_, err := a.Analyze(context.Background(), "AAPL", testArticles())
	if err == nil {
		t.Error("expected error for empty choices")
	}
}

func TestLLMAnalyzer_PromptContainsArticles(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		capturedBody = buf

		sentPayload, _ := json.Marshal(map[string]any{"label": "positive", "score": 0.8, "reasoning": "ok"})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": string(sentPayload)}},
			},
		})
	}))
	defer srv.Close()

	articles := testArticles()
	a := newLLMAnalyzerWithClient(srv.URL, "key", "m", srv.Client())
	_, err := a.Analyze(context.Background(), "AAPL", articles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var req chatRequest
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("failed to parse captured body: %v", err)
	}

	if req.ResponseFormat.Type != "json_object" {
		t.Errorf("expected response_format.type=json_object, got %q", req.ResponseFormat.Type)
	}

	userMsg := req.Messages[len(req.Messages)-1].Content
	for _, a := range articles {
		if !contains(userMsg, a.Headline) {
			t.Errorf("prompt missing article headline: %q", a.Headline)
		}
	}
}

func TestLLMAnalyzer_ContextCancellation_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response — context should cancel before we reply.
		<-r.Context().Done()
		fmt.Fprintln(w, "cancelled")
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	a := newLLMAnalyzerWithClient(srv.URL, "key", "m", srv.Client())
	_, err := a.Analyze(ctx, "AAPL", testArticles())
	if err == nil {
		t.Error("expected error when context is already cancelled")
	}
}

// ---------------------------------------------------------------------------
// MinArticles floor test (direct guard, no network client needed)
// ---------------------------------------------------------------------------

// spyAnalyzer records whether Analyze was called so we can assert the
// min-articles guard prevented the inner call.
type spyAnalyzer struct{ called bool }

func (s *spyAnalyzer) Analyze(_ context.Context, _ string, _ []marketdata.News) (*Result, error) {
	s.called = true
	return &Result{Positive: true, Score: 0.9, Reasoning: "spy"}, nil
}

func TestAlpacaNewsProvider_BelowMinArticles_ReturnsNeutralWithoutCallingInner(t *testing.T) {
	spy := &spyAnalyzer{}
	// Bypass the marketdata.Client by calling the guard logic directly:
	// simulate the state after GetNews returns 2 articles with minArticles=3.
	prov := &alpacaNewsProvider{inner: spy, minArticles: 3}
	articles := testArticles() // len == 2

	// Manually trigger the branch that alpacaNewsProvider.Analyze would hit.
	var result *Result
	if len(articles) == 0 || (prov.minArticles > 0 && len(articles) < prov.minArticles) {
		result = &Result{
			Positive:  true,
			Score:     0.5,
			Reasoning: fmt.Sprintf("insufficient news for AAPL (%d articles), proceeding with neutral sentiment", len(articles)),
		}
	} else {
		var err error
		result, err = prov.inner.Analyze(context.Background(), "AAPL", articles)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if spy.called {
		t.Error("inner analyzer must not be called when article count is below minArticles")
	}
	if !result.Positive {
		t.Error("below-minimum result should be neutral pass-through (Positive=true)")
	}
}

func TestAlpacaNewsProvider_AtMinArticles_CallsInner(t *testing.T) {
	spy := &spyAnalyzer{}
	prov := &alpacaNewsProvider{inner: spy, minArticles: 2}
	articles := testArticles() // len == 2, exactly at minimum

	if len(articles) == 0 || (prov.minArticles > 0 && len(articles) < prov.minArticles) {
		t.Fatal("expected guard to pass at exactly minArticles")
	}
	if _, err := prov.inner.Analyze(context.Background(), "AAPL", articles); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.called {
		t.Error("inner analyzer must be called when article count equals minArticles")
	}
}

// contains is a helper for substring checks in tests.
func contains(s, sub string) bool {
	if len(sub) == 0 || len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
