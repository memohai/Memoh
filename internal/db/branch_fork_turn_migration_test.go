package db

import (
	"context"
	"database/sql"
	"testing"
)

func TestSQLiteBranchForkTurnIDMismatchRepairMigration(t *testing.T) {
	ctx := context.Background()
	dsn := tempSQLiteMigrationDSN(t)
	target := MigrationTarget{Driver: DriverSQLite, DSN: dsn}

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFSUpTo(t, 28), "up", nil); err != nil {
		t.Fatalf("migrate through 0028: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	seedSQLiteForkTurnRepairBase(t, db)
	if _, err := db.ExecContext(ctx, `
UPDATE bot_session_branches
SET fork_from_turn_id = '00000000-0000-0000-0000-000000003099',
    fork_from_turn_seq = 1
WHERE id = '00000000-0000-0000-0000-000000003011'
`); err != nil {
		t.Fatalf("corrupt fork turn id: %v", err)
	}
	closeMigrationSQLite(t, db)

	if err := RunMigrateTarget(nil, target, sqliteMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate through latest: %v", err)
	}

	db = openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	var forkFromTurnID string
	var forkFromTurnSeq int64
	if err := db.QueryRowContext(ctx, `
SELECT fork_from_turn_id, fork_from_turn_seq
FROM bot_session_branches
WHERE id = '00000000-0000-0000-0000-000000003011'
`).Scan(&forkFromTurnID, &forkFromTurnSeq); err != nil {
		t.Fatalf("select repaired fork branch: %v", err)
	}
	if forkFromTurnID != "00000000-0000-0000-0000-000000003020" || forkFromTurnSeq != 1 {
		t.Fatalf("fork boundary = (%s, %d), want correct turn id/seq", forkFromTurnID, forkFromTurnSeq)
	}
}

func seedSQLiteForkTurnRepairBase(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`INSERT OR IGNORE INTO users(id,email,role) VALUES('00000000-0000-0000-0000-000000003001','fork-repair@example.com','member')`,
		`INSERT OR IGNORE INTO bots(id,owner_user_id,type,name,display_name) VALUES('00000000-0000-0000-0000-000000003002','00000000-0000-0000-0000-000000003001','personal','fork-repair','Fork Repair')`,
		`INSERT OR IGNORE INTO bot_sessions(id,bot_id,type,title,metadata,created_by_user_id) VALUES('00000000-0000-0000-0000-000000003003','00000000-0000-0000-0000-000000003002','chat','Fork Repair','{}','00000000-0000-0000-0000-000000003001')`,
		`INSERT OR IGNORE INTO bot_session_branches(id,session_id,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000003010','00000000-0000-0000-0000-000000003003','2026-06-14 12:00:00','2026-06-14 12:00:00')`,
		`INSERT OR IGNORE INTO bot_session_branches(id,session_id,parent_branch_id,fork_from_message_id,fork_from_seq,fork_from_turn_id,fork_from_turn_seq,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000003011','00000000-0000-0000-0000-000000003003','00000000-0000-0000-0000-000000003010','00000000-0000-0000-0000-000000003022',2,'00000000-0000-0000-0000-000000003020',1,'2026-06-14 12:10:00','2026-06-14 12:10:00')`,
		`UPDATE bot_sessions SET active_branch_id='00000000-0000-0000-0000-000000003011' WHERE id='00000000-0000-0000-0000-000000003003'`,
		`INSERT OR IGNORE INTO bot_history_turns(id,session_id,branch_id,turn_seq,request_message_id,final_assistant_message_id,status,created_at,updated_at,completed_at) VALUES('00000000-0000-0000-0000-000000003020','00000000-0000-0000-0000-000000003003','00000000-0000-0000-0000-000000003010',1,'00000000-0000-0000-0000-000000003021','00000000-0000-0000-0000-000000003022','completed','2026-06-14 12:01:00','2026-06-14 12:02:00','2026-06-14 12:02:00')`,
		`INSERT OR IGNORE INTO bot_history_messages(id,bot_id,session_id,branch_id,branch_seq,turn_id,turn_message_seq,role,content,metadata,created_at) VALUES('00000000-0000-0000-0000-000000003021','00000000-0000-0000-0000-000000003002','00000000-0000-0000-0000-000000003003','00000000-0000-0000-0000-000000003010',1,'00000000-0000-0000-0000-000000003020',1,'user','{"content":"request"}','{}','2026-06-14 12:01:00')`,
		`INSERT OR IGNORE INTO bot_history_messages(id,bot_id,session_id,branch_id,branch_seq,turn_id,turn_message_seq,role,content,metadata,created_at) VALUES('00000000-0000-0000-0000-000000003022','00000000-0000-0000-0000-000000003002','00000000-0000-0000-0000-000000003003','00000000-0000-0000-0000-000000003010',2,'00000000-0000-0000-0000-000000003020',2,'assistant','{"content":"reply"}','{}','2026-06-14 12:02:00')`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("exec fork repair seed %q: %v", stmt, err)
		}
	}
}
