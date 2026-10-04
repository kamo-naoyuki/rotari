package artifact

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// pythonFormat matches printf-style placeholders such as %s, %05d, and
// %(name)s, whose value is filled in at run time.
var pythonFormat = regexp.MustCompile(`%(\([A-Za-z_][A-Za-z0-9_]*\))?[-#0 +]*[0-9]*(\.[0-9]+)?[sdifrxXeEgGc]`)

// pythonToken is one lexical token of Python source: a name, a string, or an
// operator. Numbers, comments, and whitespace are dropped.
type pythonToken struct {
	kind  pythonTokenKind
	value string
	// literal reports a string whose value is fixed: not an f-string or a
	// bytes literal.
	literal      bool
	line, column int
}

type pythonTokenKind int

const (
	tokenName pythonTokenKind = iota
	tokenString
	tokenOperator
)

// python inspects the Python file at file without running or importing
// it: the default of each argparse add_argument call, classified with its
// option name as key, and every other fixed string literal, classified with
// the name it is assigned or passed to, or the dictionary key it follows.
func (c *collector) python(file string, data []byte) {
	tokens, err := pythonTokens(string(data))
	if err != nil {
		c.diagnose(file, "not inspected: "+err.Error())
		return
	}
	consumed := map[int]bool{}
	add := func(index int, key string) {
		token := tokens[index]
		consumed[index] = true
		if !token.literal || pythonFormat.MatchString(token.value) || strings.ContainsAny(token.value, "{}") {
			return
		}
		if rule, ok := Classify(token.value, key, Interpolated); ok {
			c.add(token.value, Source{Kind: KindPython, Rule: rule, Key: key, File: file, Location: fmt.Sprintf("%d:%d", token.line, token.column)})
		}
	}
	for index, token := range tokens {
		if token.kind == tokenName && token.value == "add_argument" && index+1 < len(tokens) && tokens[index+1].value == "(" && tokens[index+1].kind == tokenOperator {
			option, defaultIndex := pythonArgument(tokens, index+2)
			if defaultIndex >= 0 {
				add(defaultIndex, option)
			}
		}
	}
	for index, token := range tokens {
		if token.kind != tokenString || consumed[index] {
			continue
		}
		add(index, pythonKey(tokens, index))
	}
}

// pythonArgument reads the arguments of an add_argument call that starts at
// tokens[start] and returns its option name, preferring a long one, then
// dest, and the index of its default string, or -1.
func pythonArgument(tokens []pythonToken, start int) (string, int) {
	var options []string
	dest, defaultIndex := "", -1
	depth := 0
	argument := []int{}
	finish := func() {
		switch {
		case len(argument) == 1 && tokens[argument[0]].kind == tokenString && strings.HasPrefix(tokens[argument[0]].value, "-"):
			options = append(options, tokens[argument[0]].value)
		case len(argument) == 3 && tokens[argument[0]].kind == tokenName && tokens[argument[1]].value == "=" && tokens[argument[2]].kind == tokenString:
			switch tokens[argument[0]].value {
			case "default":
				defaultIndex = argument[2]
			case "dest":
				dest = tokens[argument[2]].value
			}
		}
		argument = argument[:0]
	}
	for index := start; index < len(tokens); index++ {
		token := tokens[index]
		if token.kind == tokenOperator {
			switch token.value {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth == 0 {
					finish()
					return chooseOption(options, dest), defaultIndex
				}
				depth--
			case ",":
				if depth == 0 {
					finish()
					continue
				}
			}
		}
		argument = append(argument, index)
	}
	return "", -1
}

func chooseOption(options []string, dest string) string {
	for _, option := range options {
		if strings.HasPrefix(option, "--") {
			return option
		}
	}
	if dest != "" {
		return dest
	}
	if len(options) > 0 {
		return options[0]
	}
	return ""
}

// pythonKey returns the key context of the string at index: NAME in
// "NAME = s" or "NAME=s", or KEY in "'KEY': s"; "" otherwise.
func pythonKey(tokens []pythonToken, index int) string {
	if index < 2 {
		return ""
	}
	previous, before := tokens[index-1], tokens[index-2]
	switch {
	case previous.kind == tokenOperator && previous.value == "=" && before.kind == tokenName:
		return before.value
	case previous.kind == tokenOperator && previous.value == ":" && before.kind == tokenString && before.literal:
		return before.value
	}
	return ""
}

