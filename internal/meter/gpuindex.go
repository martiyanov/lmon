package meter

// gpuIndex — какую видеокарту показывать в P/H. Меняется один раз при старте
// (SetGPUIndex), дальше читается без блокировок: измерения идут последовательно.
var gpuIndex int

// SetGPUIndex задаёт индекс GPU (0 — первая). Отрицательный — как 0.
func SetGPUIndex(n int) {
	if n < 0 {
		n = 0
	}
	gpuIndex = n
}

// GPUIndex возвращает выбранный индекс.
func GPUIndex() int { return gpuIndex }

// GPUPercent — загрузка выбранной GPU (см. SetGPUIndex).
func GPUPercent() int { return GPUPercentAt(GPUIndex()) }

// GPUTempC — температура выбранной GPU.
func GPUTempC() int { return GPUTempCAt(GPUIndex()) }
