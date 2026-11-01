package tdp

import (
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/mangohud"

	"testing"
	"time"
)

func frames(fps, gpu float64) []mangohud.Sample {
	out := make([]mangohud.Sample, 30)
	for i := range out {
		out[i] = mangohud.Sample{FPS: fps, Frametime: 1000 / fps, GPULoad: gpu}
	}
	return out
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
	if hw.w != 7 || g.Status().Drops != 3 {
		t.Fatalf("hw %d status %+v", hw.w, g.Status())
	}
}

func TestGovBusyGpuHolds(t *testing.T) {
	g, hw, now := newGov(10, 3)

	for i := 0; i < 60; i++ {
		now = now.Add(time.Second)
		g.step(now, 60, frames(60, 97))
	}
	if hw.w != 10 {
		t.Fatalf("dropped with no headroom, hw %d", hw.w)
	}
}

func TestGovRaises(t *testing.T) {
	g, hw, now := newGov(8, 3)

	now = now.Add(time.Second)
	g.step(now, 60, frames(55, 99))
	if hw.w != 10 {
		t.Fatalf("expected +2, hw %d", hw.w)
	}

	// raised just now, give it a sec
	now = now.Add(200 * time.Millisecond)
	g.step(now, 60, frames(55, 99))
	if hw.w != 10 {
		t.Fatalf("raised too soon, hw %d", hw.w)
	}

	now = now.Add(time.Second)
	g.step(now, 60, frames(30, 99))
	if hw.w != 15 {
		t.Fatalf("expected jump to max, hw %d", hw.w)
	}
}

func TestGovBlamesDrop(t *testing.T) {
	g, hw, now := newGov(9, 3)

	now = now.Add(16 * time.Second)
	g.step(now, 60, frames(60, 50))
	if hw.w != 8 {
		t.Fatalf("expected drop, hw %d", hw.w)
	}

	now = now.Add(2 * time.Second)
	g.step(now, 60, frames(54, 99))
	if g.floor != 9 || hw.w != 10 {
		t.Fatalf("floor %d hw %d", g.floor, hw.w)
	}

	// floor decays back after a while with no misses
	for i := 0; i < 300; i++ {
		now = now.Add(time.Second)
		g.step(now, 60, frames(60, 50))
	}
	if g.floor >= 9 {
		t.Fatalf("floor didnt decay, %d", g.floor)
	}
}
