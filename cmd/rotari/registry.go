package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type serverRecord struct {
	BaseDir   string `json:"base_dir"`
	Project   string `json:"project"`
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
	LastSeen  string `json:"last_seen"`
}

type knownBaseDir struct {
	BaseDir string
	Sources []string
	PID     int
}

func serverRecordPath(masterDir, baseDir, project string) string {
	sum := sha256.Sum256([]byte(baseDir + "\x00" + project))
	return filepath.Join(masterDir, hex.EncodeToString(sum[:])[:16]+".json")
}

func registerServer(masterDir string, record serverRecord) error {
	if err := os.MkdirAll(masterDir, state.DirectoryMode()); err != nil {
		return err
	}
	return state.WriteJSON(serverRecordPath(masterDir, record.BaseDir, record.Project), record)
}

func unregisterServer(masterDir, baseDir, project string) error {
	err := os.Remove(serverRecordPath(masterDir, baseDir, project))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func touchServerRecord(masterDir, baseDir, project string) error {
	path := serverRecordPath(masterDir, baseDir, project)
	var record serverRecord
	if err := jsonStore().ReadJSON(path, &record); err != nil {
		return err
	}
	record.LastSeen = nowRFC3339()
	return state.WriteJSON(path, record)
}

func listServers(masterDir string) ([]serverRecord, error) {
	entries, err := os.ReadDir(masterDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	servers := make([]serverRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(masterDir, entry.Name())
		var record serverRecord
		if err := jsonStore().ReadJSON(path, &record); err != nil {
			if errors.Is(err, state.ErrInvalidJSON) {
				_ = os.Remove(path)
			}
			continue
		}
		paths, err := state.ResolveProjectPaths(record.BaseDir, record.Project)
		if record.BaseDir == "" || err != nil {
			_ = os.Remove(path)
			continue
		}
		if pid, running := serverinternal.Running(paths.ProjectDir); !running || pid != record.PID {
			_ = os.Remove(path)
			continue
		}
		record.LastSeen = nowRFC3339()
		_ = state.WriteJSON(path, record)
		servers = append(servers, record)
	}
	sort.Slice(servers, func(i, j int) bool {
		if servers[i].BaseDir != servers[j].BaseDir {
			return servers[i].BaseDir < servers[j].BaseDir
		}
		return servers[i].Project < servers[j].Project
	})
	return servers, nil
}

func formatServerList(servers []serverRecord) string {
	if len(servers) == 0 {
		return "no running servers"
	}
	lines := make([]string, 0, len(servers)+1)
	lines = append(lines, fmt.Sprintf("%-8s %-24s %-20s %s", "PID", "LAST_SEEN", "PROJECT", "BASE_DIR"))
	for _, server := range servers {
		lines = append(lines, fmt.Sprintf("%-8d %-24s %-20s %s", server.PID, server.LastSeen, server.Project, server.BaseDir))
	}
	return strings.Join(lines, "\n")
}

func listKnownBaseDirs(masterDir string, servers []serverRecord) ([]knownBaseDir, error) {
	type knownBaseDirState struct {
		sources map[string]bool
		pid     int
	}
	known := make(map[string]knownBaseDirState)
	registryBaseDirs, fromRuns, err := basedirregistry.Discover(masterDir)
	if err != nil {
		return nil, err
	}
	if fromRuns {
		// Migrate a master directory from before the basedir registry.
		for _, baseDir := range registryBaseDirs {
			_ = basedirregistry.Open(masterDir).Register(baseDir)
		}
	}
	for _, baseDir := range registryBaseDirs {
		value := known[baseDir]
		if value.sources == nil {
			value.sources = make(map[string]bool)
		}
		value.sources["run registry"] = true
		known[baseDir] = value
	}
	for _, server := range servers {
		value := known[server.BaseDir]
		if value.sources == nil {
			value.sources = make(map[string]bool)
		}
		value.sources["server registry"] = true
		value.pid = server.PID
		known[server.BaseDir] = value
	}
	baseDirs := make([]string, 0, len(known))
	for baseDir := range known {
		baseDirs = append(baseDirs, baseDir)
	}
	sort.Strings(baseDirs)
	result := make([]knownBaseDir, 0, len(baseDirs))
	for _, baseDir := range baseDirs {
		value := known[baseDir]
		sources := make([]string, 0, len(value.sources))
		for source := range value.sources {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		result = append(result, knownBaseDir{BaseDir: baseDir, Sources: sources, PID: value.pid})
	}
	return result, nil
}
