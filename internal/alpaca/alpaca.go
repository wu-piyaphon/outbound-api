package alpaca

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	marketdatastream "github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

// NewAlpacaClient returns an Alpaca trading client configured with the
// provided credentials and base URL (paper or live endpoint).
func NewAlpacaClient(APIKey, APISecret, BaseURL string) *alpaca.Client {
	c := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    APIKey,
		APISecret: APISecret,
		BaseURL:   BaseURL,
	})

	return c
}

// NewMarketDataClient returns a market data client targeting the IEX feed,
// which provides free real-time data without requiring a live brokerage account.
func NewMarketDataClient(APIKey, APISecret string) *marketdata.Client {
	c := marketdata.NewClient(
		marketdata.ClientOpts{
			APIKey:    APIKey,
			APISecret: APISecret,
			Feed:      marketdata.IEX,
		},
	)

	return c
}

// NewStocksStreamClient creates a WebSocket stocks stream client that forwards
// incoming bars for symbols into barChan. The caller owns barChan and must
// ensure it is not closed while the client is running.
//
// The handler uses a non-blocking send so a momentarily busy worker pool never
// stalls the stream callback. When the buffer is full the bar is dropped and
// logged as a warning so signal starvation is visible in production logs.
func NewStocksStreamClient(APIKey, APISecret string, symbols []string, barChan chan<- marketdatastream.Bar) *marketdatastream.StocksClient {
	barHandler := func(bar marketdatastream.Bar) {
		select {
		case barChan <- bar:
		default:
			slog.Warn("bar dropped: channel full", "symbol", bar.Symbol)
		}
	}

	c := marketdatastream.NewStocksClient(
		marketdata.IEX,
		marketdatastream.WithBars(barHandler, symbols...),
		marketdatastream.WithCredentials(APIKey, APISecret),
	)

	return c
}

// SubscribeToBars adds symbols to an already-connected stream client and wires
// their bar events to barChan using the same non-blocking handler pattern as
// NewStocksStreamClient.
func SubscribeToBars(c *marketdatastream.StocksClient, barChan chan<- marketdatastream.Bar, symbols ...string) error {
	barHandler := func(bar marketdatastream.Bar) {
		select {
		case barChan <- bar:
		default:
			slog.Warn("bar dropped: channel full", "symbol", bar.Symbol)
		}
	}

	err := c.SubscribeToBars(barHandler, symbols...)
	if err != nil {
		return fmt.Errorf("SubscribeToBars: %w", err)
	}

	return nil
}

// UnsubscribeFromBars removes symbols from the active stream subscription.
func UnsubscribeFromBars(c *marketdatastream.StocksClient, symbols ...string) error {
	err := c.UnsubscribeFromBars(symbols...)
	if err != nil {
		return fmt.Errorf("UnsubscribeFromBars: %w", err)
	}

	return nil
}

// StreamTradeUpdates opens a WebSocket stream for account-level trade update
// events and forwards each update to tradeUpdateChan. It blocks until ctx is
// cancelled or the connection fails, at which point the error is returned.
//
// The handler uses a non-blocking send to avoid stalling the WebSocket callback
// thread if the consumer falls behind. A dropped update is logged as a warning;
// the connectWithRetry supervisor reconnects and replays events, and
// ApplyTradeUpdates is idempotent on terminal-status trades, so replay is safe.
func StreamTradeUpdates(
	ctx context.Context,
	c *alpaca.Client,
	tradeUpdateChan chan<- alpaca.TradeUpdate,
) error {
	handler := func(tu alpaca.TradeUpdate) {
		select {
		case tradeUpdateChan <- tu:
		default:
			slog.Warn("trade update dropped: channel full", "event", tu.Event, "order_id", tu.Order.ID)
		}
	}
	if err := c.StreamTradeUpdates(ctx, handler, alpaca.StreamTradeUpdatesRequest{}); err != nil {
		return fmt.Errorf("StreamTradeUpdates: %w", err)
	}
	return nil
}
