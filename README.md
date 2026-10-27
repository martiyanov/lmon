# lmon

A minimal one-line system monitor for your terminal. A fixed header row,
then a stream of measurements — ping to the internet, ping to your gateway,
load average, memory pressure and CPU temperature — colored green / yellow /
red so you can spot trouble at a glance.

```
  N   G   L  M%  T°
--- --- --- --- ----
 12   2   1  62  55°
 11   2   1  63  55°
118   3   1  63  56°   ← Wi-Fi hiccup, gateway is fine → it's the uplink
```

**Status:** macOS only at the moment (a Linux port and a Go rewrite are on
the roadmap). Requires `zsh`; the temperature column needs
[`smctemp`](https://github.com/holtmann/osx/tree/master/smctemp)
(optional — shows `0` without it).

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/martiyanov/lmon/main/lmon -o lmon
chmod +x lmon
sudo mv lmon /usr/local/bin/
```

(or just clone the repo and copy `lmon` anywhere in your `PATH`)

## Usage

```sh
lmon          # run forever, one line per second
lmon 20       # print 20 lines and exit
lmon help     # built-in reference
```

### Columns

| Col | Meaning | Green | Yellow | Red |
|-----|---------|-------|--------|-----|
| `N` | ping to an external host (`LMON_TARGET`, default `8.8.8.8`), ms | ≤ 50 | ≤ 150 | > 150 |
| `G` | ping to your gateway (`LMON_GW`, default `192.168.0.1`), ms | ≤ 10 | ≤ 50 | > 50 |
| `L` | load average (integer part) | ≤ 4 | ≤ 8 | > 8 |
| `M` | used memory, % | ≤ 60 | ≤ 85 | > 85 |
| `T` | CPU temperature, °C | ≤ 60 | ≤ 75 | > 75 |

If `N` and `G` jump together — the problem is local (Wi-Fi / router).
If only `N` jumps — it's the uplink or your provider.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LMON_TARGET` | `8.8.8.8` | external host to ping |
| `LMON_GW` | `192.168.0.1` | your gateway |
| `LMON_INTERVAL` | `1` | seconds between lines (fractional ok) |

```sh
LMON_TARGET=1.1.1.1 LMON_INTERVAL=2 lmon 10
```

The first line after start may show an inflated ping: the Wi-Fi radio
needs to wake up. Values settle after that.

## License

[MIT](LICENSE)
