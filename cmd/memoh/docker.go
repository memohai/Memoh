package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

type dockerComposeOptions struct {
	projectDir string
	files      []string
	profiles   []string
	envFile    string
}

type serverContainerdOptions struct {
	dockerComposeOptions
	service     string
	namespace   string
	noNamespace bool
	noTTY       bool
}

func runDockerCompose(ctx context.Context, opts dockerComposeOptions, composeArgs []string) error {
	dockerCmd, err := resolveDockerCommand(ctx)
	if err != nil {
		return err
	}

	args := append(buildComposeBaseArgs(opts), composeArgs...)
	if err := runExternalCommand(ctx, opts.projectDir, dockerCmd, args); err != nil {
		return fmt.Errorf("run docker compose: %w", err)
	}
	return nil
}

func runServerContainerd(ctx context.Context, opts serverContainerdOptions, ctrArgs []string) error {
	dockerCmd, err := resolveDockerCommand(ctx)
	if err != nil {
		return err
	}

	args := buildContainerdExecArgs(opts, ctrArgs)
	if err := runExternalCommand(ctx, opts.projectDir, dockerCmd, args); err != nil {
		return fmt.Errorf("run ctr in %s service: %w", opts.service, err)
	}
	return nil
}

func resolveDockerCommand(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, errors.New("docker is not installed")
	}

	dockerCmd := []string{"docker"}
	if !canRun(ctx, dockerCmd, "info") {
		if _, err := exec.LookPath("sudo"); err == nil && canRun(ctx, []string{"sudo", "docker"}, "info") {
			dockerCmd = []string{"sudo", "docker"}
		} else {
			return nil, errors.New("cannot connect to the Docker daemon")
		}
	}

	if !canRun(ctx, dockerCmd, "compose", "version") {
		return nil, errors.New("docker compose v2 is required")
	}

	return dockerCmd, nil
}

func canRun(ctx context.Context, command []string, args ...string) bool {
	if len(command) == 0 {
		return false
	}
	cmd := exec.CommandContext(ctx, command[0], append(command[1:], args...)...) //nolint:gosec // trusted local tooling
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Stdin = nil
	return cmd.Run() == nil
}

func runExternalCommand(ctx context.Context, dir string, base []string, args []string) error {
	if len(base) == 0 {
		return errors.New("missing command")
	}

	cmd := exec.CommandContext(ctx, base[0], append(base[1:], args...)...) //nolint:gosec // trusted local tooling
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildComposeBaseArgs(opts dockerComposeOptions) []string {
	args := []string{"compose"}
	if opts.projectDir != "" {
		args = append(args, "--project-directory", opts.projectDir)
	}
	for _, file := range opts.files {
		args = append(args, "-f", file)
	}
	for _, profile := range opts.profiles {
		args = append(args, "--profile", profile)
	}
	if opts.envFile != "" {
		args = append(args, "--env-file", opts.envFile)
	}
	return args
}

func buildContainerdExecArgs(opts serverContainerdOptions, ctrArgs []string) []string {
	args := append(buildComposeBaseArgs(opts.dockerComposeOptions), "exec")
	if opts.noTTY || shouldDisableTTY() {
		args = append(args, "-T")
	}
	args = append(args, opts.service, "ctr")
	if !opts.noNamespace && opts.namespace != "" && !containsCtrNamespaceArg(ctrArgs) {
		args = append(args, "-n", opts.namespace)
	}
	return append(args, ctrArgs...)
}

func containsCtrNamespaceArg(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-n", arg == "--namespace":
			return true
		case strings.HasPrefix(arg, "--namespace="), strings.HasPrefix(arg, "-n="):
			return true
		}
	}
	return false
}

func shouldDisableTTY() bool {
	return !isTerminalFile(os.Stdin) || !isTerminalFile(os.Stdout)
}

func isTerminalFile(file *os.File) bool {
	fd := file.Fd()
	const maxInt = int(^uint(0) >> 1)
	if fd > uintptr(maxInt) {
		return false
	}
	return term.IsTerminal(int(fd))
}

func parseComposeOptions(args []string) (dockerComposeOptions, []string, error) {
	opts := dockerComposeOptions{}
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}

		switch {
		case arg == "--project-dir":
			value, next, err := requireNextValue(args, i, arg)
			if err != nil {
				return opts, nil, err
			}
			opts.projectDir = value
			i = next
		case strings.HasPrefix(arg, "--project-dir="):
			opts.projectDir = strings.TrimPrefix(arg, "--project-dir=")
			i++
		case arg == "-f", arg == "--file":
			value, next, err := requireNextValue(args, i, arg)
			if err != nil {
				return opts, nil, err
			}
			opts.files = append(opts.files, value)
			i = next
		case strings.HasPrefix(arg, "--file="):
			opts.files = append(opts.files, strings.TrimPrefix(arg, "--file="))
			i++
		case strings.HasPrefix(arg, "-f="):
			opts.files = append(opts.files, strings.TrimPrefix(arg, "-f="))
			i++
		case arg == "--profile":
			value, next, err := requireNextValue(args, i, arg)
			if err != nil {
				return opts, nil, err
			}
			opts.profiles = append(opts.profiles, value)
			i = next
		case strings.HasPrefix(arg, "--profile="):
			opts.profiles = append(opts.profiles, strings.TrimPrefix(arg, "--profile="))
			i++
		case arg == "--env-file":
			value, next, err := requireNextValue(args, i, arg)
			if err != nil {
				return opts, nil, err
			}
			opts.envFile = value
			i = next
		case strings.HasPrefix(arg, "--env-file="):
			opts.envFile = strings.TrimPrefix(arg, "--env-file=")
			i++
		default:
			return opts, args[i:], nil
		}
	}

	return opts, args[i:], nil
}

func parseContainerdOptions(args []string) (serverContainerdOptions, []string, error) {
	opts := serverContainerdOptions{
		service:   "server",
		namespace: defaultContainerdNamespace(),
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}

		switch {
		case arg == "--server-service":
			value, next, err := requireNextValue(args, i, arg)
			if err != nil {
				return opts, nil, err
			}
			opts.service = value
			i = next
		case strings.HasPrefix(arg, "--server-service="):
			opts.service = strings.TrimPrefix(arg, "--server-service=")
			i++
		case arg == "--namespace":
			value, next, err := requireNextValue(args, i, arg)
			if err != nil {
				return opts, nil, err
			}
			opts.namespace = value
			opts.noNamespace = false
			i = next
		case strings.HasPrefix(arg, "--namespace="):
			opts.namespace = strings.TrimPrefix(arg, "--namespace=")
			opts.noNamespace = false
			i++
		case arg == "--no-namespace":
			opts.noNamespace = true
			i++
		case arg == "--no-tty":
			opts.noTTY = true
			i++
		default:
			composeOpts, remaining, err := parseComposeOptions(args[i:])
			if err != nil {
				return opts, nil, err
			}
			opts.dockerComposeOptions = composeOpts
			return opts, remaining, nil
		}
	}

	return opts, args[i:], nil
}

func requireNextValue(args []string, index int, flagName string) (string, int, error) {
	if index+1 >= len(args) {
		return "", 0, fmt.Errorf("flag %s requires a value", flagName)
	}
	return args[index+1], index + 2, nil
}

func defaultContainerdNamespace() string {
	cfg, err := provideConfig()
	if err != nil {
		return configDefaultContainerdNamespace
	}
	if strings.TrimSpace(cfg.Containerd.Namespace) == "" {
		return configDefaultContainerdNamespace
	}
	return cfg.Containerd.Namespace
}

const configDefaultContainerdNamespace = "default"
