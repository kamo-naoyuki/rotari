package jobcontrol

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const (
	retryAccepting      = state.ManualRetryAcceptingFileName
	retryRequestPrefix  = state.ManualRetryRequestPrefix
	retryResponseSuffix = state.ManualRetryResponseSuffix
)

type retryRequestFile struct {
	StateVersion int      `json:"state_version"`
	ID           string   `json:"id"`
	RunID        string   `json:"run_id"`
	JobIDs       []string `json:"job_ids"`
	PartialArray bool     `json:"partial_array"`
	RequestedAt  string   `json:"requested_at"`
}

type retryResponseFile struct {
	StateVersion int                     `json:"state_version"`
	Response     run.ManualRetryResponse `json:"response"`
	RespondedAt  string                  `json:"responded_at"`
}

// SubmitSelectedRetry resolves against the active run and creates its request
// while holding one state lock. This closes the selection/submit race for CLI,
// Web, and MCP callers.
func (controller Controller) SubmitSelectedRetry(baseDir, projectName, runID string, selection RetrySelection, ifRevision string, timeout time.Duration) (run.ManualRetryResponse, error) {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return run.ManualRetryResponse{}, err
	}
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return run.ManualRetryResponse{}, fmt.Errorf("lock project state: %w", err)
	}
	inspection, err := project.InspectConsistent(paths, false)
	if err != nil {
		release()
		return run.ManualRetryResponse{}, err
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		release()
		return run.ManualRetryResponse{}, err
	}
	if inspection.State != project.Running || inspection.RunID != runID || meta.Phase != "running" {
		release()
		return run.ManualRetryResponse{}, fmt.Errorf("run %q of project %q ended or began cancelling before accepting retry; repeating retry now starts a new retry run", runID, paths.ProjectName)
	}
	if _, err := project.CheckRevision(paths, project.Guard{IfRevision: ifRevision}); err != nil {
		release()
		return run.ManualRetryResponse{}, err
	}
	_, jobIDs, err := controller.selectRetryRun(paths, runID, selection, time.Now())
	if err != nil {
		release()
		return run.ManualRetryResponse{}, err
	}
	requestID, requestDir, err := createRetryRequestLocked(paths, runID, jobIDs, selection.PartialArray)
	if err != nil {
		release()
		return run.ManualRetryResponse{}, err
	}
	release()
	return awaitRetryResponse(paths, runID, requestID, requestDir, timeout)
}

func createRetryRequestLocked(paths state.ProjectPaths, runID string, jobIDs []string, partialArray bool) (string, string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", "", fmt.Errorf("create retry request ID: %w", err)
	}
	requestID := hex.EncodeToString(random[:])
	requestDir := filepath.Join(paths.RunsDir, runID)
	if _, err := os.Stat(filepath.Join(requestDir, retryAccepting)); err != nil {
		return "", "", fmt.Errorf("run %q is not accepting manual retries", runID)
	}
	request := retryRequestFile{StateVersion: model.StateVersion, ID: requestID, RunID: runID, JobIDs: append([]string(nil), jobIDs...), PartialArray: partialArray, RequestedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := state.WriteJSON(filepath.Join(requestDir, retryRequestPrefix+requestID+".json"), request); err != nil {
		return "", "", fmt.Errorf("write retry request: %w", err)
	}
	return requestID, requestDir, nil
}

func awaitRetryResponse(paths state.ProjectPaths, runID, requestID, requestDir string, timeout time.Duration) (run.ManualRetryResponse, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	responsePath := filepath.Join(requestDir, retryRequestPrefix+requestID+retryResponseSuffix)
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	for {
		var saved retryResponseFile
		if err := store.ReadJSON(responsePath, &saved); err == nil {
			if err := validateRetryVersion(responsePath, saved.StateVersion); err != nil {
				return run.ManualRetryResponse{}, err
			}
			return saved.Response, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return run.ManualRetryResponse{}, fmt.Errorf("read retry response: %w", err)
		}
		inspection, inspectErr := project.Inspect(paths, false)
		if inspectErr == nil && (inspection.State != project.Running || inspection.RunID != runID) {
			return run.ManualRetryResponse{}, fmt.Errorf("run %q of project %q ended before accepting retry request %s; repeating retry starts a new retry run", runID, paths.ProjectName, requestID)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return run.ManualRetryResponse{}, fmt.Errorf("retry request %s for run %q has no response; acceptance status is unknown", requestID, runID)
		}
	}
}

