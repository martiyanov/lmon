# lmon

A minimal one-line system monitor for your terminal. A fixed header row,
then a stream of measurements — ping to the internet, ping to your gateway,
load average, memory pressure and CPU temperature — colored green / yellow /
red so you can spot trouble at a glance.

```
  N   G   L  M%   T°
--- --- --- --- ----
 12   2   1  62  55°
 11   2   1  63  55°
118   3   1  63  56°   ← Wi-Fi hiccup, gateway is fine → it's the uplink
```

Single static binary for **macOS** (arm64/amd64) and **Linux** (amd64/arm64),
written in Go. The original zsh version is kept in [legacy/](legacy/lmon.zsh).

## Install

With Go 1.22+:

```sh
go install github.com/martiyanov/lmon@latest
```

Or build from source:

```sh
git clone https://github.com/martiyanov/lmon
cd lmon && go build -o /usr/local/bin/lmon ./cmd/lmon
```

**Requirements:** the system `ping` utility (used as-is, no root needed).
On macOS the temperature column additionally needs
[`smctemp`](https://github.com/holtmann/osx/tree/master/smctemp)
(optional — shows `0` without it). On Linux the temperature is read from
sysfs (`hwmon` / `thermal_zone`), no extra tools required.

## Usage

```sh
lmon                    # run forever, one line per second
lmon 20                 # print 20 lines and exit
lmon help               # built-in reference (Russian)
lmon --version          # version
```

### Columns

| Col | Meaning | Green | Yellow | Red |
|-----|---------|-------|--------|-----|
| `N` | ping to an external host (`--target`, default `8.8.8.8`), ms | ≤ 50 | ≤ 150 | > 150 |
| `G` | ping to your gateway (`--gw`, default `192.168.0.1`), ms | ≤ 10 | ≤ 50 | > 50 |
| `L` | load average (integer part) | ≤ 4 | ≤ 8 | > 8 |
| `M` | used memory, % | ≤ 60 | ≤ 85 | > 85 |
| `T` | CPU temperature, °C | ≤ 60 | ≤ 75 | > 75 |

If `N` and `G` jump together — the problem is local (Wi-Fi / router).
If only `N` jumps — it's the uplink or your provider. An unreachable host
shows `999`.

### Options (flags and environment variables)

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--target` | `LMON_TARGET` | `8.8.8.8` | external host to ping |
| `--gw` | `LMON_GW` | `192.168.0.1` | your gateway |
| `--interval` | `LMON_INTERVAL` | `1` | seconds between lines (fractional ok) |

```sh
lmon --target 1.1.1.1 --interval 2 10
LMON_GW=192.168.1.1 lmon
```

The first line after start may show an inflated ping: the Wi-Fi radio
needs to wake up. Values settle after that.

## How it works

Ping runs through the system `ping` binary (flags adapted per OS), so no
privileges are required. Load average comes from the kernel (`sysctl` on
macOS, `/proc/loadavg` on Linux). Memory: `vm_stat` on macOS,
`/proc/meminfo` on Linux. Colors respect `NO_COLOR`.

## License

[MIT](LICENSE)
