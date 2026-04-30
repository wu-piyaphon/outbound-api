package sentiment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
)

type Result struct {
	Positive  bool
	Score     float64 // 0.0 (fully negative) to 1.0 (fully positive)
	Reasoning string
}

type Provider interface {
	Analyze(ctx context.Context, symbol string) (*Result, error)
}

type alpacaNewsProvider struct {
	client *marketdata.Client
}

func NewAlpacaNewsProvider(client *marketdata.Client) Provider {
	return &alpacaNewsProvider{client: client}
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

func (p *alpacaNewsProvider) Analyze(_ context.Context, symbol string) (*Result, error) {
	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)

	articles, err := p.client.GetNews(marketdata.GetNewsRequest{
		Symbols:    []string{symbol},
		Start:      start,
		End:        end,
		TotalLimit: 10,
	})
	if err != nil {
		return &Result{
			Positive:  true,
			Score:     0.5,
			Reasoning: fmt.Sprintf("news unavailable for %s, proceeding with neutral sentiment", symbol),
		}, nil
	}

	if len(articles) == 0 {
		return &Result{
			Positive:  true,
			Score:     0.5,
			Reasoning: fmt.Sprintf("no recent news for %s, proceeding with neutral sentiment", symbol),
		}, nil
	}

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

	reasoning := fmt.Sprintf(
		"news sentiment for %s: %.0f%% positive (%d pos, %d neg signals across %d articles)",
		symbol, score*100, posScore, negScore, len(articles),
	)

	return &Result{Positive: positive, Score: score, Reasoning: reasoning}, nil
}
