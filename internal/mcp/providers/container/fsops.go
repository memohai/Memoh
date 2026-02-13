package container

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	mcpgw "github.com/memohai/memoh/internal/mcp"
)

// FileEntry represents a filesystem entry returned by ExecList.
type FileEntry struct {
	Path    string
	IsDir   bool
	Size    int64
	Mode    uint32
	ModTime time.Time
}

// ExecRead reads a file inside the container via cat.
func ExecRead(ctx context.Context, runner ExecRunner, botID, workDir, filePath string) (string, error) {
	result, err := runner.ExecWithCapture(ctx, mcpgw.ExecRequest{
		BotID:   botID,
		Command: []string{"/bin/sh", "-c", "cat " + ShellQuote(filePath)},
		WorkDir: workDir,
	})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("%s", strings.TrimSpace(result.Stderr))
	}
	return result.Stdout, nil
}

// ExecWrite writes content to a file inside the container using base64 encoding
// to avoid shell escaping issues.
func ExecWrite(ctx context.Context, runner ExecRunner, botID, workDir, filePath, content string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	dir := path.Dir(filePath)
	script := fmt.Sprintf("mkdir -p %s && echo %s | base64 -d > %s",
		ShellQuote(dir), ShellQuote(encoded), ShellQuote(filePath))
	result, err := runner.ExecWithCapture(ctx, mcpgw.ExecRequest{
		BotID:   botID,
		Command: []string{"/bin/sh", "-c", script},
		WorkDir: workDir,
	})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s", strings.TrimSpace(result.Stderr))
	}
	return nil
}

// ExecList lists directory entries inside the container via find + stat.
// Output format per line: <name>|<type>|<size>|<mode>|<mtime_epoch>
func ExecList(ctx context.Context, runner ExecRunner, botID, workDir, dirPath string, recursive bool) ([]FileEntry, error) {
	depthFlag := "-maxdepth 1"
	if recursive {
		depthFlag = ""
	}
	// Use find to get entries, skip the root dir itself, then stat each entry.
	// busybox stat -c format: %n=name, %F=type, %s=size, %a=octal mode, %Y=mtime epoch
	script := fmt.Sprintf(
		`find %s %s ! -path %s -exec stat -c '%%n|%%F|%%s|%%a|%%Y' {} \;`,
		ShellQuote(dirPath), depthFlag, ShellQuote(dirPath),
	)
	result, err := runner.ExecWithCapture(ctx, mcpgw.ExecRequest{
		BotID:   botID,
		Command: []string{"/bin/sh", "-c", script},
		WorkDir: workDir,
	})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(result.Stderr))
	}
	return parseStatOutput(result.Stdout, dirPath), nil
}

// parseStatOutput parses lines of "fullpath|type|size|mode|mtime" into FileEntry slices.
func parseStatOutput(output, basePath string) []FileEntry {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	entries := make([]FileEntry, 0, len(lines))
	// Normalize base path for computing relative paths.
	base := strings.TrimSuffix(basePath, "/")
	if base == "" || base == "." {
		base = ""
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}
		fullPath := parts[0]
		fileType := parts[1]
		sizeStr := parts[2]
		modeStr := parts[3]
		mtimeStr := parts[4]

		// Compute relative path from base.
		rel := fullPath
		if base != "" {
			rel = strings.TrimPrefix(fullPath, base+"/")
		}
		if rel == "" || rel == "." {
			continue
		}

		isDir := strings.Contains(fileType, "directory")
		size, _ := strconv.ParseInt(sizeStr, 10, 64)
		mode64, _ := strconv.ParseUint(modeStr, 8, 32)
		mtimeEpoch, _ := strconv.ParseInt(mtimeStr, 10, 64)
		modTime := time.Unix(mtimeEpoch, 0)

		entries = append(entries, FileEntry{
			Path:    rel,
			IsDir:   isDir,
			Size:    size,
			Mode:    uint32(mode64),
			ModTime: modTime,
		})
	}
	return entries
}

