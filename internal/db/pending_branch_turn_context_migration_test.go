package db

import (
	"context"
	"database/sql"
	"testing"
)

func TestSQLitePendingBranchTurnContextMigrationPreservesOldRows(t *testing.T) {
	ctx := context.Background()
	dsn := tempSQLiteMigrationDSN(t)
	target := MigrationTarget{Driver: DriverSQLite, DSN: dsn}

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFSUpTo(t, 26), "up", nil); err != nil {
		t.Fatalf("migrate through 0026: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	recreateSQLitePendingTablesBeforePersistContext(t, db)
	seedSQLitePendingContextBase(t, db)
	if _, err := db.ExecContext(ctx, `
INSERT INTO tool_approval_requests (
  id, bot_id, session_id, tool_call_id, tool_name, operation, tool_input, short_id, status,
  decision_reason, prompt_external_message_id, source_platform, reply_target,
  conversation_type, created_at, decided_at
) VALUES (
  '00000000-0000-0000-0000-000000002004',
  '00000000-0000-0000-0000-000000002002',
  '00000000-0000-0000-0000-000000002003',
  'call-approval', 'exec', 'exec', '{"cmd":"date"}', 1, 'approved',
  'ok', 'external-approval', 'local', 'reply-target', 'private',
  '2026-06-14 11:23:00', '2026-06-14 11:24:00'
)`); err != nil {
		t.Fatalf("insert old tool approval row: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO user_input_requests (
  id, bot_id, session_id, tool_call_id, tool_name, short_id, status,
  input_json, ui_payload_json, result_json, provider_metadata,
  prompt_external_message_id, source_platform, reply_target, conversation_type,
  expires_at, created_at, responded_at, canceled_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-000000002005',
  '00000000-0000-0000-0000-000000002002',
  '00000000-0000-0000-0000-000000002003',
  'call-input', 'ask_user', 2, 'submitted',
  '{"question":"pick"}', '{}', '{"answer":"a"}', '{}',
  'external-input', 'web', 'reply-target', 'private',
  '2026-06-15 00:00:00', '2026-06-14 11:25:00',
  '2026-06-14 11:26:00', '2026-06-14 11:27:00', '2026-06-14 11:28:00'
)`); err != nil {
		t.Fatalf("insert old user input row: %v", err)
	}
	closeMigrationSQLite(t, db)

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate through latest: %v", err)
	}

	db = openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	var approval pendingApprovalContextRow
	if err := db.QueryRowContext(ctx, `
SELECT prompt_external_message_id, source_platform, reply_target, conversation_type,
       created_at, decided_at, persist_branch_id, persist_turn_id
FROM tool_approval_requests
WHERE id = '00000000-0000-0000-0000-000000002004'
`).Scan(
		&approval.PromptExternalMessageID,
		&approval.SourcePlatform,
		&approval.ReplyTarget,
		&approval.ConversationType,
		&approval.CreatedAt,
		&approval.DecidedAt,
		&approval.PersistBranchID,
		&approval.PersistTurnID,
	); err != nil {
		t.Fatalf("select migrated tool approval: %v", err)
	}
	assertPendingApprovalContext(t, approval, pendingApprovalContextRow{
		PromptExternalMessageID: "external-approval",
		SourcePlatform:          "local",
		ReplyTarget:             "reply-target",
		ConversationType:        "private",
		CreatedAt:               "2026-06-14 11:23:00",
		DecidedAt:               sql.NullString{String: "2026-06-14 11:24:00", Valid: true},
	})

	var input pendingUserInputContextRow
	if err := db.QueryRowContext(ctx, `
SELECT prompt_external_message_id, source_platform, reply_target, conversation_type,
       expires_at, created_at, responded_at, canceled_at, updated_at,
       persist_branch_id, persist_turn_id
FROM user_input_requests
WHERE id = '00000000-0000-0000-0000-000000002005'
`).Scan(
		&input.PromptExternalMessageID,
		&input.SourcePlatform,
		&input.ReplyTarget,
		&input.ConversationType,
		&input.ExpiresAt,
		&input.CreatedAt,
		&input.RespondedAt,
		&input.CanceledAt,
		&input.UpdatedAt,
		&input.PersistBranchID,
		&input.PersistTurnID,
	); err != nil {
		t.Fatalf("select migrated user input: %v", err)
	}
	assertPendingUserInputContext(t, input, pendingUserInputContextRow{
		PromptExternalMessageID: "external-input",
		SourcePlatform:          "web",
		ReplyTarget:             "reply-target",
		ConversationType:        "private",
		ExpiresAt:               sql.NullString{String: "2026-06-15 00:00:00", Valid: true},
		CreatedAt:               "2026-06-14 11:25:00",
		RespondedAt:             sql.NullString{String: "2026-06-14 11:26:00", Valid: true},
		CanceledAt:              sql.NullString{String: "2026-06-14 11:27:00", Valid: true},
		UpdatedAt:               "2026-06-14 11:28:00",
	})
}

func TestSQLitePendingBranchTurnContextRepairMigrationUnshiftsBadRows(t *testing.T) {
	ctx := context.Background()
	dsn := tempSQLiteMigrationDSN(t)
	target := MigrationTarget{Driver: DriverSQLite, DSN: dsn}

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFSUpTo(t, 27), "up", nil); err != nil {
		t.Fatalf("migrate through 0027: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	seedSQLitePendingContextBase(t, db)
	if _, err := db.ExecContext(ctx, `
INSERT INTO tool_approval_requests (
  id, bot_id, session_id, tool_call_id, tool_name, operation, tool_input, short_id, status,
  decision_reason, prompt_external_message_id, source_platform, reply_target,
  conversation_type, created_at, decided_at
) VALUES (
  '00000000-0000-0000-0000-000000002104',
  '00000000-0000-0000-0000-000000002002',
  '00000000-0000-0000-0000-000000002003',
  'call-approval-corrupt', 'exec', 'exec', '{"cmd":"date"}', 3, 'approved',
  'ok', 'wrong-external', 'wrong-platform', 'wrong-reply', 'wrong-type',
  '2026-06-14 10:00:00', NULL
)`); err != nil {
		t.Fatalf("insert tool approval row for repair: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO user_input_requests (
  id, bot_id, session_id, tool_call_id, tool_name, short_id, status,
  input_json, ui_payload_json, result_json, provider_metadata,
  prompt_external_message_id, source_platform, reply_target, conversation_type,
  expires_at, created_at, responded_at, canceled_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-000000002105',
  '00000000-0000-0000-0000-000000002002',
  '00000000-0000-0000-0000-000000002003',
  'call-input-corrupt', 'ask_user', 4, 'submitted',
  '{"question":"pick"}', '{}', '{"answer":"a"}', '{}',
  'wrong-external', 'wrong-platform', 'wrong-reply', 'wrong-type',
  NULL, '2026-06-14 10:01:00', NULL, NULL, '2026-06-14 10:02:00'
)`); err != nil {
		t.Fatalf("insert user input row for repair: %v", err)
	}
	corruptSQLitePendingContextRows(t, db)
	closeMigrationSQLite(t, db)

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate through 0028 repair: %v", err)
	}

	db = openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	var approval pendingApprovalContextRow
	if err := db.QueryRowContext(ctx, `
SELECT prompt_external_message_id, source_platform, reply_target, conversation_type,
       created_at, decided_at, persist_branch_id, persist_turn_id
FROM tool_approval_requests
WHERE id = '00000000-0000-0000-0000-000000002104'
`).Scan(
		&approval.PromptExternalMessageID,
		&approval.SourcePlatform,
		&approval.ReplyTarget,
		&approval.ConversationType,
		&approval.CreatedAt,
		&approval.DecidedAt,
		&approval.PersistBranchID,
		&approval.PersistTurnID,
	); err != nil {
		t.Fatalf("select repaired tool approval: %v", err)
	}
	assertPendingApprovalContext(t, approval, pendingApprovalContextRow{
		PromptExternalMessageID: "external-approval",
		SourcePlatform:          "local",
		ReplyTarget:             "reply-target",
		ConversationType:        "private",
		CreatedAt:               "2026-06-14 11:23:00",
		DecidedAt:               sql.NullString{String: "2026-06-14 11:24:00", Valid: true},
	})

	var input pendingUserInputContextRow
	if err := db.QueryRowContext(ctx, `
SELECT prompt_external_message_id, source_platform, reply_target, conversation_type,
       expires_at, created_at, responded_at, canceled_at, updated_at,
       persist_branch_id, persist_turn_id
FROM user_input_requests
WHERE id = '00000000-0000-0000-0000-000000002105'
`).Scan(
		&input.PromptExternalMessageID,
		&input.SourcePlatform,
		&input.ReplyTarget,
		&input.ConversationType,
		&input.ExpiresAt,
		&input.CreatedAt,
		&input.RespondedAt,
		&input.CanceledAt,
		&input.UpdatedAt,
		&input.PersistBranchID,
		&input.PersistTurnID,
	); err != nil {
		t.Fatalf("select repaired user input: %v", err)
	}
	assertPendingUserInputContext(t, input, pendingUserInputContextRow{
		PromptExternalMessageID: "external-input",
		SourcePlatform:          "web",
		ReplyTarget:             "reply-target",
		ConversationType:        "private",
		ExpiresAt:               sql.NullString{String: "2026-06-15 00:00:00", Valid: true},
		CreatedAt:               "2026-06-14 11:25:00",
		RespondedAt:             sql.NullString{String: "2026-06-14 11:26:00", Valid: true},
		CanceledAt:              sql.NullString{String: "2026-06-14 11:27:00", Valid: true},
		UpdatedAt:               "2026-06-14 11:28:00",
	})
}

