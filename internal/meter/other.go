//go:build !darwin && !linux && !windows

package meter

import "errors"

func LoadAvg() (float64, error) {
	return 0, errors.New("load average: платформа не поддерживается")
}
func MemPercent() int       { return 0 }
func TempC() int            { return 0 }
func CPUSample() (int, int) { return -1, -1 }
func SwapPercent() int      { return 0 }
func DiskPercent() int      { return -1 }
func BatteryPercent() int   { return -1 }

func DefaultGateway(target string) (string, error) {
	return "", errors.New("gateway: платформа не поддерживается")
}
