# How the code is laid out

This goes over what every file in the repo does and how they talk to each other. The README covers what the plugin does from the user's side, so this is more for anyone (me included, a few months from now) who wants to change something and needs to know where it lives.

## The big picture

There are three layers. The frontend is a React panel in the Quick Access Menu, written in TypeScript and bundled with rollup. Decky only lets the frontend call into a Python file, so `main.py` is a thin layer that starts the Go backend and forwards every call to it over a unix socket. The Go backend runs as root and does all the actual work, which is writing the power caps to sysfs, reading frametimes out of mangoapp's CSV logs, running the tuner and the live governor, and watching the charger.

A call from the panel goes like this.

```
TuneSection.tsx -> backend.ts (callable "tune")
  -> main.py Plugin.tune -> one JSON line over tdptuner.sock
  -> server.go handle("tune") -> tdp.Tuner.Start
```

Every reply comes back as `{"ok": true, "data": ...}` or `{"ok": false, "error": "..."}`, and the frontend shows a toast for anything that isn't ok.

## Root

### `main.py`

The Decky side of the plugin. On load (`_main`) it starts `bin/tdptuner` with the socket, profiles and settings paths pointing into Decky's runtime and settings folders, sends its stdout and stderr to `tdptuner.log`, and waits up to 2 seconds for the socket to show up. Every method the frontend can call (`status`, `tune`, `stop`, `apply`, `live`, `set_settings`, `reset`, `forget`) goes through `_call`, which starts the backend again if it died, writes one JSON line and reads one back. Calls time out after 10 seconds so a stuck backend doesn't hang the panel. On unload it sends SIGTERM and kills the process if it hasn't exited after 5 seconds.

### `plugin.json`

Decky's manifest. The `root` flag is what makes Decky run `main.py` (and so the backend) as root, which the sysfs writes need.

### `package.json`, `pnpm-lock.yaml`, `rollup.config.js`, `tsconfig.json`

The frontend build setup, mostly as it came from decky-plugin-template. `pnpm build` runs rollup and puts the bundle in `dist/index.js`. The version in `package.json` is the one Decky shows, and CI checks the release tag against it.

### `scripts/package.sh`

Zips up whatever is already built into `out/DeckyTDPTuner-<version>.zip`, with the Go binary under `bin/`. It doesn't build anything itself and bails out if `dist/index.js` or `backend/out/tdptuner` is missing.

### `.github/workflows/ci.yml`

Runs on pushes to main, tags and PRs. It checks `gofmt`, runs `go vet` and `go test -race`, builds both halves, packages the zip and uploads it as an artifact. On a `v*` tag it also creates a GitHub release with the zip, after checking that the tag matches `package.json` (anything starting with `v0.` gets marked as a prerelease).

### `assets/tune.jpg`

The screenshot at the top of the README.

## Backend (`backend/`)

### `Makefile`

`make` builds a static binary (`CGO_ENABLED=0`, stripped) into `out/tdptuner`, `make test` runs the tests and `make clean` removes `out/`.

### `Dockerfile`, `entrypoint.sh`

Only used by `decky plugin build`, which builds the backend inside the holo go toolchain image by running `make` in `/backend`.

### `cmd/tdptuner/main.go`

The entry point. It finds the amdgpu hwmon and clamps its max to `-max` (15W by default). With `-info` or `-set` it prints what it found (and sets the cap) and exits, which is the bit that's handy over ssh.

Otherwise it wires everything together. The tuner gets its own log reader and turns mangoapp logging on with `log_interval=0` (every frame), and its cleanup turns logging off again and deletes the log folder since those CSVs get big. The governor gets a second log reader with logging at 100ms, plus `Prune` to delete old CSVs while it runs. It also looks for the GPU clock controls, and if they aren't there the charger feature just leaves the clock alone. Then it starts the charger watcher and the socket server. On SIGTERM, SIGINT or the server dying it stops the tuner and governor, unpins the GPU, turns logging off and puts the power cap back to what it was before the backend started.

