package tools

import (
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/userinput"
)

// ToolName identifies a built-in Memoh agent tool.
type ToolName string

const (
	ToolRead                ToolName = "read"
	ToolWrite               ToolName = "write"
	ToolList                ToolName = "list"
	ToolEdit                ToolName = "edit"
	ToolExec                ToolName = "exec"
	ToolApplyPatch          ToolName = "apply_patch"
	ToolListBackground      ToolName = "list_background"
	ToolGetBackgroundStatus ToolName = "get_background_status"
	ToolKillBackground      ToolName = "kill_background"

	ToolSend  ToolName = "send"
	ToolReact ToolName = "react"
	ToolSpeak ToolName = "speak"

	ToolGetContacts    ToolName = "get_contacts"
	ToolListSessions   ToolName = "list_sessions"
	ToolGetMessages    ToolName = "get_messages"
	ToolSearchMessages ToolName = "search_messages"
	ToolSearchMemory   ToolName = memprovider.ToolSearchMemory
	ToolListSkills     ToolName = "list_skills"
	ToolUseSkill       ToolName = "use_skill"
	ToolSpawnAgent     ToolName = "spawn_agent"
	ToolSendMessage    ToolName = "send_message"
	ToolWaitAgent      ToolName = "wait_agent"
	ToolListAgents     ToolName = "list_agents"

	ToolListSchedule   ToolName = "list_schedule"
	ToolGetSchedule    ToolName = "get_schedule"
	ToolCreateSchedule ToolName = "create_schedule"
	ToolUpdateSchedule ToolName = "update_schedule"
	ToolDeleteSchedule ToolName = "delete_schedule"

	ToolBrowserAction        ToolName = "browser_action"
	ToolBrowserObserve       ToolName = "browser_observe"
	ToolComputerObserve      ToolName = "computer_observe"
	ToolComputerAction       ToolName = "computer_action"
	ToolBrowserRemoteSession ToolName = "browser_remote_session"

	ToolWebSearch       ToolName = "web_search"
	ToolWebFetch        ToolName = "web_fetch"
	ToolGenerateImage   ToolName = "generate_image"
	ToolTranscribeAudio ToolName = "transcribe_audio"
	ToolAskUser         ToolName = userinput.ToolNameAskUser

	ToolListEmailAccounts ToolName = "list_email_accounts"
	ToolSendEmail         ToolName = "send_email"
	ToolListEmail         ToolName = "list_email"
	ToolReadEmail         ToolName = "read_email"
)

var builtInToolNames = map[ToolName]struct{}{
	ToolRead: {}, ToolWrite: {}, ToolList: {}, ToolEdit: {}, ToolExec: {}, ToolApplyPatch: {}, ToolListBackground: {}, ToolGetBackgroundStatus: {}, ToolKillBackground: {},
	ToolSend: {}, ToolReact: {}, ToolSpeak: {},
	ToolGetContacts: {}, ToolListSessions: {}, ToolGetMessages: {}, ToolSearchMessages: {}, ToolSearchMemory: {}, ToolListSkills: {}, ToolUseSkill: {}, ToolSpawnAgent: {}, ToolSendMessage: {}, ToolWaitAgent: {}, ToolListAgents: {},
	ToolListSchedule: {}, ToolGetSchedule: {}, ToolCreateSchedule: {}, ToolUpdateSchedule: {}, ToolDeleteSchedule: {},
	ToolBrowserAction: {}, ToolBrowserObserve: {}, ToolComputerObserve: {}, ToolComputerAction: {}, ToolBrowserRemoteSession: {},
	ToolWebSearch: {}, ToolWebFetch: {}, ToolGenerateImage: {}, ToolTranscribeAudio: {}, ToolAskUser: {},
	ToolListEmailAccounts: {}, ToolSendEmail: {}, ToolListEmail: {}, ToolReadEmail: {},
}

func (n ToolName) String() string {
	return string(n)
}

func toolRef(name ToolName) string {
	return "`" + name.String() + "`"
}

func IsBuiltInToolName(name string) bool {
	_, ok := builtInToolNames[ToolName(name)]
	return ok
}

func BuiltInToolNames() []ToolName {
	names := make([]ToolName, 0, len(builtInToolNames))
	for name := range builtInToolNames {
		names = append(names, name)
	}
	return names
}
