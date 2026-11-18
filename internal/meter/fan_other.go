//go:build !linux

package meter

// FanPercent — на macOS/Windows публичного источника нет:
// на Windows нужен LHM/HWiNFO, на macOS — SMC/powermetrics с правами.
func FanPercent() int { return -1 }
