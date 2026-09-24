package main

import (
	"fmt"
	"runtime/debug"
)

var version = "dev"

func printVersion() {
	fmt.Println(versionString())
}

// versionString reports the release version, or for local "dev" builds
// (no ldflags override), appends the embedded VCS commit so dev builds
// remain clearly distinguishable from tagged releases.
func versionString() string {
	if version != "dev" {
		return "rotari " + version
	}
	if suffix := devBuildSuffix(); suffix != "" {
		return "rotari " + version + " " + suffix
	}
	return "rotari " + version
}

func devBuildSuffix() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return ""
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		revision += "-dirty"
	}
	return "(" + revision + ")"
}