// WatchRetryRequests polls one run's request directory and forwards requests
// to the engine. stop closes the acceptance marker and answers every
// unprocessed request with run-ended before returning.
func WatchRetryRequests(paths state.ProjectPaths, runID string, requests chan<- run.ManualRetryRequest) func() {
	requestDir := filepath.Join(paths.RunsDir, runID)
	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		dispatched := map[string]bool{}
		poll := func() {
			entries, err := os.ReadDir(requestDir)
			if err != nil {
				return
			}
			for _, entry := range entries {
				name := entry.Name()
				id, isRequest := retryRequestID(entry)
				if !isRequest || dispatched[name] {
					continue
				}
				path := filepath.Join(requestDir, name)
				var request retryRequestFile
				store := state.NewStore(state.DirectoryMode(), state.FileMode())
				if err := store.ReadJSON(path, &request); err != nil || request.ID != id || request.RunID != runID {
					_ = writeRetryResponse(paths, runID, id, run.ManualRetryResponse{ID: id, Rejected: retryRejections(nil, "invalid retry request")})
					dispatched[name] = true
					continue
				}
				if err := validateRetryVersion(path, request.StateVersion); err != nil {
					_ = writeRetryResponse(paths, runID, request.ID, run.ManualRetryResponse{ID: request.ID, Rejected: retryRejections(request.JobIDs, err.Error())})
					dispatched[name] = true
					continue
				}
				responsePath := filepath.Join(requestDir, retryRequestPrefix+request.ID+retryResponseSuffix)
				if _, err := os.Stat(responsePath); err == nil {
					dispatched[name] = true
					continue
				}
				commit := func(response run.ManualRetryResponse) (run.ManualRetryResponse, error) {
					return commitRetryResponse(paths, runID, response)
				}
				select {
				case requests <- run.ManualRetryRequest{ID: request.ID, JobIDs: request.JobIDs, PartialArray: request.PartialArray, Commit: commit}:
					dispatched[name] = true
				case <-stop:
					return
				}
			}
		}
		for {
			select {
			case <-ticker.C:
				poll()
			case <-stop:
				_ = CloseRetryRequests(paths, runID)
				return
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(stop)
			<-done
		})
	}
}

// CloseRetryRequests fences new submissions and answers requests that the
// engine did not accept before it ended. It is also called by Runner.Finish
// when execution failed before its poller started.
func CloseRetryRequests(paths state.ProjectPaths, runID string) error {
	if err := closeRetryChannel(paths, runID); err != nil {
		return err
	}
	return pollUndispatchedAsEnded(paths, runID)
}

func commitRetryResponse(paths state.ProjectPaths, runID string, response run.ManualRetryResponse) (run.ManualRetryResponse, error) {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return run.ManualRetryResponse{}, err
	}
	defer release()
	if !retryChannelOpen(paths, runID) {
		return persistEndedRetryResponse(paths, runID, response, "run ended")
	}
	inspection, err := project.InspectConsistent(paths, false)
	if err != nil {
		return run.ManualRetryResponse{}, err
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return run.ManualRetryResponse{}, err
	}
	if inspection.State != project.Running || inspection.RunID != runID || meta.Phase != "running" {
		return persistEndedRetryResponse(paths, runID, response, "run ended or is cancelling")
	}
	markers, err := persistPendingRetryMarkers(paths, runID, response)
	if err != nil {
		return run.ManualRetryResponse{}, err
	}
	if err := writeRetryResponseLocked(paths, runID, response); err != nil {
		rollbackRetryMarkers(markers)
		return run.ManualRetryResponse{}, err
	}
	return response, nil
}

func persistEndedRetryResponse(paths state.ProjectPaths, runID string, response run.ManualRetryResponse, reason string) (run.ManualRetryResponse, error) {
	previouslyAccepted := append([]string(nil), response.Accepted...)
	response.Accepted, response.Reopened = nil, nil
	response.Rejected = append(response.Rejected, retryRejections(previouslyAccepted, reason)...)
	response.RunEnded = true
	return response, writeRetryResponseLocked(paths, runID, response)
}

type retryMarkerTransaction struct {
	pending   []string
	cancelled map[string]retryCancelMarker
}

type retryCancelMarker struct {
	contents []byte
	mode     os.FileMode
}

func persistPendingRetryMarkers(paths state.ProjectPaths, runID string, response run.ManualRetryResponse) (retryMarkerTransaction, error) {
	transaction := retryMarkerTransaction{cancelled: make(map[string]retryCancelMarker)}
	jobMarkers := make(map[string]retryJobMarkers)
	jobIDs := append(append([]string(nil), response.Accepted...), response.Reopened...)
	for _, jobID := range jobIDs {
		markers, cancelled, hasCancellation, err := preflightRetryJobMarkers(paths, runID, jobID)
		if err != nil {
			return transaction, err
		}
		jobMarkers[jobID] = markers
		if hasCancellation {
			transaction.cancelled[markers.cancelled] = cancelled
		}
	}
	for _, jobID := range jobIDs {
		if err := state.WriteJSON(jobMarkers[jobID].pending, map[string]any{"state_version": model.StateVersion, "request_id": response.ID, "accepted_at": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
			rollbackRetryMarkers(transaction)
			return retryMarkerTransaction{}, err
		}
		transaction.pending = append(transaction.pending, jobMarkers[jobID].pending)
	}
	for _, jobID := range jobIDs {
		if err := os.Remove(jobMarkers[jobID].cancelled); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackRetryMarkers(transaction)
			return retryMarkerTransaction{}, fmt.Errorf("clear previous cancellation for retried job %s: %w", jobID, err)
		}
	}
	return transaction, nil
}

