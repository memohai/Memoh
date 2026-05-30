package command

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func (h *Handler) buildFSGroup() *CommandGroup {
	g := newCommandGroup("fs", "Browse container filesystem")
	g.DefaultAction = "list" // bare /fs lands on the container root listing
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list [path] - List files in the container",
		Handler: func(cc CommandContext) (string, error) {
			if h.containerFS == nil {
				return "The file browser isn't available right now.", nil
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
			// Wrap in a code fence so the proportional font doesn't collapse the
			// indentation and columns stay aligned.
			var b strings.Builder
			fmt.Fprintf(&b, "```\n%s:\n", dir)
			for _, e := range entries {
				if e.IsDir {
					fmt.Fprintf(&b, "  %s/\n", e.Name)
				} else {
					fmt.Fprintf(&b, "  %s (%s)\n", e.Name, humanizeBytes(e.Size))
				}
			}
			b.WriteString("```")
			return b.String(), nil
		},
	})
	g.Register(SubCommand{
		Name:  "read",
		Usage: "read <path> - Read a file from the container",
		Handler: func(cc CommandContext) (string, error) {
			if h.containerFS == nil {
				return "The file browser isn't available right now.", nil
			}
			if len(cc.Args) < 1 {
				return "Usage: /fs read <path>", nil
			}
			content, err := h.containerFS.ReadFile(cc.Ctx, cc.BotID, cc.Args[0])
			if err != nil {
				return "", err
			}
			const maxRunes = 2000
			truncated := false
			if utf8.RuneCountInString(content) > maxRunes {
				content = string([]rune(content)[:maxRunes])
				truncated = true
			}
			if strings.TrimSpace(content) == "" {
				return "(empty file)", nil
			}
			out := fmt.Sprintf("```\n%s\n```", content)
			if truncated {
				// The marker is a system note, not file content — keep it outside
				// the fence so it doesn't read as part of the file.
				out += "\n_Showing the first 2000 characters. Use the web UI for the full file._"
			}
			return out, nil
		},
	})
	return g
}
