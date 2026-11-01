package tdp

import (
	"sync"
	"testing"
	"time"

	"github.com/sidharthify/DeckyTDPTuner/backend/internal/mangohud"
)

// pretend game that needs 9W for 60fps
// tuner sets it from its own goroutine
type fakeHw struct {
	mu sync.Mutex
	w  int
}

func (f *fakeHw) Set(w int) error {
	f.mu.Lock()
	f.w = w
	f.mu.Unlock()
	return nil
}

func (f *fakeHw) Get() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.w
}

type fakeSrc struct {
	hw *fakeHw
	// frame to frame swing in ms at a given watt
	jit func(w int) float64
}

func (f *fakeSrc) Skip() {
}
func (f *fakeSrc) Read() []mangohud.Sample {
	w := f.hw.Get()
	fps := 60.0
	if w < 9 {
		fps = float64(w) * 6
	}
	j := 0.0
	if f.jit != nil {
		j = f.jit(w)
	}
	out := make([]mangohud.Sample, 10)
	for i := range out {
		ft := 1000 / fps
		if i%2 == 0 {
			ft += j / 2
		} else {
			ft -= j / 2
		}
		out[i] = mangohud.Sample{FPS: fps, Frametime: ft}
	}
	return out
}

func newFake() (*Tuner, *fakeHw) {
	return newFakeJit(nil)
}

func newFakeJit(jit func(w int) float64) (*Tuner, *fakeHw) {
	hw := &fakeHw{w: 15}
	tn := NewTuner(hw, &fakeSrc{hw: hw, jit: jit}, 3, 15)
	tn.Settle = time.Millisecond
	tn.Window = 20 * time.Millisecond
	return tn, hw
}

func wait(tn *Tuner) Status {
	for i := 0; i < 500; i++ {
		s := tn.Status()
		if s.State != "tuning" {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	return tn.Status()
}

func TestSearch(t *testing.T) {
	tn, hw := newFake()
	// done runs after the state flips to done, so wait on it separately
	got := make(chan int, 1)
	tn.Start("123", 60, func(w int) {
		got <- w
	})

	s := wait(tn)
	var w int
	select {
	case w = <-got:
	case <-time.After(time.Second):
		t.Fatal("done never called")
	}
	if s.State != "done" || s.Result != 10 || w != 10 || hw.Get() != 10 {
		t.Fatalf("status %+v, got %d, hw %d", s, w, hw.Get())
	}
}

// passes at 9W but the graph only flattens out at 11W
func TestSearchJagged(t *testing.T) {
	tn, hw := newFakeJit(func(w int) float64 {
		switch w {
		case 9:
			return 4
		case 10:
			return 2.5
		}
		return 0.5
	})
	tn.Start("123", 60, nil)

	s := wait(tn)
	if s.State != "done" || s.Result != 11 || hw.Get() != 11 {
		t.Fatalf("status %+v, hw %d", s, hw.Get())
	}
}

func TestUnreachable(t *testing.T) {
	tn, hw := newFake()
	tn.Start("123", 90, nil)

	s := wait(tn)
	if s.State != "failed" || !s.Unreachable || s.Suggest != 60 || hw.Get() != 15 {
		t.Fatalf("status %+v, hw %d", s, hw.Get())
	}
}

func TestStats(t *testing.T) {
	s := []mangohud.Sample{}
	for i := 0; i < 19; i++ {
		s = append(s, mangohud.Sample{Frametime: 10})
	}
	s = append(s, mangohud.Sample{Frametime: 50})

	avg, low := stats(s)
	if int(avg) != 83 || low != 100 {
		t.Fatalf("avg %v low %v", avg, low)
	}
}
