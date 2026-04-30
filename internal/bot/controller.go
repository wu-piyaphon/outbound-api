package bot

import (
	"fmt"
	"sync/atomic"
)

// State represents the operational state of the trading bot.
type State int32

const (
	StateRunning State = iota // actively processing bars and placing orders
	StatePaused               // stream is live but signal evaluation is skipped
	StateStopped              // stream may still be connected; no orders will fire
)

// String returns the lowercase name of the state, used in logs and HTTP responses.
func (s State) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StatePaused:
		return "paused"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// Controller manages the bot's operational state using lock-free atomic
// compare-and-swap operations so it is safe to call from multiple goroutines.
type Controller struct {
	state int32
}

// NewController returns a Controller initialised to initialState.
func NewController(initialState State) *Controller {
	c := &Controller{}
	atomic.StoreInt32(&c.state, int32(initialState))
	return c
}

// State returns the current operational state.
func (c *Controller) State() State {
	return State(atomic.LoadInt32(&c.state))
}

// IsActive reports whether the bot is in StateRunning and should process bars.
func (c *Controller) IsActive() bool {
	return c.State() == StateRunning
}

// Start transitions the bot from paused or stopped to running.
// Returns an error if the bot is already running.
func (c *Controller) Start() error {
	if atomic.CompareAndSwapInt32(&c.state, int32(StatePaused), int32(StateRunning)) {
		return nil
	}
	if atomic.CompareAndSwapInt32(&c.state, int32(StateStopped), int32(StateRunning)) {
		return nil
	}
	return fmt.Errorf("bot is already running")
}

// Pause transitions the bot from running to paused, halting signal evaluation
// while keeping the stream connection alive. Returns an error if not running.
func (c *Controller) Pause() error {
	if atomic.CompareAndSwapInt32(&c.state, int32(StateRunning), int32(StatePaused)) {
		return nil
	}
	return fmt.Errorf("bot must be running to pause; current state: %s", c.State())
}

// Stop transitions the bot from running or paused to stopped.
// Returns an error if the bot is already stopped.
func (c *Controller) Stop() error {
	if atomic.CompareAndSwapInt32(&c.state, int32(StateRunning), int32(StateStopped)) {
		return nil
	}
	if atomic.CompareAndSwapInt32(&c.state, int32(StatePaused), int32(StateStopped)) {
		return nil
	}
	return fmt.Errorf("bot is already stopped")
}
