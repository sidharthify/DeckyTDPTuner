package server

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/sidharthify/DeckyTDPTuner/backend/internal/gpu"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/hwmon"
)

func TestGPUCap(t *testing.T) {
	s := &Server{gpu: &gpu.GPU{Min: 200, Max: 1600}}

	for w, want := range map[int]int{3: 400, 5: 800, 8: 1200, 10: 1600, 15: 1600, 1: 200} {
		if got := s.gpuCap(w); got != want {
			t.Errorf("%dW: got %d want %d", w, got, want)
		}
	}
}

func fakeHw(t *testing.T, watts int) *hwmon.Hwmon {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "power1_cap"), []byte(strconv.Itoa(watts*1000000)), 0644)
	return &hwmon.Hwmon{Dir: d, Min: 4, Max: 15}
}

func waitWatts(hw *hwmon.Hwmon, want int) int {
	for i := 0; i < 200; i++ {
		if hw.Get() == want {
			break
		}
		time.Sleep(time.Millisecond)
	}
	return hw.Get()
}

func TestRampDown(t *testing.T) {
	hw := fakeHw(t, 15)
	s := &Server{hw: hw, rampStep: time.Millisecond}

	s.mu.Lock()
	s.rampTo(5)
	s.mu.Unlock()

	if got := waitWatts(hw, 5); got != 5 {
		t.Fatalf("ramp ended at %dW", got)
	}
}

func TestRampCancel(t *testing.T) {
	hw := fakeHw(t, 15)
	s := &Server{hw: hw, rampStep: 20 * time.Millisecond}

	s.mu.Lock()
	s.rampTo(5)
	s.mu.Unlock()

	time.Sleep(50 * time.Millisecond)
	s.mu.Lock()
	s.cancelRamp()
	s.mu.Unlock()
	stopped := hw.Get()

	time.Sleep(100 * time.Millisecond)
	if hw.Get() != stopped || stopped <= 5 {
		t.Fatalf("kept going after cancel, %dW -> %dW", stopped, hw.Get())
	}
}
