package main

import (
	"fmt"
	"strconv"
	"strings"
)

func showBaseDirs(masterDir string) int {
	servers, err := listServers(masterDir)
	if err != nil {
		printErrorf("failed to list servers: %v", err)
		return 1
	}
	baseDirs, err := listKnownBaseDirs(masterDir, servers)
	if err != nil {
		printErrorf("failed to list known state directories: %v", err)
		return 1
	}
	fmt.Printf("%s\n%s %s\n", cyan("=== BASE DIRECTORIES ==="), cyan("Master directory:"), masterDir)
	if len(baseDirs) == 0 {
		fmt.Println("No known state directories.")
		return 0
	}
	fmt.Printf("\n%s\n", cyan(fmt.Sprintf("Known state directories: %d", len(baseDirs))))
	fmt.Println(cyan(fmt.Sprintf("%-8s %-24s %s", "PID", "SOURCE", "BASE DIRECTORY")))
	for _, baseDir := range baseDirs {
		pid := "-"
		if baseDir.PID != 0 {
			pid = strconv.Itoa(baseDir.PID)
		}
		fmt.Printf("%-8s %-24s %s\n", pid, strings.Join(baseDir.Sources, ", "), baseDir.BaseDir)
	}
	return 0
}
