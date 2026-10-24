package main

import "github.com/rotari-dev/rotari/internal/runregistry"

// Test shorthands for the default run registry.

type runLocation = runregistry.Location

func registerRunLocation(location runLocation) error {
	registry, err := runregistry.Default()
	if err != nil {
		return err
	}
	return registry.Register(location)
}
