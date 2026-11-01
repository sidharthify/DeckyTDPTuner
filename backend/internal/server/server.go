package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/sidharthify/DeckyTDPTuner/backend/internal/gpu"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/hwmon"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/profiles"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/tdp"
)

type Request struct {
	Cmd    string `json:"cmd"`
	AppID  string `json:"appid"`
	Target int    `json:"target"`
	On     bool   `json:"on"`

	Settings *Settings `json:"settings"`
}

type Server struct {
	hw       *hwmon.Hwmon
	tuner    *tdp.Tuner
	gov      *tdp.Governor
	profiles *profiles.Store
	gpu      *gpu.GPU // nil if we couldnt find one

	settingsPath string

	mu      sync.Mutex
	set     Settings
	running string // appid of the game thats up, if any
	ac      bool
	boosted bool // full power on charger is applied right now
	pinned  bool // we're holding the gpu clock
}

func New(hw *hwmon.Hwmon, g *gpu.GPU, tuner *tdp.Tuner, gov *tdp.Governor, profiles *profiles.Store, settingsPath string) *Server {
	s := &Server{hw: hw, gpu: g, tuner: tuner, gov: gov, profiles: profiles, settingsPath: settingsPath}
	s.loadSettings()
	return s
}

func (s *Server) govern(appid string, pr profiles.Profile) error {
	// tuned value is the smooth one now, dont let it go under that
	return s.gov.Start(appid, pr.Target, pr.Watts, pr.Watts)
}

func (s *Server) handle(req Request) (any, error) {
	switch req.Cmd {
	case "status":
		s.mu.Lock()
		charger := map[string]any{
			"ac":       s.ac,
			"boosted":  s.boosted,
			"settings": s.set,
			"watts":    s.chargerWatts(),
			"gpu_min":  0,
			"gpu_max":  0,
			"gpu_cap":  s.gpuCap(s.chargerWatts()),

			"gpu_full_watts": GPUFullWatts,
		}
		s.mu.Unlock()
		if s.gpu != nil {
			charger["gpu_min"] = s.gpu.Min
			charger["gpu_max"] = s.gpu.Max
		}
		out := map[string]any{"status": s.tuner.Status(), "governor": s.gov.Status(), "min": s.hw.Min, "max": s.hw.Max, "charger": charger}
		if pr, ok := s.profiles.Get(req.AppID); ok {
			out["profile"] = pr
		}
		return out, nil

	case "tune":
		if req.AppID == "" || req.Target <= 0 {
			return nil, fmt.Errorf("need appid and target")
		}
		appid, target := req.AppID, req.Target
		s.mu.Lock()
		if s.boosted {
			// dont tune with the gpu pinned, the watcher puts it back after
			s.unboost()
		}
		s.mu.Unlock()
		s.gov.Stop()
		err := s.tuner.Start(appid, target, func(watts int) {
			if err := s.profiles.Set(appid, watts, target); err != nil {
				log.Printf("saving profile for %s: %v", appid, err)
			}
			if pr, _ := s.profiles.Get(appid); pr.Live {
				s.govern(appid, pr)
			}
		})
		return nil, err

	case "stop":
		s.tuner.Stop()
		return nil, nil

	case "apply":
		if s.tuner.Status().State == "tuning" {
			return nil, fmt.Errorf("tuning right now")
		}
		s.mu.Lock()
		s.running = req.AppID
		boosted := s.boosted
		s.mu.Unlock()

		pr, ok := s.profiles.Get(req.AppID)
		if !ok {
			return nil, nil
		}
		// on the charger, unplugging applies it later
		if boosted {
			return pr, nil
		}
		if pr.Live {
			return pr, s.govern(req.AppID, pr)
		}
		// live got turned off before it was ever tuned
		if pr.Watts == 0 {
			return nil, nil
		}
		log.Printf("applying %dW for %s", pr.Watts, req.AppID)
		return pr, s.hw.Set(pr.Watts)

	case "live":
		if req.AppID == "" || (req.On && req.Target <= 0) {
			return nil, fmt.Errorf("need appid and target")
		}
		pr, err := s.profiles.SetLive(req.AppID, req.On, req.Target)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		boosted := s.boosted
		s.mu.Unlock()
		// tuner starts it when its done, and the charger does on unplug
		if s.tuning() || boosted {
			return pr, nil
		}
		if req.On {
			return pr, s.govern(req.AppID, pr)
		}
		s.gov.Stop()
		if pr.Watts > 0 {
			return pr, s.hw.Set(pr.Watts)
		}
		return pr, nil

	case "reset":
		if s.tuner.Status().State == "tuning" {
			return nil, fmt.Errorf("tuning right now")
		}
		s.gov.Stop()
		s.mu.Lock()
		s.running = ""
		boosted := s.boosted
		s.mu.Unlock()
		// stay at full power while plugged in, game or not
		if boosted {
			return nil, nil
		}
		return nil, s.hw.Restore()

	case "settings":
		if req.Settings == nil {
			return nil, fmt.Errorf("need settings")
		}
		return nil, s.SetSettings(*req.Settings)

	case "forget":
		if s.gov.Status().AppID == req.AppID {
			s.gov.Stop()
		}
		return nil, s.profiles.Delete(req.AppID)

	case "profiles":
		return s.profiles.All(), nil
	}
	return nil, fmt.Errorf("unknown cmd %q", req.Cmd)
}

func (s *Server) conn(c net.Conn) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	enc := json.NewEncoder(c)

	for sc.Scan() {
		var req Request
		resp := map[string]any{"ok": true}

		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			resp["ok"] = false
			resp["error"] = err.Error()
		} else if data, err := s.handle(req); err != nil {
			resp["ok"] = false
			resp["error"] = err.Error()
		} else {
			resp["data"] = data
		}
		enc.Encode(resp)
	}
}

func (s *Server) Serve(path string) error {
	os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	log.Printf("listening on %s", path)

	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go s.conn(c)
	}
}
