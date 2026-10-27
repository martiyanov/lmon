//go:build !darwin && !linux

package meter

import "errors"

func LoadAvg() (float64, error) { return 0, errors.New("load average: платформа не поддерживается") }
func MemPercent() int           { return 0 }
func TempC() int                { return 0 }
