package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
)

type Request struct {
	Cmd    string `json:"cmd"`
	AppID  string `json:"appid"`
	Target int    `json:"target"`
	On     bool   `json:"on"`
}

type Server struct {
	hw       *Hwmon
	tuner    *Tuner
	gov      *Governor
	profiles *Profiles
}

func (s *Server) govern(appid string, pr Profile) error {
	// tuned value already has 1W of headroom on top, the floor doesnt need it
	base := pr.Watts - 1
	return s.gov.Start(appid, pr.Target, pr.Watts, base)
}

func (s *Server) handle(req Request) (any, error) {
	switch req.Cmd {
	case "status":
		out := map[string]any{"status": s.tuner.Status(), "governor": s.gov.Status(), "min": s.hw.Min, "max": s.hw.Max}
		if pr, ok := s.profiles.Get(req.AppID); ok {
			out["profile"] = pr
		}
		return out, nil

	case "tune":
		if req.AppID == "" || req.Target <= 0 {
			return nil, fmt.Errorf("need appid and target")
		}
		appid, target := req.AppID, req.Target
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
		pr, ok := s.profiles.Get(req.AppID)
		if !ok {
			return nil, nil
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
		if s.tuner.Status().State == "tuning" {
			// tuner starts it when its done
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
		return nil, s.hw.Restore()

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
