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

type fakeSrc struct{ hw *fakeHw }

func (f *fakeSrc) Skip() {
}
func (f *fakeSrc) Read() []mangohud.Sample {
	fps := 60.0
	if w := f.hw.Get(); w < 9 {
		fps = float64(w) * 6
	}
	out := make([]mangohud.Sample, 10)
	for i := range out {
		out[i] = mangohud.Sample{FPS: fps, Frametime: 1000 / fps}
	}
	return out
}

func newFake() (*Tuner, *fakeHw) {
	hw := &fakeHw{w: 15}
	tn := NewTuner(hw, &fakeSrc{hw}, 3, 15)
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

func TestUnreachable(t *testing.T) {
	tn, hw := newFake()
	tn.Start("123", 90, nil)

	s := wait(tn)
	if s.State != "failed" || hw.Get() != 15 {
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