func TestSQLitePendingBranchTurnContextRepairMigrationHandlesExternalIDBranchCollision(t *testing.T) {
	ctx := context.Background()
	dsn := tempSQLiteMigrationDSN(t)
	target := MigrationTarget{Driver: DriverSQLite, DSN: dsn}

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFSUpTo(t, 27), "up", nil); err != nil {
		t.Fatalf("migrate through 0027: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	seedSQLitePendingContextBase(t, db)
	seedSQLitePendingContextBranchIDCollisions(t, db)
	if _, err := db.ExecContext(ctx, `
INSERT INTO tool_approval_requests (
  id, bot_id, session_id, tool_call_id, tool_name, operation, tool_input, short_id, status,
  decision_reason, prompt_external_message_id, source_platform, reply_target,
  conversation_type, created_at, decided_at
) VALUES (
  '00000000-0000-0000-0000-000000002204',
  '00000000-0000-0000-0000-000000002002',
  '00000000-0000-0000-0000-000000002003',
  'call-approval-collision', 'exec', 'exec', '{"cmd":"date"}', 7, 'approved',
  'ok', 'wrong-external', 'wrong-platform', 'wrong-reply', 'wrong-type',
  '2026-06-14 10:00:00', NULL
)`); err != nil {
		t.Fatalf("insert tool approval collision row: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO user_input_requests (
  id, bot_id, session_id, tool_call_id, tool_name, short_id, status,
  input_json, ui_payload_json, result_json, provider_metadata,
  prompt_external_message_id, source_platform, reply_target, conversation_type,
  expires_at, created_at, responded_at, canceled_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-000000002205',
  '00000000-0000-0000-0000-000000002002',
  '00000000-0000-0000-0000-000000002003',
  'call-input-collision', 'ask_user', 8, 'submitted',
  '{"question":"pick"}', '{}', '{"answer":"a"}', '{}',
  'wrong-external', 'wrong-platform', 'wrong-reply', 'wrong-type',
  NULL, '2026-06-14 10:01:00', NULL, NULL, '2026-06-14 10:02:00'
)`); err != nil {
		t.Fatalf("insert user input collision row: %v", err)
	}
	corruptSQLitePendingContextRowsByID(t, db,
		"00000000-0000-0000-0000-000000002204",
		"external-approval",
		"00000000-0000-0000-0000-000000002205",
		"external-input",
	)
	closeMigrationSQLite(t, db)

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate through latest: %v", err)
	}

	db = openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	var approval pendingApprovalContextRow
	if err := db.QueryRowContext(ctx, `
SELECT prompt_external_message_id, source_platform, reply_target, conversation_type,
       created_at, decided_at, persist_branch_id, persist_turn_id
FROM tool_approval_requests
WHERE id = '00000000-0000-0000-0000-000000002204'
`).Scan(
		&approval.PromptExternalMessageID,
		&approval.SourcePlatform,
		&approval.ReplyTarget,
		&approval.ConversationType,
		&approval.CreatedAt,
		&approval.DecidedAt,
		&approval.PersistBranchID,
		&approval.PersistTurnID,
	); err != nil {
		t.Fatalf("select repaired tool approval collision: %v", err)
	}
	assertPendingApprovalContext(t, approval, pendingApprovalContextRow{
		PromptExternalMessageID: "external-approval",
		SourcePlatform:          "local",
		ReplyTarget:             "reply-target",
		ConversationType:        "private",
		CreatedAt:               "2026-06-14 11:23:00",
		DecidedAt:               sql.NullString{String: "2026-06-14 11:24:00", Valid: true},
	})

	var input pendingUserInputContextRow
	if err := db.QueryRowContext(ctx, `
SELECT prompt_external_message_id, source_platform, reply_target, conversation_type,
       expires_at, created_at, responded_at, canceled_at, updated_at,
       persist_branch_id, persist_turn_id
FROM user_input_requests
WHERE id = '00000000-0000-0000-0000-000000002205'
`).Scan(
		&input.PromptExternalMessageID,
		&input.SourcePlatform,
		&input.ReplyTarget,
		&input.ConversationType,
		&input.ExpiresAt,
		&input.CreatedAt,
		&input.RespondedAt,
		&input.CanceledAt,
		&input.UpdatedAt,
		&input.PersistBranchID,
		&input.PersistTurnID,
	); err != nil {
		t.Fatalf("select repaired user input collision: %v", err)
	}
	assertPendingUserInputContext(t, input, pendingUserInputContextRow{
		PromptExternalMessageID: "external-input",
		SourcePlatform:          "web",
		ReplyTarget:             "reply-target",
		ConversationType:        "private",
		ExpiresAt:               sql.NullString{String: "2026-06-15 00:00:00", Valid: true},
		CreatedAt:               "2026-06-14 11:25:00",
		RespondedAt:             sql.NullString{String: "2026-06-14 11:26:00", Valid: true},
		CanceledAt:              sql.NullString{String: "2026-06-14 11:27:00", Valid: true},
		UpdatedAt:               "2026-06-14 11:28:00",
	})
}

func TestSQLitePendingBranchTurnContextMigrationPreservesValidPersistContext(t *testing.T) {
	ctx := context.Background()
	dsn := tempSQLiteMigrationDSN(t)
	target := MigrationTarget{Driver: DriverSQLite, DSN: dsn}

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFSUpTo(t, 27), "up", nil); err != nil {
		t.Fatalf("migrate through 0027: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	seedSQLitePendingContextBase(t, db)
	seedSQLiteBranchTurnContext(t, db)
	if _, err := db.ExecContext(ctx, `
INSERT INTO tool_approval_requests (
  id, bot_id, session_id, tool_call_id, tool_name, operation, tool_input, short_id, status,
  prompt_external_message_id, persist_branch_id, persist_turn_id,
  source_platform, reply_target, conversation_type, created_at
) VALUES (
  '00000000-0000-0000-0000-000000002304',
  '00000000-0000-0000-0000-000000002002',
  '00000000-0000-0000-0000-000000002003',
  'call-valid-approval', 'exec', 'exec', '{"cmd":"date"}', 5, 'pending',
  'external-valid-approval',
  '00000000-0000-0000-0000-000000002010',
  '00000000-0000-0000-0000-000000002020',
  'local', 'reply-target', 'private', '2026-06-14 12:00:00'
)`); err != nil {
		t.Fatalf("insert valid tool approval: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO user_input_requests (
  id, bot_id, session_id, tool_call_id, tool_name, short_id, status,
  input_json, ui_payload_json, result_json, provider_metadata,
  prompt_external_message_id, persist_branch_id, persist_turn_id,
  source_platform, reply_target, conversation_type, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-000000002305',
  '00000000-0000-0000-0000-000000002002',
  '00000000-0000-0000-0000-000000002003',
  'call-valid-input', 'ask_user', 6, 'pending',
  '{"question":"pick"}', '{}', '{}', '{}',
  'external-valid-input',
  '00000000-0000-0000-0000-000000002010',
  '00000000-0000-0000-0000-000000002020',
  'web', 'reply-target', 'private', '2026-06-14 12:01:00', '2026-06-14 12:01:00'
)`); err != nil {
		t.Fatalf("insert valid user input: %v", err)
	}
	closeMigrationSQLite(t, db)

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate through latest: %v", err)
	}

	db = openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	var approval pendingApprovalContextRow
	if err := db.QueryRowContext(ctx, `
SELECT prompt_external_message_id, source_platform, reply_target, conversation_type,
       created_at, decided_at, persist_branch_id, persist_turn_id
FROM tool_approval_requests
WHERE id = '00000000-0000-0000-0000-000000002304'
`).Scan(
		&approval.PromptExternalMessageID,
		&approval.SourcePlatform,
		&approval.ReplyTarget,
		&approval.ConversationType,
		&approval.CreatedAt,
		&approval.DecidedAt,
		&approval.PersistBranchID,
		&approval.PersistTurnID,
	); err != nil {
		t.Fatalf("select valid tool approval: %v", err)
	}
	assertPendingApprovalContext(t, approval, pendingApprovalContextRow{
		PromptExternalMessageID: "external-valid-approval",
		SourcePlatform:          "local",
		ReplyTarget:             "reply-target",
		ConversationType:        "private",
		CreatedAt:               "2026-06-14 12:00:00",
		PersistBranchID:         sql.NullString{String: "00000000-0000-0000-0000-000000002010", Valid: true},
		PersistTurnID:           sql.NullString{String: "00000000-0000-0000-0000-000000002020", Valid: true},
	})

	var input pendingUserInputContextRow
	if err := db.QueryRowContext(ctx, `
SELECT prompt_external_message_id, source_platform, reply_target, conversation_type,
       expires_at, created_at, responded_at, canceled_at, updated_at,
       persist_branch_id, persist_turn_id
FROM user_input_requests
WHERE id = '00000000-0000-0000-0000-000000002305'
`).Scan(
		&input.PromptExternalMessageID,
		&input.SourcePlatform,
		&input.ReplyTarget,
		&input.ConversationType,
		&input.ExpiresAt,
		&input.CreatedAt,
		&input.RespondedAt,
		&input.CanceledAt,
		&input.UpdatedAt,
		&input.PersistBranchID,
		&input.PersistTurnID,
	); err != nil {
		t.Fatalf("select valid user input: %v", err)
	}
	assertPendingUserInputContext(t, input, pendingUserInputContextRow{
		PromptExternalMessageID: "external-valid-input",
		SourcePlatform:          "web",
		ReplyTarget:             "reply-target",
		ConversationType:        "private",
		CreatedAt:               "2026-06-14 12:01:00",
		UpdatedAt:               "2026-06-14 12:01:00",
		PersistBranchID:         sql.NullString{String: "00000000-0000-0000-0000-000000002010", Valid: true},
		PersistTurnID:           sql.NullString{String: "00000000-0000-0000-0000-000000002020", Valid: true},
	})
}

