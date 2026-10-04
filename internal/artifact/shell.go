package artifact

import (
	"fmt"
	"path"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// maxShellDepth bounds nested shell source: the code operand of a shell in
// argv is depth 1, and a shell -c or heredoc inside it depth 2.
const maxShellDepth = 3

// shellOrigin says where a piece of shell source came from: the argv index
// of a -c operand, or a script file, and the line:column path to source
// nested inside it.
type shellOrigin struct {
	index    int
	file     string
	location string
}

func (origin shellOrigin) source(position syntax.Pos) Source {
	source := Source{Kind: KindShell, File: origin.file, Location: origin.location + positionText(position)}
	if origin.file == "" {
		source.Index = indexOf(origin.index)
	}
	return source
}

// nested returns the origin of shell source found at position, such as the
// code of a bash -c inside a script.
func (origin shellOrigin) nested(position syntax.Pos) shellOrigin {
	origin.location += positionText(position) + "/"
	return origin
}

func (origin shellOrigin) describe() string {
	name := origin.file
	if name == "" {
		name = fmt.Sprintf("argument %d", origin.index)
	}
	if origin.location != "" {
		name += " at " + strings.TrimSuffix(origin.location, "/")
	}
	return name
}

func positionText(position syntax.Pos) string {
	return fmt.Sprintf("%d:%d", position.Line(), position.Col())
}

func shellVariant(dialect interpreterKind) syntax.LangVariant {
	switch dialect {
	case bashShell:
		return syntax.LangBash
	case zshShell:
		return syntax.LangZsh
	}
	return syntax.LangPOSIX
}

// shell inspects shell source without running it: the literal arguments of
// each simple command, classified like argv words, and literal targets of
// file-opening redirections (PATH-R1). Plain $NAME and ${NAME} expand when
// rotari fixes the value for the attempt (PATH-E1); any other expansion,
// substitution, or glob skips the word.
func (c *collector) shell(code string, dialect interpreterKind, origin shellOrigin, depth int) {
	if depth > maxShellDepth {
		c.diagnose(origin.describe(), fmt.Sprintf("not inspected: shell source nested deeper than %d", maxShellDepth))
		return
	}
	if len(code) > MaxSourceBytes {
		c.diagnose(origin.describe(), fmt.Sprintf("not inspected: larger than %d bytes", MaxSourceBytes))
		return
	}
	file, err := syntax.NewParser(syntax.Variant(shellVariant(dialect))).Parse(strings.NewReader(code), "")
	if err != nil {
		c.diagnose(origin.describe(), "not inspected: cannot parse shell source")
		return
	}
	inspector := &shellInspector{collector: c, origin: origin, dialect: dialect, depth: depth, assigned: assignedNames(file)}
	syntax.Walk(file, func(node syntax.Node) bool {
		if statement, ok := node.(*syntax.Stmt); ok {
			inspector.statement(statement)
		}
		return true
	})
}

type shellInspector struct {
	collector *collector
	origin    shellOrigin
	dialect   interpreterKind
	depth     int
	// assigned holds the names the source assigns itself, which PATH-E1
	// does not expand.
	assigned map[string]bool
	// changedDirectory is set after the first cd, pushd, or popd: relative
	// references after it have no known base.
	changedDirectory bool
}

func (inspector *shellInspector) add(value string, source Source) {
	if inspector.changedDirectory && !path.IsAbs(value) {
		return
	}
	inspector.collector.add(value, source)
}

// statement inspects one statement: its simple command, then its
// redirections, in source order.
func (inspector *shellInspector) statement(statement *syntax.Stmt) {
	shape := commandShape{code: -1, text: -1, script: -1}
	if call, ok := statement.Cmd.(*syntax.CallExpr); ok {
		shape = inspector.call(call)
	}
	for _, redirect := range statement.Redirs {
		switch redirect.Op {
		case syntax.RdrOut, syntax.AppOut, syntax.RdrIn, syntax.RdrInOut, syntax.ClbOut, syntax.RdrAll, syntax.AppAll:
			value, expanded, ok := inspector.literal(redirect.Word)
			if !ok || value == "" || IsSpecialSink(value) {
				continue
			}
			source := inspector.origin.source(redirect.Word.Pos())
			source.Rule, source.Direction, source.Expanded = RuleRedirection, redirect.Op.String(), expanded
			inspector.add(value, source)
		case syntax.Hdoc, syntax.DashHdoc:
			// Only a shell that reads its commands from standard input runs
			// the body, and only a quoted delimiter keeps the outer shell
			// from expanding it first.
			if !shape.stdin || !quotedDelimiter(redirect.Word) {
				continue
			}
			if body, ok := heredocBody(redirect.Hdoc); ok {
				inspector.collector.shell(body, shape.dialect, inspector.origin.nested(redirect.Hdoc.Pos()), inspector.depth+1)
			}
		}
	}
}

// call inspects a simple command: its prefix assignments like environment
// entries, then its words like argv. It returns the command's shape.
func (inspector *shellInspector) call(call *syntax.CallExpr) commandShape {
	for _, assign := range call.Assigns {
		if assign.Name == nil || assign.Value == nil || assign.Array != nil || assign.Index != nil {
			continue
		}
		value, expanded, ok := inspector.literal(assign.Value)
		if !ok {
			continue
		}
		if rule, accepted := Classify(value, assign.Name.Value, Literal); accepted {
			source := inspector.origin.source(assign.Value.Pos())
			source.Rule, source.Key, source.Expanded = rule, assign.Name.Value, expanded
			inspector.add(value, source)
		}
	}
	words := make([]string, len(call.Args))
	expanded := make([]bool, len(call.Args))
	for index, word := range call.Args {
		value, wasExpanded, ok := inspector.literal(word)
		if !ok {
			// A control character is excluded everywhere, so the word is
			// never classified but still holds its position.
			value = "\x00"
		}
		words[index], expanded[index] = value, wasExpanded
	}
	inspector.collector.commandWords(words, inspector.add, func(index int) Source {
		source := inspector.origin.source(call.Args[index].Pos())
		source.Expanded = expanded[index]
		return source
	}, func(code string, dialect interpreterKind, index int) {
		if !strings.Contains(code, "\x00") {
			inspector.collector.shell(code, dialect, inspector.origin.nested(call.Args[index].Pos()), inspector.depth+1)
		}
	})
	if len(words) > 0 {
		switch path.Base(words[0]) {
		case "cd", "pushd", "popd":
			inspector.changedDirectory = true
		}
	}
	return recognizeCommand(words)
}

// literal returns the value of word after quote removal, and whether
// PATH-E1 expanded a variable in it. It reports false for a word whose value
// is not fixed: any other parameter expansion, a command, process, or
// arithmetic substitution, an unquoted glob, brace expansion, or leading ~.
func (inspector *shellInspector) literal(word *syntax.Word) (string, bool, bool) {
	if word == nil {
		return "", false, false
	}
	var value strings.Builder
	expanded := false
	for index, part := range word.Parts {
		switch part := part.(type) {
		case *syntax.Lit:
			text, ok := unquotedText(part.Value, index == 0, inspector.dialect != posixShell)
			if !ok {
				return "", false, false
			}
			value.WriteString(text)
		case *syntax.SglQuoted:
			if part.Dollar {
				return "", false, false
			}
			value.WriteString(part.Value)
		case *syntax.DblQuoted:
			if part.Dollar {
				return "", false, false
			}
			for _, inner := range part.Parts {
				switch inner := inner.(type) {
				case *syntax.Lit:
					value.WriteString(doubleQuotedText(inner.Value))
				case *syntax.ParamExp:
					text, ok := inspector.variable(inner)
					if !ok {
						return "", false, false
					}
					value.WriteString(text)
					expanded = true
				default:
					return "", false, false
				}
			}
		case *syntax.ParamExp:
			text, ok := inspector.variable(part)
			// Unquoted, the value would be split into words and globbed.
			if !ok || strings.ContainsAny(text, " \t\n*?[") {
				return "", false, false
			}
			value.WriteString(text)
			expanded = true
		default:
			return "", false, false
		}
	}
	return value.String(), expanded, true
}

// variable returns the PATH-E1 value of a plain $NAME or ${NAME}.
func (inspector *shellInspector) variable(expansion *syntax.ParamExp) (string, bool) {
	plain := expansion.Param != nil && expansion.Flags == nil &&
		!expansion.Excl && !expansion.Length && !expansion.Width && !expansion.IsSet &&
		expansion.NestedParam == nil && expansion.Index == nil &&
		len(expansion.Modifiers) == 0 && expansion.Slice == nil &&
		expansion.Repl == nil && expansion.Names == 0 && expansion.Exp == nil
	if !plain || inspector.assigned[expansion.Param.Value] {
		return "", false
	}
	value, ok := inspector.collector.variables[expansion.Param.Value]
	return value, ok
}

// unquotedText removes backslash escapes from an unquoted literal. It
// reports false for an unescaped glob character, a leading ~, or, with
// braces, a brace expansion such as {a,b} or {1..3}.
func unquotedText(raw string, first, braces bool) (string, bool) {
	var text strings.Builder
	open := -1
	for index := 0; index < len(raw); index++ {
		character := raw[index]
		switch {
		case character == '\\':
			index++
			if index < len(raw) && raw[index] != '\n' {
				text.WriteByte(raw[index])
			}
			continue
		case character == '*' || character == '?' || character == '[':
			return "", false
		case character == '~' && first && index == 0:
			return "", false
		case braces && character == '{':
			open = text.Len()
		case braces && character == '}' && open >= 0:
			inside := text.String()[open+1:]
			if strings.Contains(inside, ",") || strings.Contains(inside, "..") {
				return "", false
			}
		}
		text.WriteByte(character)
	}
	return text.String(), true
}

// doubleQuotedText removes the backslashes that escape $, `, ", \, or a
// newline inside double quotes; others stay.
func doubleQuotedText(raw string) string {
	var text strings.Builder
	for index := 0; index < len(raw); index++ {
		if raw[index] == '\\' && index+1 < len(raw) && strings.ContainsRune("$`\"\\\n", rune(raw[index+1])) {
			index++
			if raw[index] != '\n' {
				text.WriteByte(raw[index])
			}
			continue
		}
		text.WriteByte(raw[index])
	}
	return text.String()
}

// quotedDelimiter reports whether a heredoc delimiter is quoted: 'SH',
// "SH", or \SH.
func quotedDelimiter(word *syntax.Word) bool {
	if word == nil {
		return false
	}
	for _, part := range word.Parts {
		switch part := part.(type) {
		case *syntax.SglQuoted, *syntax.DblQuoted:
			return true
		case *syntax.Lit:
			if strings.Contains(part.Value, "\\") {
				return true
			}
		}
	}
	return false
}

func heredocBody(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}
	var body strings.Builder
	for _, part := range word.Parts {
		literal, ok := part.(*syntax.Lit)
		if !ok {
			return "", false
		}
		body.WriteString(literal.Value)
	}
	return body.String(), true
}

// assignedNames returns the variables a shell source sets itself, by
// assignment, declaration, for loop, read, getopts, or unset.
func assignedNames(file *syntax.File) map[string]bool {
	names := map[string]bool{}
	syntax.Walk(file, func(node syntax.Node) bool {
		switch node := node.(type) {
		case *syntax.Assign:
			if node.Name != nil {
				names[node.Name.Value] = true
			}
		case *syntax.ForClause:
			if loop, ok := node.Loop.(*syntax.WordIter); ok && loop.Name != nil {
				names[loop.Name.Value] = true
			}
		case *syntax.CallExpr:
			if len(node.Args) == 0 {
				break
			}
			switch node.Args[0].Lit() {
			case "read", "getopts", "unset", "export", "local", "declare", "typeset", "readonly":
				for _, word := range node.Args[1:] {
					if name := word.Lit(); name != "" && !strings.HasPrefix(name, "-") {
						name, _, _ = strings.Cut(name, "=")
						names[name] = true
					}
				}
			}
		}
		return true
	})
	return names
}
