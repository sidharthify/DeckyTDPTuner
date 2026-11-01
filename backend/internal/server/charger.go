package server

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/sidharthify/DeckyTDPTuner/backend/internal/power"
)

type Settings struct {
	FullPower bool `json:"full_power"`
}

func (s *Server) loadSettings() {
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

// caller holds s.mu
func (s *Server) boost() {
	s.gov.Stop()
	if err := s.hw.Set(s.hw.Max); err != nil {
		log.Printf("charger: set %dW: %v", s.hw.Max, err)
	}
	if s.gpu != nil {
		if err := s.gpu.Pin(0); err != nil {
			log.Printf("charger: pin gpu: %v", err)
		}
	}
	s.boosted = true
	log.Printf("charger: plugged in, %dW and gpu pinned", s.hw.Max)
}

// caller holds s.mu
func (s *Server) unboost() {
	if s.gpu != nil {
		if err := s.gpu.Unpin(); err != nil {
			log.Printf("charger: unpin gpu: %v", err)
		}
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
	if s.boosted && s.gpu != nil {
		s.gpu.Unpin()
	}
}
