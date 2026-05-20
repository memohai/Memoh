package aidevengine

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const displayName = "AI 开发引擎"
const codexVersionCheckTimeout = 3 * time.Second

type StatusResponse struct {
	Status       string `json:"status"`
	AuthStatus   string `json:"authStatus"`
	DisplayName  string `json:"displayName"`
	Version      string `json:"version,omitempty"`
	ErrorSummary string `json:"errorSummary,omitempty"`
}

type Capability struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type CapabilitiesResponse struct {
	DisplayName  string       `json:"displayName"`
	Capabilities []Capability `json:"capabilities"`
}

type Service struct {
	detectCodexVersion func(context.Context) (string, error)
}

func NewService() *Service {
	return &Service{
		detectCodexVersion: detectCodexVersion,
	}
}

func (s *Service) Status(ctx context.Context) StatusResponse {
	version, err := s.detectCodexVersion(ctx)
	if err != nil {
		return StatusResponse{
			Status:       "unavailable",
			AuthStatus:   "not_configured",
			DisplayName:  displayName,
			ErrorSummary: safeErrorSummary(err),
		}
	}
	return StatusResponse{
		Status:      "available",
		AuthStatus:  "not_configured",
		DisplayName: displayName,
		Version:     strings.TrimSpace(version),
	}
}

func (s *Service) Capabilities(ctx context.Context) CapabilitiesResponse {
	_ = ctx
	return CapabilitiesResponse{
		DisplayName: displayName,
		Capabilities: []Capability{
			{Key: "read_project", Name: "读取项目", Enabled: false},
			{Key: "modify_files", Name: "修改文件", Enabled: false},
			{Key: "execute_commands", Name: "执行命令", Enabled: false},
			{Key: "browser_operations", Name: "浏览器操作", Enabled: false},
			{Key: "git_operations", Name: "Git 操作", Enabled: false},
		},
	}
}

func detectCodexVersion(ctx context.Context) (string, error) {
	checkCtx, cancel := context.WithTimeout(ctx, codexVersionCheckTimeout)
	defer cancel()

	name := "codex"
	if runtime.GOOS == "windows" {
		name = "codex.cmd"
	}
	output, err := exec.CommandContext(checkCtx, name, "--version").CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func safeErrorSummary(err error) string {
	summary := strings.TrimSpace(err.Error())
	summary = strings.ReplaceAll(summary, "\r", " ")
	summary = strings.ReplaceAll(summary, "\n", " ")
	if len(summary) > 240 {
		return summary[:240]
	}
	return summary
}
