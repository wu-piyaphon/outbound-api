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
	"github.com/wu-piyaphon/outbound-api/internal/model"
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

	accountTransferRepo := repository.NewAccountTransferRepository(pool)
	tradeRepo := repository.NewTradeRepository(pool)
	signalRepo := repository.NewSignalRepository(pool)
	transactor := database.NewTransactor(pool)

	marketDataClient := alpaca.NewMarketDataClient(cfg.AlpacaAPIKey, cfg.AlpacaAPISecret)
	alpacaClient := alpaca.NewAlpacaClient(cfg.AlpacaAPIKey, cfg.AlpacaAPISecret, cfg.AlpacaBaseURL)

	signalService := service.NewSignalService(signalRepo, marketDataClient)
	tradeService := service.NewTradeService(tradeRepo, accountTransferRepo, transactor, alpacaClient)
	accountTransferService := service.NewAccountTransferService(accountTransferRepo)

	barChan := make(chan stream.Bar)

	streamClient := alpaca.NewStocksStreamClient(cfg.AlpacaAPIKey, cfg.AlpacaAPISecret, watchlists, barChan)

	go func() {
		if err := streamClient.Connect(streamCtx); err != nil {
			log.Printf("Alpaca stream client stopped with error: %v", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		subscribed := make(map[string]struct{})
		for _, s := range watchlists {
			subscribed[s] = struct{}{}
		}

		for {
			select {
			case <-ticker.C:
				fetchedWatchlists, err := watchlistService.GetAllActive(streamCtx)
				if err != nil {
					log.Printf("failed to get watchlists: %v", err)
					continue
				}

				currentSet := make(map[string]struct{})
				for _, s := range fetchedWatchlists {
					currentSet[s] = struct{}{}
				}

				newSubscribedSymbols := []string{}
				unsubscribedSymbols := []string{}

				for s := range currentSet {
					if _, exist := subscribed[s]; !exist {
						newSubscribedSymbols = append(newSubscribedSymbols, s)
					}
				}

				for s := range subscribed {
					if _, exist := currentSet[s]; !exist {
						unsubscribedSymbols = append(unsubscribedSymbols, s)
					}
				}

				if len(newSubscribedSymbols) > 0 {
					err = alpaca.SubscribeToBars(streamClient, barChan, newSubscribedSymbols...)
					if err != nil {
						log.Printf("failed to subscribe to new symbols: %v", err)
					} else {
						log.Printf("subscribed to new symbols: %v", newSubscribedSymbols)
						for _, s := range newSubscribedSymbols {
							subscribed[s] = struct{}{}
						}
					}
				}

				if len(unsubscribedSymbols) > 0 {
					err = alpaca.UnsubscribeFromBars(streamClient, unsubscribedSymbols...)
					if err != nil {
						log.Printf("failed to unsubscribe from symbols: %v", err)
					} else {
						log.Printf("unsubscribed from symbols: %v", unsubscribedSymbols)
						for _, s := range unsubscribedSymbols {
							delete(subscribed, s)
						}
					}
				}

			case <-streamCtx.Done():
				return
			}
		}
	}()

	const numWorkers = 5

	for range numWorkers {
		go func() {
			for {
				select {
				case bar := <-barChan:
					signal, err := signalService.EvaluateSignal(streamCtx, bar.Symbol)
					if err != nil {
						log.Printf("failed to evaluate signal for %s: %v", bar.Symbol, err)
					}

					availableBudget, err := accountTransferService.GetAvailableBudget(streamCtx)
					if err != nil {
						log.Printf("failed to get active account transfer: %v", err)
					}

					if signal != nil {
						switch signal.Side {
						case model.SideBuy:
							_, err := tradeService.ExecuteBuyTrade(streamCtx, signal, availableBudget)
							if err != nil {
								log.Printf("failed to execute buy trade for %s: %v", signal.Symbol, err)
							}
						case model.SideSell:
							_, err := tradeService.ExecuteSellTrade(streamCtx, signal)
							if err != nil {
								log.Printf("failed to execute sell trade for %s: %v", signal.Symbol, err)
							}
						default:
							log.Printf("unknown signal side for %s: %s", signal.Symbol, signal.Side)
							continue
						}
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
