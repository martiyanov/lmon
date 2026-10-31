//go:build windows

package meter

// GPUPercent/GPUTempC: у Windows нет надёжного API без прав администратора
// и вендорских библиотек (PDH-счётчики GPU локализованы и капризны,
// температура — только NVAPI/LHM). Честные «нет данных».
func GPUPercent() int { return -1 }
func GPUTempC() int   { return -1 }
