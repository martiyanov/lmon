//go:build !linux && !darwin

package meter

func GPUPowerW() int           { return -1 }
func NVMeTempC() int           { return -1 }
func DiskIO() (int, int)       { return -1, -1 }
func NetRate() (int, int, int) { return -1, -1, -1 }
func PSIMemory() int           { return -1 }
func FailedUnits() int         { return -1 }
func RebootRequired() int      { return -1 }
func CPUPowerW() int           { return -1 }
func NVMeWearPct() int         { return -1 }
