//go:build linux

package meter

func DiskRate() (int, int) { return -1, -1 }
func MemoryPressure() int  { return -1 }
