package bot

import (
	"sync"
	"testing"
)

func TestNewController_defaultState(t *testing.T) {
	c := NewController(StateRunning)
	if c.State() != StateRunning {
		t.Fatalf("expected running, got %s", c.State())
	}
	if !c.IsActive() {
		t.Fatal("IsActive should be true when running")
	}
}

func TestController_Start(t *testing.T) {
	tests := []struct {
		name        string
		initial     State
		wantErr     bool
		wantState   State
	}{
		{"from stopped", StateStopped, false, StateRunning},
		{"from paused", StatePaused, false, StateRunning},
		{"already running", StateRunning, true, StateRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewController(tt.initial)
			err := c.Start()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}
			if c.State() != tt.wantState {
				t.Fatalf("state = %s, want %s", c.State(), tt.wantState)
			}
		})
	}
}

func TestController_Pause(t *testing.T) {
	tests := []struct {
		name      string
		initial   State
		wantErr   bool
		wantState State
	}{
		{"from running", StateRunning, false, StatePaused},
		{"from stopped", StateStopped, true, StateStopped},
		{"from paused", StatePaused, true, StatePaused},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewController(tt.initial)
			err := c.Pause()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Pause() error = %v, wantErr %v", err, tt.wantErr)
			}
			if c.State() != tt.wantState {
				t.Fatalf("state = %s, want %s", c.State(), tt.wantState)
			}
		})
	}
}

func TestController_Stop(t *testing.T) {
	tests := []struct {
		name      string
		initial   State
		wantErr   bool
		wantState State
	}{
		{"from running", StateRunning, false, StateStopped},
		{"from paused", StatePaused, false, StateStopped},
		{"already stopped", StateStopped, true, StateStopped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewController(tt.initial)
			err := c.Stop()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Stop() error = %v, wantErr %v", err, tt.wantErr)
			}
			if c.State() != tt.wantState {
				t.Fatalf("state = %s, want %s", c.State(), tt.wantState)
			}
		})
	}
}

func TestController_StateString(t *testing.T) {
	cases := map[State]string{
		StateRunning: "running",
		StatePaused:  "paused",
		StateStopped: "stopped",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", state, got, want)
		}
	}
}

func TestController_ConcurrentAccess(t *testing.T) {
	c := NewController(StateRunning)
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				_ = c.IsActive()
			case 1:
				_ = c.State()
			case 2:
				_ = c.Stop()
				_ = c.Start()
			}
		}(i)
	}
	wg.Wait()
}
