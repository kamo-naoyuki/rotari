package model

import "time"

func FormatDisplayTimestamp(value string) string {
	if value == "" || value == "-" {
		return value
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return timestamp.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02 15:04:05 JST")
}
