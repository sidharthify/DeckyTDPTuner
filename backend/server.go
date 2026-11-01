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
}

type Server struct {
	hw       *Hwmon
	tuner    *Tuner
	profiles *Profiles
}

func (s *Server) handle(req Request) (any, error) {
	switch req.Cmd {
	case "status":
		out := map[string]any{"status": s.tuner.Status(), "min": s.hw.Min, "max": s.hw.Max}
		if pr, ok := s.profiles.Get(req.AppID); ok {
			out["profile"] = pr
		}
		return out, nil

	case "tune":
		if req.AppID == "" || req.Target <= 0 {
			return nil, fmt.Errorf("need appid and target")
		}
		appid, target := req.AppID, req.Target
		err := s.tuner.Start(appid, target, func(watts int) {
			if err := s.profiles.Set(appid, watts, target); err != nil {
				log.Printf("saving profile for %s: %v", appid, err)
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
		log.Printf("applying %dW for %s", pr.Watts, req.AppID)
		return pr, s.hw.Set(pr.Watts)

	case "reset":
		if s.tuner.Status().State == "tuning" {
			return nil, fmt.Errorf("tuning right now")
		}
		return nil, s.hw.Restore()

	case "forget":
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