type retryJobMarkers struct {
	pending   string
	cancelled string
}

func preflightRetryJobMarkers(paths state.ProjectPaths, runID, jobID string) (retryJobMarkers, retryCancelMarker, bool, error) {
	jobDir, err := state.SafeJoin(filepath.Join(paths.RunsDir, runID), jobID)
	if err != nil {
		return retryJobMarkers{}, retryCancelMarker{}, false, err
	}
	if err := os.MkdirAll(jobDir, state.DirectoryMode()); err != nil {
		return retryJobMarkers{}, retryCancelMarker{}, false, err
	}
	markers := retryJobMarkers{
		pending:   filepath.Join(jobDir, state.ManualRetryPendingFileName),
		cancelled: filepath.Join(jobDir, "cancelled"),
	}
	if _, err := os.Stat(markers.pending); err == nil {
		return retryJobMarkers{}, retryCancelMarker{}, false, fmt.Errorf("job %q already has a manual retry pending", jobID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return retryJobMarkers{}, retryCancelMarker{}, false, err
	}
	contents, err := os.ReadFile(markers.cancelled)
	if errors.Is(err, os.ErrNotExist) {
		return markers, retryCancelMarker{}, false, nil
	}
	if err != nil {
		return retryJobMarkers{}, retryCancelMarker{}, false, err
	}
	info, err := os.Stat(markers.cancelled)
	if err != nil {
		return retryJobMarkers{}, retryCancelMarker{}, false, err
	}
	return markers, retryCancelMarker{contents: contents, mode: info.Mode().Perm()}, true, nil
}

func rollbackRetryMarkers(transaction retryMarkerTransaction) {
	for _, path := range transaction.pending {
		_ = os.Remove(path)
	}
	for path, marker := range transaction.cancelled {
		_ = os.WriteFile(path, marker.contents, marker.mode)
	}
}

func closeRetryChannel(paths state.ProjectPaths, runID string) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return err
	}
	defer release()
	if err := os.Remove(filepath.Join(paths.RunsDir, runID, retryAccepting)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("close retry request channel for run %s: %w", runID, err)
	}
	return nil
}

func pollUndispatchedAsEnded(paths state.ProjectPaths, runID string) error {
	requestDir := filepath.Join(paths.RunsDir, runID)
	entries, err := os.ReadDir(requestDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		id, isRequest := retryRequestID(entry)
		if !isRequest {
			continue
		}
		responsePath := filepath.Join(requestDir, retryRequestPrefix+id+retryResponseSuffix)
		if _, err := os.Stat(responsePath); err == nil {
			continue
		}
		if err := writeRetryResponse(paths, runID, id, run.ManualRetryResponse{ID: id, Rejected: []run.ManualRetryRejection{{Reason: "run ended before accepting request"}}, RunEnded: true}); err != nil {
			return fmt.Errorf("reject retry request %s for ended run %s: %w", id, runID, err)
		}
	}
	return nil
}

func writeRetryResponse(paths state.ProjectPaths, runID, id string, response run.ManualRetryResponse) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return err
	}
	defer release()
	return writeRetryResponseLocked(paths, runID, response)
}

func writeRetryResponseLocked(paths state.ProjectPaths, runID string, response run.ManualRetryResponse) error {
	path := filepath.Join(paths.RunsDir, runID, retryRequestPrefix+response.ID+retryResponseSuffix)
	return state.WriteJSON(path, retryResponseFile{StateVersion: model.StateVersion, Response: response, RespondedAt: time.Now().UTC().Format(time.RFC3339Nano)})
}

// retryRequestID returns the request ID of a request file in a run root.
func retryRequestID(entry os.DirEntry) (string, bool) {
	name := entry.Name()
	if entry.IsDir() || !strings.HasPrefix(name, retryRequestPrefix) || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, retryResponseSuffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(name, retryRequestPrefix), ".json")
	return id, id != ""
}

func retryChannelOpen(paths state.ProjectPaths, runID string) bool {
	_, err := os.Stat(filepath.Join(paths.RunsDir, runID, retryAccepting))
	return err == nil
}

func validateRetryVersion(path string, version int) error {
	if version == 0 {
		return fmt.Errorf("%s is missing state_version", path)
	}
	if version > model.StateVersion {
		return fmt.Errorf("%w: %s has state version %d; upgrade rotari", state.ErrNewerStateVersion, path, version)
	}
	return nil
}

func retryRejections(jobIDs []string, reason string) []run.ManualRetryRejection {
	rejected := make([]run.ManualRetryRejection, 0, len(jobIDs))
	for _, id := range jobIDs {
		rejected = append(rejected, run.ManualRetryRejection{JobID: id, Reason: reason})
	}
	return rejected
}
