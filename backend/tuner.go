package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

type capper interface {
	Set(watts int) error
	Get() int
}

type frameSource interface {
	Skip()
	Read() []Sample
}

type Status struct {
	State   string  `json:"state"` // idle, tuning, done, failed
	AppID   string  `json:"appid"`
	Target  int     `json:"target"`
	Watts   int     `json:"watts"`
	Lo      int     `json:"lo"`
	Hi      int     `json:"hi"`
	FPS     float64 `json:"fps"`
	Low     float64 `json:"low"`
	Result  int     `json:"result"`
	Message string  `json:"message"`
}

type Tuner struct {
	hw  capper
	src frameSource
	// gets called before every step, used to keep mangoapp logging on
	prep func() error
	// and this once its over
	cleanup func()

	Min, Max int
	Headroom int
	Settle   time.Duration
	Window   time.Duration
	Tol      float64 // how far under target avg fps can be
	LowTol   float64 // same for the 5% lows

	mu     sync.Mutex
	st     Status
	cancel context.CancelFunc
}

func NewTuner(hw capper, src frameSource, min, max int) *Tuner {
	return &Tuner{
		hw:       hw,
		src:      src,
		Min:      min,
		Max:      max,
		Headroom: 1,
		Settle:   5 * time.Second,
		Window:   10 * time.Second,
		Tol:      0.03,
		LowTol:   0.15,
		st:       Status{State: "idle"},
	}
}

func (t *Tuner) Status() Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.st
	s.Watts = t.hw.Get()
	return s
}

func (t *Tuner) update(f func(s *Status)) {
	t.mu.Lock()
	f(&t.st)
	t.mu.Unlock()
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// returns avg fps and fps at the 95th percentile frametime
func stats(s []Sample) (float64, float64) {
	if len(s) == 0 {
		return 0, 0
	}
	ft := make([]float64, len(s))
	sum := 0.0
	for i, x := range s {
		ft[i] = x.Frametime
		sum += x.Frametime
	}
	sort.Float64s(ft)
	p95 := ft[int(math.Ceil(float64(len(ft))*0.95))-1]
	return float64(len(s)) * 1000 / sum, 1000 / p95
}

func (t *Tuner) measure(ctx context.Context, watts, target int) (bool, error) {
	if err := t.hw.Set(watts); err != nil {
		return false, err
	}
	if t.prep != nil {
		if err := t.prep(); err != nil {
			return false, err
		}
	}
	t.update(func(s *Status) {
		s.Message = fmt.Sprintf("trying %dW", watts)
	})

	if err := sleepCtx(ctx, t.Settle); err != nil {
		return false, err
	}
	t.src.Skip()

	var samples []Sample
	end := time.Now().Add(t.Window)
	for time.Now().Before(end) {
		if err := sleepCtx(ctx, t.Window/40); err != nil {
			return false, err
		}
		samples = append(samples, t.src.Read()...)
	}
	samples = append(samples, t.src.Read()...)

	if len(samples) < 20 {
		return false, fmt.Errorf("only got %d frames, is the game running and logging on?", len(samples))
	}

	avg, low := stats(samples)
	t.update(func(s *Status) {
		s.FPS = avg
		s.Low = low
	})

	ok := avg >= float64(target)*(1-t.Tol) && low >= float64(target)*(1-t.LowTol)
	return ok, nil
}

func (t *Tuner) search(ctx context.Context, target int) (int, error) {
	lo, hi := t.Min, t.Max
	t.update(func(s *Status) {
		s.Lo, s.Hi = lo, hi
	})

	ok, err := t.measure(ctx, hi, target)
	if err != nil {
		return 0, err
	}
	if !ok {
		return hi, fmt.Errorf("can't hold %d fps even at %dW", target, hi)
	}

	// smallest watts that still passes
	for lo < hi {
		mid := (lo + hi) / 2
		ok, err := t.measure(ctx, mid, target)
		if err != nil {
			return 0, err
		}
		if ok {
			hi = mid
		} else {
			lo = mid + 1
		}
		t.update(func(s *Status) {
			s.Lo, s.Hi = lo, hi
		})
	}

	res := hi + t.Headroom
	if res > t.Max {
		res = t.Max
	}
	return res, nil
}

// Start kicks off a tune in the background, done gets the result if it worked
func (t *Tuner) Start(appid string, target int, done func(watts int)) error {
	t.mu.Lock()
	if t.st.State == "tuning" {
		t.mu.Unlock()
		return fmt.Errorf("already tuning %s", t.st.AppID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.st = Status{State: "tuning", AppID: appid, Target: target}
	t.mu.Unlock()

	before := t.hw.Get()

	go func() {
		defer cancel()
		if t.cleanup != nil {
			defer t.cleanup()
		}
		res, err := t.search(ctx, target)

		if err != nil {
			t.hw.Set(before)
			t.update(func(s *Status) {
				s.State = "failed"
				s.Message = err.Error()
				if ctx.Err() != nil {
					s.State = "idle"
					s.Message = "stopped"
				}
			})
			return
		}

		t.hw.Set(res)
		t.update(func(s *Status) {
			s.State = "done"
			s.Result = res
			s.Message = fmt.Sprintf("%dW holds %d fps", res, target)
		})
		if done != nil {
			done(res)
		}
	}()
	return nil
}

func (t *Tuner) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
	}
}
