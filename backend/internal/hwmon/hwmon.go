package hwmon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// sysfs takes microwatts
const uw = 1000000

type Hwmon struct {
	Dir  string
	Fast bool // power2_cap, the deck has both

	Min int
	Max int

	orig1 int
	orig2 int
}

func readInt(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

func writeInt(path string, v int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(v)), 0644)
}

func Find() (*Hwmon, error) {
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, d := range dirs {
		name, err := os.ReadFile(filepath.Join(d, "name"))
		if err != nil || strings.TrimSpace(string(name)) != "amdgpu" {
			continue
		}
		if h, err := Open(d); err == nil {
			return h, nil
		}
	}
	return nil, fmt.Errorf("no amdgpu hwmon with power1_cap")
}

// Open uses d as is, whatever the cap is right now counts as the original
func Open(d string) (*Hwmon, error) {
	if _, err := os.Stat(filepath.Join(d, "power1_cap")); err != nil {
		return nil, err
	}

	h := &Hwmon{Dir: d, Min: 3, Max: 15}
	if v, err := readInt(filepath.Join(d, "power1_cap_min")); err == nil && v > 0 {
		h.Min = v / uw
	}
	if v, err := readInt(filepath.Join(d, "power1_cap_max")); err == nil && v > 0 {
		h.Max = v / uw
	}

	h.orig1, _ = readInt(filepath.Join(d, "power1_cap"))
	if v, err := readInt(filepath.Join(d, "power2_cap")); err == nil {
		h.Fast = true
		h.orig2 = v
	}
	return h, nil
}

func (h *Hwmon) Get() int {
	v, err := readInt(filepath.Join(h.Dir, "power1_cap"))
	if err != nil {
		return 0
	}
	return v / uw
}

func (h *Hwmon) Set(watts int) error {
	if watts < h.Min {
		watts = h.Min
	}
	if watts > h.Max {
		watts = h.Max
	}

	if err := writeInt(filepath.Join(h.Dir, "power1_cap"), watts*uw); err != nil {
		return err
	}
	if h.Fast {
		// keep fast limit the same as slow, otherwise it boosts past it for a few secs
		if err := writeInt(filepath.Join(h.Dir, "power2_cap"), watts*uw); err != nil {
			return err
		}
	}
	return nil
}

func (h *Hwmon) Restore() error {
	if h.orig1 == 0 {
		return nil
	}
	if err := writeInt(filepath.Join(h.Dir, "power1_cap"), h.orig1); err != nil {
		return err
	}
	if h.Fast && h.orig2 != 0 {
		return writeInt(filepath.Join(h.Dir, "power2_cap"), h.orig2)
	}
	return nil
}

// Orig is what the cap was before we touched it, in watts
func (h *Hwmon) Orig() int {
	return h.orig1 / uw
}
