package artifact

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// commandShape is what recognizeCommand finds in one command's words.
type commandShape struct {
	// commandWords are the indexes of recognized launcher and interpreter
	// words. They name programs, not artifacts.
	commandWords []int
	// code is the index of the interpreter's code operand, or -1. The whole
	// word is excluded from ordinary classification (PATH-X4), including a
	// node --eval=CODE word that carries the code after its option name.
	code int
	// shell reports that the code operand is shell source, which only shell
	// inspection may read.
	shell bool
	// text is the index of a text command such as echo, or -1. Its
	// arguments are text to print, not references (PATH-X5).
	text int
	// dialect is the recognized shell, or notInterpreter.
	dialect interpreterKind
	// script is the index of a shell's script operand when it has no -c,
	// or -1.
	script int
	// stdin reports a shell with neither -c nor a script operand, which
	// reads its commands from standard input.
	stdin bool
}

// maxLauncherDepth bounds how many nested listed launchers are followed.
const maxLauncherDepth = 4

type interpreterKind int

const (
	notInterpreter interpreterKind = iota
	posixShell
	bashShell
	zshShell
	pythonInterpreter
	perlInterpreter
	nodeInterpreter
)

var pythonName = regexp.MustCompile(`^(python|python2|python3|python3\.[0-9]+|pypy|pypy3)$`)

func interpreterOf(word string) interpreterKind {
	switch name := strings.ToLower(path.Base(word)); {
	case name == "sh" || name == "dash":
		return posixShell
	case name == "bash":
		return bashShell
	case name == "zsh":
		return zshShell
	case pythonName.MatchString(name):
		return pythonInterpreter
	case name == "perl":
		return perlInterpreter
	case name == "node":
		return nodeInterpreter
	}
	return notInterpreter
}

// isTextCommand reports whether word runs a command whose arguments are
// text it prints, never files it opens: echo and printf.
func isTextCommand(word string) bool {
	switch strings.ToLower(path.Base(word)) {
	case "echo", "printf":
		return true
	}
	return false
}

func isLauncher(word string) bool {
	switch strings.ToLower(path.Base(word)) {
	case "env", "timeout", "srun":
		return true
	}
	return false
}

// srunFreeTextOptions take a separate value that plausibly resembles a
// command, so the value is never taken for the wrapped interpreter. Add an
// option only for a demonstrated false match.
var srunFreeTextOptions = []string{"--job-name", "-J", "--partition", "-p"}

// recognizeCommand finds a code-bearing interpreter invocation or a text
// command (echo, printf): one at words[0], or one found by scanning the
// arguments of a recognized launcher (env, timeout, srun). The arguments of
// any other command are never searched, so an interpreter-looking word
// passed to it stays an ordinary argument.
func recognizeCommand(words []string) commandShape {
	shape := commandShape{code: -1, text: -1, script: -1}
	index := 0
	for depth := 0; index < len(words); depth++ {
		if isTextCommand(words[index]) {
			shape.commandWords = append(shape.commandWords, index)
			shape.text = index
			return shape
		}
		if kind := interpreterOf(words[index]); kind != notInterpreter {
			shape.commandWords = append(shape.commandWords, index)
			if isShell(kind) {
				shape.dialect, shape.shell = kind, true
				shape.code, shape.script, shape.stdin = shellOperands(kind, words, index+1)
				return shape
			}
			shape.code = codeOperand(kind, words, index+1)
			return shape
		}
		if !isLauncher(words[index]) || depth >= maxLauncherDepth {
			return shape
		}
		shape.commandWords = append(shape.commandWords, index)
		next := scanLauncher(words, index)
		if next < 0 {
			return shape
		}
		index = next
	}
	return shape
}

// scanLauncher returns the index of the first listed interpreter, text
// command, or launcher among the launcher's arguments, or -1. It is a best-effort scan,
// not a launcher grammar: only srun's free-text option values are skipped.
func scanLauncher(words []string, launcher int) int {
	srun := strings.EqualFold(path.Base(words[launcher]), "srun")
	for index := launcher + 1; index < len(words); index++ {
		if srun && slices.Contains(srunFreeTextOptions, words[index]) {
			index++
			continue
		}
		if interpreterOf(words[index]) != notInterpreter || isTextCommand(words[index]) || isLauncher(words[index]) {
			return index
		}
	}
	return -1
}

