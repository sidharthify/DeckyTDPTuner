package tdp

import (
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/mangohud"

	"context"
	"log"
	"math"
	"sync"
	"time"
)

// Governor keeps adjusting the cap while a game runs. raises right away when
// frames start missing, lowers 1W at a time only after a long stable stretch

type GovStatus struct {
	Running bool    `json:"running"`
	AppID   string  `json:"appid"`
	Target  int     `json:"target"`
	Watts   int     `json:"watts"`
	Floor   int     `json:"floor"`
	FPS     float64 `json:"fps"`
	GPU     float64 `json:"gpu"`
	Raises  int     `json:"raises"`
	Drops   int     `json:"drops"`
	// at max and still under target
	Maxed   bool   `json:"maxed"`
	Message string `json:"message"`
}

type Governor struct {
	hw      capper
	src     frameSource
	Prep    func() error
	Prune   func()
	Cleanup func()

	Min, Max   int
	Tick       time.Duration
	RaiseStep  int
	RaiseBig   int           // when fps is way under target
	RaiseGap   time.Duration // dont raise again before the last raise had time to kick in
	LowerAfter time.Duration
	Blame      time.Duration // a miss this soon after a drop means the drop caused it
	FloorDecay time.Duration
	MissTol    float64 // fps under target*MissTol counts as a miss
	WinTicks   int     // how many ticks of history a miss is judged on
	BusyMax    float64 // gpu load under this counts as headroom
	JitterRise float64 // a drop that makes jitter this much worse gets undone
	JitterMin  float64 // ms, ignore rises smaller than this

	mu     sync.Mutex
	st     GovStatus
	cancel context.CancelFunc
	done   chan struct{}

	watts, base, floor int
	dropFrom           int
	dropJit            float64 // jitter right before the last drop, -1 once checked
	lastChange         time.Time
	lastDrop           time.Time
	lastMiss           time.Time
	floorAt            time.Time
	hist               []tickStat
}

type tickStat struct {
	fps float64
	n   int
	fts []float64
}

func NewGovernor(hw capper, src frameSource, min, max int) *Governor {
	return &Governor{
		hw:         hw,
		src:        src,
		Min:        min,
		Max:        max,
		Tick:       500 * time.Millisecond,
		RaiseStep:  2,
		RaiseBig:   4,
		RaiseGap:   2 * time.Second,
		LowerAfter: 15 * time.Second,
		Blame:      5 * time.Second,
		FloorDecay: 2 * time.Minute,
		MissTol:    0.93,
		WinTicks:   4,
		BusyMax:    85,
		JitterRise: 0.33,
		JitterMin:  0.5,
	}
}

func (g *Governor) Status() GovStatus {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.st
}

func (g *Governor) set(w int) {
	if w < g.Min {
		w = g.Min
	}
	if w > g.Max {
		w = g.Max
	}
	g.watts = w
	if err := g.hw.Set(w); err != nil {
		log.Printf("governor: set %dW: %v", w, err)
	}
}

// reset starts from scratch, start is where the cap begins and base is the
// lowest the floor can decay back to
func (g *Governor) reset(now time.Time, start, base int) {
	if base < g.Min {
		base = g.Min
	}
	if start <= 0 {
		start = g.Max
	}
	g.base, g.floor = base, base
	g.lastChange, g.lastMiss, g.floorAt = now, now, now
	g.lastDrop = time.Time{}
	g.dropJit = -1
	g.hist = nil
	g.set(start)
}

