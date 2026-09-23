package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func writeRunContext(paths pathSet, runID, cwd string) error {
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		return err
	}
	context := captureRunContext(cwd)
	if configPath := effectiveConfigPath(paths.baseDir, paths.queueName); configPath != "" {
		context.ConfigPaths = []string{configPath}
	}
	if context.StartedLoad != nil {
		if err := appendLoadSample(loadSamplesPath(paths, runID), LoadSample{At: nowRFC3339Nano(), LoadAverage: *context.StartedLoad}); err != nil {
			return err
		}
	}
	return state.SaveContext(jsonStore(), runDir, context)
}

func finishRunContext(paths pathSet, runID string) error {
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		return err
	}
	context := RunContext{}
	if loaded, err := state.LoadContext(jsonStore(), runDir); err == nil {
		context = loaded
	}
	context.FinishedLoad = readLoadAverage()
	if context.FinishedLoad != nil {
		if err := appendLoadSample(loadSamplesPath(paths, runID), LoadSample{At: nowRFC3339Nano(), LoadAverage: *context.FinishedLoad}); err != nil {
			return err
		}
	}
	return state.SaveContext(jsonStore(), runDir, context)
}

func captureRunContext(cwd string) RunContext {
	hostname, _ := os.Hostname()
	return RunContext{CWD: cwd, Hostname: hostname, StartedLoad: readLoadAverage()}
}

const loadSampleInterval = 10 * time.Second

func startRunLoadSampling(paths pathSet, runID string) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(loadSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = appendRunLoadSample(paths, runID)
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

func loadSamplesPath(paths pathSet, runID string) string {
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		return ""
	}
	return filepath.Join(runDir, "load_samples.jsonl")
}

func appendLoadSample(path string, sample LoadSample) error {
	if err := os.MkdirAll(filepath.Dir(path), stateDirMode()); err != nil {
		return err
	}
	data, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, stateFileMode())
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

func readLoadSamples(path string) []LoadSample {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var samples []LoadSample
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var sample LoadSample
		if json.Unmarshal(line, &sample) == nil {
			samples = append(samples, sample)
		}
	}
	return samples
}

func appendRunLoadSample(paths pathSet, runID string) error {
	load := readLoadAverage()
	if load == nil {
		return nil
	}
	return appendLoadSample(loadSamplesPath(paths, runID), LoadSample{At: nowRFC3339Nano(), LoadAverage: *load})
}

func readLoadAverage() *LoadAverage {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return nil
	}
	one, oneErr := strconv.ParseFloat(fields[0], 64)
	five, fiveErr := strconv.ParseFloat(fields[1], 64)
	fifteen, fifteenErr := strconv.ParseFloat(fields[2], 64)
	if oneErr != nil || fiveErr != nil || fifteenErr != nil {
		return nil
	}
	return &LoadAverage{One: one, Five: five, Fifteen: fifteen}
}
