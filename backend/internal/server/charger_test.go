package server

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/sidharthify/DeckyTDPTuner/backend/internal/gpu"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/hwmon"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/mangohud"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/profiles"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/tdp"
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

func TestSettingsPatch(t *testing.T) {
	s := &Server{hw: fakeHw(t, 10), settingsPath: filepath.Join(t.TempDir(), "settings.json")}
	s.set = Settings{FullPower: true, Watts: 12, PinGPU: true}

	if err := s.SetSettings([]byte(`{"gpu_clock": 1200}`)); err != nil {
		t.Fatal(err)
	}
	if !s.set.FullPower || s.set.Watts != 12 || s.set.GPUClock != 1200 || !s.set.PinGPU {
		t.Fatalf("patch clobbered other fields: %+v", s.set)
	}

	if err := s.SetSettings([]byte(`{"watts": -1}`)); err == nil || s.set.Watts != 12 {
		t.Fatalf("negative watts went through: %+v", s.set)
	}
}

type noFrames struct{}

func (noFrames) Skip() {
}

func (noFrames) Read() []mangohud.Sample {
	return nil
}

// turning live off mid game shouldnt throw away the floor it was running with
func TestLiveOffKeepsFloor(t *testing.T) {
	hw := fakeHw(t, 15)
	gov := tdp.NewGovernor(hw, noFrames{}, 4, 15)
	pr := profiles.Load(filepath.Join(t.TempDir(), "profiles.json"))
	pr.Set("123", 5, 60)
	pr.SetLive("123", true, 60)

	s := &Server{hw: hw, gov: gov, profiles: pr, tuner: tdp.NewTuner(hw, noFrames{}, 4, 15)}
	s.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	s.set.MinWatts = 7

	if _, err := s.handle(Request{Cmd: "apply", AppID: "123"}); err != nil {
		t.Fatal(err)
	}
	if f := gov.Status().Floor; f != 7 {
		t.Fatalf("governor not running at the 7W floor, floor %d", f)
	}

	// floor setting goes back to default, the governor still knows 7W was needed
	s.set.MinWatts = 0
	s.DefaultMin = 4

	if _, err := s.handle(Request{Cmd: "live", AppID: "123", On: false}); err != nil {
		t.Fatal(err)
	}
	if hw.Get() != 7 {
		t.Fatalf("live off went to %dW, wanted the 7W floor", hw.Get())
	}
}
