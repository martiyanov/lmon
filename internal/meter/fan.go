package meter

// FanInfo — один вентилятор: имя, обороты, PWM-процент и режим отображения.
type FanInfo struct {
	Name    string
	RPM     int    // -1, если обороты не отдаются
	Percent int    // -1, если PWM/процент не отдаются
	Kind    string // "rpm" (значение = RPM/10) или "pct" (0..100)
}