// applyEdit performs the fuzzy text replacement logic on raw file content.
// Returns the updated content or an error.
func applyEdit(raw, filePath, oldText, newText string) (string, error) {
	bom, content := stripBOM(raw)
	originalEnding := detectLineEnding(content)
	normalizedContent := normalizeToLF(content)
	normalizedOld := normalizeToLF(oldText)
	normalizedNew := normalizeToLF(newText)
	match := fuzzyFindText(normalizedContent, normalizedOld)
	if !match.Found {
		return "", fmt.Errorf(
			"could not find the exact text in %s. the old text must match exactly including all whitespace and newlines",
			filePath,
		)
	}
	fuzzyContent := normalizeForFuzzyMatch(normalizedContent)
	fuzzyOld := normalizeForFuzzyMatch(normalizedOld)
	occurrences := strings.Count(fuzzyContent, fuzzyOld)
	if occurrences > 1 {
		return "", fmt.Errorf(
			"found %d occurrences of the text in %s. the text must be unique. please provide more context to make it unique",
			occurrences,
			filePath,
		)
	}
	baseContent := match.ContentForReplacement
	updated := baseContent[:match.Index] + normalizedNew + baseContent[match.Index+match.MatchLength:]
	if baseContent == updated {
		return "", fmt.Errorf(
			"no changes made to %s. the replacement produced identical content. this might indicate an issue with special characters or the text not existing as expected",
			filePath,
		)
	}
	return bom + restoreLineEndings(updated, originalEnding), nil
}

// ShellQuote wraps a string in single quotes, escaping embedded single quotes.
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexByte(s, '\'') < 0 {
		return "'" + s + "'"
	}
	var b strings.Builder
	b.WriteByte('\'')
	for _, c := range s {
		if c == '\'' {
			b.WriteString("'\\''")
		} else {
			b.WriteRune(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// ---------- fuzzy matching helpers (pure string processing, unchanged) ----------

type fuzzyMatchResult struct {
	Found                 bool
	Index                 int
	MatchLength           int
	ContentForReplacement string
}

func detectLineEnding(content string) string {
	crlfIdx := strings.Index(content, "\r\n")
	lfIdx := strings.Index(content, "\n")
	if lfIdx == -1 {
		return "\n"
	}
	if crlfIdx == -1 {
		return "\n"
	}
	if crlfIdx < lfIdx {
		return "\r\n"
	}
	return "\n"
}

func normalizeToLF(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func restoreLineEndings(text, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

func stripBOM(content string) (string, string) {
	const bom = "\uFEFF"
	if strings.HasPrefix(content, bom) {
		return bom, content[len(bom):]
	}
	return "", content
}

func normalizeForFuzzyMatch(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	trimmed := strings.Join(lines, "\n")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u2018', '\u2019', '\u201A', '\u201B':
			return '\''
		case '\u201C', '\u201D', '\u201E', '\u201F':
			return '"'
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			return '-'
		case '\u00A0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
			return ' '
		default:
			return r
		}
	}, trimmed)
}

func fuzzyFindText(content, oldText string) fuzzyMatchResult {
	exactIndex := strings.Index(content, oldText)
	if exactIndex != -1 {
		return fuzzyMatchResult{
			Found:                 true,
			Index:                 exactIndex,
			MatchLength:           len(oldText),
			ContentForReplacement: content,
		}
	}
	fuzzyContent := normalizeForFuzzyMatch(content)
	fuzzyOld := normalizeForFuzzyMatch(oldText)
	fuzzyIndex := strings.Index(fuzzyContent, fuzzyOld)
	if fuzzyIndex == -1 {
		return fuzzyMatchResult{
			Found:                 false,
			Index:                 -1,
			MatchLength:           0,
			ContentForReplacement: content,
		}
	}
	return fuzzyMatchResult{
		Found:                 true,
		Index:                 fuzzyIndex,
		MatchLength:           len(fuzzyOld),
		ContentForReplacement: fuzzyContent,
	}
}
