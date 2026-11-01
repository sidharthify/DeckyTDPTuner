package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/sidharthify/DeckyTDPTuner/backend/internal/hwmon"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/mangohud"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/profiles"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/server"
	"github.com/sidharthify/DeckyTDPTuner/backend/internal/tdp"
)

func main() {
	info := flag.Bool("info", false, "print detected hwmon and exit")
	set := flag.Int("set", 0, "set tdp in watts and exit")
	minW := flag.Int("min", 3, "lowest tdp the tuner will try")
	maxW := flag.Int("max", 15, "highest tdp the tuner will try")
	sock := flag.String("sock", "/tmp/tdptuner.sock", "socket for the plugin to talk to")
	profPath := flag.String("profiles", "profiles.json", "where tuned values go")
	logDir := flag.String("logdir", "/tmp/tdptuner", "where mangoapp should write its logs")
	flag.Parse()

	hw, err := hwmon.Find()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// dont go outside what the driver allows
	if *minW > hw.Min {
		hw.Min = *minW
	}
	if *maxW < hw.Max {
		hw.Max = *maxW
	}

	if *set > 0 {
		if err := hw.Set(*set); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *info || *set > 0 {
		fmt.Printf("%s: %dW (range %d-%dW, fast limit: %v)\n", hw.Dir, hw.Get(), hw.Min, hw.Max, hw.Fast)
		return
	}

	tn := tdp.NewTuner(hw, mangohud.NewLogTail(*logDir), hw.Min, hw.Max)
	tn.Prep = func() error {
		return mangohud.EnableLogging(*logDir, 0)
	}
	tn.Cleanup = func() {
		if err := mangohud.DisableLogging(); err != nil {
			log.Printf("turning mangoapp logging off: %v", err)
		}
		// csvs get big with log_interval=0
		os.RemoveAll(*logDir)
	}

	gl := mangohud.NewLogTail(*logDir)
	gov := tdp.NewGovernor(hw, gl, hw.Min, hw.Max)
	gov.Prep = func() error {
		return mangohud.EnableLogging(*logDir, 100)
	}
	gov.Prune = gl.Prune
	gov.Cleanup = tn.Cleanup

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		log.Printf("exiting, putting tdp back to %dW", hw.Orig())
		tn.Stop()
		gov.Stop()
		mangohud.DisableLogging()
		hw.Restore()
		os.Remove(*sock)
		os.Exit(0)
	}()

	srv := server.New(hw, tn, gov, profiles.Load(*profPath))
	log.Fatal(srv.Serve(*sock))
}