type pendingApprovalContextRow struct {
	PromptExternalMessageID string
	SourcePlatform          string
	ReplyTarget             string
	ConversationType        string
	CreatedAt               string
	DecidedAt               sql.NullString
	PersistBranchID         sql.NullString
	PersistTurnID           sql.NullString
}

type pendingUserInputContextRow struct {
	PromptExternalMessageID string
	SourcePlatform          string
	ReplyTarget             string
	ConversationType        string
	ExpiresAt               sql.NullString
	CreatedAt               string
	RespondedAt             sql.NullString
	CanceledAt              sql.NullString
	UpdatedAt               string
	PersistBranchID         sql.NullString
	PersistTurnID           sql.NullString
}

func recreateSQLitePendingTablesBeforePersistContext(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`DROP TABLE IF EXISTS tool_approval_requests`,
		`CREATE TABLE tool_approval_requests (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  operation TEXT NOT NULL,
  tool_input TEXT NOT NULL,
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  decision_reason TEXT NOT NULL DEFAULT '',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  decided_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  requested_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  decided_at TEXT,
  CONSTRAINT tool_approval_operation_check CHECK (operation IN ('read', 'write', 'exec')),
  CONSTRAINT tool_approval_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'cancelled')),
  CONSTRAINT tool_approval_short_id_unique UNIQUE (session_id, short_id),
  CONSTRAINT tool_approval_tool_call_unique UNIQUE (session_id, tool_call_id)
)`,
		`DROP TABLE IF EXISTS user_input_requests`,
		`CREATE TABLE user_input_requests (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL DEFAULT 'ask_user',
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  input_json TEXT NOT NULL,
  ui_payload_json TEXT NOT NULL DEFAULT '{}',
  result_json TEXT NOT NULL DEFAULT '{}',
  provider_metadata TEXT NOT NULL DEFAULT '{}',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  responded_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  assistant_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  tool_result_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  expires_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  responded_at TEXT,
  canceled_at TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT user_input_tool_name_check CHECK (tool_name = 'ask_user'),
  CONSTRAINT user_input_status_check CHECK (status IN ('pending', 'submitted', 'canceled', 'expired', 'failed')),
  CONSTRAINT user_input_short_id_unique UNIQUE (session_id, short_id)
)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("exec pending table setup %q: %v", stmt, err)
		}
	}
}

func seedSQLitePendingContextBase(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`INSERT OR IGNORE INTO users(id,email,role) VALUES('00000000-0000-0000-0000-000000002001','pending-context@example.com','member')`,
		`INSERT OR IGNORE INTO bots(id,owner_user_id,type,name,display_name) VALUES('00000000-0000-0000-0000-000000002002','00000000-0000-0000-0000-000000002001','personal','pending-context','Pending Context')`,
		`INSERT OR IGNORE INTO bot_sessions(id,bot_id,type,title,metadata,created_by_user_id) VALUES('00000000-0000-0000-0000-000000002003','00000000-0000-0000-0000-000000002002','chat','Pending Context','{}','00000000-0000-0000-0000-000000002001')`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("exec seed %q: %v", stmt, err)
		}
	}
}

func seedSQLiteBranchTurnContext(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`INSERT OR IGNORE INTO bot_session_branches(id,session_id,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000002010','00000000-0000-0000-0000-000000002003','2026-06-14 11:30:00','2026-06-14 11:30:00')`,
		`UPDATE bot_sessions SET active_branch_id='00000000-0000-0000-0000-000000002010' WHERE id='00000000-0000-0000-0000-000000002003'`,
		`INSERT OR IGNORE INTO bot_history_turns(id,session_id,branch_id,turn_seq,status,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000002020','00000000-0000-0000-0000-000000002003','00000000-0000-0000-0000-000000002010',1,'running','2026-06-14 11:31:00','2026-06-14 11:31:00')`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("exec branch turn seed %q: %v", stmt, err)
		}
	}
}

