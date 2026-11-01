package main

import (
	"flag"
	"fmt"
	"log"
	"os"
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

	hw, err := findHwmon()
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
		fmt.Printf("%s: %dW (range %d-%dW, fast limit: %v)\n", hw.dir, hw.Get(), hw.Min, hw.Max, hw.fast)
		return
	}

	tn := NewTuner(hw, NewLogTail(*logDir), hw.Min, hw.Max)
	tn.prep = func() error {
		return EnableLogging(*logDir)
	}
	tn.cleanup = func() {
		if err := DisableLogging(); err != nil {
			log.Printf("turning mangoapp logging off: %v", err)
		}
		// csvs get big with log_interval=0
		os.RemoveAll(*logDir)
	}

	srv := &Server{hw: hw, tuner: tn, profiles: LoadProfiles(*profPath)}
	log.Fatal(srv.Serve(*sock))
}
