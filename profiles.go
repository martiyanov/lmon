package main

// profileColumns — осмысленные наборы для быстрого старта.
// Колонки, которых нет в ОС, отбросит layoutWithDropped.
func profileColumns(name string) []string {
	switch name {
	case "server":
		// сеть, CPU, память, температура, диск и ожидание диска
		return []string{"N", "G", "C", "L", "M", "T", "U", "S", "I", "K"}
	case "desktop":
		// без load average и iowait, зато с батареей
		return []string{"N", "G", "C", "M", "T", "U", "S", "B"}
	case "network":
		// только то, что важно при проблемах с сетью
		return []string{"N", "G", "C", "M", "T", "U"}
	}
	return nil
}