### `internal/hwmon/hwmon.go`

Finds the hwmon folder named `amdgpu` that has `power1_cap` and reads the allowed range from `power1_cap_min` and `power1_cap_max` (falling back to 3-15W if those aren't there). Sysfs works in microwatts and everything else in the code works in whole watts, so this is the only place that converts. `Set` clamps to the range and writes both `power1_cap` and `power2_cap` (the fast limit), because if only the slow one is lowered the APU boosts past it for a few seconds. Whatever the caps were when it opened them is saved, and `Restore` writes those back.

### `internal/gpu/gpu.go`

The GPU clock part of "Full power on charger". `Find` looks for a card with `pp_od_clk_voltage` and parses the SCLK range out of the `OD_RANGE` section. `Pin` sets `power_dpm_force_performance_level` to manual and writes the same clock as both the min and max SCLK, which is the same thing Steam's manual GPU clock slider does. `Unpin` resets the OD table before going back to auto, otherwise auto keeps using whatever was pinned.

### `internal/power/power.go`

`OnAC` goes through `/sys/class/power_supply` and returns true if any supply of type `Mains` says it's online (that's `ACAD` on the Deck).

### `internal/mangohud/mangoapp.go`

Turns mangoapp's CSV logging on and off. Steam owns the mangoapp config file and rewrites it whenever the performance overlay level changes, so the plugin finds the file by reading `MANGOHUD_CONFIGFILE` out of the running mangoapp process's environment (falling back to the newest `/tmp/mangohud.*`) and appends a small block to it.

```
# tdptuner
output_folder=/tmp/tdptuner
autostart_log=1
log_interval=100
```

The `# tdptuner` line marks the block so `stripBlock` can take it out again, which is what `DisableLogging` does and also what happens when the interval changes. It also makes the log folder world writable because mangoapp runs as the deck user and not as root.

### `internal/mangohud/logtail.go`

