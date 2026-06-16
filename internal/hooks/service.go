package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

type ToolRunner interface {
	RunHookTool(ctx context.Context, toolName string, input map[string]any) (any, error)
}

type Service struct {
	logger   *slog.Logger
	provider bridge.Provider
}

var emptyConfigFile = []byte("{\n  \"version\": 1,\n  \"enabled\": true,\n  \"hooks\": []\n}\n")

func NewService(log *slog.Logger, provider bridge.Provider) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		logger:   log.With(slog.String("service", "hooks")),
		provider: provider,
	}
}

func (s *Service) Load(ctx context.Context, botID string) (Config, bool, error) {
	if s == nil || s.provider == nil {
		return Config{Version: 1, Enabled: boolPtr(false)}, false, nil
	}
	client, err := s.provider.MCPClient(ctx, strings.TrimSpace(botID))
	if err != nil {
		return Config{}, false, err
	}
	rc, err := client.ReadRaw(ctx, DefaultConfigPath)
	if err != nil {
		if errors.Is(err, bridge.ErrNotFound) {
			if err := client.WriteFile(ctx, DefaultConfigPath, emptyConfigFile); err != nil {
				return Config{}, false, err
			}
			cfg, err := ParseConfig(emptyConfigFile)
			if err != nil {
				return Config{}, false, err
			}
			return cfg, true, nil
		}
		return Config{}, false, err
	}
	defer func() { _ = rc.Close() }()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return Config{}, false, err
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		return Config{}, true, err
	}
	return cfg, true, nil
}

func (s *Service) Run(ctx context.Context, req Request, runner ToolRunner) (Result, error) {
	req.Version = 1
	if strings.TrimSpace(req.Event) == "" {
		return Result{}, errors.New("hook event is required")
	}
	cfg, _, err := s.Load(ctx, req.BotID)
	if err != nil {
		return Result{}, err
	}
	return s.RunConfig(ctx, cfg, req, runner)
}

func (s *Service) RunConfig(ctx context.Context, cfg Config, req Request, runner ToolRunner) (Result, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return Result{}, err
	}
	result := Result{
		Decision:         DecisionAllow,
		RuntimeSupported: RuntimeSupported(req.Event),
	}
	matches := cfg.Match(req)
	result.HooksMatched = len(matches)
	if len(matches) == 0 {
		return result, nil
	}
	for _, hook := range matches {
		hookReq := req
		hookReq.Version = 1
		hookReq.HookName = hook.Name
		for _, action := range hook.Actions {
			actionResult, err := s.runAction(ctx, cfg, hookReq, action, runner)
			result.ActionResults = append(result.ActionResults, actionResult)
			result.ActionsRun++
			if err != nil {
				onError := normalizeOnError(action.OnError)
				if onError == OnErrorIgnore {
					if s != nil && s.logger != nil {
						s.logger.Warn("hook action failed but was ignored",
							slog.String("event", req.Event),
							slog.String("hook", hook.Name),
							slog.String("action", action.Type),
							slog.String("error", err.Error()),
						)
					}
					continue
				}
				if onError == OnErrorBlock {
					result.Decision = DecisionDeny
					result.Reason = firstNonEmpty(actionResult.Reason, err.Error())
					return result, fmt.Errorf("%w: %s", ErrDenied, result.Reason)
				}
				return result, err
			}
			mergeDecision(&result, actionResult)
			if result.Decision == DecisionDeny {
				return result, fmt.Errorf("%w: %s", ErrDenied, result.Reason)
			}
		}
	}
	return result, nil
}

func (s *Service) runAction(ctx context.Context, cfg Config, req Request, action HookAction, runner ToolRunner) (ActionResult, error) {
	switch strings.TrimSpace(action.Type) {
	case ActionCommand:
		return s.runCommand(ctx, cfg, req, action)
	case ActionTool:
		return s.runTool(ctx, req, action, runner)
	case ActionMCPTool:
		return ActionResult{ActionType: action.Type, Error: ErrUnsupported.Error()}, ErrUnsupported
	default:
		err := fmt.Errorf("unsupported hook action type %q", action.Type)
		return ActionResult{ActionType: action.Type, Error: err.Error()}, err
	}
}

