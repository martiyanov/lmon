package meter

import "math"

// clampPercent приводит значение к шкале 0..100; NaN — «нет данных».
func clampPercent(v float64) int {
	if math.IsNaN(v) {
		return -1
	}
	n := int(math.Round(v))
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}
