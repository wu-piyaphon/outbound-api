package alpaca

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

// StreamSupervisor owns a single Alpaca stocks bar stream across reconnects.
// The Alpaca SDK rejects a second Connect() on the same client instance, so
// the supervisor builds a fresh *stream.StocksClient on every connect attempt
// and seeds it with the current desired symbol set. Subscription changes
// from callers update that set and, if a client is currently live, are
// applied immediately; otherwise they take effect on the next reconnect.
type StreamSupervisor struct {
	apiKey    string
	apiSecret string
	barChan   chan<- stream.Bar

	mu      sync.Mutex
	client  *stream.StocksClient
	symbols map[string]struct{}
}

// NewStreamSupervisor creates a supervisor seeded with initialSymbols. The
// caller owns barChan and must not close it while Run is active.
func NewStreamSupervisor(apiKey, apiSecret string, initialSymbols []string, barChan chan<- stream.Bar) *StreamSupervisor {
	symbols := make(map[string]struct{}, len(initialSymbols))
	for _, s := range initialSymbols {
		symbols[s] = struct{}{}
	}
	return &StreamSupervisor{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		barChan:   barChan,
		symbols:   symbols,
	}
}

// Run blocks supervising the bar stream until ctx is cancelled.
//
// The Alpaca SDK's Connect returns nil once the websocket handshake completes
// and then runs the stream in background goroutines; the actual termination
// error arrives on Terminated(). Treating Connect's nil return as "stream
// ended" would open a second client while the first is still live and trip
// Alpaca's one-connection-per-key limit.
func (s *StreamSupervisor) Run(ctx context.Context) {
	ConnectWithRetry(ctx, "bar stream", func() error {
		client := newStocksStreamClient(s.apiKey, s.apiSecret, s.Symbols(), s.barChan)
		if err := client.Connect(ctx); err != nil {
			return err
		}
		s.setClient(client)
		defer s.setClient(nil)
		select {
		case err := <-client.Terminated():
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

// Symbols returns a snapshot of the currently subscribed symbols.
func (s *StreamSupervisor) Symbols() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.symbols))
	for sym := range s.symbols {
		out = append(out, sym)
	}
	return out
}

// Subscribe adds symbols to the desired set. If a client is live the SDK is
// asked to subscribe immediately; otherwise the symbols are picked up on the
// next reconnect.
func (s *StreamSupervisor) Subscribe(symbols ...string) error {
	s.mu.Lock()
	for _, sym := range symbols {
		s.symbols[sym] = struct{}{}
	}
	c := s.client
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	return subscribeToBars(c, s.barChan, symbols...)
}

// Unsubscribe removes symbols from the desired set, applying the change to
// the live client if one is connected.
func (s *StreamSupervisor) Unsubscribe(symbols ...string) error {
	s.mu.Lock()
	for _, sym := range symbols {
		delete(s.symbols, sym)
	}
	c := s.client
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	return unsubscribeFromBars(c, symbols...)
}

func (s *StreamSupervisor) setClient(c *stream.StocksClient) {
	s.mu.Lock()
	s.client = c
	s.mu.Unlock()
}

// ConnectWithRetry runs connectFn in a loop, backing off exponentially after
// each failure. It returns only when ctx is cancelled.
//
// After any return from connectFn, ctx is checked first — both nil and error
// returns can result from context cancellation. If nil is returned without
// context cancellation (unexpected SDK behaviour), the backoff is reset and
// the connection is retried immediately so the supervisor does not stop
// silently.
func ConnectWithRetry(ctx context.Context, name string, connectFn func() error) {
	const maxDelay = 64 * time.Second
	delay := time.Second

	for {
		slog.Info("connecting", "stream", name)
		err := connectFn()

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
			slog.Warn("stream closed without error; reconnecting immediately", "stream", name)
			delay = time.Second
		}
	}
}
