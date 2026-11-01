package main

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Sample struct {
	FPS       float64
	Frametime float64 // ms
	GPULoad   float64 // percent, -1 if the log doesnt have it
}

// LogTail follows the newest mangohud csv in a folder
type LogTail struct {
	dir string

	file    string
	off     int64
	partial string
	fpsCol  int
	ftCol   int
	gpuCol  int
}

func NewLogTail(dir string) *LogTail {
	return &LogTail{dir: dir, fpsCol: -1, ftCol: -1, gpuCol: -1}
}

func (t *LogTail) newest() string {
	files, _ := filepath.Glob(filepath.Join(t.dir, "*.csv"))
	best := ""
	var bestTime time.Time
	for _, f := range files {
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

// Skip jumps to the end of whatever is there now, so old frames dont count
func (t *LogTail) Skip() {
	t.Read()
}

func (t *LogTail) Read() []Sample {
	f := t.newest()
	if f == "" {
		return nil
	}
	if f != t.file {
		t.file = f
		t.off = 0
		t.partial = ""
		t.fpsCol, t.ftCol, t.gpuCol = -1, -1, -1
	}

	fd, err := os.Open(f)
	if err != nil {
		return nil
	}
	defer fd.Close()

	if _, err := fd.Seek(t.off, io.SeekStart); err != nil {
		return nil
	}
	b, err := io.ReadAll(fd)
	if err != nil {
		return nil
	}
	t.off += int64(len(b))

	return t.parse(string(b))
}

func (t *LogTail) parse(chunk string) []Sample {
	data := t.partial + chunk
	lines := strings.Split(data, "\n")
	// last one might be half written
	t.partial = lines[len(lines)-1]
	lines = lines[:len(lines)-1]

	var out []Sample
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		cols := strings.Split(l, ",")

		if cols[0] == "fps" {
			t.fpsCol, t.ftCol, t.gpuCol = -1, -1, -1
			for i, c := range cols {
				switch c {
				case "fps":
					t.fpsCol = i
				case "frametime":
					t.ftCol = i
				case "gpu_load":
					t.gpuCol = i
				}
			}
			continue
		}
		if t.fpsCol < 0 || t.ftCol < 0 || len(cols) <= t.fpsCol || len(cols) <= t.ftCol {
			continue
		}

		fps, err1 := strconv.ParseFloat(cols[t.fpsCol], 64)
		ft, err2 := strconv.ParseFloat(cols[t.ftCol], 64)
		if err1 != nil || err2 != nil || ft <= 0 {
			continue
		}
		gpu := -1.0
		if t.gpuCol >= 0 && len(cols) > t.gpuCol {
			if v, err := strconv.ParseFloat(cols[t.gpuCol], 64); err == nil {
				gpu = v
			}
		}
		out = append(out, Sample{FPS: fps, Frametime: ft, GPULoad: gpu})
	}
	return out
}
