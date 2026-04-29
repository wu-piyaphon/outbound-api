package alpaca

import (
	"context"
	"fmt"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	marketdatastream "github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

func NewAlpacaClient(APIKey, APISecret, BaseURL string) *alpaca.Client {
	c := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    APIKey,
		APISecret: APISecret,
		BaseURL:   BaseURL,
	})

	return c
}

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

func NewStocksStreamClient(APIKey, APISecret string, symbols []string, barChan chan<- marketdatastream.Bar) *marketdatastream.StocksClient {

	barHandler := func(bar marketdatastream.Bar) {
		barChan <- bar
	}

	c := marketdatastream.NewStocksClient(
		marketdata.IEX,
		marketdatastream.WithBars(barHandler, symbols...),
		marketdatastream.WithCredentials(APIKey, APISecret),
	)

	return c
}

func SubscribeToBars(c *marketdatastream.StocksClient, barChan chan<- marketdatastream.Bar, symbols ...string) error {
	barHandler := func(bar marketdatastream.Bar) {
		barChan <- bar
	}

	err := c.SubscribeToBars(barHandler, symbols...)
	if err != nil {
		return fmt.Errorf("SubscribeToBars: %w", err)
	}

	return nil
}

func UnsubscribeFromBars(c *marketdatastream.StocksClient, symbols ...string) error {
	err := c.UnsubscribeFromBars(symbols...)
	if err != nil {
		return fmt.Errorf("UnsubscribeFromBars: %w", err)
	}

	return nil
}

func StreamTradeUpdates(
	ctx context.Context,
	c *alpaca.Client,
	tradeUpdateChan chan<- alpaca.TradeUpdate,
) error {
	handler := func(tu alpaca.TradeUpdate) {
		tradeUpdateChan <- tu
	}
	if err := c.StreamTradeUpdates(ctx, handler, alpaca.StreamTradeUpdatesRequest{}); err != nil {
		return fmt.Errorf("StreamTradeUpdates: %w", err)
	}
	return nil
}
