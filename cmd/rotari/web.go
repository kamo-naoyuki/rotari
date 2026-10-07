package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/notification"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/webui"
)

// cmdWeb serves the embedded Web UI or writes a static export of project state.
func cmdWeb(args []string) int {
	flags, code := parseWebFlags(args)
	if code != 0 {
		return code
	}
	baseDir, _, err := stateinternal.ResolveBaseDir(flags.basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	options := webOptions(baseDir, flags.allowControl, flags.notifications)
	loadedNotifications, err := notification.Load(baseDir, "")
	if err != nil {
		printErrorf("failed to load notification configuration: %v", err)
		return 1
	}
	options.NotificationSettings = loadedNotifications.Settings.Browser
	if flags.staticDir != "" {
		options.StaticArtifactContents = flags.staticArtifactContents
		options.StaticArtifactsCopied = func(files int, bytes int64) {
			if files == 0 {
				return
			}
			printWarningf("Copied %d artifact files (%d bytes) into %s; anyone who can read the export can read them.", files, bytes, flags.staticDir)
		}
		return generateStaticWeb(flags.staticDir, options)
	}
	return serveWeb(flags, baseDir, options)
}

type webCommandFlags struct {
	basedir, host, staticDir, authToken string
	port                                int
	allowControl, notifications         bool
	portExplicit                        bool
	// artifactRoots are absolute --artifact-root directories.
	artifactRoots []string
	// staticArtifactContents copies artifact files into a static export.
	staticArtifactContents bool
}

func parseWebFlags(args []string) (webCommandFlags, int) {
	fs := newFlagSet("web")
	basedir := cliString(fs, "basedir", "")
	host := cliString(fs, "host", "127.0.0.1")
	port := cliInt(fs, "port", webui.DefaultPort)
	staticDir := cliString(fs, "static-dir", "")
	allowControl := cliBool(fs, "allow-control", true)
	authToken := cliString(fs, "auth-token", "")
	notifications := cliBool(fs, "notifications", true)
	var artifactRoots stringSliceFlag
	cliValue(fs, &artifactRoots, "artifact-root")
	staticArtifactContents := cliBool(fs, "static-artifact-contents", false)
	if err := cliParse(fs, args); err != nil {
		return webCommandFlags{}, 1
	}
	if len(fs.Args()) != 0 || *port < 0 || *port > 65535 {
		printError("usage: " + cliUsage("web"))
		return webCommandFlags{}, 1
	}
	if *staticDir != "" {
		serverOptions := webStaticServerOptions(fs)
		if len(serverOptions) > 0 {
			printErrorf("--%s cannot be combined with --static-dir; these options only apply to the live web server", strings.Join(serverOptions, ", --"))
			return webCommandFlags{}, 1
		}
	}
	if *staticArtifactContents && *staticDir == "" {
		printError("--static-artifact-contents requires --static-dir")
		return webCommandFlags{}, 1
	}
	flags := webCommandFlags{basedir: *basedir, host: *host, port: *port, staticDir: *staticDir, authToken: *authToken, allowControl: *allowControl, notifications: *notifications, staticArtifactContents: *staticArtifactContents}
	for _, root := range artifactRoots {
		absolute, err := filepath.Abs(root)
		if info, statErr := os.Stat(absolute); err != nil || statErr != nil || !info.IsDir() {
			printErrorf("--artifact-root %s is not a directory", root)
			return webCommandFlags{}, 1
		}
		flags.artifactRoots = append(flags.artifactRoots, absolute)
	}
	fs.Visit(func(flag *flag.Flag) {
		flags.portExplicit = flags.portExplicit || flag.Name == "port"
	})
	return flags, 0
}

func generateStaticWeb(output string, options webui.Options) int {
	if err := webui.GenerateStatic(output, options); err != nil {
		printErrorf("failed to generate static web: %v", err)
		return 1
	}
	return 0
}

func serveWeb(flags webCommandFlags, baseDir string, options webui.Options) int {
	registry, err := basedirregistry.Default()
	if err != nil {
		printErrorf("failed to resolve base directory registry: %v", err)
		return 1
	}
	options.BaseDirs, err = registry.BaseDirs()
	if err != nil {
		printErrorf("failed to list registered base directories: %v", err)
		return 1
	}
	options.ArtifactRoots = flags.artifactRoots
	if !webui.IsLoopbackHost(flags.host) && flags.authToken == "" {
		controlWarning := "registered basedir paths, job logs, artifact files under job working directories and --artifact-root, and environment variable names"
		if flags.allowControl {
			controlWarning = "registered basedir paths, job logs, artifact files under job working directories and --artifact-root, environment variable names, and job control (cancel/suspend/resume/change/remove/copy) operations"
		}
		printErrorf("WARNING: --host %s exposes %s over unauthenticated HTTP.", flags.host, controlWarning)
	}
	handler := webui.Handler(options)
	if flags.authToken != "" {
		handler = webui.WithAuthToken(handler, flags.authToken)
	}
	listener, err := webui.Listen(flags.host, flags.port, !flags.portExplicit)
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

func webStaticServerOptions(fs *flag.FlagSet) []string {
	var incompatible []string
	for _, name := range []string{"allow-control", "artifact-root", "auth-token", "host", "port"} {
		provided := cliOptionSet(fs, name)
		if !provided {
			if envName := cliEnvironmentVariable(name); envName != "" {
				_, provided = os.LookupEnv(envName)
			}
		}
		if !provided {
			_, provided = configValue(name)
		}
		if provided {
			incompatible = append(incompatible, name)
		}
	}
	return incompatible
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
func webOptions(baseDir string, allowControl, notifications bool) webui.Options {
	return webui.Options{
		BaseDir: baseDir, RootBaseDir: baseDir, AllowControl: allowControl, Notifications: notifications,
		Store: jsonStore(), Editor: queueEditor(), Controller: jobController(),
		Executors:              executorRegistry.Names(),
		Environments:           environmentDefinitions(),
		ConfigTemplate:         func() ([]byte, error) { return configTemplate("toml") },
		ConfigTemplateForScope: func(scope string) ([]byte, error) { return configTemplate("toml", scope) },
	}
}
