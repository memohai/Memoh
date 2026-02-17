// Package main is the entry point for the Memoh CLI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	gocni "github.com/containerd/go-cni"
)

func main() {
	flag.CommandLine.SetOutput(io.Discard)
	containerID := flag.String("container-id", "", "")
	flag.Parse()

	if len(flag.Args()) > 0 {
		switch flag.Arg(0) {
		case "cni-setup":
			os.Exit(runCNISetup(flag.Args()[1:]))
		case "cni-remove":
			os.Exit(runCNIRemove(flag.Args()[1:]))
		case "cni-check":
			os.Exit(runCNICheck(flag.Args()[1:]))
		case "cni-status":
			os.Exit(runCNIStatus(flag.Args()[1:]))
		}
	}

	if *containerID == "" {
		os.Exit(2)
	}

	cmd := buildMCPCommand(*containerID)
	if err := runWithStdio(cmd); err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func buildMCPCommand(containerID string) *exec.Cmd {
	execID := "mcp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if runtime.GOOS == "darwin" {
		return exec.Command(
			"limactl",
			"shell",
			"--tty=false",
			"default",
			"--",
			"sudo",
			"-n",
			"ctr",
			"-n",
			"default",
			"tasks",
			"exec",
			"--exec-id",
			execID,
			containerID,
			"/mcp",
		)
	}
	return exec.Command(
		"ctr",
		"-n",
		"default",
		"tasks",
		"exec",
		"--exec-id",
		execID,
		containerID,
		"/mcp",
	)
}

func runWithStdio(cmd *exec.Cmd) error {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return err
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return err
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdin, os.Stdin)
		_ = stdin.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stdout, stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stderr, stderr)
	}()

	err = cmd.Wait()
	wg.Wait()
	return err
}

func runCNISetup(args []string) int {
	id, netns, err := parseCNIArgs(args)
	if err != nil {
		return exitWithError(err)
	}
	cni, err := newCNIFromArgs(args)
	if err != nil {
		return exitWithError(err)
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return exitWithError(err)
	}
	result, err := cni.Setup(context.Background(), id, netns)
	if err != nil {
		return exitWithError(err)
	}
	if result != nil {
		_ = json.NewEncoder(os.Stdout).Encode(result)
	}
	return 0
}

func runCNIRemove(args []string) int {
	id, netns, err := parseCNIArgs(args)
	if err != nil {
		return exitWithError(err)
	}
	cni, err := newCNIFromArgs(args)
	if err != nil {
		return exitWithError(err)
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return exitWithError(err)
	}
	if err := cni.Remove(context.Background(), id, netns); err != nil {
		return exitWithError(err)
	}
	return 0
}

func runCNICheck(args []string) int {
	id, netns, err := parseCNIArgs(args)
	if err != nil {
		return exitWithError(err)
	}
	cni, err := newCNIFromArgs(args)
	if err != nil {
		return exitWithError(err)
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return exitWithError(err)
	}
	if err := cni.Check(context.Background(), id, netns); err != nil {
		return exitWithError(err)
	}
	return 0
}

func runCNIStatus(args []string) int {
	cni, err := newCNIFromArgs(args)
	if err != nil {
		return exitWithError(err)
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return exitWithError(err)
	}
	if err := cni.Status(); err != nil {
		return exitWithError(err)
	}
	return 0
}

func parseCNIArgs(args []string) (string, string, error) {
	fs := flag.NewFlagSet("cni", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	id := fs.String("id", "", "")
	netns := fs.String("netns", "", "")
	pid := fs.Int("pid", 0, "")
	_ = fs.String("conf-dir", "", "")
	_ = fs.String("bin-dir", "", "")
	_ = fs.String("if-prefix", "", "")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	if *id == "" {
		return "", "", errors.New("missing --id")
	}
	if *netns == "" && *pid == 0 {
		return "", "", errors.New("missing --netns or --pid")
	}
	if *netns == "" {
		*netns = filepath.Join("/proc", strconv.Itoa(*pid), "ns", "net")
	}
	return *id, *netns, nil
}

func newCNIFromArgs(args []string) (gocni.CNI, error) {
	fs := flag.NewFlagSet("cni", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	confDir := fs.String("conf-dir", "", "")
	binDir := fs.String("bin-dir", "", "")
	ifPrefix := fs.String("if-prefix", "", "")
	_ = fs.String("id", "", "")
	_ = fs.String("netns", "", "")
	_ = fs.Int("pid", 0, "")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	opts := []gocni.Opt{}
	if strings.TrimSpace(*binDir) != "" {
		opts = append(opts, gocni.WithPluginDir([]string{*binDir}))
	}
	if strings.TrimSpace(*confDir) != "" {
		opts = append(opts, gocni.WithPluginConfDir(*confDir))
	}
	if strings.TrimSpace(*ifPrefix) != "" {
		opts = append(opts, gocni.WithInterfacePrefix(*ifPrefix))
	}
	return gocni.New(opts...)
}

func exitWithError(err error) int {
	_, _ = fmt.Fprintln(os.Stderr, err.Error())
	return 1
}
