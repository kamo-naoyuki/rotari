package main

import (
	"flag"
	"testing"
)

func TestConfirmationEnvironmentVariablesCannotBypassSafety(t *testing.T) {
	for _, command := range []string{"cancel", "suspend", "resume"} {
		t.Run(command, func(t *testing.T) {
			t.Setenv(cliCommandEnvironmentVariable(command, "yes"), "true")
			fs := flag.NewFlagSet(command, flag.ContinueOnError)
			yes := cliBool(fs, "yes", false)
			if *yes {
				t.Fatal("environment must not bypass explicit confirmation")
			}
			if err := fs.Parse([]string{"--yes"}); err != nil {
				t.Fatal(err)
			}
			if !*yes {
				t.Fatal("explicit --yes must remain supported")
			}
		})
	}
}