// pythonTokens splits Python source into names, strings, and operators.
func pythonTokens(source string) ([]pythonToken, error) {
	var tokens []pythonToken
	line, lineStart := 1, 0
	for index := 0; index < len(source); {
		character := source[index]
		column := index - lineStart + 1
		switch {
		case character == '\n':
			line, lineStart = line+1, index+1
			index++
		case character == ' ' || character == '\t' || character == '\r' || character == '\f' || character == '\\':
			index++
		case character == '#':
			for index < len(source) && source[index] != '\n' {
				index++
			}
		case isNameStart(character):
			end := index
			for end < len(source) && isNamePart(source[end]) {
				end++
			}
			name := source[index:end]
			if end < len(source) && (source[end] == '\'' || source[end] == '"') && isStringPrefix(name) {
				token, next, lines, err := readPythonString(source, end, strings.ToLower(name))
				if err != nil {
					return nil, fmt.Errorf("%w at line %d", err, line)
				}
				token.line, token.column = line, column
				tokens = append(tokens, token)
				if lines > 0 {
					line += lines
					lineStart = strings.LastIndexByte(source[:next], '\n') + 1
				}
				index = next
				continue
			}
			tokens = append(tokens, pythonToken{kind: tokenName, value: name, line: line, column: column})
			index = end
		case character == '\'' || character == '"':
			token, next, lines, err := readPythonString(source, index, "")
			if err != nil {
				return nil, fmt.Errorf("%w at line %d", err, line)
			}
			token.line, token.column = line, column
			tokens = append(tokens, token)
			if lines > 0 {
				line += lines
				lineStart = strings.LastIndexByte(source[:next], '\n') + 1
			}
			index = next
		case character >= '0' && character <= '9' || character == '.' && index+1 < len(source) && source[index+1] >= '0' && source[index+1] <= '9':
			for index < len(source) && (isNamePart(source[index]) || source[index] == '.') {
				index++
			}
		default:
			value := string(character)
			if index+1 < len(source) && source[index+1] == '=' && strings.IndexByte("=!<>:+-*/%&|^@", character) >= 0 {
				value += "="
			}
			tokens = append(tokens, pythonToken{kind: tokenOperator, value: value, line: line, column: column})
			index += len(value)
		}
	}
	return tokens, nil
}

func isNameStart(character byte) bool {
	return character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= 0x80
}

func isNamePart(character byte) bool {
	return isNameStart(character) || character >= '0' && character <= '9'
}

func isStringPrefix(name string) bool {
	switch strings.ToLower(name) {
	case "r", "u", "b", "f", "br", "rb", "fr", "rf", "t", "tr", "rt":
		return true
	}
	return false
}

var errUnterminatedString = fmt.Errorf("unterminated string")

// tokenString reads the string literal whose opening quote is at
// source[start], with the lower-case prefix before it. It returns the token,
// the index after the closing quote, and the newlines it spans.
func readPythonString(source string, start int, prefix string) (pythonToken, int, int, error) {
	quote := source[start]
	delimiter := string(quote)
	if strings.HasPrefix(source[start:], strings.Repeat(string(quote), 3)) {
		delimiter = strings.Repeat(string(quote), 3)
	}
	raw := strings.Contains(prefix, "r")
	var value strings.Builder
	lines := 0
	for index := start + len(delimiter); index < len(source); index++ {
		if strings.HasPrefix(source[index:], delimiter) {
			token := pythonToken{kind: tokenString, value: value.String()}
			token.literal = !strings.ContainsAny(prefix, "bft")
			return token, index + len(delimiter), lines, nil
		}
		character := source[index]
		switch {
		case character == '\n':
			if len(delimiter) == 1 {
				return pythonToken{}, 0, 0, errUnterminatedString
			}
			lines++
			value.WriteByte(character)
		case character == '\\' && index+1 < len(source):
			next := source[index+1]
			index++
			if next == '\n' {
				lines++
				if raw {
					value.WriteString("\\\n")
				}
				continue
			}
			if raw {
				value.WriteByte('\\')
				value.WriteByte(next)
				continue
			}
			switch next {
			case 'n':
				value.WriteByte('\n')
			case 't':
				value.WriteByte('\t')
			case '\\', '\'', '"':
				value.WriteByte(next)
			default:
				value.WriteByte('\\')
				value.WriteByte(next)
			}
		default:
			value.WriteByte(character)
		}
	}
	return pythonToken{}, 0, 0, errUnterminatedString
}

// pythonFiles inspects every .py candidate referenced by an argument, an
// environment value, or shell source, through read. Modules run with -m and
// files the Python code imports are not followed.
func (c *collector) pythonFiles(read SourceReader) {
	var files []string
	for _, candidate := range c.result.Candidates {
		if candidate.Basis == BasisUnresolved || !strings.EqualFold(path.Ext(candidate.Path), ".py") {
			continue
		}
		for _, source := range candidate.Sources {
			if source.Kind == KindArgument || source.Kind == KindEnvironment || source.Kind == KindShell {
				files = append(files, candidate.Path)
				break
			}
		}
	}
	for _, file := range files {
		data, err := read(file)
		if err != nil {
			c.diagnose(file, "not inspected: "+err.Error())
			continue
		}
		c.python(file, data)
	}
}
