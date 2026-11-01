package mangohud

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

	// fallback, steam puts it here anyway. old sessions leave theirs behind
	m, _ := filepath.Glob("/tmp/mangohud.*")
	best := ""
	var bestTime time.Time
	for _, f := range m {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		if st.ModTime().After(bestTime) {
			best = f
			bestTime = st.ModTime()
		}
	}
	return best
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

// interval is in ms, 0 logs every frame
func EnableLogging(dir string, interval int) error {
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
	block := fmt.Sprintf("%s\noutput_folder=%s\nautostart_log=1\nlog_interval=%d\n", marker, dir, interval)
	if strings.Contains(string(b), block) {
		return nil
	}

	// might be there with a different interval
	s := strings.TrimRight(stripBlock(string(b)), "\n")
	s += "\n" + block
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
