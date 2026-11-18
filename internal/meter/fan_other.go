//go:build !linux

package meter

// FanList/FanValues/FanPercent — на macOS/Windows публичного источника
// вентиляторов нет: нужны LHM/HWiNFO/SMC с правами.
func FanList() []FanInfo { return nil }
func FanValues() []int   { return nil }
func FanPercent() int    { return -1 }
