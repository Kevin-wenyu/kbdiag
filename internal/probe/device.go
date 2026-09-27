package probe

import (
	"fmt"
	"os"
	"syscall"
)

// deviceOf names the device a directory lives on (st_dev, following
// symlinks), which tells filesystems apart; statfs's f_fsid is zero on some
// of them. Swapped out in tests.
var deviceOf = func(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprint(st.Dev), nil
	}
	return "", nil
}
