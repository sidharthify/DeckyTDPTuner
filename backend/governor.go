package main

import (
	"context"
	"log"
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
	Message string  `json:"message"`
}

type Governor struct {
	hw      capper
	src     frameSource
	prep    func() error
	prune   func()
	cleanup func()

	Min, Max   int
	Tick       time.Duration
	RaiseStep  int
	RaiseGap   time.Duration // dont raise again before the last raise had time to kick in
	LowerAfter time.Duration
	Blame      time.Duration // a miss this soon after a drop means the drop caused it
	FloorDecay time.Duration
	MissFrac   float64
	BusyMax    float64 // gpu load under this counts as headroom

	mu     sync.Mutex
	st     GovStatus
	cancel context.CancelFunc
	done   chan struct{}

	watts, base, floor int
	dropFrom           int
	lastChange         time.Time
	lastDrop           time.Time
	lastMiss           time.Time
	floorAt            time.Time
}

func NewGovernor(hw capper, src frameSource, min, max int) *Governor {
	return &Governor{
		hw:         hw,
		src:        src,
		Min:        min,
		Max:        max,
		Tick:       500 * time.Millisecond,
		RaiseStep:  2,
		RaiseGap:   time.Second,
		LowerAfter: 15 * time.Second,
		Blame:      5 * time.Second,
		FloorDecay: 2 * time.Minute,
		MissFrac:   0.2,
		BusyMax:    85,
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
	g.set(start)
}

func (g *Governor) step(now time.Time, target int, s []Sample) {
	if len(s) == 0 {
		return
	}

	budget := 1000 / float64(target)
	over := 0
	fps, gpu := 0.0, 0.0
	gpuN := 0
	for _, x := range s {
		if x.Frametime > budget*1.15 {
			over++
		}
		fps += x.FPS
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

	miss := float64(over)/float64(len(s)) >= g.MissFrac || fps < float64(target)*0.95
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
		// way off, dont crawl up
		if fps < float64(target)*0.8 {
			w = g.Max
		}
		g.set(w)
		g.lastChange = now
		g.mu.Lock()
		g.st.Raises++
		g.mu.Unlock()
		return
	}

	// one heavy scene shouldnt pin the floor up for the rest of the session
	if g.floor > g.base && now.Sub(g.lastMiss) > g.FloorDecay && now.Sub(g.floorAt) > g.FloorDecay {
		g.floor--
		g.floorAt = now
	}

	headroom := gpu < 0 || gpu < g.BusyMax
	if headroom && g.watts > g.floor && now.Sub(g.lastChange) >= g.LowerAfter && now.Sub(g.lastMiss) >= g.LowerAfter {
		g.dropFrom = g.watts
		g.set(g.watts - 1)
		g.lastDrop = now
		g.lastChange = now
		g.mu.Lock()
		g.st.Drops++
		g.mu.Unlock()
	}
}

func (g *Governor) Start(appid string, target, start, base int) error {
	g.Stop()

	if g.prep != nil {
		if err := g.prep(); err != nil {
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
	if g.cleanup != nil {
		defer g.cleanup()
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
			if n%20 == 0 && g.prep != nil {
				g.prep()
			}
			if n%120 == 0 && g.prune != nil {
				g.prune()
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
