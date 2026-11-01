# DeckyTDPTuner

## Overview
>[!WARNING]
> This plugin runs as **root** and writes straight to the amdgpu power limits in sysfs (`power1_cap` and `power2_cap`). It never goes outside the range the driver reports, but if something goes wrong a reboot puts everything back to stock.

Pick a target framerate in the Quick Access Menu, press "Find lowest TDP" and leave the game running somewhere that's roughly as heavy as the rest of it (a busy town, not a loading screen). The plugin starts at 15W, checks that the game can actually hold the target there, and then binary searches down until it finds the lowest wattage that still keeps the average and the 5% lows above the target. Each step settles for 5 seconds and samples for 10, so a full run takes about a minute. The result plus 1W of headroom gets saved for that game and is applied automatically every time you launch it.

Things to keep in mind:
- Set Steam's **framerate limit** to the same value as the target. The whole point is that the game is capped and the APU is just burning power past what it needs, so without a limit it'll happily use all 15W to render frames you'll never see.
- Leave Steam's own **TDP limit** slider off, otherwise Steam and the plugin will keep overwriting each other.
- Frametimes come from mangoapp (the thing behind the performance overlay), the plugin turns on its CSV logging while tuning and turns it off again afterwards. The overlay itself doesn't need to be visible.
- The search only looks at whatever you're doing during that minute, so if the game gets heavier later (bosses, big fights, etc) you might want to tune again there or bump the result up yourself.
- It's only been written with the Steam Deck in mind (LCD and OLED), other handhelds running SteamOS *might* work if their amdgpu hwmon exposes `power1_cap`.
- I am not responsible for thermonuclear war, bricked devices, and bla-bla, you know the old saying ;)

## Building

You need `pnpm`, `go` (1.21 or newer) and `make`.

```sh
pnpm i
pnpm build

cd backend
make
```

Then put `dist/`, `main.py`, `plugin.json`, `package.json`, `LICENSE` and `backend/out/tdptuner` (as `bin/tdptuner`) into a folder called `DeckyTDPTuner` and zip it, or let the [decky CLI](https://github.com/SteamDeckHomebrew/cli) do it for you, it builds the backend inside the holo go toolchain image:

```sh
decky plugin build .
```

To install, enable developer mode in Decky's settings and use "Install Plugin From ZIP".

## Usage

The Go backend can also be poked at directly over ssh, which is handy for checking whether your device is supported at all (it just prints the hwmon it found and the allowed range):

```sh
sudo ./tdptuner -info
sudo ./tdptuner -set 8
```

Logs end up in `tdptuner.log` under the plugin's folder in `~/homebrew/logs/`, and the saved values go in `profiles.json` under `~/homebrew/settings/`, which you can edit by hand if you want to.

## License

This project is licensed under the **BSD 3-Clause License**.

Key points to be aware of:

* You are free to use, modify, and distribute the software, in source or binary form.
* You must keep the copyright notice and the license text when redistributing.
* You can't use the name of the project or its contributors to promote anything derived from it without permission.

For full details, please refer to the [LICENSE](./LICENSE) file.

## Acknowledgments

### decky-plugin-template
The frontend build setup (`rollup.config.js`, `tsconfig.json` and most of `package.json`) comes from [`decky-plugin-template`](https://github.com/SteamDeckHomebrew/decky-plugin-template).

`decky-plugin-template` is licensed under the [BSD 3-Clause License](https://opensource.org/licenses/BSD-3-Clause).

> © Steam Deck Homebrew and contributors.

> <small>_Original source: https://github.com/SteamDeckHomebrew/decky-plugin-template_</small>

### MangoHud
Not bundled, but nothing here works without it. All the frametime data comes from [`MangoHud`](https://github.com/flightlessmango/MangoHud)'s logging, which SteamOS ships as mangoapp.

> <small>_Original source: https://github.com/flightlessmango/MangoHud_</small>

## Disclaimer

This software is provided **"as is"** without any warranty of any kind, express or implied. By using this plugin, you acknowledge that:

* Lowering the TDP can make games stutter or perform worse than you expect, the plugin only measures what happens during the tuning run.
* You are solely responsible for any consequences resulting from the use, misuse, or inability to use this software.
* The maintainers and contributors of this project are **not liable for any damage**, data loss or device malfunction that may arise.
