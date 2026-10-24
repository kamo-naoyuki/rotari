package bugreport

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/rotari-dev/rotari/internal/config"
	"github.com/rotari-dev/rotari/internal/report"
)

const redacted = "[REDACTED]"

var secretKeyParts = []string{"token", "password", "passwd", "secret", "credential", "auth", "url", "cookie", "apikey", "api_key", "api-key", "private"}

func secretKey(key string) bool {
	lower := strings.ToLower(key)
	for _, part := range secretKeyParts {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

// environmentKey names a list of NAME=VALUE entries.
func environmentKey(key string) bool {
	switch strings.ToLower(key) {
	case "env", "environment":
		return true
	}
	return false
}

// redactValue removes secrets from a decoded configuration or state value:
// values under secret-looking keys, and the values of environment entries.
func redactValue(key string, value any) any {
	if value == nil {
		return nil
	}
	if secretKey(key) {
		return redacted
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for name, item := range typed {
			if environmentKey(key) && name != "name" {
				result[name] = redacted
				continue
			}
			result[name] = redactValue(name, item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactValue(key, item)
		}
		return result
	case string:
		if environmentKey(key) {
			return redactAssignment(typed)
		}
	}
	return value
}

// redactAssignment replaces the value of a NAME=VALUE entry, except for the
// ROTARI_* variables rotari sets, whose values are its own state.
func redactAssignment(entry string) string {
	name, _, found := strings.Cut(entry, "=")
	if !found {
		return redacted
	}
	if strings.HasPrefix(name, "ROTARI_") && !secretKey(name) {
		return entry
	}
	return name + "=" + redacted
}

// redactEnvironment lists ROTARI_* variables, with secret-looking values
// replaced.
func redactEnvironment(environ []string) []string {
	var listed []string
	for _, entry := range environ {
		name, value, found := strings.Cut(entry, "=")
		if !found || !strings.HasPrefix(name, "ROTARI_") {
			continue
		}
		if secretKey(name) && value != "" {
			value = redacted
		}
		listed = append(listed, name+"="+value)
	}
	return listed
}

// redactJSON decodes a JSON document and encodes it again without secrets.
func redactJSON(data []byte) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return encodeJSON(redactValue("", value), "  ")
}

func encodeJSON(value any, indent string) ([]byte, error) {
	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indent)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return []byte(builder.String()), nil
}

// redactJSONLines applies redactJSON to each line of a JSON Lines file. A
// line that is not JSON is dropped, since it cannot be checked for secrets.
func redactJSONLines(data []byte) []byte {
	var builder strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var value any
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			builder.WriteString("{\"bug_report\":\"unreadable line omitted\"}\n")
			continue
		}
		encoded, err := encodeJSON(redactValue("", value), "")
		if err != nil {
			continue
		}
		builder.Write(encoded)
	}
	return []byte(builder.String())
}

// redactConfig parses a configuration file by its extension and encodes it
// as TOML without secrets.
func redactConfig(path string, data []byte) ([]byte, error) {
	values, err := config.Parse(path, data)
	if err != nil {
		return nil, err
	}
	redactedValues, _ := redactValue("", map[string]any(values)).(map[string]any)
	return config.Marshal(redactedValues)
}

// pathRedactor replaces known paths and hostnames with placeholders, then
// applies the run report's generic patterns.
type pathRedactor struct {
	replacer *strings.Replacer
}

func newPathRedactor(known map[string]string) pathRedactor {
	pairs := make([]string, 0, 2*len(known))
	for _, value := range sortedByLengthDescending(known) {
		pairs = append(pairs, value, known[value])
	}
	return pathRedactor{replacer: strings.NewReplacer(pairs...)}
}

func (redactor pathRedactor) apply(text string) string {
	if redactor.replacer == nil {
		return text
	}
	return report.RedactPatterns(redactor.replacer.Replace(text))
}

func sortedByLengthDescending(known map[string]string) []string {
	values := make([]string, 0, len(known))
	for value := range known {
		if len(value) > 1 {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(left, right int) bool {
		if len(values[left]) != len(values[right]) {
			return len(values[left]) > len(values[right])
		}
		return values[left] < values[right]
	})
	return values
}
