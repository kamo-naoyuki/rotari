package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/notification"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const webhookBatchWindow = 10 * time.Second

type pendingWebhookBatch struct {
	settings notification.WebhookSettings
	events   []notification.Event
	timer    *time.Timer
}

type webhookNotificationManager struct {
	mu      sync.Mutex
	pending map[string]*pendingWebhookBatch
	after   func(time.Duration, func()) *time.Timer
	now     func() time.Time
	send    func(notification.WebhookSettings, notification.Batch)
}

func newWebhookNotificationManager() *webhookNotificationManager {
	manager := &webhookNotificationManager{
		pending: make(map[string]*pendingWebhookBatch),
		after:   time.AfterFunc,
		now:     time.Now,
	}
	manager.send = sendWebhookBatch
	return manager
}

var webhookNotifications = newWebhookNotificationManager()

func (manager *webhookNotificationManager) JobFinished(paths state.ProjectPaths, runID, runName string, job model.JobSpec, result model.JobResult) {
	event := notification.NewJobEvent(paths.ProjectName, runID, runName, job, result, manager.now())
	event.SetDiagnosisOutdated(diagnose.Outdated(result))
	if runDir, err := state.SafeJoin(paths.RunsDir, runID); err == nil {
		event.SetTimestamps(state.ReadJobTimestamp(runDir, job.ID, "submitted_at"), state.ReadJobTimestamp(runDir, job.ID, "finished_at"))
	}
	manager.add(paths, runID, event, false)
}

func (manager *webhookNotificationManager) RunFinished(paths state.ProjectPaths, runID string, _ int) {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		printErrorf("WARNING: cannot prepare webhook notification: %v", err)
		return
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		printErrorf("WARNING: cannot load run summary for webhook: %v", err)
		return
	}
	manager.add(paths, runID, notification.NewRunEvent(paths.ProjectName, summary, manager.now()), true)
}

func (manager *webhookNotificationManager) add(paths state.ProjectPaths, runID string, event notification.Event, flush bool) {
	key := paths.BaseDir + "\x00" + paths.ProjectName + "\x00" + runID
	manager.mu.Lock()
	pending := manager.pending[key]
	if pending == nil {
		settings, err := loadRunWebhookSettings(paths, runID)
		if err != nil {
			manager.mu.Unlock()
			printErrorf("WARNING: cannot load webhook settings: %v", err)
			return
		}
		pending = &pendingWebhookBatch{settings: settings}
		manager.pending[key] = pending
		pending.timer = manager.after(webhookBatchWindow, func() { manager.flush(key) })
	}
	pending.events = append(pending.events, event)
	if !flush {
		manager.mu.Unlock()
		return
	}
	if pending.timer != nil {
		pending.timer.Stop()
	}
	delete(manager.pending, key)
	manager.mu.Unlock()
	manager.deliver(pending)
}

func (manager *webhookNotificationManager) flush(key string) {
	manager.mu.Lock()
	pending := manager.pending[key]
	delete(manager.pending, key)
	manager.mu.Unlock()
	if pending != nil {
		manager.deliver(pending)
	}
}

func (manager *webhookNotificationManager) deliver(pending *pendingWebhookBatch) {
	batch := notification.NewBatch(pending.events, pending.settings.ChannelSettings, manager.now())
	if pending.settings.URL != "" && !batch.Empty() {
		manager.send(pending.settings, batch)
	}
}

func loadRunWebhookSettings(paths state.ProjectPaths, runID string) (notification.WebhookSettings, error) {
	loaded, err := loadRunNotificationSettings(paths, runID)
	if err != nil {
		return notification.WebhookSettings{}, err
	}
	settings := loaded.Webhook
	if value := strings.TrimSpace(os.Getenv(envWebhookURL)); value != "" {
		settings.URL = value
	}
	settings.URL = strings.TrimSpace(settings.URL)
	return settings, nil
}

func loadRunNotificationSettings(paths state.ProjectPaths, runID string) (notification.Settings, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return notification.Settings{}, err
	}
	context, err := state.LoadContext(jsonStore(), runDir)
	if err == nil {
		for _, fileName := range context.ConfigSnapshotFiles {
			if fileName != notification.FileName {
				continue
			}
			data, err := os.ReadFile(filepath.Join(runDir, "configs", fileName))
			if err != nil {
				return notification.Settings{}, err
			}
			return notification.Parse(data)
		}
	}
	loaded, err := notification.Load(paths.BaseDir, paths.ProjectName)
	return loaded.Settings, err
}

func sendWebhookBatch(settings notification.WebhookSettings, batch notification.Batch) {
	parsed, err := url.Parse(settings.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		printErrorf("WARNING: invalid %s URL", envWebhookURL)
		return
	}
	payload := notification.NewPayload(batch, settings.Fields)
	encoder, ok := webhookEncoders[normalizeWebhookFormat(settings.Format)]
	if !ok {
		printErrorf("WARNING: unsupported webhook format %q", settings.Format)
		return
	}
	encodedPayload, err := encoder.Encode(payload)
	if err != nil {
		printErrorf("WARNING: cannot prepare webhook notification: %v", err)
		return
	}
	data, err := json.Marshal(encodedPayload)
	if err != nil {
		printErrorf("WARNING: cannot encode webhook notification: %v", err)
		return
	}
	request, err := http.NewRequest(http.MethodPost, settings.URL, bytes.NewReader(data))
	if err != nil {
		printErrorf("WARNING: cannot create webhook notification: %v", err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		printErrorf("WARNING: webhook notification failed: %v", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		printErrorf("WARNING: webhook notification returned HTTP %d", response.StatusCode)
	}
}
