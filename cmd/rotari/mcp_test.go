package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

func TestMCPStdioError(t *testing.T) {
	closing := &jsonrpc.Error{Code: -32004, Message: "server is closing"}
	for _, test := range []struct {
		name  string
		err   error
		clean bool
	}{
		{name: "normal shutdown", clean: true},
		{name: "SDK write after EOF", err: fmt.Errorf("%w: %v", closing, io.EOF), clean: true},
		{name: "unexpected EOF", err: fmt.Errorf("%w: %v", closing, io.ErrUnexpectedEOF)},
		{name: "broken output", err: fmt.Errorf("%w: broken pipe", closing)},
		{name: "closing without EOF", err: closing},
		{name: "cancelled", err: context.Canceled},
		{name: "parse error", err: &jsonrpc.Error{Code: jsonrpc.CodeParseError, Message: "server is closing: EOF"}},
		{name: "untyped lookalike", err: errors.New("server is closing: EOF")},
		{name: "joined failure", err: errors.Join(fmt.Errorf("%w: %v", closing, io.EOF), errors.New("broken pipe"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := mcpStdioError(test.err)
			if test.clean {
				if got != nil {
					t.Fatalf("mcpStdioError(%v) = %v, want nil", test.err, got)
				}
			} else if got != test.err {
				t.Fatalf("mcpStdioError(%v) = %v, want original error", test.err, got)
			}
		})
	}
}
