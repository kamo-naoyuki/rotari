package run

import (
	"os"
	"strconv"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func ReadLoadAverage() *model.LoadAverage {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(data))
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
