package alpaca

import (
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

func NewStocksStreamClient(APIKey, APISecret string, symbols []string) *stream.StocksClient {

	barHandler := func(bar stream.Bar) {
		// TODO: Handle incoming bar data here
	}

	c := stream.NewStocksClient(
		marketdata.IEX,
		stream.WithBars(barHandler, symbols...),
		stream.WithCredentials(APIKey, APISecret),
	)

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
