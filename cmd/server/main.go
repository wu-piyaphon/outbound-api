package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/alpaca"
	"github.com/wu-piyaphon/outbound-api/internal/bot"
	"github.com/wu-piyaphon/outbound-api/internal/config"
	"github.com/wu-piyaphon/outbound-api/internal/database"
	bothttp "github.com/wu-piyaphon/outbound-api/internal/http"
	"github.com/wu-piyaphon/outbound-api/internal/indicator"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
	"github.com/wu-piyaphon/outbound-api/internal/sentiment"
	"github.com/wu-piyaphon/outbound-api/internal/service"

	alpacaSDK "github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
)

// seedIndicators fetches 14 months of daily bars for symbol and seeds the
// indicator cache. Runs at startup and once per day per symbol.
func seedIndicators(symbol string, client *marketdata.Client, cache *indicator.IndicatorCache) {
	bars, err := client.GetBars(symbol, marketdata.GetBarsRequest{
		TimeFrame: marketdata.OneDay,
		Start:     time.Now().AddDate(-1, -2, 0),
		End:       time.Now(),
	})
	if err != nil {
		log.Printf("seedIndicators: GetBars for %s: %v", symbol, err)
		return
	}

	ibars := make([]indicator.Bar, len(bars))
	for i, b := range bars {
		ibars[i] = indicator.Bar{
			High:  decimal.NewFromFloat(b.High),
			Low:   decimal.NewFromFloat(b.Low),
			Close: decimal.NewFromFloat(b.Close),
		}
	}

	if err := cache.Seed(symbol, ibars, 200, 14, 14); err != nil {
		log.Printf("seedIndicators: Seed for %s: %v", symbol, err)
		return
	}

	log.Printf("seedIndicators: %s ready", symbol)
}

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

	// Seed indicator cache once per symbol at startup — zero REST calls in hot path.
	indicatorCache := indicator.NewIndicatorCache()
	for _, symbol := range watchlists {
		seedIndicators(symbol, marketDataClient, indicatorCache)
	}

	sentimentProvider := sentiment.NewAlpacaNewsProvider(marketDataClient)

	signalService := service.NewSignalService(signalRepo, tradeRepo, indicatorCache, sentimentProvider)
	tradeService := service.NewTradeService(tradeRepo, accountTransferRepo, signalRepo, transactor, alpacaClient, cfg.RiskPerTradePct, cfg.ATRRiskMultiplier)
	accountTransferService := service.NewAccountTransferService(accountTransferRepo)

	initialBotState := bot.StateRunning
	if !cfg.BotAutoStart {
		initialBotState = bot.StateStopped
		log.Println("Bot started in stopped state — use POST /bot/start to begin trading")
	}
	botController := bot.NewController(initialBotState)

	barChan := make(chan stream.Bar)

	streamClient := alpaca.NewStocksStreamClient(cfg.AlpacaAPIKey, cfg.AlpacaAPISecret, watchlists, barChan)

	go func() {
		if err := streamClient.Connect(streamCtx); err != nil {
			log.Printf("Alpaca stream client stopped with error: %v", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Watchlist refresh: checks for symbol additions/removals every 30 seconds
	// and seeds the indicator cache for any newly added symbols.
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

				var newSymbols, removedSymbols []string

				for s := range currentSet {
					if _, exist := subscribed[s]; !exist {
						newSymbols = append(newSymbols, s)
					}
				}

				for s := range subscribed {
					if _, exist := currentSet[s]; !exist {
						removedSymbols = append(removedSymbols, s)
					}
				}

				if len(newSymbols) > 0 {
					err = alpaca.SubscribeToBars(streamClient, barChan, newSymbols...)
					if err != nil {
						log.Printf("failed to subscribe to new symbols: %v", err)
					} else {
						log.Printf("subscribed to new symbols: %v", newSymbols)
						for _, s := range newSymbols {
							subscribed[s] = struct{}{}
							// Seed indicators for the new symbol before bars arrive.
							seedIndicators(s, marketDataClient, indicatorCache)
						}
					}
				}

				if len(removedSymbols) > 0 {
					err = alpaca.UnsubscribeFromBars(streamClient, removedSymbols...)
					if err != nil {
						log.Printf("failed to unsubscribe from symbols: %v", err)
					} else {
						log.Printf("unsubscribed from symbols: %v", removedSymbols)
						for _, s := range removedSymbols {
							delete(subscribed, s)
						}
					}
				}

			case <-streamCtx.Done():
				return
			}
		}
	}()

	// Daily re-seed: refresh indicators once every 24 hours so that the EMA,
	// RSI, and ATR values incorporate each new day's bar without restarting.
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Println("daily indicator re-seed starting")
				watchlists, err := watchlistService.GetAllActive(streamCtx)
				if err != nil {
					log.Printf("daily re-seed: failed to get watchlists: %v", err)
					continue
				}
				for _, s := range watchlists {
					seedIndicators(s, marketDataClient, indicatorCache)
				}
				log.Println("daily indicator re-seed complete")
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
					if !botController.IsActive() {
						continue
					}

					livePrice := decimal.NewFromFloat(bar.Close)

					err := tradeService.EvaluateAndExecuteExits(streamCtx, bar.Symbol, livePrice)
					if err != nil {
						log.Printf("failed to check exit conditions for %s: %v", bar.Symbol, err)
					}

					entrySignal, err := signalService.EvaluateBuySignal(streamCtx, bar.Symbol, livePrice)
					if err != nil {
						log.Printf("failed to evaluate signal for %s: %v", bar.Symbol, err)
					}

					availableBudget, err := accountTransferService.GetAvailableBudget(streamCtx)
					if err != nil || availableBudget == nil {
						log.Printf("failed to get active account transfer: %v", err)
						continue
					}

					if entrySignal != nil {
						_, err := tradeService.ExecuteBuyTrade(streamCtx, entrySignal, availableBudget)
						if err != nil {
							log.Printf("failed to execute buy trade for %s: %v", entrySignal.Symbol, err)
						}
					}

				case <-streamCtx.Done():
					return
				}
			}
		}()
	}

	tradeUpdateChan := make(chan alpacaSDK.TradeUpdate, 64)
	go func() {
		if err := alpaca.StreamTradeUpdates(streamCtx, alpacaClient, tradeUpdateChan); err != nil {
			log.Printf("trade updates stream stopped: %v", err)
		}
	}()

	go func() {
		for {
			select {
			case update := <-tradeUpdateChan:
				status, ok := alpaca.MapAlpacaEventToStatus(update.Event)
				if !ok {
					log.Printf("unknown alpaca event: %s", update.Event)
					continue
				}
				if err := tradeService.ApplyTradeUpdates(streamCtx, update, status); err != nil {
					log.Printf("failed to handle trade update: %v", err)
				}
			case <-streamCtx.Done():
				return
			}
		}
	}()

	botHandlers := bothttp.NewBotHandlers(botController)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/bot/start", botHandlers.Start)
	http.HandleFunc("/bot/pause", botHandlers.Pause)
	http.HandleFunc("/bot/stop", botHandlers.Stop)
	http.HandleFunc("/bot/status", botHandlers.Status)

	go func() {
		if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil && err != http.ErrServerClosed {
			log.Printf("failed to start HTTP server: %v", err)
		}
	}()

	log.Printf("Server listening on :%s | bot state: %s", cfg.Port, botController.State())
	<-quit
	log.Println("shutting down...")
	streamCancel()
}
