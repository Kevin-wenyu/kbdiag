//go:build !linux && !darwin

package probe

import (
	"errors"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func statfsOS(path string) (facts.Disk, error) {
	return facts.Disk{}, &pathError{path, errors.New("statfs not supported on this platform")}
}
