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
	switch goos {
	case "windows":
		switch id {
		case "L", // load average в Windows отсутствует
			"T", // нет API температуры CPU без внешнего ПО
			"I", // iowait есть только в Linux
			"S", // swap без счётчиков производительности не читается
			"P", // утилизация GPU без nvidia-smi недоступна
			"H": // температура GPU — та же история
			return false
		}
	case "darwin":
		switch id {
		case "I", // iowait на macOS ядром не отдаётся
			"H": // GPUTempC: powermetrics под sudo — не показываем
			return false
		}
	case "linux":
		// В Linux доступны все колонки; отдельные датчики (GPU, TMP)
		// зависят от железа — для них остаются подсказки при -1.
	default:
		switch id {
		case "L", "T", "I", "S", "P", "H":
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
	}
	return id
}
