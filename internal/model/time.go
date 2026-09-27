package model

import "time"

// FormatDisplayTimestamp formats a stored RFC3339 time for people in
// time.Local, which Go sets from TZ, or from the system zone when TZ is unset.
// Values that do not parse, including "" and "-", are returned unchanged.
func FormatDisplayTimestamp(value string) string {
	if value == "" || value == "-" {
		return value
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return timestamp.In(time.Local).Format("2006-01-02 15:04:05 MST")
}
