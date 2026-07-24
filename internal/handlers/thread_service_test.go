package handlers

import (
	acpprofileadapter "github.com/memohai/memoh/internal/agent/adapter/acpprofile"
	thread "github.com/memohai/memoh/internal/chat/thread"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func newThreadServiceForTest(queries dbstore.Queries) *thread.Service {
	service := thread.NewService(nil, queries, nil)
	service.SetACPSetupValidator(acpprofileadapter.NewCatalog())
	return service
}
