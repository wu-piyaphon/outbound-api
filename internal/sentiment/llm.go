package sentiment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
)

// llmAnalyzer calls an OpenAI-compatible chat-completions endpoint (e.g.
// DeepSeek) to score a batch of news articles in a single request.
// Fail-closed: any API or parsing error is returned as an error so callers
// skip the trade rather than proceeding on uncertain sentiment.
type llmAnalyzer struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewLLMAnalyzer constructs an ArticleAnalyzer that calls the OpenAI-
// compatible endpoint at baseURL (e.g. "https://api.deepseek.com/v1") with
// the supplied API key and model name.
func NewLLMAnalyzer(baseURL, apiKey, model string) ArticleAnalyzer {
	return &llmAnalyzer{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// newLLMAnalyzerWithClient is a test-only constructor that injects a custom
// HTTP client so tests can point the analyzer at an httptest.Server.
func newLLMAnalyzerWithClient(baseURL, apiKey, model string, client *http.Client) ArticleAnalyzer {
	return &llmAnalyzer{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: client,
	}
}

// chatRequest is the body sent to the /chat/completions endpoint.
type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	ResponseFormat struct {
		Type string `json:"type"`
	} `json:"response_format"`
	Temperature float64 `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the minimal subset of the OpenAI response we need.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// llmSentimentResponse is the JSON object the model is instructed to return.
type llmSentimentResponse struct {
	Label     string  `json:"label"`     // "positive" | "neutral" | "negative"
	Score     float64 `json:"score"`     // 0.0–1.0
	Reasoning string  `json:"reasoning"` // brief explanation
}

// Analyze builds a single batched prompt from all article headlines and
// summaries, calls the LLM, and parses the structured JSON response.
// Returns an error on any API or parse failure (fail-closed).
func (l *llmAnalyzer) Analyze(ctx context.Context, symbol string, articles []marketdata.News) (*Result, error) {
	prompt := l.buildPrompt(symbol, articles)

	reqBody := chatRequest{
		Model: l.model,
		Messages: []chatMessage{
			{
				Role: "system",
				Content: "You are a financial news sentiment analyst. " +
					"Return ONLY a JSON object with fields: " +
					"\"label\" (one of \"positive\", \"neutral\", \"negative\"), " +
					"\"score\" (float 0.0=fully negative to 1.0=fully positive), " +
					"\"reasoning\" (one-sentence explanation). " +
					"No markdown, no extra fields.",
			},
			{Role: "user", Content: prompt},
		},
		Temperature: 0,
	}
	reqBody.ResponseFormat.Type = "json_object"

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: unexpected status %d: %s", resp.StatusCode, truncate(string(respBytes), 200))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("llm: unmarshal chat response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("llm: API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("llm: empty choices in response")
	}

	content := chatResp.Choices[0].Message.Content
	var sentResp llmSentimentResponse
	if err := json.Unmarshal([]byte(content), &sentResp); err != nil {
		return nil, fmt.Errorf("llm: unmarshal sentiment payload: %w", err)
	}

	if sentResp.Label != "positive" && sentResp.Label != "neutral" && sentResp.Label != "negative" {
		return nil, fmt.Errorf("llm: unexpected label %q", sentResp.Label)
	}

	return &Result{
		// Only "negative" vetoes; neutral and positive both pass through.
		Positive:  sentResp.Label != "negative",
		Score:     sentResp.Score,
		Reasoning: fmt.Sprintf("[llm] %s", sentResp.Reasoning),
	}, nil
}

// buildPrompt constructs the user message that lists all article headlines
// and summaries for the LLM to assess in one shot.
func (l *llmAnalyzer) buildPrompt(symbol string, articles []marketdata.News) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Analyze the sentiment of the following %d recent news articles about %s:\n\n", len(articles), symbol)
	for i, a := range articles {
		summary := strings.TrimSpace(a.Summary)
		if summary == "" {
			summary = "(no summary)"
		}
		fmt.Fprintf(&sb, "%d. %s — %s\n", i+1, strings.TrimSpace(a.Headline), summary)
	}
	return sb.String()
}

// truncate caps s to at most n bytes for safe error message inclusion.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
