package bot

import (
	"fmt"
	"sync/atomic"
)

type State int32

const (
	StateRunning State = iota
	StatePaused
	StateStopped
)

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

type Controller struct {
	state int32
}

func NewController(initialState State) *Controller {
	c := &Controller{}
	atomic.StoreInt32(&c.state, int32(initialState))
	return c
}

func (c *Controller) State() State {
	return State(atomic.LoadInt32(&c.state))
}

func (c *Controller) IsActive() bool {
	return c.State() == StateRunning
}

func (c *Controller) Start() error {
	if c.State() == StateRunning {
		return fmt.Errorf("bot is already running")
	}
	atomic.StoreInt32(&c.state, int32(StateRunning))
	return nil
}

func (c *Controller) Pause() error {
	if c.State() != StateRunning {
		return fmt.Errorf("bot must be running to pause; current state: %s", c.State())
	}
	atomic.StoreInt32(&c.state, int32(StatePaused))
	return nil
}

func (c *Controller) Stop() error {
	if c.State() == StateStopped {
		return fmt.Errorf("bot is already stopped")
	}
	atomic.StoreInt32(&c.state, int32(StateStopped))
	return nil
}
