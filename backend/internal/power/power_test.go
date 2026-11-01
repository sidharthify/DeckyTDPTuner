package power

import (
	"os"
	"path/filepath"
	"testing"
)

func supply(t *testing.T, name, typ, online string) {
	d := filepath.Join(root, name)
	os.MkdirAll(d, 0755)
	os.WriteFile(filepath.Join(d, "type"), []byte(typ+"\n"), 0644)
	os.WriteFile(filepath.Join(d, "online"), []byte(online+"\n"), 0644)
}

func TestOnAC(t *testing.T) {
	root = t.TempDir()

	supply(t, "BAT1", "Battery", "1")
	supply(t, "ACAD", "Mains", "0")
	if OnAC() {
		t.Fatal("battery online shouldnt count")
	}

	supply(t, "ACAD", "Mains", "1")
	if !OnAC() {
		t.Fatal("ACAD online but not on AC")
	}
}
