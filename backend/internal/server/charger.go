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
		return s.govern(s.running, pr)
	}
	if pr.Watts > 0 {
		return s.hw.Set(pr.Watts)
	}
	return s.hw.Restore()
}

func (s *Server) chargerWatts() int {
	if s.set.Watts <= 0 {
		return s.hw.Max
	}
	return s.set.Watts
}

// caller holds s.mu, also used to re-apply when settings change while plugged in
func (s *Server) boost() {
	s.gov.Stop()
	w := s.chargerWatts()
	if err := s.hw.Set(w); err != nil {
		log.Printf("charger: set %dW: %v", w, err)
	}

	if s.gpu != nil && s.set.PinGPU {
		if err := s.gpu.Pin(s.set.GPUClock); err != nil {
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

// SetSettings saves new charger prefs and applies them straight away if plugged in
func (s *Server) SetSettings(n Settings) error {
	if n.Watts < 0 || n.GPUClock < 0 {
		return fmt.Errorf("watts and gpu_clock cant be negative")
	}

	s.mu.Lock()
	s.set = n
	err := s.saveSettings()
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
