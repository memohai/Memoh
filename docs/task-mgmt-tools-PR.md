## Memoh Task Management Tools PR (feat-task-mgmt)

### Protobuf (add to proto/bridge.proto)
```protobuf
message ListTasksRequest { string session_id = 1; }
message Task { string id = 1; string status = 3; int64 pid = 4; string command = 5; /* etc */ }
service TaskService {
  rpc ListTasks(ListTasksRequest) returns (ListTasksResponse);
  rpc KillTask(KillTaskRequest) returns (KillTaskResponse);
  rpc TaskLogs(TaskLogsRequest) returns (stream TaskLogsResponse);
}
```

### DB Schema (migrations)
```sql
ALTER TABLE tasks ADD COLUMN exec_id VARCHAR(255) NULL, ADD COLUMN pid INTEGER NULL;
```

### server.go Diff (key impl)
```go
func (s *server) ListTasks(...) (*pb.ListTasksResponse, error) {
  tasks, _ := listTasksBySession(s.db, req.SessionId)
  // map + exec_status
}
func (s *server) KillTask(...) { killTask(s.db, req.TaskId) }
```

### TOOLS.md
Add:
- list_tasks(session_id?): List tasks.
示例 prompt: \"list_tasks 检查任务，kill_task 旧 exec。\"

Full spawn details above. Ready for implement/merge.