func (g *Governor) step(now time.Time, target int, s []mangohud.Sample) {
	if len(s) == 0 {
		return
	}

	// in live mode mangoapp only logs every 100ms and the frametime column is
	// whatever single frame landed there, so go by the fps column instead
	fps, gpu := 0.0, 0.0
	gpuN := 0
	fts := make([]float64, 0, len(s))
	for _, x := range s {
		fps += x.FPS
		fts = append(fts, x.Frametime)
		if x.GPULoad >= 0 {
			gpu += x.GPULoad
			gpuN++
		}
	}
	fps /= float64(len(s))
	if gpuN > 0 {
		gpu /= float64(gpuN)
	} else {
		gpu = -1
	}

	g.mu.Lock()
	g.st.FPS, g.st.GPU = fps, gpu
	g.mu.Unlock()

	g.hist = append(g.hist, tickStat{fps: fps, n: len(s), fts: fts})
	if len(g.hist) > g.WinTicks {
		g.hist = g.hist[1:]
	}
	win, n := 0.0, 0
	for _, h := range g.hist {
		win += h.fps * float64(h.n)
		n += h.n
	}
	win /= float64(n)

	// one bad tick is a blip, has to be low over the window and still low now
	lim := float64(target) * g.MissTol
	miss := len(g.hist) >= 2 && win < lim && fps < lim
	g.mu.Lock()
	g.st.Maxed = miss && g.watts >= g.Max
	g.mu.Unlock()
	if miss {
		g.lastMiss = now

		if !g.lastDrop.IsZero() && now.Sub(g.lastDrop) < g.Blame && g.dropFrom > g.floor {
			g.floor = g.dropFrom
			g.floorAt = now
		}

		if g.watts >= g.Max || now.Sub(g.lastChange) < g.RaiseGap {
			return
		}
		w := g.watts + g.RaiseStep
		if win < float64(target)*0.75 {
			w = g.watts + g.RaiseBig
		}
		g.set(w)
		g.lastChange = now
		// judge the new watts on fresh frames only
		g.hist = nil
		g.dropJit = -1
		g.mu.Lock()
		g.st.Raises++
		g.mu.Unlock()
		return
	}

	// fps can hold fine while the graph turns into a sawtooth, so once
	// theres a full window at the new watts compare it with before the drop
	jit := g.jitter()
	if g.dropJit >= 0 && g.watts < g.dropFrom && len(g.hist) >= g.WinTicks && now.Sub(g.lastDrop) < g.Blame*2 {
		worse := jit > g.dropJit*(1+g.JitterRise) && jit-g.dropJit > g.JitterMin
		if worse {
			g.floor = g.dropFrom
			g.floorAt = now
			g.lastMiss = now
			g.set(g.dropFrom)
			g.lastChange = now
			g.hist = nil
			g.mu.Lock()
			g.st.Raises++
			g.mu.Unlock()
		}
		g.dropJit = -1
		if worse {
			return
		}
	}

	// one heavy scene shouldnt pin the floor up for the rest of the session
	if g.floor > g.base && now.Sub(g.lastMiss) > g.FloorDecay && now.Sub(g.floorAt) > g.FloorDecay {
		g.floor--
		g.floorAt = now
	}

	headroom := gpu < 0 || gpu < g.BusyMax
	if headroom && g.watts > g.floor && len(g.hist) >= g.WinTicks && now.Sub(g.lastChange) >= g.LowerAfter && now.Sub(g.lastMiss) >= g.LowerAfter {
		g.dropFrom = g.watts
		g.dropJit = jit
		g.hist = nil
		g.set(g.watts - 1)
		g.lastDrop = now
		g.lastChange = now
		g.mu.Lock()
		g.st.Drops++
		g.mu.Unlock()
	}
}

// mean jump between consecutive logged frametimes over the window. at 100ms
// logging these are samples not every frame, but the spread still goes up
// when the graph gets jagged
func (g *Governor) jitter() float64 {
	var all []float64
	for _, h := range g.hist {
		all = append(all, h.fts...)
	}
	if len(all) < 2 {
		return 0
	}
	sum := 0.0
	for i := 1; i < len(all); i++ {
		sum += math.Abs(all[i] - all[i-1])
	}
	return sum / float64(len(all)-1)
}

func (g *Governor) Start(appid string, target, start, base int) error {
	g.Stop()

	if g.Prep != nil {
		if err := g.Prep(); err != nil {
			g.mu.Lock()
			g.st = GovStatus{Message: err.Error()}
			g.mu.Unlock()
			return err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	g.mu.Lock()
	g.cancel = cancel
	g.done = done
	g.st = GovStatus{Running: true, AppID: appid, Target: target}
	g.mu.Unlock()
	g.reset(time.Now(), start, base)
	g.src.Skip()
	log.Printf("governor: %s at %d fps, starting at %dW, floor %dW", appid, target, g.watts, g.floor)

	go func() {
		g.loop(ctx, target)
		close(done)
	}()
	return nil
}

func (g *Governor) loop(ctx context.Context, target int) {
	tick := time.NewTicker(g.Tick)
	defer tick.Stop()
	if g.Cleanup != nil {
		defer g.Cleanup()
	}

	n := 0
	lastFrame := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			n++
			// steam rewrites the mangoapp config sometimes
			if n%20 == 0 && g.Prep != nil {
				g.Prep()
			}
			if n%120 == 0 && g.Prune != nil {
				g.Prune()
			}

			s := g.src.Read()
			if len(s) > 0 {
				lastFrame = now
			}
			g.step(now, target, s)

			msg := ""
			if now.Sub(lastFrame) > 10*time.Second {
				msg = "no frames from mangoapp"
			}
			g.mu.Lock()
			g.st.Watts = g.watts
			g.st.Floor = g.floor
			g.st.Message = msg
			g.mu.Unlock()
		}
	}
}

// Stop waits for the loop to exit so its cleanup cant race whoever starts logging next
func (g *Governor) Stop() {
	g.mu.Lock()
	cancel, done := g.cancel, g.done
	g.cancel, g.done = nil, nil
	g.st.Running = false
	g.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