func seedSQLitePendingContextBranchIDCollisions(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`INSERT OR IGNORE INTO bot_session_branches(id,session_id,created_at,updated_at) VALUES('external-approval','00000000-0000-0000-0000-000000002003','2026-06-14 11:30:00','2026-06-14 11:30:00')`,
		`INSERT OR IGNORE INTO bot_session_branches(id,session_id,parent_branch_id,fork_from_message_id,created_at,updated_at) VALUES('external-input','00000000-0000-0000-0000-000000002003','external-approval','external-approval-message','2026-06-14 11:31:00','2026-06-14 11:31:00')`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("exec branch id collision seed %q: %v", stmt, err)
		}
	}
}

func corruptSQLitePendingContextRows(t *testing.T, db *sql.DB) {
	t.Helper()
	corruptSQLitePendingContextRowsByID(t, db,
		"00000000-0000-0000-0000-000000002104",
		"external-approval",
		"00000000-0000-0000-0000-000000002105",
		"external-input",
	)
}

func corruptSQLitePendingContextRowsByID(t *testing.T, db *sql.DB, approvalID, approvalExternalID, inputID, inputExternalID string) {
	t.Helper()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("open sqlite conn: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Fatalf("close sqlite conn: %v", err)
		}
	}()
	stmts := []string{
		`PRAGMA foreign_keys = OFF`,
		`UPDATE tool_approval_requests
SET persist_branch_id = '` + approvalExternalID + `',
    persist_turn_id = 'local',
    prompt_external_message_id = 'reply-target',
    source_platform = 'private',
    reply_target = '2026-06-14 11:23:00',
    conversation_type = '2026-06-14 11:24:00'
WHERE id = '` + approvalID + `'`,
		`UPDATE user_input_requests
SET persist_branch_id = '` + inputExternalID + `',
    persist_turn_id = 'web',
    prompt_external_message_id = 'reply-target',
    source_platform = 'private',
    reply_target = '2026-06-15 00:00:00',
    conversation_type = '2026-06-14 11:25:00',
    expires_at = '2026-06-14 11:26:00',
    created_at = '2026-06-14 11:27:00',
    responded_at = '2026-06-14 11:28:00'
WHERE id = '` + inputID + `'`,
		`PRAGMA foreign_keys = ON`,
	}
	for _, stmt := range stmts {
		if _, err := conn.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("exec corrupt pending row %q: %v", stmt, err)
		}
	}
}

