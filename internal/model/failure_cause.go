package model

import (
	"slices"
	"strings"
)

// FailureCauseDiagnosis is the kind of a failure cause that a saved rule
// diagnosis names; the other causes are FailureKind* values.
const FailureCauseDiagnosis = "diagnosis"

// FailureCause returns the one cause a failed result is grouped under, as
// its kind and name, and empty strings for a success. Causes rotari records
// itself (blocked, cancelled, timeout) come first because they are certain;
// then the latest saved rule diagnosis, named by its rule; then the result's
// first remaining failure kind. Failure summaries group jobs by it and
// --filter-failure-cause selects by it.
func FailureCause(result JobResult) (kind, name string) {
	kinds := FailureKinds(result)
	if len(kinds) == 0 {
		return "", ""
	}
	for _, recorded := range []string{FailureKindBlocked, FailureKindCancelled, FailureKindTimeout} {
		if slices.Contains(kinds, recorded) {
			return recorded, recorded
		}
	}
	if len(result.Diagnoses) > 0 {
		return FailureCauseDiagnosis, result.Diagnoses[0].Name
	}
	return kinds[0], kinds[0]
}

// FailureCauseID returns the selector ID of a failure cause or diagnosis
// name: lowercase letters and digits, with each other run of characters
// replaced by one hyphen. A failure kind is its own ID.
func FailureCauseID(name string) string {
	var builder strings.Builder
	separator := false
	for _, character := range strings.ToLower(name) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			builder.WriteRune(character)
			separator = false
			continue
		}
		separator = true
	}
	return builder.String()
}
