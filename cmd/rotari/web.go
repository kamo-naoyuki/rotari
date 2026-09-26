package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/webui"
)

// cmdWeb serves the embedded Web UI or writes a static export of project state.
func cmdWeb(args []string) int {
	fs := newFlagSet("web")
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	host := cliString(fs, "host", "127.0.0.1")
	port := cliInt(fs, "port", webui.DefaultPort)
	staticDir := cliString(fs, "static-dir", "")
	allowControl := cliBool(fs, "allow-control", true)
	authToken := cliString(fs, "auth-token", "")
	notifications := cliBool(fs, "notifications", true)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 || *port < 0 || *port > 65535 {
		printError("usage: " + cliUsage("web"))
		return 1
	}
	portExplicit := false
	fs.Visit(func(flag *flag.Flag) {
		portExplicit = portExplicit || flag.Name == "port"
	})
	baseDir, _, err := stateinternal.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	options := webOptions(baseDir, *queueNameOption, *allowControl, *notifications)
	if *staticDir != "" {
		if err := webui.GenerateStatic(*staticDir, options); err != nil {
			printErrorf("failed to generate static web: %v", err)
			return 1
		}
		return 0
	}
	if !webui.IsLoopbackHost(*host) && *authToken == "" {
		controlWarning := "job logs and environment variable names"
		if *allowControl {
			controlWarning = "job logs, environment variable names, and job control (cancel/suspend/resume/change/remove/copy) operations"
		}
		printErrorf("WARNING: --host %s exposes %s over unauthenticated HTTP.", *host, controlWarning)
	}
	handler := webui.Handler(options)
	if *authToken != "" {
		handler = webui.WithAuthToken(handler, *authToken)
	}
	listener, err := webui.Listen(*host, *port, !portExplicit)
	if err != nil {
		printErrorf("web server failed: %v", err)
		return 1
	}
	server := &http.Server{Handler: handler}
	go func() {
		<-interruptSignal()
		_ = server.Close()
	}()
	fmt.Printf("rotari web listening at http://%s\n", listener.Addr())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		printErrorf("web server failed: %v", err)
		return 1
	}
	return 0
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func interruptSignal() <-chan os.Signal {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return signals
}

// webOptions wires the Web UI to this command's state, executors, and CLI
// metadata.
func webOptions(baseDir, projectFilter string, allowControl, notifications bool) webui.Options {
	return webui.Options{
		BaseDir: baseDir, ProjectFilter: projectFilter, AllowControl: allowControl, Notifications: notifications,
		Store: jsonStore(), Editor: queueEditor(), Controller: jobController(),
		Executors: executorRegistry.Names(), Environments: environmentDefinitions(), Commands: cliCommandDocs(),
		ConfigTemplate: func() ([]byte, error) { return configTemplate("toml") },
	}
}

// cliCommandDocs describes every command for the Web UI's docs page.
func cliCommandDocs() []webui.CommandDoc {
	docs := make([]webui.CommandDoc, 0, len(cliCommandSpecs))
	for _, command := range cliCommandSpecs {
		doc := webui.CommandDoc{Name: command.Name, Description: command.Description, Usage: cliUsage(command.Name)}
		for _, flagSpec := range command.Flags {
			values := flagSpec.ValueName
			if len(flagSpec.Values) > 0 {
				values = strings.Join(flagSpec.Values, ", ")
			}
			doc.Flags = append(doc.Flags, webui.FlagDoc{Name: flagSpec.Name, Description: flagSpec.Description, Values: values})
		}
		for _, subcommand := range command.Subcommands {
			doc.Subcommands = append(doc.Subcommands, webui.SubcommandDoc{Name: subcommand.Name, Description: subcommand.Description})
		}
		docs = append(docs, doc)
	}
	return docs
}
