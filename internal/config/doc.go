// Package config finds and parses rotari's config files.
//
// A config file is config.yaml, config.toml, or config.json in the global
// config directory, a base directory, or a project directory. This package
// owns where those files are looked for, which scope applies, and how each
// format is parsed. What the keys mean (CLI option defaults) and the config
// template belong to cmd/rotari.
package config
