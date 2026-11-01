package gpu

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type GPU struct {
	Dir string
	// sclk range from OD_RANGE, mhz
	Min, Max int
}

func Find() (*GPU, error) {
	cards, _ := filepath.Glob("/sys/class/drm/card*/device/pp_od_clk_voltage")
	for _, f := range cards {
		g := &GPU{Dir: filepath.Dir(f)}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		g.Min, g.Max = parseRange(string(b))
		if g.Max == 0 {
			continue
		}
		return g, nil
	}
	return nil, fmt.Errorf("no amdgpu card with pp_od_clk_voltage")
}

// looks for "SCLK:     200Mhz       1600Mhz" under OD_RANGE
func parseRange(s string) (int, int) {
	inRange := false
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "OD_RANGE") {
			inRange = true
			continue
		}
		if !inRange || !strings.HasPrefix(l, "SCLK:") {
			continue
		}
		f := strings.Fields(strings.TrimPrefix(l, "SCLK:"))
		if len(f) < 2 {
			return 0, 0
		}
		lo, _ := strconv.Atoi(strings.TrimSuffix(strings.ToLower(f[0]), "mhz"))
		hi, _ := strconv.Atoi(strings.TrimSuffix(strings.ToLower(f[1]), "mhz"))
		return lo, hi
	}
	return 0, 0
}

func (g *GPU) write(name, v string) error {
	return os.WriteFile(filepath.Join(g.Dir, name), []byte(v), 0644)
}

// Pin locks min and max sclk to mhz, same thing steam's manual gpu clock does
func (g *GPU) Pin(mhz int) error {
	if mhz <= 0 || mhz > g.Max {
		mhz = g.Max
	}
	if err := g.write("power_dpm_force_performance_level", "manual"); err != nil {
		return err
	}
	for _, cmd := range []string{
		fmt.Sprintf("s 0 %d\n", mhz),
		fmt.Sprintf("s 1 %d\n", mhz),
		"c\n",
	} {
		if err := g.write("pp_od_clk_voltage", cmd); err != nil {
			return fmt.Errorf("%q: %w", strings.TrimSpace(cmd), err)
		}
	}
	return nil
}

func (g *GPU) Unpin() error {
	// reset the od table first or auto keeps whatever we pinned
	g.write("pp_od_clk_voltage", "r\n")
	g.write("pp_od_clk_voltage", "c\n")
	return g.write("power_dpm_force_performance_level", "auto")
}
