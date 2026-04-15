package alpaca

import (
	"fmt"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

func NewAlpacaClient(APIKey, APISecret, BaseURL string) *alpaca.Client {
	c := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    APIKey,
		APISecret: APISecret,
		BaseURL:   BaseURL,
	})

	return c
}

func NewStocksStreamClient(APIKey, APISecret string, symbols []string, barChan chan<- stream.Bar) *stream.StocksClient {

	barHandler := func(bar stream.Bar) {
		barChan <- bar
	}

	c := stream.NewStocksClient(
		marketdata.IEX,
		stream.WithBars(barHandler, symbols...),
		stream.WithCredentials(APIKey, APISecret),
	)

	return c
}

func SubscribeToBars(c *stream.StocksClient, barChan chan<- stream.Bar, symbols ...string) error {
	barHandler := func(bar stream.Bar) {
		barChan <- bar
	}

	err := c.SubscribeToBars(barHandler, symbols...)
	if err != nil {
		return fmt.Errorf("SubscribeToBars: %w", err)
	}

	return nil
}

func UnsubscribeFromBars(c *stream.StocksClient, symbols ...string) error {
	err := c.UnsubscribeFromBars(symbols...)
	if err != nil {
		return fmt.Errorf("UnsubscribeFromBars: %w", err)
	}

	return nil
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
