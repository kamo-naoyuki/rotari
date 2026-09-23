package run

import "strings"

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
	errorText := strings.ToLower(strings.TrimSpace(resultError))
	return errorText == "cancelled" || errorText == "canceled" || strings.HasPrefix(errorText, "cancelled ") || strings.HasPrefix(errorText, "canceled ")
}