func assertPendingApprovalContext(t *testing.T, got, want pendingApprovalContextRow) {
	t.Helper()
	if got.PromptExternalMessageID != want.PromptExternalMessageID ||
		got.SourcePlatform != want.SourcePlatform ||
		got.ReplyTarget != want.ReplyTarget ||
		got.ConversationType != want.ConversationType ||
		got.CreatedAt != want.CreatedAt ||
		got.DecidedAt != want.DecidedAt {
		t.Fatalf("tool approval context = %#v, want %#v", got, want)
	}
	if got.PersistBranchID != want.PersistBranchID || got.PersistTurnID != want.PersistTurnID {
		t.Fatalf("tool approval persist context = (%#v, %#v), want (%#v, %#v)",
			got.PersistBranchID, got.PersistTurnID, want.PersistBranchID, want.PersistTurnID)
	}
}

func assertPendingUserInputContext(t *testing.T, got, want pendingUserInputContextRow) {
	t.Helper()
	if got.PromptExternalMessageID != want.PromptExternalMessageID ||
		got.SourcePlatform != want.SourcePlatform ||
		got.ReplyTarget != want.ReplyTarget ||
		got.ConversationType != want.ConversationType ||
		got.ExpiresAt != want.ExpiresAt ||
		got.CreatedAt != want.CreatedAt ||
		got.RespondedAt != want.RespondedAt ||
		got.CanceledAt != want.CanceledAt ||
		got.UpdatedAt != want.UpdatedAt {
		t.Fatalf("user input context = %#v, want %#v", got, want)
	}
	if got.PersistBranchID != want.PersistBranchID || got.PersistTurnID != want.PersistTurnID {
		t.Fatalf("user input persist context = (%#v, %#v), want (%#v, %#v)",
			got.PersistBranchID, got.PersistTurnID, want.PersistBranchID, want.PersistTurnID)
	}
}
