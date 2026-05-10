// Package sentiment scores recent news for a symbol. The live execution path
// uses the keyword analyzer; the shadow path uses the LLM analyzer. Results
// are memoised per symbol via CachedProvider to bound news API traffic.
package sentiment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
)

// Result carries the sentiment verdict for a symbol.
type Result struct {
	Positive  bool
	Score     float64 // 0.0 (fully negative) to 1.0 (fully positive)
	Reasoning string
}

// Provider evaluates sentiment for a given stock symbol.
type Provider interface {
	Analyze(ctx context.Context, symbol string) (*Result, error)
}

// ArticleAnalyzer scores a pre-fetched set of news articles for a given symbol.
// Implementations include the keyword scorer (live path) and the LLM scorer (shadow path).
type ArticleAnalyzer interface {
	Analyze(ctx context.Context, symbol string, articles []marketdata.News) (*Result, error)
}

// alpacaNewsClient is the subset of the Alpaca market data client used to load
// headlines for sentiment scoring.
type alpacaNewsClient interface {
	GetNews(req marketdata.GetNewsRequest) ([]marketdata.News, error)
}

// alpacaNewsProvider fetches recent articles from Alpaca News, then delegates
// scoring to an inner ArticleAnalyzer. Alpaca News API failures return an error
// (fail-closed: callers skip sentiment-backed buys until news can be read).
// When there are no articles or fewer than minArticles (when minArticles > 0),
// it returns a neutral pass-through result without calling the inner analyzer.
type alpacaNewsProvider struct {
	client      alpacaNewsClient
	inner       ArticleAnalyzer
	minArticles int
}

// NewAlpacaNewsProvider constructs a Provider that fetches news from Alpaca and
// scores it with inner. Set minArticles to 0 to disable the floor check.
func NewAlpacaNewsProvider(client alpacaNewsClient, inner ArticleAnalyzer, minArticles int) Provider {
	return &alpacaNewsProvider{client: client, inner: inner, minArticles: minArticles}
}

func (p *alpacaNewsProvider) Analyze(ctx context.Context, symbol string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("sentiment.Analyze: context cancelled before GetNews: %w", err)
	}

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)

	articles, err := p.client.GetNews(marketdata.GetNewsRequest{
		Symbols:    []string{symbol},
		Start:      start,
		End:        end,
		TotalLimit: 10,
	})
	if err != nil {
		slog.Warn("sentiment: Alpaca news API failed", "symbol", symbol, "error", err)
		return nil, fmt.Errorf("sentiment: get news for %s: %w", symbol, err)
	}

	if len(articles) == 0 || (p.minArticles > 0 && len(articles) < p.minArticles) {
		return &Result{
			Positive:  true,
			Score:     0.5,
			Reasoning: fmt.Sprintf("insufficient news for %s (%d articles), proceeding with neutral sentiment", symbol, len(articles)),
		}, nil
	}

	return p.inner.Analyze(ctx, symbol, articles)
}

var positiveKeywords = []string{
	"beat", "exceeds", "surge", "soar", "rally", "gain", "upgrade",
	"strong", "growth", "profit", "revenue", "record", "bullish",
	"outperform", "rises", "jumps", "climbs", "boost", "positive",
	"buy", "overweight", "raises", "breakthrough", "partnership",
}

var negativeKeywords = []string{
	"miss", "fall", "drop", "plunge", "decline", "loss", "downgrade",
	"weak", "risk", "concern", "bearish", "crash", "cut", "lowered",
	"underperform", "falls", "slides", "tumbles", "warning", "fraud",
	"investigation", "lawsuit", "bankrupt", "recall", "layoffs", "sell",
}

type keywordAnalyzer struct{}

// NewKeywordAnalyzer returns an ArticleAnalyzer that scores articles using
// positive/negative keyword counts. Used by the live trading path.
func NewKeywordAnalyzer() ArticleAnalyzer {
	return &keywordAnalyzer{}
}

func (k *keywordAnalyzer) Analyze(_ context.Context, symbol string, articles []marketdata.News) (*Result, error) {
	posScore := 0
	negScore := 0

	for _, article := range articles {
		text := strings.ToLower(article.Headline + " " + article.Summary)
		for _, kw := range positiveKeywords {
			if strings.Contains(text, kw) {
				posScore++
			}
		}
		for _, kw := range negativeKeywords {
			if strings.Contains(text, kw) {
				negScore++
			}
		}
	}

	total := posScore + negScore
	if total == 0 {
		return &Result{
			Positive:  true,
			Score:     0.5,
			Reasoning: fmt.Sprintf("neutral news sentiment for %s (%d articles)", symbol, len(articles)),
		}, nil
	}

	score := float64(posScore) / float64(total)
	positive := score >= 0.4

	return &Result{
		Positive:  positive,
		Score:     score,
		Reasoning: fmt.Sprintf("news sentiment for %s: %.0f%% positive (%d pos, %d neg signals across %d articles)", symbol, score*100, posScore, negScore, len(articles)),
	}, nil
}

type cachedEntry struct {
	result    *Result
	expiresAt time.Time
}

// cachedProvider wraps a Provider and caches Analyze results per symbol for
// ttl duration, avoiding redundant news API calls on every bar tick.
type cachedProvider struct {
	inner Provider
	ttl   time.Duration
	mu    sync.RWMutex
	cache map[string]cachedEntry
}

// NewCachedProvider returns a Provider that memoises Analyze results per
// symbol for ttl. A zero or negative ttl falls through to the inner provider
// on every call.
func NewCachedProvider(inner Provider, ttl time.Duration) Provider {
	return &cachedProvider{
		inner: inner,
		ttl:   ttl,
		cache: make(map[string]cachedEntry),
	}
}

func (c *cachedProvider) Analyze(ctx context.Context, symbol string) (*Result, error) {
	if c.ttl > 0 {
		c.mu.RLock()
		if entry, ok := c.cache[symbol]; ok && time.Now().Before(entry.expiresAt) {
			c.mu.RUnlock()
			return entry.result, nil
		}
		c.mu.RUnlock()
	}

	result, err := c.inner.Analyze(ctx, symbol)
	if err != nil {
		return nil, err
	}

	if c.ttl > 0 {
		c.mu.Lock()
		c.cache[symbol] = cachedEntry{result: result, expiresAt: time.Now().Add(c.ttl)}
		c.mu.Unlock()
	}

	return result, nil
}
