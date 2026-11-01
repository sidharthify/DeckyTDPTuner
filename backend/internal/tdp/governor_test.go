package tdp

import (
	"testing"
	"time"

	"github.com/sidharthify/DeckyTDPTuner/backend/internal/mangohud"
)

// 5 rows like mangoapp at log_interval=100, one with a nasty single frame
func frames(fps, gpu float64) []mangohud.Sample {
	out := make([]mangohud.Sample, 5)
	for i := range out {
		out[i] = mangohud.Sample{FPS: fps, Frametime: 1000 / fps, GPULoad: gpu}
	}
	out[2].Frametime *= 3
	return out
}

func tick(g *Governor, now *time.Time, fps, gpu float64) {
	*now = now.Add(500 * time.Millisecond)
	g.step(*now, 60, frames(fps, gpu))
}

func newGov(start, base int) (*Governor, *fakeHw, time.Time) {
	hw := &fakeHw{}
	g := NewGovernor(hw, nil, 3, 15)
	now := time.Unix(1000, 0)
	g.reset(now, start, base)
	return g, hw, now
}

func TestGovLowers(t *testing.T) {
	g, hw, now := newGov(10, 7)

	for i := 0; i < 120; i++ {
		now = now.Add(time.Second)
		g.step(now, 60, frames(60, 50))
	}
	// 1W per 15s, so 10 -> 7 well within 2 minutes, never under the floor
	if hw.Get() != 7 || g.Status().Drops != 3 {
		t.Fatalf("hw %d status %+v", hw.Get(), g.Status())
	}
}

func TestGovBusyGpuHolds(t *testing.T) {
	g, hw, now := newGov(10, 3)

	for i := 0; i < 60; i++ {
		now = now.Add(time.Second)
		g.step(now, 60, frames(60, 97))
	}
	if hw.Get() != 10 {
		t.Fatalf("dropped with no headroom, hw %d", hw.Get())
	}
}

func TestGovIgnoresBlip(t *testing.T) {
	g, hw, now := newGov(8, 3)

	tick(g, &now, 60, 99)
	tick(g, &now, 60, 99)
	tick(g, &now, 40, 99)
	tick(g, &now, 60, 99)
	tick(g, &now, 60, 99)
	if hw.Get() != 8 || g.Status().Raises != 0 {
		t.Fatalf("raised on a blip, hw %d", hw.Get())
	}
}

func TestGovRaises(t *testing.T) {
	g, hw, now := newGov(8, 3)

	tick(g, &now, 54, 99)
	if hw.Get() != 8 {
		t.Fatalf("raised after one tick, hw %d", hw.Get())
	}
	tick(g, &now, 54, 99)
	tick(g, &now, 54, 99)
	tick(g, &now, 54, 99)
	if hw.Get() != 10 {
		t.Fatalf("expected +2, hw %d", hw.Get())
	}

	// way under target, bigger step but not straight to max
	for i := 0; i < 4; i++ {
		tick(g, &now, 30, 99)
	}
	if hw.Get() != 14 {
		t.Fatalf("expected +4, hw %d", hw.Get())
	}
}

func TestGovBlamesDrop(t *testing.T) {
	g, hw, now := newGov(9, 3)

	for i := 0; i < 32; i++ {
		tick(g, &now, 60, 50)
	}
	if hw.Get() != 8 {
		t.Fatalf("expected drop, hw %d", hw.Get())
	}

	tick(g, &now, 54, 99)
	tick(g, &now, 54, 99)
	tick(g, &now, 54, 99)
	tick(g, &now, 54, 99)
	if g.floor != 9 || hw.Get() != 10 {
		t.Fatalf("floor %d hw %d", g.floor, hw.Get())
	}

	// floor decays back after a while with no misses
	for i := 0; i < 600; i++ {
		tick(g, &now, 60, 50)
	}
	if g.floor >= 9 {
		t.Fatalf("floor didnt decay, %d", g.floor)
	}
}

func TestGovMaxed(t *testing.T) {
	g, _, now := newGov(15, 3)

	tick(g, &now, 50, 99)
	tick(g, &now, 50, 99)
	if !g.Status().Maxed {
		t.Fatal("should say max isnt enough")
	}

	for i := 0; i < 4; i++ {
		tick(g, &now, 60, 99)
	}
	if g.Status().Maxed {
		t.Fatal("still maxed after recovering")
	}
}
