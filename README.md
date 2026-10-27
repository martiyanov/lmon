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

Single static binary for **macOS**, **Linux** and **Windows 10+**
(amd64/arm64), written in Go. The original zsh version is kept in
[legacy/](legacy/lmon.zsh). Design principles: see
[CONCEPT.md](CONCEPT.md).

## Install

**macOS (Homebrew)**:

```sh
brew install martiyanov/tap/lmon
```

**macOS** (без Homebrew, Apple Silicon):

```sh
curl -fsSL https://github.com/martiyanov/lmon/releases/latest/download/lmon_darwin_arm64.tar.gz | tar -xz
sudo mv lmon /usr/local/bin/
```

On an Intel Mac use `lmon_darwin_amd64.tar.gz`.

**Debian / Ubuntu** (and other `.deb` systems):

```sh
curl -fsSLO https://github.com/martiyanov/lmon/releases/latest/download/lmon_amd64.deb
sudo dpkg -i lmon_amd64.deb
```

On ARM machines use `lmon_arm64.deb`. Fedora/RHEL: pick the `.rpm` asset
from the [releases page](https://github.com/martiyanov/lmon/releases).

**Other Linux**:

```sh
curl -fsSL https://github.com/martiyanov/lmon/releases/latest/download/lmon_linux_amd64.tar.gz | tar -xz -C /tmp
sudo mv /tmp/lmon /usr/local/bin/
```

**Windows 10+** (PowerShell):

```powershell
curl.exe -fsSL -o lmon.zip https://github.com/martiyanov/lmon/releases/latest/download/lmon_windows_amd64.zip
Expand-Archive lmon.zip; Move-Item lmon\lmon.exe "$env:LOCALAPPDATA\Microsoft\WindowsApps" -Force
```

(then run `lmon` in Windows Terminal; on ARM use `lmon_windows_arm64.zip`).
On Windows the `L` (load average) and `T` (temperature) columns show `·` —
the OS does not expose those readings; everything else works.

**If you have Go 1.22+**:

```sh
go install github.com/martiyanov/lmon@latest
```

All binaries are static, no dependencies beyond the system `ping`.

**Requirements:** the system `ping` utility (used as-is, no root needed).
On macOS the temperature column additionally needs
[`smctemp`](https://github.com/holtmann/osx/tree/master/smctemp)
(optional — shows `0` without it). On Linux the temperature is read from
sysfs (`hwmon` / `thermal_zone`), no extra tools required.

## Usage

```sh
lmon                    # run forever, one line per second
lmon 20                 # print 20 lines and exit
lmon -b                 # color blocks instead of digits
lmon --cpu --swap       # add extra columns
lmon help               # built-in reference (Russian)
lmon --version          # version
```

### Columns

The base set is always shown; extra columns are enabled with flags:

| Col | Meaning | Flag | Green | Yellow | Red |
|-----|---------|------|-------|--------|-----|
| `N` | ping to an external host (`--target`, default `8.8.8.8`), ms | — | ≤ 50 | ≤ 150 | > 150 |
| `G` | ping to your gateway (`--gw`, auto-detected by default), ms | — | ≤ 10 | ≤ 50 | > 50 |
| `L` | load average (integer part) | — | ≤ 4 | ≤ 8 | > 8 |
| `M` | used memory, % | — | ≤ 60 | ≤ 85 | > 85 |
| `T` | CPU temperature, °C | — | ≤ 60 | ≤ 75 | > 75 |
| `C` | CPU usage, % | `--cpu` | ≤ 50 | ≤ 80 | > 80 |
| `I` | iowait, % (Linux only, `·` on macOS) | `--iowait` | ≤ 5 | ≤ 20 | > 20 |
| `S` | swap used, % | `--swap` | ≤ 30 | ≤ 70 | > 70 |
| `U` | root disk used, % | `--disk` | ≤ 80 | ≤ 90 | > 90 |
| `B` | battery charge, % (inverted: more is better) | `--batt` | ≥ 50 | ≥ 20 | < 20 |

### Blocks mode

`lmon -b` (`--blocks`) replaces digits with colored squares `███` —
one per column, with a continuous green → yellow → red gradient along
each column's scale. You read the status with peripheral vision instead
of parsing numbers. A dim square (or `·`) means no sensor on this
machine; `999` means the host is unreachable.

### Reading bad numbers

- `N` and `G` red together — local problem: Wi-Fi, router, cable.
  Only `N` red — the uplink or your provider.
- `L` high while `C` is low — the system waits (disk, network, locks)
  rather than computes; high `I` confirms it's the disk (Linux).
- `M` red and `S` growing — out of memory, swapping started; everything
  slows down.
- `C` ≈ 100 for a long time — a process is eating the CPU; find it
  with `top`.
- `T` > 80–90 °C — overheating/throttling: dust, fans, sustained load.
- `U` > 90 — the disk is nearly full; the system misbehaves.

### Options (flags and environment variables)

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--target` | `LMON_TARGET` | `8.8.8.8` | external host to ping |
| `--gw` | `LMON_GW` | auto-detect | your gateway |
| `--interval` | `LMON_INTERVAL` | `1` | seconds between lines (fractional ok) |
| `-b`, `--blocks` | — | off | color blocks instead of digits |
| `--cpu` `--iowait` `--swap` `--disk` `--batt` | — | off | add extra columns |

```sh
lmon --target 1.1.1.1 --interval 2 10
lmon -b --cpu --batt 30
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
