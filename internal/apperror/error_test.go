package apperror

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestErrorKeepsStableCodeAndPrivateCause(t *testing.T) {
	cause := errors.New("dial unix /run/memoh/bridge.sock: connection refused")
	err := fmt.Errorf("start workspace: %w", Wrap(CodeWorkspaceUnreachable, cause, nil))

	if got := CodeOf(err); got != CodeWorkspaceUnreachable {
		t.Fatalf("CodeOf() = %q, want %q", got, CodeWorkspaceUnreachable)
	}
	if got := CauseOf(err); !errors.Is(got, cause) {
		t.Fatalf("CauseOf() = %v, want original cause", got)
	}
	if errors.Is(err, cause) {
		t.Fatal("infrastructure cause leaked through errors.Is")
	}
	if strings.Contains(err.Error(), cause.Error()) {
		t.Fatal("infrastructure cause leaked through Error()")
	}
}

func TestProblemFromUsesCatalogAndDoesNotExposeCause(t *testing.T) {
	err := Wrap(CodeWorkspaceUnreachable, errors.New("secret runtime detail"), nil)
	problem, ok := ProblemFrom(err, "req-1")
	if !ok {
		t.Fatal("ProblemFrom() did not recognize AppError")
	}
	if problem.Status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", problem.Status, http.StatusServiceUnavailable)
	}
	if problem.Type != "urn:memoh:error:workspace.unreachable" {
		t.Fatalf("type = %q", problem.Type)
	}
	if problem.Detail != "The workspace could not be reached." {
		t.Fatalf("detail = %q", problem.Detail)
	}
	if problem.RequestID != "req-1" {
		t.Fatalf("request_id = %q", problem.RequestID)
	}
}

func TestPublicFromIsSharedByTransportAdapters(t *testing.T) {
	err := New(CodeBotNameTaken, map[string]string{"field": "name"})
	public, ok := PublicFrom(err, "req-public")
	if !ok {
		t.Fatal("PublicFrom() did not recognize AppError")
	}
	if public.Code != CodeBotNameTaken || public.Detail != "This name is already taken." {
		t.Fatalf("public error = %#v", public)
	}
	if public.Args["field"] != "name" || public.RequestID != "req-public" {
		t.Fatalf("public error metadata = %#v", public)
	}
}

func TestArgsAreCopiedAtInputAndOutput(t *testing.T) {
	args := map[string]string{
		"field":             "name",
		"provider_response": "secret provider payload",
	}
	err := New(CodeBotNameTaken, args)
	args["field"] = "changed"

	got := ArgsOf(err)
	if got["field"] != "name" {
		t.Fatalf("stored field = %q", got["field"])
	}
	got["field"] = "changed again"
	if ArgsOf(err)["field"] != "name" {
		t.Fatal("ArgsOf returned mutable internal state")
	}
	if _, ok := got["provider_response"]; ok {
		t.Fatal("undeclared arg crossed the public error boundary")
	}

	workspaceErr := Wrap(CodeWorkspaceUnreachable, errors.New("private"), map[string]string{"path": "/secret"})
	problem, ok := ProblemFrom(workspaceErr, "req-2")
	if !ok {
		t.Fatal("ProblemFrom() did not recognize workspace error")
	}
	if len(problem.Args) != 0 {
		t.Fatalf("workspace args = %#v, want empty allowlisted metadata", problem.Args)
	}
}

func TestLookupDoesNotExposeMutableCatalogState(t *testing.T) {
	definition, ok := Lookup(CodeBotNameTaken)
	if !ok {
		t.Fatal("bot.name_taken missing from catalog")
	}
	definition.AllowedArgs[0] = "changed"

	fresh, _ := Lookup(CodeBotNameTaken)
	if fresh.AllowedArgs[0] != "field" {
		t.Fatalf("catalog allowed args were mutated: %#v", fresh.AllowedArgs)
	}
}

func TestSkillLayoutErrorsUseConflictContract(t *testing.T) {
	for _, code := range []Code{
		CodeSkillBuiltinReadOnly,
		CodeSkillRegistryLayoutConflict,
		CodeRegistrySkillLayoutConflict,
	} {
		definition, ok := Lookup(code)
		if !ok {
			t.Fatalf("%s missing from catalog", code)
		}
		if definition.HTTPStatus != http.StatusConflict {
			t.Fatalf("%s status = %d, want 409", code, definition.HTTPStatus)
		}
		if strings.TrimSpace(definition.Detail) == "" {
			t.Fatalf("%s has empty fallback detail", code)
		}
	}
}

func TestSkillSaveErrorsUseStableContracts(t *testing.T) {
	tests := []struct {
		code   Code
		status int
	}{
		{code: CodeSkillNameTaken, status: http.StatusConflict},
		{code: CodeSkillSaveFailed, status: http.StatusInternalServerError},
	}
	for _, test := range tests {
		definition, ok := Lookup(test.code)
		if !ok {
			t.Fatalf("%s missing from catalog", test.code)
		}
		if definition.HTTPStatus != test.status {
			t.Fatalf("%s status = %d, want %d", test.code, definition.HTTPStatus, test.status)
		}
		if strings.TrimSpace(definition.Detail) == "" {
			t.Fatalf("%s has empty fallback detail", test.code)
		}
	}
}

func TestRegistryUpstreamErrorsUseStableContracts(t *testing.T) {
	tests := []struct {
		code   Code
		status int
	}{
		{code: CodeRegistryUnavailable, status: http.StatusBadGateway},
		{code: CodeRegistrySkillNotFound, status: http.StatusNotFound},
		{code: CodeRegistrySkillInvalid, status: http.StatusBadGateway},
	}
	for _, test := range tests {
		definition, ok := Lookup(test.code)
		if !ok {
			t.Fatalf("%s missing from catalog", test.code)
		}
		if definition.HTTPStatus != test.status {
			t.Fatalf("%s status = %d, want %d", test.code, definition.HTTPStatus, test.status)
		}
		if strings.TrimSpace(definition.Detail) == "" {
			t.Fatalf("%s has empty fallback detail", test.code)
		}
	}
}

func TestRegistrySkillInstallFailedUsesPrivateServerErrorContract(t *testing.T) {
	definition, ok := Lookup(CodeRegistrySkillInstallFailed)
	if !ok {
		t.Fatal("registry.skill_install_failed missing from catalog")
	}
	if definition.HTTPStatus != http.StatusInternalServerError {
		t.Fatalf("registry.skill_install_failed status = %d, want 500", definition.HTTPStatus)
	}
	if strings.TrimSpace(definition.Detail) == "" {
		t.Fatal("registry.skill_install_failed has empty fallback detail")
	}
}
