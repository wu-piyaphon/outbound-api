package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
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
	"github.com/wu-piyaphon/outbound-api/migrations"

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
		slog.Error("seedIndicators: GetBars failed", "symbol", symbol, "error", err)
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
		slog.Error("seedIndicators: Seed failed", "symbol", symbol, "error", err)
		return
	}

	slog.Info("seedIndicators: ready", "symbol", symbol)
}

// connectWithRetry runs connectFn in a loop, backing off exponentially after
// each failure. It returns only when ctx is cancelled.
//
// After any return from connectFn, ctx is checked first — both nil and error
// returns can result from context cancellation. If nil is returned without
// context cancellation (unexpected SDK behaviour), the backoff is reset and the
// connection is retried immediately so the supervisor does not stop silently.
func connectWithRetry(ctx context.Context, name string, connectFn func() error) {
	const maxDelay = 64 * time.Second
	delay := time.Second

	for {
		slog.Info("connecting", "stream", name)
		err := connectFn()

		// Check context before inspecting the error — the SDK may return nil on
		// a context-cancelled disconnect (clean shutdown) or an error wrapping
		// context.Canceled.
		if ctx.Err() != nil {
			return
		}

		if err != nil {
			slog.Warn("stream disconnected", "stream", name, "error", err, "retry_in", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			if delay < maxDelay {
				delay *= 2
			}
		} else {
			// SDK returned nil without context cancellation — unexpected clean
			// disconnect. Reset backoff (connection was healthy) and retry.
			slog.Warn("stream closed without error; reconnecting immediately", "stream", name)
			delay = time.Second
		}
	}
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to the database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	slog.Info("connected to database")

	if err := database.Migrate(cfg.DatabaseURL, migrations.FS); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	watchlistRepo := repository.NewWatchlistRepository(pool)
	watchlistService := service.NewWatchlistService(watchlistRepo)

	watchlists, err := watchlistService.GetAllActive(ctx)
	if err != nil {
		slog.Error("failed to get watchlists", "error", err)
		os.Exit(1)
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

	// Sentiment results are cached for 5 minutes per symbol to avoid a network
	// round-trip on every bar tick during hot-path signal evaluation.
	sentimentProvider := sentiment.NewCachedProvider(
		sentiment.NewAlpacaNewsProvider(marketDataClient),
		5*time.Minute,
	)

	signalService := service.NewSignalService(signalRepo, tradeRepo, indicatorCache, sentimentProvider)
	tradeService := service.NewTradeService(tradeRepo, accountTransferRepo, signalRepo, transactor, alpacaClient, cfg.RiskPerTradePct, cfg.ATRRiskMultiplier, cfg.TakeProfitMultiplier)
	accountTransferService := service.NewAccountTransferService(accountTransferRepo)

	initialBotState := bot.StateRunning
	if !cfg.BotAutoStart {
		initialBotState = bot.StateStopped
		slog.Info("bot started in stopped state — use POST /bot/start to begin trading")
	}
	botController := bot.NewController(initialBotState)

	// barChan is owned by the stream client; workers are read-only consumers.
	// Buffered to absorb bursts; excess bars are dropped with a log warning.
	barChan := make(chan stream.Bar, 200)

	streamClient := alpaca.NewStocksStreamClient(cfg.AlpacaAPIKey, cfg.AlpacaAPISecret, watchlists, barChan)

	// wg tracks all long-lived goroutines so main can wait for a clean drain on shutdown.
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		connectWithRetry(streamCtx, "bar stream", func() error {
			return streamClient.Connect(streamCtx)
		})
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Watchlist refresh: checks for symbol additions/removals every 30 seconds
	// and seeds the indicator cache for any newly added symbols.
	wg.Add(1)
	go func() {
		defer wg.Done()
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
					slog.Error("watchlist refresh: failed to get watchlists", "error", err)
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
						slog.Error("watchlist refresh: failed to subscribe to new symbols", "symbols", newSymbols, "error", err)
					} else {
						slog.Info("watchlist refresh: subscribed to new symbols", "symbols", newSymbols)
						for _, s := range newSymbols {
							subscribed[s] = struct{}{}
							seedIndicators(s, marketDataClient, indicatorCache)
						}
					}
				}

				if len(removedSymbols) > 0 {
					err = alpaca.UnsubscribeFromBars(streamClient, removedSymbols...)
					if err != nil {
						slog.Error("watchlist refresh: failed to unsubscribe from symbols", "symbols", removedSymbols, "error", err)
					} else {
						slog.Info("watchlist refresh: unsubscribed from symbols", "symbols", removedSymbols)
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
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				slog.Info("daily indicator re-seed starting")
				watchlists, err := watchlistService.GetAllActive(streamCtx)
				if err != nil {
					slog.Error("daily re-seed: failed to get watchlists", "error", err)
					continue
				}
				for _, s := range watchlists {
					seedIndicators(s, marketDataClient, indicatorCache)
				}
				slog.Info("daily indicator re-seed complete")
			case <-streamCtx.Done():
				return
			}
		}
	}()

	const numWorkers = 5
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case bar := <-barChan:
					if !botController.IsActive() {
						continue
					}

					livePrice := decimal.NewFromFloat(bar.Close)

					if err := tradeService.EvaluateAndExecuteExits(streamCtx, bar.Symbol, livePrice); err != nil {
						slog.Error("failed to check exit conditions", "symbol", bar.Symbol, "error", err)
					}

					entrySignal, err := signalService.EvaluateBuySignal(streamCtx, bar.Symbol, livePrice)
					if err != nil {
						slog.Error("failed to evaluate buy signal", "symbol", bar.Symbol, "error", err)
					}

					if entrySignal != nil {
						availableBudget, err := accountTransferService.GetAvailableBudget(streamCtx)
						if err != nil || availableBudget == nil {
							slog.Error("failed to get active account transfer", "error", err)
							continue
						}
						if _, err = tradeService.ExecuteBuyTrade(streamCtx, entrySignal, availableBudget); err != nil {
							slog.Error("failed to execute buy trade", "symbol", entrySignal.Symbol, "error", err)
						}
					}

				case <-streamCtx.Done():
					return
				}
			}
		}()
	}

	tradeUpdateChan := make(chan alpacaSDK.TradeUpdate, 64)

	// Supervised so fill/cancel events are not lost after a disconnect.
	wg.Add(1)
	go func() {
		defer wg.Done()
		connectWithRetry(streamCtx, "trade updates stream", func() error {
			return alpaca.StreamTradeUpdates(streamCtx, alpacaClient, tradeUpdateChan)
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case update := <-tradeUpdateChan:
				status, ok := alpaca.MapAlpacaEventToStatus(update.Event)
				if !ok {
					slog.Warn("unknown alpaca event", "event", update.Event)
					continue
				}
				if err := tradeService.ApplyTradeUpdates(streamCtx, update, status); err != nil {
					slog.Error("failed to handle trade update", "error", err)
				}
			case <-streamCtx.Done():
				return
			}
		}
	}()

	mux := http.NewServeMux()
	botHandlers := bothttp.NewBotHandlers(botController, cfg.BotAPIKey)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, pingCancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer pingCancel()
		if err := pool.Ping(pingCtx); err != nil {
			slog.Warn("health check: DB ping failed", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/bot/start", botHandlers.RequireAPIKey(botHandlers.Start))
	mux.HandleFunc("/bot/pause", botHandlers.RequireAPIKey(botHandlers.Pause))
	mux.HandleFunc("/bot/stop", botHandlers.RequireAPIKey(botHandlers.Stop))
	mux.HandleFunc("/bot/status", botHandlers.RequireAPIKey(botHandlers.Status))

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	slog.Info("server listening", "port", cfg.Port, "bot_state", botController.State())
	<-quit
	slog.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	streamCancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("all workers stopped")
	case <-time.After(10 * time.Second):
		slog.Warn("shutdown timed out waiting for workers to stop")
	}
}