Follows the newest CSV in the log folder, kind of like `tail -f`. It remembers how far into the file it has read, keeps any half written last line around for the next read, and switches to a new file (resetting everything) when mangoapp starts a new one. Column positions come from the header line starting with `fps`, and every row turns into a `Sample` with the fps, the frametime in ms and the GPU load (-1 if the log doesn't have it). `Skip` throws away everything logged so far so frames from before a change don't count, and `Prune` deletes every CSV except the one being read.

### `internal/profiles/profiles.go`

The per game values in `profiles.json`, keyed by Steam appid. Each one has the tuned watts, the target fps, when it was tuned and whether live mode is on. Every change writes the whole file to a `.tmp` first and renames it over the real one, so a crash halfway through doesn't wipe it. Retuning keeps the live flag as it was, and turning live on before ever tuning saves a profile with 0 watts.

### `internal/tdp/tuner.go`

The "Find lowest TDP" button. `Start` runs the search in a goroutine and `Stop` cancels it and waits for it to finish, so nothing else touches the cap until the tuner has put it back.

Each step (`measure`) sets the cap, waits 5 seconds for it to settle, skips the frames logged in the meantime and then collects frames for 10 seconds. A step passes when the average fps is within 3% of the target and the fps at the 95th percentile frametime (the 5% lows) is within 15%. It also works out jitter, which is the average jump in ms between one frametime and the next, so a flat graph is close to 0 and a sawtooth one isn't even if the average looks fine. Fewer than 20 frames fails the whole run, because that means the game isn't running or logging isn't on.

`search` does this in order.

1. Measure at max. If the game is more than 15% over the target there, Steam's framerate limit isn't set and every watt would pass, so it stops and says so.
2. If max doesn't pass, it marks the result as unreachable and suggests the highest multiple of 5 (20 or more) that would have passed with what max managed.
3. Binary search between the floor and max for the lowest passing wattage.
4. Measure that wattage again for its jitter and keep going up 1W at a time for as long as the next watt is at least 25% smoother.
5. If it never went up in step 4 it adds 1W of headroom, otherwise the smoother wattage is the result as is.

On success the cap is set to the result and the `done` callback saves it. On failure or a stop the cap goes back to what it was before tuning started.

### `internal/tdp/governor.go`

Live mode. It runs a loop every 500ms, reads whatever frames came in since the last tick and decides what to do with the cap in `step`. Live mode logs every 100ms, and at that interval the frametime column is just whichever frame landed on the sample, so it goes by the fps column instead and keeps the last 4 ticks (2 seconds) as its window.

- A miss is when both the window average and the latest tick are under 93% of the target, so one bad tick on its own doesn't count. On a miss it raises by 2W (4W if the window is under 75% of the target), but not again until 2 seconds after the last change so the previous raise has time to kick in. If it's already at max and still missing it sets `Maxed`, which is what the red text in the panel is for.
- A miss within 5 seconds of a drop means the drop caused it, so the wattage it dropped from becomes the new floor.
- For games that were never tuned it also compares jitter before and after a drop, and if a full window at the new wattage is more than 33% (and more than 0.5ms) worse it goes straight back up and raises the floor. Tuned games skip this because the tuner already checked smoothness and the floor never goes under the tuned value, and in practice the sampled jitter above that was just noise that kept undoing drops after unplugging.
- The floor decays back down 1W every 2 minutes without misses, so one heavy scene doesn't pin it up for the whole session.
- It lowers by 1W only when the GPU is under 85% busy (or the log has no GPU load), it's above the floor, there's a full window, and it's been 15 seconds since the last change and the last miss. Above the tuned value that wait is 5 seconds instead, since that range is already known to be fine.

Every 20 ticks it puts the mangoapp config block back (Steam sometimes rewrites it), every 120 ticks it prunes old CSVs, and if nothing has come in for 10 seconds the status says "no frames from mangoapp". `Start` takes a start wattage and a base separately, so after unplugging it can start from the charger wattage and ease down to the tuned value on its own.

### `internal/server/server.go`

The socket server and the commands. Each connection reads JSON lines and answers each one with a JSON line.

- `status` returns the tuner and governor status, the allowed range, the charger state and settings, and the saved profile for the given appid if there is one. The panel polls this every second.
- `tune` stops the governor, cancels any ramp and drops out of full power if it's on (the charger watcher won't boost while tuning), then starts the tuner with the current floor. When the tuner finishes it saves the profile and starts the governor if live mode is on for that game.
- `stop` stops the tuner.
- `apply` is sent when a game launches. It remembers which game is running, and then either does nothing (no profile, or full power is on and unplugging will apply it later), starts the governor if live mode is on, or sets the saved wattage.
- `live` saves the live flag. Turning it on starts the governor. Turning it off stops it and sets the saved wattage, or the governor's floor if that went higher during the session, since that only went up because going lower actually caused misses.
- `reset` is sent when the game exits. It stops the governor and puts the original cap back, unless full power on charger is active.
- `settings` merges the settings patch (see `charger.go`).
- `forget` deletes the profile, stops the governor if it was running for that game and puts the cap back to stock if that game is still running.
- `profiles` returns every saved profile. Nothing in the frontend uses it right now, it's just there for poking at the socket by hand.

### `internal/server/charger.go`

Everything around `settings.json` and "Full power on charger". `WatchCharger` checks `OnAC` every 2 seconds and boosts or unboosts when that changes.

