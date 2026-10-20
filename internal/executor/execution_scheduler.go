package executor

import (
	"encoding/xml"
	"strings"
)

func executionQueueState(backend, id string, output []byte) (active, known bool) {
	text := strings.TrimSpace(string(output))
	switch backend {
	case "slurm":
		if text == "" {
			return false, true
		}
		rows := strings.Split(text, "\n")
		if len(rows) != 1 {
			return false, false
		}
		fields := strings.Split(rows[0], "|")
		if len(fields) != 2 || strings.TrimSpace(fields[0]) != id {
			return false, false
		}
		value := strings.ToLower(strings.TrimSpace(fields[1]))
		if schedulerActiveState(value) {
			return true, true
		}
		_, _, terminal, err := parseSlurmAccounting([]byte(strings.ToUpper(value) + "|0:0"))
		return false, err == nil && terminal
	case "pbs":
		if text == "" {
			return false, true
		}
		if !pbsProbeIdentity(text, id) {
			return false, false
		}
		value := parsePBSJobState(output)
		if value == "f" || value == "c" {
			return false, true
		}
		return schedulerActiveState(value), schedulerActiveState(value)
	case "sge":
		var listing struct {
			XMLName xml.Name `xml:"job_info"`
		}
		if err := xml.Unmarshal(output, &listing); err != nil || listing.XMLName.Local != "job_info" {
			return false, false
		}
		value, err := parseSGEJobState(output, id)
		if err != nil {
			return false, false
		}
		if value == "" {
			return false, true
		}
		// Eqw and suspended/held entries still belong to the scheduler.
		known = value == "pending" || value == "running" || value == "suspended" || value == "error"
		return known, known
	case "lsf":
		if text == "" {
			return false, true
		}
		fields := strings.Fields(text)
		if len(fields) != 2 || fields[0] != id {
			return false, false
		}
		if fields[1] == "DONE" || fields[1] == "EXIT" {
			return false, true
		}
		value := lsfStateName(strings.ToUpper(fields[1]))
		return schedulerActiveState(value), schedulerActiveState(value)
	}
	return false, false
}

func executionAccountingState(backend, id string, output []byte) (terminal, active bool) {
	text := strings.TrimSpace(string(output))
	switch backend {
	case "slurm":
		// An allocation query must produce exactly one allocation/task row.
		if len(strings.Split(text, "\n")) != 1 {
			return false, false
		}
		fields := strings.Split(text, "|")
		if len(fields) != 3 || strings.TrimSpace(fields[0]) != id {
			return false, false
		}
		_, value, ended, err := parseSlurmAccounting([]byte(strings.Join(fields[1:], "|")))
		return err == nil && ended, err == nil && schedulerActiveState(value)
	case "pbs":
		if !pbsProbeIdentity(text, id) {
			return false, false
		}
		value := parsePBSJobState(output)
		if schedulerActiveState(value) {
			return false, true
		}
		_, _, ended, err := parsePBSAccounting(output)
		return (value == "f" || value == "c") && err == nil && ended, false
	case "sge":
		// qacct can return several records. Without a unique identity do not
		// let an older completed task stand in for the selected execution.
		found := 0
		exitFields, failedFields := 0, 0
		for _, line := range strings.Split(text, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 {
				switch fields[0] {
				case "taskid":
					if fields[1] != "undefined" && fields[1] != "0" {
						return false, false
					}
				case "exit_status":
					exitFields++
				case "failed":
					failedFields++
				}
			}
			if len(fields) == 2 && fields[0] == "jobnumber" {
				if fields[1] != id {
					return false, false
				}
				found++
			}
		}
		if found != 1 || exitFields != 1 || failedFields != 1 {
			return false, false
		}
		_, ended, err := parseSGEAccounting(output)
		return err == nil && ended, false
	case "lsf":
		if len(strings.Split(text, "\n")) != 1 {
			return false, false
		}
		fields := strings.Fields(text)
		if len(fields) < 2 || fields[0] != id {
			return false, false
		}
		_, value, ended := parseLSFAccounting([]byte(strings.Join(fields[1:], " ")))
		return ended, schedulerActiveState(value)
	}
	return false, false
}

func schedulerActiveState(value string) bool {
	switch value {
	case "running", "pending", "queued", "held", "suspended", "waiting", "configuring", "completing", "resizing", "requeued", "requeue_fed", "requeue_hold", "signaling", "stage_out", "stopped", "provisioning", "prov":
		return true
	default:
		return false
	}
}

func pbsProbeIdentity(output, id string) bool {
	count := 0
	states := 0
	for _, line := range strings.Split(output, "\n") {
		if key, _, found := strings.Cut(line, "="); found && strings.EqualFold(strings.TrimSpace(key), "job_state") {
			states++
		}
		if key, value, found := strings.Cut(strings.TrimSpace(line), ":"); found && strings.EqualFold(strings.TrimSpace(key), "Job Id") {
			if strings.TrimSpace(value) != id {
				return false
			}
			count++
		}
	}
	return count == 1 && states == 1
}
