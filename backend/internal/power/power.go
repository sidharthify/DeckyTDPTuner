package power

import (
	"os"
	"path/filepath"
	"strings"
)

var root = "/sys/class/power_supply"

func read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// OnAC is true if any mains supply says online, ACAD on the deck
func OnAC() bool {
	dirs, _ := filepath.Glob(filepath.Join(root, "*"))
	for _, d := range dirs {
		if read(filepath.Join(d, "type")) != "Mains" {
			continue
		}
		if read(filepath.Join(d, "online")) == "1" {
			return true
		}
	}
	return false
}
