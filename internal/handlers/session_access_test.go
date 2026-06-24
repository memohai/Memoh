package handlers

import (
	"testing"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/session"
)

func TestCanAccessSessionScopesChatToCreator(t *testing.T) {
	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	otherUserID := "cccccccc-cccc-cccc-cccc-cccccccccccc"

	if !canAccessSession(session.Session{Type: session.TypeChat, CreatedByUserID: userID}, userID, []string{bots.PermissionChat}) {
		t.Fatal("chat permission should access own chat session")
	}
	if canAccessSession(session.Session{Type: session.TypeChat, CreatedByUserID: otherUserID}, userID, []string{bots.PermissionChat}) {
		t.Fatal("chat permission should not access another user's chat session")
	}
	if canAccessSession(session.Session{Type: session.TypeChat}, userID, []string{bots.PermissionChat}) {
		t.Fatal("chat permission should not access legacy sessions without a creator")
	}
}

func TestCanAccessSessionAllowsChatOwnerToReadOwnSubagent(t *testing.T) {
	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	otherUserID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	sess := session.Session{Type: session.TypeSubagent, CreatedByUserID: userID}

	if !canAccessSession(sess, userID, []string{bots.PermissionChat}) {
		t.Fatal("chat permission should access own subagent session")
	}
	if canAccessSession(sess, otherUserID, []string{bots.PermissionChat}) {
		t.Fatal("chat permission should not access another user's subagent session")
	}
	if !canAccessSession(sess, otherUserID, []string{bots.PermissionManage}) {
		t.Fatal("manage should access all subagent sessions")
	}
}

func TestCanAccessSessionAllowsWorkspaceExecForOwnACP(t *testing.T) {
	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	sess := session.Session{Type: session.TypeACPAgent, CreatedByUserID: userID}

	if canAccessSession(sess, userID, []string{bots.PermissionChat}) {
		t.Fatal("chat permission should not access ACP sessions")
	}
	if !canAccessSession(sess, userID, []string{bots.PermissionWorkspaceExec}) {
		t.Fatal("workspace_exec should access own ACP sessions")
	}
	if !canAccessSession(sess, "other", []string{bots.PermissionManage}) {
		t.Fatal("manage should access all sessions")
	}
}
