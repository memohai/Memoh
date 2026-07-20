package main

import (
	"strings"
	"testing"
)

func TestRunAccountCommandValidatesRecoveryInputBeforeOpeningDatabase(t *testing.T) {
	t.Setenv("CONFIG_PATH", "/path/that/does/not/exist")

	for name, tc := range map[string]struct {
		args     []string
		password string
		want     string
	}{
		"command":  {args: []string{"unknown"}, want: "usage:"},
		"identity": {args: []string{"recover-admin", " "}, password: "secret", want: "username or email is required"},
		"password": {args: []string{"recover-admin", "admin"}, password: "\n", want: "new password is required"},
		"limit":    {args: []string{"recover-admin", "admin"}, password: strings.Repeat("x", 4097), want: "password exceeds"},
	} {
		t.Run(name, func(t *testing.T) {
			err := runAccountCommand(tc.args, strings.NewReader(tc.password))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runAccountCommand() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}
