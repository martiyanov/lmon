package main

// droppedColumn — колонка, скрытая из вывода, потому что в этой ОС
// её нечем измерить. Объясняется стартовой подсказкой, запуск не
// блокирует.
type droppedColumn struct {
	id     string
	head   string
	reason string
}

// columnSupportedByOS отвечает, есть ли у lmon честный источник данных
// для колонки в данной ОС. Неподдерживаемые колонки не показываются:
// «·» на месте датчика только путает.
func columnSupportedByOS(id, goos string) bool {
	// Динамические колонки нескольких GPU: G0/G1 — как P, T0/T1 — как H.
	if len(id) == 2 && id[1] >= '0' && id[1] <= '9' {
		switch id[0] {
		case 'G':
			return columnSupportedByOS("P", goos)
		case 'T':
			return columnSupportedByOS("H", goos)
		case 'F':
			return columnSupportedByOS("F", goos)
		}
	}
	if id == "GF" {
		return columnSupportedByOS("F", goos)
	}
	switch goos {
	case "windows":
		switch id {
		case "L", // load average в Windows отсутствует
			"T",                                                          // нет официального API температуры CPU (только --temp-cmd)
			"I",                                                          // iowait есть только в Linux
			"F",                                                          // скорость вентиляторов: нужен LHM/HWiNFO
			"W", "Y", "A", "Q", "R", "J", "E", "Z", "O", "X", "PW", "WR": // Linux-метрики
			return false
		}
		// S (pagefile) читается через PDH; P/H — NVML/nvidia-smi/PDH;
		// если источника нет, колонка покажет «·» и подсказку.
	case "darwin":
		switch id {
		case "I", // iowait на macOS ядром не отдаётся
			"H",                                           // GPUTempC: powermetrics под sudo — не показываем
			"F",                                           // вентиляторы: нужен SMC/powermetrics с правами
			"W", "Y", "A", "Q", "Z", "O", "X", "PW", "WR": // Linux-метрики (R/J/E читаются из netstat)
			return false
		}
		// K (занятость диска) читается через IOKit, без sudo.
	case "linux":
		switch id {
		case "F":
			// FAN читается из hwmon PWM; если датчиков нет — колонка покажет «·».
		case "K": // занятость диска: на Linux пока нет (для I/O есть IOW)
			return false
		}
		// Остальные колонки в Linux доступны; отдельные датчики (GPU, TMP)
		// зависят от железа — для них остаются подсказки при -1.
	default:
		switch id {
		case "L", "T", "I", "S", "P", "H", "F",
			"W", "Y", "A", "Q", "R", "J", "E", "Z", "O", "X", "PW", "WR":
			return false
		}
	}
	return true
}

// supportedReason объясняет, почему колонка скрыта, для подсказки.
func supportedReason(id string) string {
	switch id {
	case "L":
		return "load average"
	case "T":
		return "температура CPU"
	case "I":
		return "iowait"
	case "S":
		return "swap"
	case "P":
		return "загрузка GPU"
	case "H":
		return "температура GPU"
	case "K":
		return "занятость диска"
	case "F":
		return "скорость вентиляторов"
	}
	return id
}
