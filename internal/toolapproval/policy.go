package toolapproval

import (
	"encoding/json"
	"path"
	"strconv"
	"strings"

	"github.com/memohai/memoh/internal/settings"
)

func needsApproval(cfg settings.ToolApprovalConfig, toolName string, input any) bool {
	cfg = settings.NormalizeToolApprovalConfig(cfg)
	if !cfg.Enabled {
		return false
	}

	args := inputMap(input)
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "write":
		target := normalizeContainerPath(readString(args, "path"))
		if matchesAnyGlob(target, cfg.Write.ForceReviewGlobs) {
			return true
		}
		if matchesAnyGlob(target, cfg.Write.BypassGlobs) {
			return false
		}
		if !cfg.Write.RequireApproval {
			return false
		}
		return true
	case "edit":
		target := normalizeContainerPath(readString(args, "path"))
		if matchesAnyGlob(target, cfg.Edit.ForceReviewGlobs) {
			return true
		}
		if matchesAnyGlob(target, cfg.Edit.BypassGlobs) {
			return false
		}
		if !cfg.Edit.RequireApproval {
			return false
		}
		return true
	case "exec":
		exe, ok := simpleExecutable(readString(args, "command"))
		if !ok {
			return cfg.Exec.RequireApproval
		}
		if matchesCommand(exe, cfg.Exec.ForceReviewCommands) {
			return true
		}
		if matchesCommand(exe, cfg.Exec.BypassCommands) {
			return false
		}
		return cfg.Exec.RequireApproval
	default:
		return false
	}
}

func inputMap(input any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	if m, ok := input.(map[string]any); ok {
		return m
	}
	data, err := json.Marshal(input)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{}
	}
	return m
}

func readString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func normalizeContainerPath(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "/data" || p == "/tmp" {
		return p
	}
	p = strings.TrimPrefix(p, "./")
	if p == "" {
		return "."
	}
	return path.Clean(p)
}

func matchesAnyGlob(target string, patterns []string) bool {
	target = normalizeContainerPath(target)
	for _, raw := range patterns {
		pattern := normalizeContainerPath(raw)
		if pattern == "." || pattern == "" {
			continue
		}
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "/**")
			relativePrefix := strings.TrimLeft(prefix, "/")
			if prefix == "/data" && !strings.HasPrefix(target, "/") {
				return true
			}
			if target == prefix || strings.HasPrefix(target, prefix+"/") ||
				target == relativePrefix || strings.HasPrefix(target, relativePrefix+"/") {
				return true
			}
			continue
		}
		if ok, _ := path.Match(pattern, target); ok {
			return true
		}
		if !strings.Contains(pattern, "/") {
			if ok, _ := path.Match(pattern, path.Base(target)); ok {
				return true
			}
		}
	}
	return false
}

func simpleExecutable(command string) (string, bool) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return "", false
	}
	if strings.Contains(cmd, "&&") || strings.Contains(cmd, "||") ||
		strings.ContainsAny(cmd, ";|`") || strings.Contains(cmd, "$(") {
		return "", false
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", false
	}
	exe := strings.Trim(fields[0], `"'`)
	if exe == "" {
		return "", false
	}
	if unquoted, err := strconv.Unquote(exe); err == nil {
		exe = unquoted
	}
	return path.Base(exe), true
}

func matchesCommand(exe string, allowed []string) bool {
	exe = strings.ToLower(strings.TrimSpace(exe))
	for _, cmd := range allowed {
		if exe == strings.ToLower(strings.TrimSpace(cmd)) {
			return true
		}
	}
	return false
}
