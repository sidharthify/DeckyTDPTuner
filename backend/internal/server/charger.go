package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/sidharthify/DeckyTDPTuner/backend/internal/power"
)

type Settings struct {
	FullPower bool `json:"full_power"`
	// 0 means max for both
	Watts    int  `json:"watts"`
	GPUClock int  `json:"gpu_clock"`
	PinGPU   bool `json:"pin_gpu"`

	// lowest tdp tuning and live mode will use, 0 means DefaultMin
	MinWatts int `json:"min_watts"`
}

func (s *Server) loadSettings() {
	s.set = Settings{PinGPU: true}
	b, err := os.ReadFile(s.settingsPath)
	if err == nil {
		json.Unmarshal(b, &s.set)
	}
}

func (s *Server) saveSettings() error {
	b, err := json.MarshalIndent(s.set, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.settingsPath, b, 0644)
}

func (s *Server) tuning() bool {
	return s.tuner.Status().State == "tuning"
}

// put back whatever the running game would normally get. caller holds s.mu
func (s *Server) restoreGame() error {
	pr, ok := s.profiles.Get(s.running)
	if s.running == "" || !ok {
		return s.hw.Restore()
	}
	if pr.Live {
		// start from where the charger had it and let it come down on its own,
		// jumping straight to tuned watts in a heavy area is a nasty stutter
		start := s.chargerWatts()
		if start < pr.Watts {
			start = pr.Watts
		}
		return s.gov.Start(s.running, pr.Target, start, pr.Watts, s.floorWatts())
	}
	if pr.Watts > 0 {
		return s.rampTo(s.atLeastFloor(pr.Watts))
	}
	return s.hw.Restore()
}

// rampTo walks the cap down 1W per step instead of dropping it all at once.
// anything else that sets watts bumps rampGen, which stops it. caller holds s.mu
func (s *Server) rampTo(target int) error {
	s.rampGen++
	gen := s.rampGen
	step := s.rampStep
	if step == 0 {
		step = 500 * time.Millisecond
	}
	from := s.hw.Get()
	if from <= target {
		return s.hw.Set(target)
	}

	go func() {
		for w := from - 1; w >= target; w-- {
			time.Sleep(step)
			s.mu.Lock()
			if s.rampGen != gen || s.boosted {
				s.mu.Unlock()
				return
			}
			if err := s.hw.Set(w); err != nil {
				log.Printf("ramp: set %dW: %v", w, err)
			}
			s.mu.Unlock()
		}
	}()
	return nil
}

// caller holds s.mu
func (s *Server) cancelRamp() {
	s.rampGen++
}

// under this the pinned clock gets scaled down with tdp, 1600MHz at 3W
// just starves the cpu since they share the same budget
const GPUFullWatts = 10

func (s *Server) gpuCap(watts int) int {
	if s.gpu == nil {
		return 0
	}
	c := s.gpu.Max * watts / GPUFullWatts / 100 * 100
	if c > s.gpu.Max {
		c = s.gpu.Max
	}
	if c < s.gpu.Min {
		c = s.gpu.Min
	}
	return c
}

func (s *Server) floorWatts() int {
	w := s.set.MinWatts
	if w <= 0 {
		w = s.DefaultMin
	}
	if w < s.hw.Min {
		w = s.hw.Min
	}
	if w > s.hw.Max {
		w = s.hw.Max
	}
	return w
}

// saved values from before the floor got raised shouldnt sneak under it
func (s *Server) atLeastFloor(w int) int {
	if f := s.floorWatts(); w < f {
		return f
	}
	return w
}

func (s *Server) chargerWatts() int {
	if s.set.Watts <= 0 {
		return s.hw.Max
	}
	return s.set.Watts
}

// caller holds s.mu, also used to re-apply when settings change while plugged in
func (s *Server) boost() {
	s.cancelRamp()
	s.gov.Stop()
	w := s.chargerWatts()
	if err := s.hw.Set(w); err != nil {
		log.Printf("charger: set %dW: %v", w, err)
	}

	if s.gpu != nil && s.set.PinGPU {
		clk := s.set.GPUClock
		limit := s.gpuCap(w)
		if clk <= 0 || clk > limit {
			clk = limit
		}
		if err := s.gpu.Pin(clk); err != nil {
			log.Printf("charger: pin gpu: %v", err)
		} else {
			s.pinned = true
		}
	} else if s.pinned {
		s.gpu.Unpin()
		s.pinned = false
	}
	s.boosted = true
	log.Printf("charger: plugged in, %dW, gpu pinned: %v", w, s.pinned)
}

// caller holds s.mu
func (s *Server) unboost() {
	if s.pinned {
		if err := s.gpu.Unpin(); err != nil {
			log.Printf("charger: unpin gpu: %v", err)
		}
		s.pinned = false
	}
	s.boosted = false
	log.Printf("charger: back to normal")

	// tuner owns the cap right now
	if s.tuning() {
		return
	}
	if err := s.restoreGame(); err != nil {
		log.Printf("charger: restore: %v", err)
	}
}

func (s *Server) checkCharger() {
	ac := power.OnAC()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ac = ac
	want := s.set.FullPower && ac && !s.tuning()
	if want && !s.boosted {
		s.boost()
	} else if !want && s.boosted {
		s.unboost()
	}
}

// puts a changed floor into effect for the running game. caller holds s.mu
func (s *Server) applyFloor() error {
	if s.boosted || s.tuning() {
		return nil
	}
	gs := s.gov.Status()
	if gs.Running {
		pr, _ := s.profiles.Get(gs.AppID)
		return s.gov.Start(gs.AppID, gs.Target, gs.Watts, pr.Watts, s.floorWatts())
	}
	pr, ok := s.profiles.Get(s.running)
	if s.running == "" || !ok || pr.Live || pr.Watts == 0 {
		return nil
	}
	s.cancelRamp()
	return s.hw.Set(s.atLeastFloor(pr.Watts))
}

// SetSettings merges whatever fields are in patch into the saved settings, so
// two parts of the panel cant overwrite each other with stale values. applies
// straight away if plugged in
func (s *Server) SetSettings(patch json.RawMessage) error {
	s.mu.Lock()
	oldFloor := s.floorWatts()
	n := s.set
	if err := json.Unmarshal(patch, &n); err != nil {
		s.mu.Unlock()
		return err
	}
	if n.Watts < 0 || n.GPUClock < 0 || n.MinWatts < 0 {
		s.mu.Unlock()
		return fmt.Errorf("watts, gpu_clock and min_watts cant be negative")
	}

	s.set = n
	err := s.saveSettings()
	if s.floorWatts() != oldFloor {
		if ferr := s.applyFloor(); ferr != nil {
			log.Printf("applying new floor: %v", ferr)
		}
	}
	if s.boosted && s.set.FullPower {
		s.boost()
	}
	s.mu.Unlock()

	s.checkCharger()
	return err
}

func (s *Server) WatchCharger() {
	for {
		s.checkCharger()
		time.Sleep(2 * time.Second)
	}
}

// Close lets go of the gpu clock if we were holding it
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pinned {
		s.gpu.Unpin()
	}
}
