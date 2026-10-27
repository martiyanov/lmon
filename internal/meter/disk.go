//go:build darwin || linux

package meter

import (
	"golang.org/x/sys/unix"
)

// DiskPercent возвращает долю занятого места на корневом разделе в процентах.
func DiskPercent() int {
	var st unix.Statfs_t
	if err := unix.Statfs("/", &st); err != nil {
		return -1
	}
	if st.Blocks == 0 {
		return 0
	}
	return int((st.Blocks - st.Bfree) * 100 / st.Blocks)
}
