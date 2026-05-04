package local

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// TailOptions controls Tail behavior. Defaults are designed to mimic
// the conventional `tail -n 200` / `tail -f` shell invocation.
type TailOptions struct {
	// Tail prints at most this many trailing lines from the existing
	// file before switching to follow mode (if Follow is true). Zero
	// disables the historical pre-print; negative values mean "all".
	Tail int
	// Follow leaves the file handle open and prints additional lines
	// as they are appended. Set to false for a one-shot read.
	Follow bool
}

// Tail prints the last Options.Tail lines of path to w, optionally
// continuing to stream new lines until ctx is cancelled. If the file
// does not exist Tail waits up to one second for it to appear (the
// server may be in mid-startup), then surfaces the missing-file error.
func Tail(ctx context.Context, path string, opts TailOptions, w io.Writer) error {
	if err := waitForFile(ctx, path, time.Second); err != nil {
		return err
	}
	if opts.Tail != 0 {
		if err := printLastLines(path, opts.Tail, w); err != nil {
			return err
		}
	}
	if !opts.Follow {
		return nil
	}
	return follow(ctx, path, w)
}

func waitForFile(ctx context.Context, path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("log file not found: %s", path)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// printLastLines reads the file backwards in 4KB chunks until it has
// captured at most `n` newline-separated lines (or the entire file if
// n < 0). The implementation is a pragmatic in-memory tail rather than
// a fully streaming one — log files are rotated by desktop separately
// and stay in the low-MB range.
func printLastLines(path string, n int, w io.Writer) error {
	if n < 0 {
		raw, err := os.ReadFile(path) //nolint:gosec // path is from UserDataDir
		if err != nil {
			return err
		}
		_, err = w.Write(raw)
		return err
	}
	file, err := os.Open(path) //nolint:gosec // path is from UserDataDir
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return err
	}
	size := stat.Size()
	if size == 0 {
		return nil
	}

	const chunk int64 = 4096
	var buf bytes.Buffer
	pos := size
	lines := 0
	for pos > 0 && lines <= n {
		readSize := chunk
		if pos < chunk {
			readSize = pos
		}
		pos -= readSize
		segment := make([]byte, readSize)
		if _, err := file.ReadAt(segment, pos); err != nil {
			return err
		}
		// Prepend (we are reading backwards).
		next := bytes.NewBuffer(make([]byte, 0, int(readSize)+buf.Len()))
		next.Write(segment)
		next.Write(buf.Bytes())
		buf = *next
		lines = bytes.Count(buf.Bytes(), []byte("\n"))
	}
	scanner := bufio.NewScanner(strings.NewReader(buf.String()))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	collected := make([]string, 0, n)
	for scanner.Scan() {
		collected = append(collected, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(collected) > n {
		collected = collected[len(collected)-n:]
	}
	for _, line := range collected {
		if _, err := fmt.Fprintln(w, line); err != nil { //nolint:gosec // log lines come from a trusted local server log file
			return err
		}
	}
	return nil
}

func follow(ctx context.Context, path string, w io.Writer) error {
	file, err := os.Open(path) //nolint:gosec // path is from UserDataDir
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	reader := bufio.NewReader(file)
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		n, err := reader.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
		}
	}
}