`boost` stops the governor and any ramp, sets the charger wattage (max if it's 0) and pins the GPU clock if that's turned on. The clock is capped by `gpuCap`, which scales it linearly below 10W and rounds it down to 100MHz, so at 5W on a 1600MHz GPU you get 800MHz. That's because the CPU and GPU share the same power budget and pinning the full clock at a low TDP just starves the CPU. `unboost` unpins the GPU and calls `restoreGame`, which puts back whatever the running game would normally get. With live mode on that means starting the governor from the charger wattage, without it `rampTo` walks the cap down 1W every 500ms in a goroutine. Anything else that sets the cap bumps `rampGen`, which makes a running ramp stop at its next step.

`SetSettings` merges only the fields in the patch into the saved settings, so the charger section and the advanced section can't overwrite each other with stale values. If the lowest TDP changed, `applyFloor` restarts the governor with the new floor or bumps a saved wattage that's now under it. If full power is active it boosts again so new watts or clocks apply straight away.

### Tests

- `gpu/gpu_test.go` parses a Deck style `pp_od_clk_voltage` and one with no `OD_RANGE`.
- `power/power_test.go` uses a fake `power_supply` folder.
- `mangohud/mangohud_test.go` checks that the log tail picks the columns out of a SteamOS mangoapp header and holds on to a half written line until it's finished, and that `stripBlock` only removes our block.
- `tdp/tuner_test.go` runs the search against a fake cap and a fake frame source where the fps depends on the wattage, covering a normal search, a jagged one that should climb, an unreachable target and an uncapped game.
- `tdp/governor_test.go` drives `step` with made up timestamps, covering lowering, holding when the GPU is busy, ignoring a single bad tick, raising, blaming a drop, hitting max, undoing a jagged drop and easing down to the tuned value.
- `server/charger_test.go` covers the GPU clock scaling, the ramp and cancelling it, settings patches, live off keeping the learned floor, forget restoring the cap and a floor change applying to the running game.

## Frontend (`src/`)

### `index.tsx`

Defines the plugin for Decky (name, icon, the `Content` panel) and starts the game watcher from `lifetime.ts`, unregistering it when the plugin unloads.

### `lifetime.ts`

Listens to Steam's app lifetime notifications. When a game starts it calls `apply` and shows a toast with the wattage if there was a saved one. When a game exits it calls `stop` and `reset`, but only if that's the game it applied for, so something else closing in the background doesn't reset the one you're playing. It also applies straight away on load if a game is already running, since the backend doesn't know about it otherwise.

### `backend.ts`

One `callable` per method in `main.py`, plus `check`, which shows a toast with the error when a reply isn't ok.

### `types.ts`

TypeScript versions of the JSON the backend returns (`Status`, `GovStatus`, `Profile`, `Settings`, `Charger` and `StatusData`, which wraps all of them). These have to be kept in sync with the Go structs by hand.

### `components/Content.tsx`

The panel itself. It polls `status` every second for the running game and shows the error instead of everything else if the backend doesn't answer. The target fps is kept in a map outside the component because Decky unmounts the panel every time the QAM closes, and without that the slider would snap back to 60. It starts the slider at the saved target for that game unless you've already moved it.

### `components/GameSection.tsx`

The top section, with the game name, the current TDP and the saved wattage and target if there is one.

### `components/TuneSection.tsx`

The target fps slider (20 to 90 in steps of 5, locked while tuning), the "Adjust live" toggle with the governor's current wattage, floor, fps, GPU load and raise and drop counts, the "Find lowest TDP" button (which turns into "Stop" while tuning) and the tuner's progress. When a tune comes back unreachable it shows the red warning, toasts it once and offers a button to switch to the suggested target. "Forget saved TDP" shows up whenever there's a profile and it isn't tuning.

### `components/ChargerSection.tsx`

The full power toggle, the charger TDP slider and the GPU clock toggle and slider. It keeps a local copy of the settings so the sliders don't jump around while the next poll catches up, and does the same `gpuCap` maths as the backend so the clock slider's max follows the TDP slider. Lowering the TDP below what the current clock allows pulls the clock down with it.

### `components/AdvancedSection.tsx`

Hidden behind a toggle. It has the warning about games crashing at very low TDP, the "Lowest TDP" slider (from the driver minimum up to 10W) and a button to go back to the default, which sends `min_watts: 0` so the backend falls back to its `-min` flag.