func isShell(kind interpreterKind) bool {
	return kind == posixShell || kind == bashShell || kind == zshShell
}

// codeOperand parses the options of Python, Perl, or Node starting at
// words[start] and returns the index of the code operand, or -1. Options not
// listed for the interpreter are boolean: they consume no word. Parsing stops
// at the first operand or "--".
func codeOperand(kind interpreterKind, words []string, start int) int {
	switch kind {
	case pythonInterpreter:
		return valueCodeOperand(words, start, pythonOptions)
	case perlInterpreter:
		return valueCodeOperand(words, start, perlOptions)
	case nodeInterpreter:
		return valueCodeOperand(words, start, nodeOptions)
	}
	return -1
}

// shellOperands handles sh, dash, bash, and zsh, where -c is a flag meaning
// "read commands from the first operand". A c in a valid short option bundle
// (-c, -lc, -ce, -xec) sets it; -o/+o, and for bash and zsh -O/+O, consume
// the next word per occurrence in the bundle. It returns the index of the
// code operand or, without -c, of the script operand, each -1 when absent,
// and whether the shell reads its commands from standard input: neither -c
// nor a script operand.
func shellOperands(kind interpreterKind, words []string, start int) (code, script int, stdin bool) {
	valueLetters := "o"
	if kind != posixShell {
		valueLetters = "oO"
	}
	command := false
	index := start
	for index < len(words) {
		word := words[index]
		if word == "--" || word == "-" {
			index++
			break
		}
		if strings.HasPrefix(word, "--") {
			index++
			if kind == bashShell && (word == "--rcfile" || word == "--init-file") {
				index++
			}
			continue
		}
		if len(word) < 2 || (word[0] != '-' && word[0] != '+') || !isLetters(word[1:]) {
			break
		}
		index++
		for _, letter := range word[1:] {
			switch {
			case letter == 'c' && word[0] == '-':
				command = true
			case strings.ContainsRune(valueLetters, letter):
				index++
			}
		}
	}
	switch {
	case index >= len(words):
		return -1, -1, !command
	case command:
		return index, -1, false
	}
	return -1, index, false
}

func isLetters(value string) bool {
	for _, letter := range value {
		if (letter < 'a' || letter > 'z') && (letter < 'A' || letter > 'Z') {
			return false
		}
	}
	return value != ""
}

// optionGrammar lists one interpreter's code options and the options that
// take a value. Short value options also accept an attached value (-Wignore);
// long ones also accept --name=value. Optional-value options take only an
// attached value and never the next word.
type optionGrammar struct {
	code []string
	// codeEquals are long code options whose --name=CODE form carries the
	// code in the same word.
	codeEquals []string
	value      []string
	// anyEquals makes every --name=value word self-contained.
	anyEquals bool
}

var (
	pythonOptions = optionGrammar{
		code:  []string{"-c"},
		value: []string{"-W", "-X", "--check-hash-based-pycs"},
	}
	// Perl's optional-value switches (-C, -F, -0, -i, -l, -x) take only an
	// attached value, so they parse as boolean words.
	perlOptions = optionGrammar{
		code:  []string{"-e", "-E"},
		value: []string{"-I", "-M", "-m"},
	}
	nodeOptions = optionGrammar{
		code:       []string{"-e", "--eval", "-p", "--print"},
		codeEquals: []string{"--eval", "--print"},
		value:      []string{"-r", "--require", "--import"},
		anyEquals:  true,
	}
)

// valueCodeOperand handles Python, Perl, and Node, whose code option takes
// the code as its value: the code operand is the word right after it.
func valueCodeOperand(words []string, start int, grammar optionGrammar) int {
	for index := start; index < len(words); index++ {
		word := words[index]
		if word == "--" || word == "-" || !strings.HasPrefix(word, "-") {
			return -1
		}
		if slices.Contains(grammar.code, word) {
			if index+1 < len(words) {
				return index + 1
			}
			return -1
		}
		if name, _, found := strings.Cut(word, "="); found && strings.HasPrefix(word, "--") {
			if slices.Contains(grammar.codeEquals, name) {
				return index
			}
			if grammar.anyEquals || slices.Contains(grammar.value, name) {
				continue
			}
		}
		if slices.Contains(grammar.value, word) {
			index++
		}
		// Anything else, including an attached short value such as -Xdev or
		// -MData::Dumper, is one self-contained word.
	}
	return -1
}
