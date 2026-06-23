package conversation

import "strings"

// ApplyBackgroundTaskSnapshots overlays live background-task state onto
// converted UI turns. This keeps persisted background_started tool results
// accurate after a page reload.
func ApplyBackgroundTaskSnapshots(turns []UITurn, tasks []UIBackgroundTask) {
	if len(turns) == 0 || len(tasks) == 0 {
		return
	}

	byID := make(map[string]UIBackgroundTask, len(tasks))
	for _, task := range tasks {
		if taskID := strings.TrimSpace(task.TaskID); taskID != "" {
			byID[taskID] = task
		}
	}
	if len(byID) == 0 {
		return
	}

	for turnIdx := range turns {
		if turns[turnIdx].Role != "assistant" {
			continue
		}
		for messageIdx := range turns[turnIdx].Messages {
			message := &turns[turnIdx].Messages[messageIdx]
			if message.Type != UIMessageTool || message.Background == nil {
				continue
			}
			task, ok := byID[strings.TrimSpace(message.Background.TaskID)]
			if !ok {
				continue
			}
			mergeBackgroundTaskIntoTool(message, task)
		}
	}
}
