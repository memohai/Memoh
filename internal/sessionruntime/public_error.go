package sessionruntime

import "strings"

const (
	runtimeUnavailableMessage    = "The session runtime is temporarily unavailable."
	runtimeRunFailedMessage      = "The agent run failed."
	runtimeInterruptedMessage    = "The agent run was interrupted."
	runtimeTargetInactiveMessage = "The agent run is no longer active."
	runtimeCommandBusyMessage    = "Another runtime command is already being processed."
	runtimeCommandFailedMessage  = "The runtime command could not be completed."
)

func runtimePublicErrorMessage(code string) (string, bool) {
	switch strings.TrimSpace(code) {
	case RuntimeErrorCodeUnavailable:
		return runtimeUnavailableMessage, true
	case RuntimeErrorCodeRunFailed:
		return runtimeRunFailedMessage, true
	case RuntimeErrorCodeInterrupted:
		return runtimeInterruptedMessage, true
	case RuntimeErrorCodeTargetInactive:
		return runtimeTargetInactiveMessage, true
	case RuntimeErrorCodeCommandBusy:
		return runtimeCommandBusyMessage, true
	case RuntimeErrorCodeCommandFailed:
		return runtimeCommandFailedMessage, true
	default:
		return "", false
	}
}

func runtimeRunPublicError(status string) (string, string) {
	if strings.EqualFold(strings.TrimSpace(status), RunStatusLost) {
		return RuntimeErrorCodeInterrupted, runtimeInterruptedMessage
	}
	return RuntimeErrorCodeRunFailed, runtimeRunFailedMessage
}

func sanitizeRunFinalization(outcome runFinalization) runFinalization {
	outcome.Status = strings.ToLower(strings.TrimSpace(outcome.Status))
	if outcome.Error == "" && outcome.ErrorCode == "" && outcome.Status != RunStatusErrored && outcome.Status != RunStatusLost {
		return outcome
	}
	outcome.ErrorCode, outcome.Error = runtimeRunPublicError(outcome.Status)
	return outcome
}

func setRuntimeRunError(run *CurrentRunView, status string) {
	if run == nil {
		return
	}
	run.ErrorCode, run.Error = runtimeRunPublicError(status)
}

func clearRuntimeRunError(run *CurrentRunView) {
	if run == nil {
		return
	}
	run.ErrorCode = ""
	run.Error = ""
}

func setRuntimeSteerError(steer *SteerState, code string) {
	if steer == nil {
		return
	}
	message, ok := runtimePublicErrorMessage(code)
	if !ok {
		code = RuntimeErrorCodeCommandFailed
		message = runtimeCommandFailedMessage
	}
	steer.ErrorCode = code
	steer.Error = message
}

func clearRuntimeSteerError(steer *SteerState) {
	if steer == nil {
		return
	}
	steer.ErrorCode = ""
	steer.Error = ""
}

// sanitizeSnapshotErrors is the final state boundary for persisted and legacy
// snapshots. It intentionally discards arbitrary error text.
func sanitizeSnapshotErrors(snapshot *Snapshot) {
	if snapshot == nil || snapshot.CurrentRunView == nil {
		return
	}
	run := snapshot.CurrentRunView
	if run.ErrorCode != "" || run.Error != "" || strings.EqualFold(run.Status, RunStatusErrored) || strings.EqualFold(run.Status, RunStatusLost) {
		setRuntimeRunError(run, run.Status)
	}
	if run.Steer != nil && (run.Steer.ErrorCode != "" || run.Steer.Error != "") {
		code := strings.TrimSpace(run.Steer.ErrorCode)
		if _, ok := runtimePublicErrorMessage(code); !ok {
			code = RuntimeErrorCodeCommandFailed
		}
		setRuntimeSteerError(run.Steer, code)
	}
}

func sanitizeRuntimeEventErrors(event Event) (Event, error) {
	if !runtimeEventHasErrors(event) {
		return event, nil
	}
	cloned, err := cloneEvent(event)
	if err != nil {
		return Event{}, err
	}
	if cloned.Snapshot != nil {
		sanitizeSnapshotErrors(cloned.Snapshot)
	}
	if cloned.Delta == nil {
		return cloned, nil
	}
	if cloned.Delta.CurrentRunView != nil {
		snapshot := Snapshot{CurrentRunView: cloned.Delta.CurrentRunView}
		sanitizeSnapshotErrors(&snapshot)
	}
	if cloned.Delta.Run != nil {
		patch := cloned.Delta.Run
		if patch.ErrorCode != nil || patch.Error != nil {
			code := ""
			if patch.ErrorCode != nil {
				code = strings.TrimSpace(*patch.ErrorCode)
			}
			message := ""
			if patch.Error != nil {
				message = strings.TrimSpace(*patch.Error)
			}
			if code == "" && message == "" {
				if patch.ErrorCode != nil {
					patch.ErrorCode = &code
				}
				if patch.Error != nil {
					patch.Error = &message
				}
			} else {
				status := RunStatusErrored
				if patch.Status != nil {
					status = *patch.Status
				}
				code, message = runtimeRunPublicError(status)
				patch.ErrorCode = &code
				patch.Error = &message
			}
		}
		if patch.Steer != nil && (patch.Steer.ErrorCode != "" || patch.Steer.Error != "") {
			code := strings.TrimSpace(patch.Steer.ErrorCode)
			if _, ok := runtimePublicErrorMessage(code); !ok {
				code = RuntimeErrorCodeCommandFailed
			}
			setRuntimeSteerError(patch.Steer, code)
		}
	}
	return cloned, nil
}

func runtimeEventHasErrors(event Event) bool {
	if event.Snapshot != nil && event.Snapshot.CurrentRunView != nil {
		run := event.Snapshot.CurrentRunView
		if run.ErrorCode != "" || run.Error != "" || (run.Steer != nil && (run.Steer.ErrorCode != "" || run.Steer.Error != "")) {
			return true
		}
	}
	if event.Delta == nil {
		return false
	}
	if run := event.Delta.CurrentRunView; run != nil {
		if run.ErrorCode != "" || run.Error != "" || (run.Steer != nil && (run.Steer.ErrorCode != "" || run.Steer.Error != "")) {
			return true
		}
	}
	patch := event.Delta.Run
	return patch != nil && (patch.ErrorCode != nil || patch.Error != nil || (patch.Steer != nil && (patch.Steer.ErrorCode != "" || patch.Steer.Error != "")))
}
