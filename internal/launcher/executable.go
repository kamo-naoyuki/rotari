package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// shellBuiltins name the first words that are not programs on PATH, so a
// command starting with one cannot be checked by name.
var shellBuiltins = map[string]bool{
	".": true, ":": true, "[": true, "alias": true, "bg": true, "break": true, "cd": true, "command": true,
	"continue": true, "echo": true, "eval": true, "exec": true, "exit": true, "export": true, "false": true,
	"fg": true, "getopts": true, "hash": true, "jobs": true, "kill": true, "printf": true, "pwd": true,
	"read": true, "readonly": true, "return": true, "set": true, "shift": true, "source": true, "test": true,
	"times": true, "trap": true, "true": true, "type": true, "ulimit": true, "umask": true, "unalias": true,
	"unset": true, "wait": true,
}

// Executable is the program a command runs first: the first word of its
// first simple command, after any variable assignments. It is "" when that
// word is not a plain literal (it uses an expansion or quoting), is a shell
// builtin, or the command does not parse, since none of those can be
// checked by name. A relative or ~ path is returned as written.
func (command Command) Executable() string {
	file, err := syntax.NewParser().Parse(strings.NewReader(command.Command), "")
	if err != nil {
		return ""
	}
	name := ""
	found := false
	syntax.Walk(file, func(node syntax.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		found = true
		if len(call.Args) == 0 {
			return false
		}
		if word := call.Args[0].Lit(); word != "" && !shellBuiltins[word] {
			name = word
		}
		return false
	})
	return name
}

// Unavailable says why the command cannot run on this host, or "" when it
// can or cannot be checked. Only its first program is looked up, on the
// Web server's PATH, which launched commands inherit; a program it starts
// later, such as the tool `conda run` runs, is not checked.
func (command Command) Unavailable() string {
	name := command.Executable()
	if name == "" {
		return ""
	}
	if strings.Contains(name, "/") || strings.HasPrefix(name, "~") {
		// A relative path is relative to the launch's working directory,
		// and a tilde is expanded by the shell; only an absolute path can
		// be checked here.
		if !strings.HasPrefix(name, "/") {
			return ""
		}
		info, err := os.Stat(name)
		if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
			return fmt.Sprintf("%s is not an executable file", name)
		}
		return ""
	}
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Sprintf("%s was not found on PATH", name)
	}
	return ""
}
