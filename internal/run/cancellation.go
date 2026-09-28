package run

import (
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func WasExplicitlyCancelled(resultError string, cancelled func() bool, schedulerPhase func() string) bool {
	if cancelled != nil && cancelled() {
		return true
	}
	if schedulerPhase != nil {
		phase := strings.ToLower(strings.TrimSpace(schedulerPhase()))
		if phase == "cancelled" || phase == "canceled" {
			return true
		}
	}
	return model.IsCancelledError(resultError)
}
