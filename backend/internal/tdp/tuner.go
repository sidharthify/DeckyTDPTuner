package tdp

import (
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/mangohud"

	"context"
	"fmt"
	"log"
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
	Read() []mangohud.Sample
}

type Status struct {
	State  string  `json:"state"` // idle, tuning, done, failed
	AppID  string  `json:"appid"`
	Target int     `json:"target"`
	Watts  int     `json:"watts"`
	Lo     int     `json:"lo"`
	Hi     int     `json:"hi"`
	FPS    float64 `json:"fps"`
	Low    float64 `json:"low"`
	Jitter float64 `json:"jitter"`
	Result int     `json:"result"`
	// set when even max cant hold target, Suggest is a target that would
	Unreachable bool   `json:"unreachable"`
	Suggest     int    `json:"suggest"`
	Message     string `json:"message"`
}

type Tuner struct {
	hw  capper
	src frameSource
	// gets called before every step, used to keep mangoapp logging on
	Prep func() error
	// and this once its over
	Cleanup func()

	Min, Max int
	Headroom int
	Settle   time.Duration
	Window   time.Duration
	Tol      float64 // how far under target avg fps can be
	LowTol   float64 // same for the 5% lows
	// how much smoother the next watt has to be before we take it
	JitterGain float64
	// avg fps this far over target at max means the game isnt capped
	UncappedTol float64

	mu     sync.Mutex
	st     Status
	cancel context.CancelFunc
}

func NewTuner(hw capper, src frameSource, min, max int) *Tuner {
	return &Tuner{
		hw:          hw,
		src:         src,
		Min:         min,
		Max:         max,
		Headroom:    1,
		Settle:      5 * time.Second,
		Window:      10 * time.Second,
		Tol:         0.03,
		LowTol:      0.15,
		JitterGain:  0.25,
		UncappedTol: 0.15,
		st:          Status{State: "idle"},
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
func stats(s []mangohud.Sample) (float64, float64) {
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

// jitter is the average jump between one frame and the next in ms, a flat
// graph is ~0 and a sawtooth one is big even when avg fps looks fine
func jitter(s []mangohud.Sample) float64 {
	if len(s) < 2 {
		return 0
	}
	sum := 0.0
	for i := 1; i < len(s); i++ {
		sum += math.Abs(s[i].Frametime - s[i-1].Frametime)
	}
	return sum / float64(len(s)-1)
}

func (t *Tuner) measure(ctx context.Context, watts, target int) (bool, float64, error) {
	if err := t.hw.Set(watts); err != nil {
		return false, 0, err
	}
	if t.Prep != nil {
		if err := t.Prep(); err != nil {
			return false, 0, err
		}
	}
	t.update(func(s *Status) {
		s.Message = fmt.Sprintf("trying %dW", watts)
	})

	if err := sleepCtx(ctx, t.Settle); err != nil {
		return false, 0, err
	}
	t.src.Skip()

	var samples []mangohud.Sample
	end := time.Now().Add(t.Window)
	for time.Now().Before(end) {
		if err := sleepCtx(ctx, t.Window/40); err != nil {
			return false, 0, err
		}
		samples = append(samples, t.src.Read()...)
	}
	samples = append(samples, t.src.Read()...)

	if len(samples) < 20 {
		return false, 0, fmt.Errorf("only got %d frames, is the game running and logging on?", len(samples))
	}

	avg, low := stats(samples)
	jit := jitter(samples)
	t.update(func(s *Status) {
		s.FPS = avg
		s.Low = low
		s.Jitter = jit
	})

	ok := avg >= float64(target)*(1-t.Tol) && low >= float64(target)*(1-t.LowTol)
	log.Printf("tune: %dW avg %.1f low %.1f jitter %.2fms over %d frames, target %d, pass %v", watts, avg, low, jit, len(samples), target, ok)
	return ok, jit, nil
}

// highest multiple of 5 that would have passed with what max managed
func suggest(avg, low, tol, lowTol float64) int {
	best := math.Min(avg/(1-tol), low/(1-lowTol))
	sug := int(best) / 5 * 5
	if sug < 20 {
		return 0
	}
	return sug
}

func (t *Tuner) search(ctx context.Context, target int) (int, error) {
	lo, hi := t.Min, t.Max
	t.update(func(s *Status) {
		s.Lo, s.Hi = lo, hi
	})

	ok, _, err := t.measure(ctx, hi, target)
	if err != nil {
		return 0, err
	}
	// the whole thing assumes the game is capped at target, if it isnt every
	// watt passes and the search just slides down to the floor
	if avg := t.Status().FPS; avg > float64(target)*(1+t.UncappedTol) {
		return hi, fmt.Errorf("game runs at %.0f fps at %dW, set Steam's framerate limit to %d first", avg, hi, target)
	}
	if !ok {
		t.update(func(s *Status) {
			s.Unreachable = true
			s.Suggest = suggest(s.FPS, s.Low, t.Tol, t.LowTol)
		})
		return hi, fmt.Errorf("%dW can't hold %d fps, best was %.0f fps", hi, target, t.Status().FPS)
	}

	// smallest watts that still passes
	for lo < hi {
		mid := (lo + hi) / 2
		ok, _, err := t.measure(ctx, mid, target)
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

	// lowest passing watt can still have a jagged frametime graph, keep
	// going up while the next one is clearly smoother
	cur := hi
	_, curJit, err := t.measure(ctx, cur, target)
	if err != nil {
		return 0, err
	}
	smoothed := false
	for cur < t.Max {
		_, nextJit, err := t.measure(ctx, cur+1, target)
		if err != nil {
			return 0, err
		}
		if nextJit >= curJit*(1-t.JitterGain) {
			break
		}
		cur++
		curJit = nextJit
		smoothed = true
	}

	// already found the knee, no need to pad it
	res := cur
	if !smoothed {
		res += t.Headroom
	}
	if res > t.Max {
		res = t.Max
	}
	return res, nil
}

// Start kicks off a tune in the background, done gets the result if it worked.
// min is the lowest watts it'll try, 0 keeps whatever it was
func (t *Tuner) Start(appid string, target, min int, done func(watts int)) error {
	t.mu.Lock()
	if t.st.State == "tuning" {
		t.mu.Unlock()
		return fmt.Errorf("already tuning %s", t.st.AppID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.st = Status{State: "tuning", AppID: appid, Target: target}
	if min > 0 {
		t.Min = min
	}
	t.mu.Unlock()

	before := t.hw.Get()

	go func() {
		defer cancel()
		res, err := t.search(ctx, target)
		// before done, which might hand logging over to the governor
		if t.Cleanup != nil {
			t.Cleanup()
		}

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
