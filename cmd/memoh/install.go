package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

const defaultInstallScriptURL = "https://memoh.sh"

func newInstallCommand() *cobra.Command {
	var version string
	var yes bool
	var force bool
	var scriptURL string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Download and run the Memoh install script",
		Long: `Download the official Memoh install script and run it when the
current directory is not already a Docker Compose deployment.`,
		Example: `  memoh install
  memoh install --yes
  memoh install --version v0.6.0
  USE_CN_MIRROR=true memoh install --yes`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstall(cmd.Context(), installOptions{
				version:   version,
				yes:       yes,
				force:     force,
				scriptURL: scriptURL,
			})
		},
	}

	cmd.Flags().StringVar(&version, "version", "", "Memoh version to install, for example v0.6.0")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Run the installer in non-interactive mode")
	cmd.Flags().BoolVar(&force, "force", false, "Run even if the current directory already contains a compose file")
	cmd.Flags().StringVar(&scriptURL, "script-url", defaultInstallScriptURL, "Install script URL")
	_ = cmd.Flags().MarkHidden("script-url")

	return cmd
}

type installOptions struct {
	version   string
	yes       bool
	force     bool
	scriptURL string
}

func runInstall(ctx context.Context, opts installOptions) error {
	if !opts.force {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve current directory: %w", err)
		}

		if composeFile, ok, err := findComposeFile(cwd); err != nil {
			return err
		} else if ok {
			return fmt.Errorf("found existing compose file %q in %s; use `memoh start` or rerun with --force", filepath.Base(composeFile), cwd)
		}
	}

	scriptPath, err := downloadInstallScript(ctx, opts.scriptURL)
	if err != nil {
		return err
	}
	defer removeFile(scriptPath)

	args := buildInstallScriptArgs(scriptPath, opts)
	cmd := exec.CommandContext(ctx, "sh", args...) //nolint:gosec // trusted script downloaded from the official URL
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildInstallScriptArgs(scriptPath string, opts installOptions) []string {
	args := []string{scriptPath}
	if opts.yes {
		args = append(args, "--yes")
	}
	if opts.version != "" {
		args = append(args, "--version", opts.version)
	}
	return args
}

func findComposeFile(dir string) (string, bool, error) {
	for _, name := range []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, true, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", false, fmt.Errorf("check compose file %s: %w", path, err)
		}
	}
	return "", false, nil
}

func downloadInstallScript(ctx context.Context, scriptURL string) (string, error) {
	parsedURL, err := validateInstallScriptURL(scriptURL)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create install script request: %w", err)
	}
	req.Header.Set("User-Agent", "memoh-cli")

	client := &http.Client{}
	resp, err := client.Do(req) //nolint:gosec // URL is restricted to https://memoh.sh by validateInstallScriptURL
	if err != nil {
		return "", fmt.Errorf("download install script: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download install script: unexpected HTTP status %s", resp.Status)
	}

	file, err := os.CreateTemp("", "memoh-install-*.sh")
	if err != nil {
		return "", fmt.Errorf("create temp install script: %w", err)
	}
	tempPath := file.Name()
	defer closeFile(file)

	if _, err := io.Copy(file, resp.Body); err != nil {
		removeFile(tempPath)
		return "", fmt.Errorf("write temp install script: %w", err)
	}
	if err := file.Chmod(0o700); err != nil {
		removeFile(tempPath)
		return "", fmt.Errorf("chmod temp install script: %w", err)
	}

	return tempPath, nil
}

func validateInstallScriptURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse install script url: %w", err)
	}
	if parsedURL.Scheme != "https" {
		return nil, errors.New("install script URL must use https")
	}
	if parsedURL.Host != "memoh.sh" {
		return nil, errors.New("install script URL host must be memoh.sh")
	}
	return parsedURL, nil
}

func removeFile(path string) {
	_ = os.Remove(path) //nolint:gosec // path comes from CreateTemp or internal cleanup targets, not user-controlled traversal input
}

func closeFile(file *os.File) {
	_ = file.Close()
}
