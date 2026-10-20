package launcher

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

// portListeners asks lsof, which macOS ships, for the processes listening on
// port. lsof run by an ordinary user lists only that user's processes, so a
// listener it cannot see stays invisible here; VerifyListener then still
// fails because the launch's group holds no exact LoopbackHost listener,
// which another user's socket on that address would prevent it from binding.
func portListeners(port int) ([]portListener, error) {
	output, err := exec.Command("lsof", "-nP", "-a", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fpn").Output()
	if err != nil {
		var exitErr *exec.ExitError
		// lsof exits 1 when nothing matches.
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(output) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("lsof: %w", err)
	}
	return parseLsofListeners(output, port, syscall.Getpgid)
}
