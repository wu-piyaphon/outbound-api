package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/joho/godotenv"
	"github.com/wu-piyaphon/outbound-api/internal/alpaca"
	"github.com/wu-piyaphon/outbound-api/internal/config"
	"github.com/wu-piyaphon/outbound-api/internal/database"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
	"github.com/wu-piyaphon/outbound-api/internal/service"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to the database: %v", err)
	}
	defer pool.Close()

	log.Println("Successfully connected to the database")

	database.Migrate(cfg.DatabaseURL)

	watchlistRepo := repository.NewWatchlistRepository(pool)
	watchlistService := service.NewWatchlistService(watchlistRepo)

	watchlists, err := watchlistService.GetAllActive(ctx)
	if err != nil {
		log.Fatalf("failed to get watchlists: %v", err)
	}

	marketDataClient := alpaca.NewMarketDataClient(cfg.AlpacaAPIKey, cfg.AlpacaAPISecret)
	alpacaClient := alpaca.NewAlpacaClient(cfg.AlpacaAPIKey, cfg.AlpacaAPISecret, cfg.AlpacaBaseURL)
	signalService := service.NewSignalService(repository.NewSignalRepository(pool), marketDataClient)
	tradeService := service.NewTradeService(repository.NewTradeRepository(pool), alpacaClient)

	barChan := make(chan stream.Bar)

	streamClient := alpaca.NewStocksStreamClient(cfg.AlpacaAPIKey, cfg.AlpacaAPISecret, watchlists, barChan)

	go func() {
		if err := streamClient.Connect(streamCtx); err != nil {
			log.Printf("Alpaca stream client stopped with error: %v", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	const numWorkers = 5

	for range numWorkers {
		go func() {
			for {
				select {
				case bar := <-barChan:
					log.Printf("Received bar for %s at %s: close=%.2f", bar.Symbol, bar.Timestamp.Format(time.RFC3339), bar.Close)
					signal, err := signalService.EvaluateSignal(streamCtx, bar.Symbol)
					if err != nil {
						log.Printf("failed to evaluate signal for %s: %v", bar.Symbol, err)
					}

					if signal != nil {
						tradeService.ExecuteTrade(streamCtx, signal)
					}

				case <-streamCtx.Done():
					return
				}
			}
		}()
	}

	<-quit
	log.Println("shutting down...")
	streamCancel()
}
