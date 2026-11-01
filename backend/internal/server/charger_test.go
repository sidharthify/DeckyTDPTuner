package server

import (
	"testing"

	"github.com/sidharthify/DeckyTDPTuner/backend/internal/gpu"
)

func TestGPUCap(t *testing.T) {
	s := &Server{gpu: &gpu.GPU{Min: 200, Max: 1600}}

	for w, want := range map[int]int{3: 400, 5: 800, 8: 1200, 10: 1600, 15: 1600, 1: 200} {
		if got := s.gpuCap(w); got != want {
			t.Errorf("%dW: got %d want %d", w, got, want)
		}
	}
}
