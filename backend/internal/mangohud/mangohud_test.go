package mangohud

import (
	"os"
	"path/filepath"
	"testing"
)

const header = `os,cpu,gpu,ram,kernel,driver,cpuscheduler
SteamOS Holo,AMD Custom APU 0405,AMD Custom GPU 0405 (RADV VANGOGH),14830584,6.5.0-valve22-1-neptune-65,Mesa 24.1.3,
--------------------FRAME METRICS--------------------
fps,frametime,cpu_load,gpu_load,cpu_temp,gpu_temp,gpu_core_clock,gpu_mem_clock,gpu_vram_used,gpu_power,ram_used,swap_used,process_rss,elapsed
`

func TestLogTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game_2026-11-01_12-00-00.csv")

	os.WriteFile(path, []byte(header+"59.9,16.7,30,80,60,55,1600,1600,1.2,9,5.1,0,2.1,100000\n60.1,16.6"), 0644)

	lt := NewLogTail(dir)
	s := lt.Read()
	if len(s) != 1 || s[0].FPS != 59.9 || s[0].Frametime != 16.7 || s[0].GPULoad != 80 {
		t.Fatalf("got %+v", s)
	}

	// finish the half line
	fd, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	fd.WriteString(",31,81,60,55,1600,1600,1.2,9,5.1,0,2.1,200000\n")
	fd.Close()

	s = lt.Read()
	if len(s) != 1 || s[0].FPS != 60.1 {
		t.Fatalf("got %+v", s)
	}

	if s = lt.Read(); len(s) != 0 {
		t.Fatalf("expected nothing, got %+v", s)
	}
}

func TestStripBlock(t *testing.T) {
	in := "fps\nframe_timing\n# tdptuner\noutput_folder=/tmp/x\nautostart_log=1\nlog_interval=0\n"
	if got := stripBlock(in); got != "fps\nframe_timing\n" {
		t.Fatalf("got %q", got)
	}
}
