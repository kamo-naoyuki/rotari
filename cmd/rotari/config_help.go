package main

import (
	"errors"
	"flag"
	"io"
)

// loadCLIConfigForCommand keeps help accessible when configuration cannot be
// loaded. Normal execution still requires successfully loaded configuration.
func loadCLIConfigForCommand(command string, args []string) error {
	err := loadCLIConfig(args)
	if err == nil || !commandHelpRequested(command, args) {
		return err
	}
	cliConfig, cliConfigPath = nil, ""
	printWarningf("WARNING: failed to load config: %v; showing help without configuration defaults", err)
	return nil
}

// commandHelpRequested parses only the public option shapes, without loading
// defaults or running a command. In particular, help used as an option value
// or as part of add/change's job command is not a CLI help request.
func commandHelpRequested(command string, args []string) bool {
	for _, spec := range cliCommandSpecs {
		if spec.Name == command {
			return parseCommandHelp(spec, args)
		}
	}
	return false
}

func parseCommandHelp(spec cliCommandSpec, args []string) bool {
	fs := flag.NewFlagSet(spec.Name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for _, option := range spec.Flags {
		registerHelpOption(fs, option.Name, option.ValueName != "")
		if short := cliShortFlagNames[option.Name]; short != "" {
			registerHelpOption(fs, short, option.ValueName != "")
		}
	}
	for {
		if err := fs.Parse(args); err != nil {
			return errors.Is(err, flag.ErrHelp)
		}
		rest := fs.Args()
		consumed := len(args) - len(rest)
		if len(rest) == 0 || (consumed > 0 && args[consumed-1] == "--") || spec.Name == "add" || spec.Name == "change" {
			return false
		}
		args = rest[1:]
	}
}

func registerHelpOption(fs *flag.FlagSet, name string, takesValue bool) {
	if takesValue {
		fs.String(name, "", "")
	} else {
		fs.Bool(name, false, "")
	}
}
