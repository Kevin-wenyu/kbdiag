package probe

import (
	"syscall"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// statfsOS counts blocks the way df does: in fragment size units.
func statfsOS(path string) (facts.Disk, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return facts.Disk{}, &pathError{path, err}
	}
	unit := uint64(st.Frsize)
	if unit == 0 {
		unit = uint64(st.Bsize)
	}
	return facts.Disk{TotalBytes: st.Blocks * unit, UsedBytes: (st.Blocks - st.Bfree) * unit, AvailBytes: st.Bavail * unit}, nil
}
