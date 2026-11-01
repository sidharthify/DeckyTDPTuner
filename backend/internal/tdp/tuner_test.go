package tdp

import (
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/mangohud"

	"testing"
	"time"
)

// pretend game that needs 9W for 60fps
type fakeHw struct{ w int }

func (f *fakeHw) Set(w int) error {
	f.w = w
	return nil
}

func (f *fakeHw) Get() int {
	return f.w
}

type fakeSrc struct{ hw *fakeHw }

func (f *fakeSrc) Skip() {
}
func (f *fakeSrc) Read() []mangohud.Sample {
	fps := 60.0
	if f.hw.w < 9 {
		fps = float64(f.hw.w) * 6
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
	got := 0
	tn.Start("123", 60, func(w int) { got = w })

	s := wait(tn)
	if s.State != "done" || s.Result != 10 || got != 10 || hw.w != 10 {
		t.Fatalf("status %+v, got %d, hw %d", s, got, hw.w)
	}
}

func TestUnreachable(t *testing.T) {
	tn, hw := newFake()
	tn.Start("123", 90, nil)

	s := wait(tn)
	if s.State != "failed" || hw.w != 15 {
		t.Fatalf("status %+v, hw %d", s, hw.w)
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
