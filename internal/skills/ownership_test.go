package skills

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDirectOwnerLifecycle(t *testing.T) {
	client := newFakeClient()
	ctx := context.Background()
	digest := strings.Repeat("a", 64)
	if err := MarkDirectOwner(ctx, client, " openai ", " documents ", " pdf ", digest); err != nil {
		t.Fatalf("MarkDirectOwner() error = %v", err)
	}
	if !HasDirectOwner(ctx, client, "openai", "documents", "pdf") {
		t.Fatal("valid direct owner marker was not found")
	}
	if err := RemoveDirectOwner(ctx, client, "openai", "documents", "pdf"); err != nil {
		t.Fatalf("RemoveDirectOwner() error = %v", err)
	}
	if HasDirectOwner(ctx, client, "openai", "documents", "pdf") {
		t.Fatal("direct owner marker remained after removal")
	}
}

func TestReadDirectOwnerFailsClosedOnReadAndValidationErrors(t *testing.T) {
	client := newFakeClient()
	ctx := context.Background()
	markerPath, err := DirectOwnerPathForIDs("openai", "documents", "pdf")
	if err != nil {
		t.Fatalf("DirectOwnerPathForIDs() error = %v", err)
	}

	client.readErrors[markerPath] = errors.New("injected read failure")
	if _, ok, err := ReadDirectOwner(ctx, client, "openai", "documents", "pdf"); err == nil || ok {
		t.Fatalf("ReadDirectOwner(read failure) = ok:%v err:%v", ok, err)
	}
	delete(client.readErrors, markerPath)
	client.files[markerPath] = "not json"
	if _, ok, err := ReadDirectOwner(ctx, client, "openai", "documents", "pdf"); err == nil || ok {
		t.Fatalf("ReadDirectOwner(corrupt marker) = ok:%v err:%v", ok, err)
	}
	delete(client.files, markerPath)
	if _, ok, err := ReadDirectOwner(ctx, client, "openai", "documents", "pdf"); err != nil || ok {
		t.Fatalf("ReadDirectOwner(missing marker) = ok:%v err:%v", ok, err)
	}
}

func TestDirectOwnerRejectsInvalidIdentityAndDigest(t *testing.T) {
	client := newFakeClient()
	ctx := context.Background()
	validDigest := strings.Repeat("a", 64)
	for name, args := range map[string][]string{
		"user namespace": {UserSkillNamespace, "personal", "pdf", validDigest},
		"invalid ID":     {"openai", "../documents", "pdf", validDigest},
		"invalid digest": {"openai", "documents", "pdf", "not-a-digest"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := MarkDirectOwner(ctx, client, args[0], args[1], args[2], args[3]); err == nil {
				t.Fatal("MarkDirectOwner() accepted invalid input")
			}
		})
	}
}
