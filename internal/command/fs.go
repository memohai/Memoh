package command

import (
	"fmt"
	"strings"
)

func (h *Handler) buildFSGroup() *CommandGroup {
	g := newCommandGroup("fs", "Browse container filesystem")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list [path] - List files in the container",
		Handler: func(cc CommandContext) (string, error) {
			if h.containerFS == nil {
				return "Container filesystem is not available.", nil
			}
			dir := "/"
			if len(cc.Args) > 0 {
				dir = cc.Args[0]
			}
			entries, err := h.containerFS.ListDir(cc.Ctx, cc.BotID, dir)
			if err != nil {
				return "", err
			}
			if len(entries) == 0 {
				return fmt.Sprintf("Directory %q is empty.", dir), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%s:\n", dir)
			for _, e := range entries {
				if e.IsDir {
					fmt.Fprintf(&b, "  %s/\n", e.Name)
				} else {
					fmt.Fprintf(&b, "  %s (%d bytes)\n", e.Name, e.Size)
				}
			}
			return b.String(), nil
		},
	})
	g.Register(SubCommand{
		Name:  "read",
		Usage: "read <path> - Read a file from the container",
		Handler: func(cc CommandContext) (string, error) {
			if h.containerFS == nil {
				return "Container filesystem is not available.", nil
			}
			if len(cc.Args) < 1 {
				return "Usage: /fs read <path>", nil
			}
			content, err := h.containerFS.ReadFile(cc.Ctx, cc.BotID, cc.Args[0])
			if err != nil {
				return "", err
			}
			const maxLen = 2000
			if len(content) > maxLen {
				content = content[:maxLen] + "\n... (truncated)"
			}
			return fmt.Sprintf("```\n%s\n```", content), nil
		},
	})
	return g
}
