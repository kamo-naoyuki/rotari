package projectrun

import (
	"os"
	"strconv"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// readLoadAverage returns the host's load average, or nil where
// /proc/loadavg is unavailable or malformed.
func readLoadAverage() *model.LoadAverage {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	return parseLoadAverage(string(data))
}

func parseLoadAverage(data string) *model.LoadAverage {
	fields := strings.Fields(data)
	if len(fields) < 3 {
		return nil
	}
	one, oneErr := strconv.ParseFloat(fields[0], 64)
	five, fiveErr := strconv.ParseFloat(fields[1], 64)
	fifteen, fifteenErr := strconv.ParseFloat(fields[2], 64)
	if oneErr != nil || fiveErr != nil || fifteenErr != nil {
		return nil
	}
	return &model.LoadAverage{One: one, Five: five, Fifteen: fifteen}
}
