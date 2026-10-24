package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/rotari-dev/rotari/internal/webui"
)

// cmdTableView serves the Web UI's Table viewer for one delimited file or
// directory, outside any project.
func cmdTableView(args []string) int {
	fs := newFlagSet("tableview")
	host := cliString(fs, "host", "127.0.0.1")
	port := cliInt(fs, "port", webui.TableViewPort)
	authToken := cliString(fs, "auth-token", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) != 1 || *port < 0 || *port > 65535 {
		printError("usage: " + cliUsage("tableview"))
		return 1
	}
	portExplicit := false
	fs.Visit(func(flag *flag.Flag) { portExplicit = portExplicit || flag.Name == "port" })
	path := fs.Args()[0]
	handler, err := webui.TableViewHandler(path)
	if err != nil {
		printErrorf("cannot view %s: %v", path, err)
		return 1
	}
	if *authToken != "" {
		handler = webui.WithAuthToken(handler, *authToken)
	} else if !webui.IsLoopbackHost(*host) {
		printErrorf("WARNING: --host %s exposes the files under %s over unauthenticated HTTP.", *host, path)
	}
	listener, err := webui.Listen(*host, *port, !portExplicit)
	if err != nil {
		printErrorf("tableview server failed: %v", err)
		return 1
	}
	server := &http.Server{Handler: handler}
	go func() {
		<-interruptSignal()
		_ = server.Close()
	}()
	fmt.Printf("rotari tableview listening at http://%s for %s\n", listener.Addr(), path)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		printErrorf("tableview server failed: %v", err)
		return 1
	}
	return 0
}
