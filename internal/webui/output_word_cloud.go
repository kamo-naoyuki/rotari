package webui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

const (
	outputWordCloudCacheFile = "output-word-cloud.json"
	outputWordCloudVersion   = 1
	outputWordCloudTermLimit = 100
)

var outputWordCloudToken = regexp.MustCompile(`[[:alpha:]][[:alnum:]_-]{2,}`)

type outputWordCloud struct {
	Version     int                   `json:"version"`
	GeneratedAt string                `json:"generated_at"`
	TotalJobs   int                   `json:"total_jobs"`
	TotalBytes  int64                 `json:"total_bytes"`
	Terms       []outputWordCloudTerm `json:"terms"`
}

type outputWordCloudTerm struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
	Jobs  int    `json:"jobs"`
}

func loadOutputWordCloud(runDir, runsDir, runID string, jobs []webprojection.Job, refresh bool) (outputWordCloud, error) {
	cachePath, err := stateinternal.SafeJoin(runDir, outputWordCloudCacheFile)
	if err != nil {
		return outputWordCloud{}, err
	}
	if !refresh {
		var cached outputWordCloud
		if err := readOutputWordCloudCache(cachePath, &cached); err == nil && cached.Version == outputWordCloudVersion {
			return cached, nil
		}
	}

	cloud, err := buildOutputWordCloud(runsDir, runID, jobs)
	if err != nil {
		return outputWordCloud{}, err
	}
	if err := writeOutputWordCloudCache(cachePath, cloud); err != nil {
		return outputWordCloud{}, err
	}
	return cloud, nil
}

func readOutputWordCloudCache(path string, cloud *outputWordCloud) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, cloud)
}

func writeOutputWordCloudCache(path string, cloud outputWordCloud) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".output-word-cloud-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	data, err := json.MarshalIndent(cloud, "", "  ")
	if err == nil {
		_, err = temporary.Write(data)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func buildOutputWordCloud(runsDir, runID string, jobs []webprojection.Job) (outputWordCloud, error) {
	counts := make(map[string]int)
	jobCounts := make(map[string]int)
	cloud := outputWordCloud{
		Version:     outputWordCloudVersion,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		TotalJobs:   len(jobs),
		Terms:       []outputWordCloudTerm{},
	}
	for _, job := range jobs {
		bytes, jobTerms, err := readOutputWordCloudJob(runsDir, runID, job, counts)
		if err != nil {
			return outputWordCloud{}, err
		}
		cloud.TotalBytes += bytes
		for token := range jobTerms {
			jobCounts[token]++
		}
	}
	return finishOutputWordCloud(cloud, counts, jobCounts), nil
}

func readOutputWordCloudJob(runsDir, runID string, job webprojection.Job, counts map[string]int) (int64, map[string]bool, error) {
	logRun, logJob, attemptID := runID, job.ID, job.AttemptID
	if job.Origin != nil && (attemptID == "" || attemptID == job.Origin.AttemptID) {
		logRun, logJob, attemptID = job.Origin.RunID, job.Origin.JobID, ""
	}
	path, err := webLogPath(runsDir, logRun, logJob, attemptID, stateinternal.StdoutFileName)
	if err != nil {
		return 0, nil, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	if !info.Mode().IsRegular() {
		return 0, nil, fmt.Errorf("output log %q is not a regular file", path)
	}
	jobTerms, err := readOutputWordCloudLog(path, counts)
	return info.Size(), jobTerms, err
}

func readOutputWordCloudLog(path string, counts map[string]int) (map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	jobTerms := make(map[string]bool)
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadString('\n')
		for _, token := range outputWordCloudToken.FindAllString(strings.ToLower(line), -1) {
			if isOutputWordCloudStopWord(token) {
				continue
			}
			counts[token]++
			jobTerms[token] = true
		}
		if errors.Is(readErr, io.EOF) {
			return jobTerms, nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}

func finishOutputWordCloud(cloud outputWordCloud, counts, jobCounts map[string]int) outputWordCloud {
	for word, count := range counts {
		cloud.Terms = append(cloud.Terms, outputWordCloudTerm{Word: word, Count: count, Jobs: jobCounts[word]})
	}
	sort.Slice(cloud.Terms, func(i, j int) bool {
		if cloud.Terms[i].Count != cloud.Terms[j].Count {
			return cloud.Terms[i].Count > cloud.Terms[j].Count
		}
		if cloud.Terms[i].Jobs != cloud.Terms[j].Jobs {
			return cloud.Terms[i].Jobs > cloud.Terms[j].Jobs
		}
		return cloud.Terms[i].Word < cloud.Terms[j].Word
	})
	if len(cloud.Terms) > outputWordCloudTermLimit {
		cloud.Terms = cloud.Terms[:outputWordCloudTermLimit]
	}
	return cloud
}

func isOutputWordCloudStopWord(word string) bool {
	switch word {
	case "and", "are", "but", "for", "from", "has", "have", "not", "the", "this", "that", "then", "there", "with", "you":
		return true
	default:
		return false
	}
}
