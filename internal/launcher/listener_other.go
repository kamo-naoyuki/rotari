//go:build !linux && !darwin

package launcher

import (
	"fmt"
	"runtime"
)

func portListeners(int) ([]portListener, error) {
	return nil, fmt.Errorf("listener ownership checks are not implemented on %s", runtime.GOOS)
}
