package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// steam owns the mangoapp config and rewrites it whenever the perf overlay
// level changes, so the block below has to be put back every now and then

const marker = "# tdptuner"

func mangoappConfig() string {
	procs, _ := filepath.Glob("/proc/[0-9]*/comm")
	for _, p := range procs {
		comm, err := os.ReadFile(p)
		if err != nil || strings.TrimSpace(string(comm)) != "mangoapp" {
			continue
		}
		env, err := os.ReadFile(filepath.Join(filepath.Dir(p), "environ"))
		if err != nil {
			continue
		}
		for _, kv := range bytes.Split(env, []byte{0}) {
			if v, ok := strings.CutPrefix(string(kv), "MANGOHUD_CONFIGFILE="); ok {
				return v
			}
		}
	}

	// fallback, steam puts it here anyway
	m, _ := filepath.Glob("/tmp/mangohud.*")
	if len(m) > 0 {
		return m[0]
	}
	return ""
}

func stripBlock(s string) string {
	var out []string
	skip := 0
	for _, l := range strings.Split(s, "\n") {
		if l == marker {
			skip = 3
			continue
		}
		if skip > 0 {
			skip--
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func EnableLogging(dir string) error {
	cfg := mangoappConfig()
	if cfg == "" {
		return fmt.Errorf("mangoapp config not found, is gamescope running?")
	}

	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}
	// mangoapp runs as deck, not root
	os.Chmod(dir, 0777)

	b, err := os.ReadFile(cfg)
	if err != nil {
		return err
	}
	if strings.Contains(string(b), marker) {
		return nil
	}

	s := strings.TrimRight(string(b), "\n")
	s += fmt.Sprintf("\n%s\noutput_folder=%s\nautostart_log=1\nlog_interval=0\n", marker, dir)
	return os.WriteFile(cfg, []byte(s), 0644)
}

func DisableLogging() error {
	cfg := mangoappConfig()
	if cfg == "" {
		return nil
	}
	b, err := os.ReadFile(cfg)
	if err != nil {
		return err
	}
	if !strings.Contains(string(b), marker) {
		return nil
	}
	return os.WriteFile(cfg, []byte(stripBlock(string(b))), 0644)
}
