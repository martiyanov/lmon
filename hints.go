package main

import (
	"fmt"
	"strings"
)

// startupHint — одна подсказка «почему колонка пустая и что сделать».
// Печатается при старте один раз и никогда не блокирует работу lmon.
type startupHint struct {
	title  string // короткая причина
	detail string // конкретное действие (команда/флаг/что установить)
}

// startupHints собирает подсказки по первому замеру: какие включённые
// колонки не дали данных и что с этим делать. goos передаётся явно,
// чтобы правила можно было тестировать без реального Windows/macOS.
func startupHints(cfg config, cols []reading, goos string, dropped []droppedColumn) []startupHint {
	lang := cfg.lang
	byID := make(map[string]reading, len(cols))
	for _, r := range cols {
		if _, ok := byID[r.id]; ok {
			continue
		}
		byID[r.id] = r
	}
	var out []startupHint
	add := func(title, detail string) {
		out = append(out, startupHint{title: title, detail: detail})
	}

	if len(dropped) > 0 {
		heads := make([]string, 0, len(dropped))
		reasons := make([]string, 0, len(dropped))
		for _, d := range dropped {
			heads = append(heads, strings.TrimSpace(d.head))
			reasons = append(reasons, d.reason)
		}
		title := lmsg(lang,
			"Hidden in this OS: "+strings.Join(heads, ", ")+" — no data source here.",
			"Скрыты в этой ОС: "+strings.Join(heads, ", ")+" — здесь для них нет источника данных.")
		var detail string
		switch goos {
		case "windows":
			detail = lmsg(lang,
				"Windows has no load average and no CPU temperature API; use --cpu for CPU load. iowait/swap/GPU are not shown for the same reason.",
				"В Windows нет load average и API температуры CPU; загрузку CPU смотрите через --cpu. iowait/swap/GPU по той же причине не показываются.")
		case "darwin":
			detail = lmsg(lang,
				"macOS gives no iowait and no GPU temperature; other columns work normally.",
				"macOS не отдаёт iowait и температуру GPU; остальные колонки работают.")
		default:
			detail = lmsg(lang,
				"Unsupported by this platform: "+strings.Join(reasons, ", ")+".",
				"Не поддерживается этой платформой: "+strings.Join(reasons, ", ")+".")
		}
		add(title, detail)
	}

	net, netOn := byID["N"]
	gw, gwOn := byID["G"]
	// Сеть: NET и GTW отдельно и вместе. 999 — узел не ответил,
	// отрицательное значение — измерения нет (нет ping/шлюза).
	if netOn && gwOn && net.v == 999 && gw.v == 999 {
		if goos == "windows" {
			add(lmsg(lang,
				"NET and GTW = 999: no ICMP reply at all — the local firewall or network drops echo.",
				"NET и GTW = 999: ICMP-эхо не проходит никуда — его режет файрвол или сеть."),
				lmsg(lang,
					"Allow echo in PowerShell (admin): New-NetFirewallRule -DisplayName \"Allow ICMPv4\" -Protocol ICMPv4 -IcmpType 8 -Action Allow; or check Wi-Fi/cable and --target.",
					"Разрешите эхо в PowerShell (админ): New-NetFirewallRule -DisplayName \"Allow ICMPv4\" -Protocol ICMPv4 -IcmpType 8 -Action Allow; или проверьте Wi-Fi/кабель и --target."))
		} else {
			add(lmsg(lang,
				"NET and GTW = 999: no ICMP reply at all — check the network/route.",
				"NET и GTW = 999: ICMP-эхо не проходит никуда — проверьте сеть/маршрут."),
				lmsg(lang,
					"Check Wi-Fi/cable, then a known host: --target 1.1.1.1 · gateway: --gw 192.168.1.1",
					"Проверьте Wi-Fi/кабель, затем известный хост: --target 1.1.1.1 · шлюз: --gw 192.168.1.1"))
		}
		netOn = false
		gwOn = false
	}
	if netOn {
		switch {
		case net.v == 999:
			detail := lmsg(lang,
				"Check the internet; on Windows the firewall often blocks ping. Allow echo (admin PowerShell): New-NetFirewallRule -DisplayName \"Allow ICMPv4\" -Protocol ICMPv4 -IcmpType 8 -Action Allow; or another host: --target 1.1.1.1",
				"Проверьте интернет; на Windows часто виноват файрвол. Разрешите эхо (PowerShell от админа): New-NetFirewallRule -DisplayName \"Allow ICMPv4\" -Protocol ICMPv4 -IcmpType 8 -Action Allow; или другой хост: --target 1.1.1.1")
			if goos == "windows" {
				detail = lmsg(lang,
					"On Windows the firewall often blocks ping. Allow echo (admin PowerShell): New-NetFirewallRule -DisplayName \"Allow ICMPv4\" -Protocol ICMPv4 -IcmpType 8 -Action Allow; or another host: --target 1.1.1.1",
					"На Windows часто виноват файрвол. Разрешите эхо (PowerShell от админа): New-NetFirewallRule -DisplayName \"Allow ICMPv4\" -Protocol ICMPv4 -IcmpType 8 -Action Allow; или другой хост: --target 1.1.1.1")
			}
			add(lmsg(lang,
				"NET = 999: no ICMP reply from the external host.",
				"NET = 999: ICMP-эхо до внешнего хоста не проходит."), detail)
		case net.v < 0:
			add(lmsg(lang,
				"NET: the ping utility is unavailable, so there is no measurement.",
				"NET: утилита ping недоступна, поэтому измерения нет."),
				lmsg(lang,
					"Install ping / check PATH, or set another --target; the column stays «·».",
					"Установите ping/проверьте PATH или задайте другой --target; колонка останется «·»."))
		}
	}
	if gwOn {
		switch {
		case gw.v < 0 && cfg.gwAuto:
			add(lmsg(lang,
				"GTW: the gateway was not detected — the column is empty.",
				"GTW: шлюз не определён — колонка пустая."),
				lmsg(lang,
					"Pass the router address explicitly: --gw 192.168.1.1",
					"Укажите адрес роутера вручную: --gw 192.168.1.1"))
		case gw.v == 999:
			detail := lmsg(lang,
				"Some routers ignore ICMP, so this can be normal. If the address is wrong, pass it explicitly: --gw 192.168.1.1",
				"Некоторые роутеры игнорируют ICMP — тогда это норма. Если адрес определён неверно, укажите вручную: --gw 192.168.1.1")
			if !cfg.gwAuto {
				detail = lmsg(lang,
					"Check the address/Wi-Fi/cable; ICMP may be filtered (see the NET hint).",
					"Проверьте адрес/Wi-Fi/кабель; ICMP может быть отфильтрован (см. подсказку NET).")
			}
			add(lmsg(lang,
				"GTW = 999: the gateway does not answer ping.",
				"GTW = 999: шлюз не отвечает на ping."), detail)
		}
	}

	if r, ok := byID["L"]; ok && r.v < 0 {
		if goos == "windows" {
			add(lmsg(lang,
				"RUN: load average does not exist on Windows.",
				"RUN: load average в Windows отсутствует."),
				lmsg(lang,
					"Use the CPU column instead (--cpu) or drop RUN from --cols.",
					"Используйте колонку CPU (--cpu) или уберите RUN из --cols."))
		} else {
			add(lmsg(lang,
				"RUN: load average is unavailable on this system.",
				"RUN: load average недоступен в этой системе."),
				lmsg(lang, "Use --cpu as a replacement.", "Используйте --cpu как замену."))
		}
	}
	if r, ok := byID["T"]; ok && r.v < 0 {
		if cfg.tempCmd != "" {
			add(lmsg(lang,
				"C°: --temp-cmd did not return a number.",
				"C°: команда --temp-cmd не вернула число."),
				lmsg(lang,
					"Check the command manually and that it prints °C (first number is used).",
					"Проверьте команду вручную и что она печатает °C (берётся первое число)."))
		} else {
			switch goos {
			case "windows":
				add(lmsg(lang,
					"C°: Windows has no CPU temperature API; use an external source.",
					"C°: в Windows нет API температуры CPU; нужен внешний источник."),
					lmsg(lang,
						"Pass --temp-cmd (e.g. LibreHardwareMonitor via PowerShell) or drop column T.",
						"Передайте --temp-cmd (например, LibreHardwareMonitor через PowerShell) или уберите колонку T."))
			case "darwin":
				add(lmsg(lang,
					"C°: the CPU temperature sensor needs smctemp.",
					"C°: для температуры CPU нужен smctemp."),
					lmsg(lang,
						"Install it: brew install smctemp — or drop column T.",
						"Установите: brew install smctemp — или уберите колонку T."))
			case "linux":
				add(lmsg(lang,
					"C°: the CPU temperature sensor is not readable.",
					"C°: датчик температуры CPU не читается."),
					lmsg(lang,
						"Check /sys/class/thermal or install lm-sensors; or drop the column.",
						"Проверьте /sys/class/thermal или поставьте lm-sensors; либо уберите колонку."))
			default:
				add(lmsg(lang,
					"C°: temperature is not available on this system.",
					"C°: температура недоступна в этой системе."),
					lmsg(lang, "Drop the column with --cols.", "Уберите колонку через --cols."))
			}
		}
	}
	if r, ok := byID["I"]; ok && r.v < 0 && goos != "linux" {
		add(lmsg(lang,
			"IOW: iowait exists only on Linux.",
			"IOW: iowait есть только в Linux."),
			lmsg(lang, "Drop the column with --cols.", "Уберите колонку через --cols."))
	}
	if r, ok := byID["S"]; ok && r.v < 0 {
		if goos == "windows" {
			add(lmsg(lang,
				"SWP: the page file is disabled or cannot be read.",
				"SWP: файл подкачки отключён или не читается."),
				lmsg(lang,
					"Enable it in sysdm.cpl → Advanced → Performance → Virtual memory, or drop the SWP column.",
					"Включите его: sysdm.cpl → Дополнительно → Быстродействие → Виртуальная память, или уберите колонку SWP."))
		} else {
			add(lmsg(lang,
				"SWP: swap is unavailable on this system.",
				"SWP: swap недоступен в этой системе."),
				lmsg(lang, "Drop the column with --cols.", "Уберите колонку через --cols."))
		}
	}
	if r, ok := byID["P"]; ok && r.v < 0 {
		add(lmsg(lang,
			"GPU: no GPU utilization source.",
			"GPU: нет источника загрузки GPU."),
			lmsg(lang,
				"Windows: NVIDIA driver (NVML/nvidia-smi) or Windows 10 1709+ for PDH GPU Engine counters; otherwise drop column P.",
				"Windows: драйвер NVIDIA (NVML/nvidia-smi) или Windows 10 1709+ для счётчиков PDH GPU Engine; иначе уберите колонку P."))
	}
	if r, ok := byID["H"]; ok && r.v < 0 {
		add(lmsg(lang,
			"G°: no GPU temperature source.",
			"G°: нет источника температуры GPU."),
			lmsg(lang,
				"Windows: NVIDIA via NVML/nvidia-smi; AMD/Intel need an external monitor (not supported yet); otherwise drop column H.",
				"Windows: NVIDIA через NVML/nvidia-smi; AMD/Intel — только внешний монитор (пока не поддержано); иначе уберите колонку H."))
	}
	if r, ok := byID["U"]; ok && r.v < 0 {
		add(lmsg(lang,
			"DSK: the disk usage could not be read.",
			"DSK: не удалось прочитать занятость диска."),
			lmsg(lang,
				"Check the system drive/permissions, or drop column U.",
				"Проверьте системный диск/права или уберите колонку U."))
	}
	if r, ok := byID["M"]; ok && r.v < 0 {
		add(lmsg(lang,
			"MEM: memory usage could not be read.",
			"MEM: не удалось прочитать память."),
			lmsg(lang, "Check permissions; otherwise drop column M.", "Проверьте права; иначе уберите колонку M."))
	}

	// именованные пинги: «VPN = 999»
	for _, p := range cfg.pings {
		r, ok := byID[p.name]
		if !ok || r.v != 999 {
			continue
		}
		add(fmt.Sprintf(lmsg(lang,
			"%s = 999: no ICMP reply from %s.",
			"%s = 999: нет ICMP-ответа от %s."), p.name, p.host),
			lmsg(lang,
				"Check the host/route; if ICMP is filtered, see the NET hint.",
				"Проверьте хост/маршрут; если ICMP фильтруется — см. подсказку NET."))
	}
	return out
}

// note печатает подсказки в области данных TUI: они прокручиваются вместе
// с таблицей и не мешают закреплённому заголовку.
func (u *ui) note(hints []startupHint) {
	if len(hints) == 0 {
		return
	}
	for _, h := range hints {
		u.noteLine("lmon: "+h.title, true)
		if h.detail != "" {
			u.noteLine("  "+h.detail, false)
		}
	}
	fmt.Fprint(u.w, "\r\n")
}

// noteLine печатает строку подсказки в области данных: заголовок — жёлтым
// (бросается в глаза), детали — тускло.
func (u *ui) noteLine(s string, title bool) {
	if u.color {
		c := dim
		if title {
			c = yel
		}
		fmt.Fprint(u.w, c+s+reset+"\r\n")
		return
	}
	fmt.Fprint(u.w, s+"\r\n")
}
