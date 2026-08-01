package skills

import (
	"context"
	"errors"
	"testing"
)

func TestShellQuoteEscapesApostrophes(t *testing.T) {
	if got, want := shellQuote("it's 'quoted'"), `'it'"'"'s '"'"'quoted'"'"''`; got != want {
		t.Fatalf("shellQuote() = %q, want %q", got, want)
	}
}

func TestArchivePublicationCommitCanRetryCleanup(t *testing.T) {
	client := &archivePublicationTestClient{deleteErrors: []error{errors.New("temporary failure"), nil}}
	publication := &ArchivePublication{
		client: client, backupDir: "/backup", targetExists: true,
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := publication.Commit(canceled); err == nil {
		t.Fatal("first Commit() error = nil")
	}
	if publication.closed {
		t.Fatal("failed cleanup closed the publication")
	}
	if err := publication.Commit(canceled); err != nil {
		t.Fatalf("second Commit() error = %v", err)
	}
	if !publication.closed || client.calls != 2 || !client.sawDeadline {
		t.Fatalf("publication = %+v, calls = %d, deadline = %v", publication, client.calls, client.sawDeadline)
	}
}

type archivePublicationTestClient struct {
	deleteErrors []error
	calls        int
	sawDeadline  bool
}

func (c *archivePublicationTestClient) DeleteFile(ctx context.Context, _ string, _ bool) error {
	c.calls++
	_, c.sawDeadline = ctx.Deadline()
	if len(c.deleteErrors) == 0 {
		return nil
	}
	err := c.deleteErrors[0]
	c.deleteErrors = c.deleteErrors[1:]
	return err
}

func (*archivePublicationTestClient) Rename(context.Context, string, string) error {
	return nil
}
