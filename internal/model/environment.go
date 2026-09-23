package model

import (
	"errors"
	"strings"
)

func ValidateEnvironment(environment []string) error {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !ValidEnvironmentName(name) || strings.ContainsRune(value, '\x00') {
			return errors.New("expected KEY=VALUE")
		}
	}
	return nil
}

func ValidEnvironmentName(name string) bool {
	for index := 0; index < len(name); index++ {
		character := name[index]
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' {
			continue
		}
		if index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return name != ""
}