func (s *Service) runCommand(ctx context.Context, cfg Config, req Request, action HookAction) (ActionResult, error) {
	res := ActionResult{ActionType: ActionCommand, Name: action.Command}
	if s == nil || s.provider == nil {
		err := errors.New("hooks workspace provider is not configured")
		res.Error = err.Error()
		return res, err
	}
	timeout, err := parseTimeout(action.Timeout)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	client, err := s.provider.MCPClient(ctx, req.BotID)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	workDir := strings.TrimSpace(action.WorkDir)
	if workDir == "" {
		workDir = strings.TrimSpace(req.Workspace.CWD)
	}
	if workDir == "" {
		workDir = DefaultWorkDir
	}
	env := make([]string, 0, len(cfg.Env)+4)
	for key, value := range cfg.Env {
		if strings.TrimSpace(key) == "" {
			continue
		}
		env = append(env, key+"="+value)
	}
	env = append(env,
		"MEMOH_HOOK_EVENT="+req.Event,
		"MEMOH_HOOK_NAME="+req.HookName,
		"MEMOH_BOT_ID="+req.BotID,
		"MEMOH_SESSION_ID="+req.SessionID,
	)
	timeoutUnits := timeout.Round(time.Second) / time.Second
	if timeoutUnits <= 0 {
		timeoutUnits = 1
	}
	if timeoutUnits > math.MaxInt32 {
		timeoutUnits = math.MaxInt32
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout+time.Second)
	defer cancel()
	execResult, err := client.ExecWithStdinEnv(execCtx, action.Command, workDir, int32(timeoutUnits), append(payload, '\n'), env)
	if execResult != nil {
		res.Stdout = trimOutput(execResult.Stdout, cfg.Defaults.MaxOutputBytes)
		res.Stderr = trimOutput(execResult.Stderr, cfg.Defaults.MaxOutputBytes)
		res.ExitCode = execResult.ExitCode
	}
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	if execResult != nil && execResult.ExitCode != 0 {
		err := fmt.Errorf("hook command exited with code %d", execResult.ExitCode)
		res.Error = err.Error()
		return res, err
	}
	if execResult != nil {
		applyActionOutput(&res, execResult.Stdout)
	} else {
		applyActionOutput(&res, "")
	}
	return res, nil
}

func (*Service) runTool(ctx context.Context, _ Request, action HookAction, runner ToolRunner) (ActionResult, error) {
	res := ActionResult{ActionType: ActionTool, Name: action.Tool}
	if runner == nil {
		err := errors.New("hook tool runner is not configured")
		res.Error = err.Error()
		return res, err
	}
	timeout, err := parseTimeout(action.Timeout)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	input := action.Input
	if input == nil {
		input = map[string]any{}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := runner.RunHookTool(runCtx, action.Tool, input)
	res.Result = output
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	applyToolOutput(&res, output)
	return res, nil
}

func applyActionOutput(result *ActionResult, stdout string) {
	raw := strings.TrimSpace(stdout)
	if raw == "" {
		result.Decision = DecisionAllow
		return
	}
	var output struct {
		Decision      string         `json:"decision"`
		Reason        string         `json:"reason"`
		AppendContext string         `json:"append_context"`
		Metadata      map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		result.Decision = DecisionAllow
		result.Metadata = map[string]any{"raw_stdout": raw}
		return
	}
	result.Decision = normalizeDecision(output.Decision)
	result.Reason = strings.TrimSpace(output.Reason)
	if output.AppendContext != "" {
		if result.Metadata == nil {
			result.Metadata = map[string]any{}
		}
		result.Metadata["append_context"] = output.AppendContext
	}
	if output.Metadata != nil {
		if result.Metadata == nil {
			result.Metadata = map[string]any{}
		}
		for k, v := range output.Metadata {
			result.Metadata[k] = v
		}
	}
}

func applyToolOutput(result *ActionResult, output any) {
	m, ok := output.(map[string]any)
	if !ok {
		raw, err := json.Marshal(output)
		if err == nil {
			_ = json.Unmarshal(raw, &m)
		}
	}
	if m == nil {
		result.Decision = DecisionAllow
		return
	}
	if decision, _ := m["decision"].(string); decision != "" {
		result.Decision = normalizeDecision(decision)
	}
	if reason, _ := m["reason"].(string); reason != "" {
		result.Reason = strings.TrimSpace(reason)
	}
	if appendContext, _ := m["append_context"].(string); appendContext != "" {
		result.Metadata = map[string]any{"append_context": appendContext}
	}
}

func mergeDecision(result *Result, actionResult ActionResult) {
	decision := normalizeDecision(actionResult.Decision)
	if decision == "" {
		decision = DecisionAllow
	}
	if actionResult.Metadata != nil {
		if appendContext, _ := actionResult.Metadata["append_context"].(string); strings.TrimSpace(appendContext) != "" {
			if result.AppendContext != "" {
				result.AppendContext += "\n"
			}
			result.AppendContext += appendContext
		}
	}
	switch decision {
	case DecisionDeny:
		result.Decision = DecisionDeny
		result.Reason = firstNonEmpty(actionResult.Reason, "hook denied action")
	case DecisionAskApproval:
		if result.Decision != DecisionDeny {
			result.Decision = DecisionAskApproval
			result.Reason = firstNonEmpty(actionResult.Reason, result.Reason)
		}
	case DecisionAppendContext:
		if result.Decision == "" || result.Decision == DecisionAllow {
			result.Decision = DecisionAppendContext
		}
		result.Reason = firstNonEmpty(actionResult.Reason, result.Reason)
	case DecisionAllow:
		if result.Decision == "" {
			result.Decision = DecisionAllow
		}
	}
}

func normalizeDecision(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case DecisionDeny:
		return DecisionDeny
	case DecisionAskApproval:
		return DecisionAskApproval
	case DecisionAppendContext:
		return DecisionAppendContext
	case DecisionAllow, "":
		return DecisionAllow
	default:
		return DecisionAllow
	}
}

func trimOutput(raw string, limit int) string {
	if limit <= 0 || len(raw) <= limit {
		return raw
	}
	return raw[:limit]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
