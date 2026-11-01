package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Profile struct {
	Watts  int   `json:"watts"`
	Target int   `json:"target"`
	Tuned  int64 `json:"tuned"`
	Live   bool  `json:"live"`
}

type Profiles struct {
	path string
	mu   sync.Mutex
	m    map[string]Profile
}

func LoadProfiles(path string) *Profiles {
	p := &Profiles{path: path, m: map[string]Profile{}}
	b, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(b, &p.m)
	}
	return p
}

func (p *Profiles) save() error {
	b, err := json.MarshalIndent(p.m, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(p.path), 0755)

	// write to tmp first so a crash doesnt eat everything
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

func (p *Profiles) Get(appid string) (Profile, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pr, ok := p.m[appid]
	return pr, ok
}

func (p *Profiles) Set(appid string, watts, target int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	live := p.m[appid].Live
	p.m[appid] = Profile{Watts: watts, Target: target, Tuned: time.Now().Unix(), Live: live}
	return p.save()
}

func (p *Profiles) SetLive(appid string, live bool, target int) (Profile, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pr := p.m[appid]
	pr.Live = live
	if live {
		pr.Target = target
	}
	p.m[appid] = pr
	return pr, p.save()
}

func (p *Profiles) Delete(appid string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.m, appid)
	return p.save()
}

func (p *Profiles) All() map[string]Profile {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]Profile, len(p.m))
	for k, v := range p.m {
		out[k] = v
	}
	return out
}
