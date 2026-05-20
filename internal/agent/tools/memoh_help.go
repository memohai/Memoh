package tools

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"math"
	"sort"
	"strings"
	"unicode"

	sdk "github.com/memohai/twilight-ai/sdk"

	helpdocs "github.com/memohai/memoh/docs"
	"github.com/memohai/memoh/internal/textutil"
)

const (
	memohHelpDefaultLimit = 5
	memohHelpMaxLimit     = 12
	memohHelpMaxTextRunes = 1800
	memohHelpBM25K1       = 1.2
	memohHelpBM25B        = 0.75
)

type memohHelpChunk struct {
	Path  string
	Title string
	Text  string
}

type memohHelpScoredChunk struct {
	chunk memohHelpChunk
	score float64
}

// MemohHelpProvider exposes bundled Memoh help documents to the agent. Chunks
// are loaded once from embedded markdown in the constructor and are read-only
// afterwards, so the provider is safe to share across goroutines.
type MemohHelpProvider struct {
	chunks []memohHelpChunk
	index  memohHelpBM25Index
}

func NewMemohHelpProvider(log *slog.Logger) *MemohHelpProvider {
	if log == nil {
		log = slog.Default()
	}
	log = log.With(slog.String("tool", "memoh_help"))
	chunks := loadMemohHelpChunks(log)
	if len(chunks) == 0 {
		log.Warn("no embedded help chunks were loaded; the help corpus contains no markdown files yet")
	}
	return &MemohHelpProvider{
		chunks: chunks,
		index:  newMemohHelpBM25Index(chunks),
	}
}

func (p *MemohHelpProvider) Tools(_ context.Context, _ SessionContext) ([]sdk.Tool, error) {
	return []sdk.Tool{
		{
			Name:        "memoh_help",
			Description: "Search bundled Memoh help documents for questions about Memoh configuration, usage, architecture, desktop/server mode, channels, memory, workspaces, schedules, skills, and troubleshooting.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Natural language question or keywords about Memoh.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of help snippets to return. Defaults to 5, max 12.",
					},
				},
				"required": []string{"query"},
			},
			Execute: func(_ *sdk.ToolExecContext, input any) (any, error) {
				return p.execMemohHelp(inputAsMap(input))
			},
		},
	}, nil
}

func (p *MemohHelpProvider) execMemohHelp(args map[string]any) (any, error) {
	query := strings.TrimSpace(StringArg(args, "query"))
	if query == "" {
		return nil, errors.New("query is required")
	}

	limit := memohHelpDefaultLimit
	if v, ok, err := IntArg(args, "limit"); err != nil {
		return nil, err
	} else if ok {
		limit = v
	}
	limit = clamp(limit, 1, memohHelpMaxLimit)

	results := searchMemohHelpIndex(p.index, query, limit)
	return map[string]any{
		"query":   query,
		"results": results,
	}, nil
}

func loadMemohHelpChunks(log *slog.Logger) []memohHelpChunk {
	var paths []string
	err := fs.WalkDir(helpdocs.HelpCorpus, "docs", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			log.Warn("walk embedded help docs", slog.String("path", path), slog.Any("err", walkErr))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		log.Warn("walk embedded help docs root", slog.Any("err", err))
		return nil
	}
	sort.Strings(paths)

	var chunks []memohHelpChunk
	for _, path := range paths {
		data, err := helpdocs.HelpCorpus.ReadFile(path)
		if err != nil {
			log.Warn("read embedded help doc", slog.String("path", path), slog.Any("err", err))
			continue
		}
		body := stripVitePressFrontmatter(string(data))
		chunks = append(chunks, splitMemohHelpDoc(path, body)...)
	}
	return chunks
}

// stripVitePressFrontmatter removes a YAML frontmatter block at the very start
// of a markdown document, if present. VitePress / Vue Press use a fenced
// `---` block on lines 1..N to declare page metadata. We strip it so the
// agent does not see raw YAML in search results. Mid-document `---` horizontal
// rules are preserved.
func stripVitePressFrontmatter(content string) string {
	rest, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		if rest2, ok2 := strings.CutPrefix(content, "---\r\n"); ok2 {
			rest = rest2
		} else {
			return content
		}
	}
	for i := 0; i < len(rest); {
		nl := strings.IndexByte(rest[i:], '\n')
		var line string
		var next int
		if nl < 0 {
			line = rest[i:]
			next = len(rest)
		} else {
			line = rest[i : i+nl]
			next = i + nl + 1
		}
		line = strings.TrimRight(line, "\r")
		if strings.TrimRight(line, " \t") == "---" {
			return rest[next:]
		}
		if nl < 0 {
			break
		}
		i = next
	}
	return content
}

func splitMemohHelpDoc(path, content string) []memohHelpChunk {
	relPath := memohHelpRelPath(path)
	lines := strings.Split(content, "\n")
	var chunks []memohHelpChunk
	title := relPath
	var body []string
	inFence := false

	flush := func() {
		text := strings.TrimSpace(strings.Join(body, "\n"))
		if text == "" {
			return
		}
		chunks = append(chunks, memohHelpChunk{
			Path:  relPath,
			Title: title,
			Text:  textutil.TruncateRunesWithSuffix(text, memohHelpMaxTextRunes+3, "..."),
		})
		body = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isMarkdownFence(trimmed) {
			inFence = !inFence
			body = append(body, line)
			continue
		}
		if !inFence && strings.HasPrefix(trimmed, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if heading != "" {
				flush()
				title = heading
				continue
			}
		}
		body = append(body, line)
	}
	flush()
	return chunks
}

func isMarkdownFence(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func memohHelpRelPath(path string) string {
	switch {
	case strings.HasPrefix(path, "docs/zh/"):
		return strings.TrimPrefix(path, "docs/")
	case strings.HasPrefix(path, "docs/"):
		return "en/" + strings.TrimPrefix(path, "docs/")
	default:
		return path
	}
}

func searchMemohHelpChunks(chunks []memohHelpChunk, query string, limit int) []map[string]any {
	return searchMemohHelpIndex(newMemohHelpBM25Index(chunks), query, limit)
}

func searchMemohHelpIndex(index memohHelpBM25Index, query string, limit int) []map[string]any {
	queryTokens := memohHelpQueryTokens(query)
	if len(queryTokens) == 0 {
		return nil
	}
	queryLower := strings.ToLower(query)
	scored := make([]memohHelpScoredChunk, 0, len(index.docs))

	for _, doc := range index.docs {
		score := scoreMemohHelpChunk(doc, queryLower, queryTokens, index)
		if score <= 0 {
			continue
		}
		scored = append(scored, memohHelpScoredChunk{chunk: doc.chunk, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}

	results := make([]map[string]any, 0, len(scored))
	for _, item := range scored {
		results = append(results, map[string]any{
			"path":  item.chunk.Path,
			"title": item.chunk.Title,
			"text":  item.chunk.Text,
		})
	}
	return results
}

type memohHelpBM25Index struct {
	docs      []memohHelpBM25Doc
	docFreq   map[string]int
	avgDocLen float64
}

type memohHelpBM25Doc struct {
	chunk      memohHelpChunk
	tf         map[string]int
	length     int
	pathLower  string
	titleLower string
	textLower  string
}

func newMemohHelpBM25Index(chunks []memohHelpChunk) memohHelpBM25Index {
	index := memohHelpBM25Index{
		docs:    make([]memohHelpBM25Doc, 0, len(chunks)),
		docFreq: map[string]int{},
	}
	totalLen := 0

	for _, chunk := range chunks {
		doc := memohHelpBM25Doc{
			chunk:      chunk,
			tf:         map[string]int{},
			pathLower:  strings.ToLower(chunk.Path),
			titleLower: strings.ToLower(chunk.Title),
			textLower:  strings.ToLower(chunk.Text),
		}
		addField := func(text string, weight int) {
			for _, token := range memohHelpTokens(text) {
				for i := 0; i < weight; i++ {
					doc.tf[token]++
					doc.length++
				}
			}
		}
		addField(chunk.Path, 3)
		addField(chunk.Title, 4)
		addField(chunk.Text, 1)
		if doc.length == 0 {
			continue
		}
		for token := range doc.tf {
			index.docFreq[token]++
		}
		totalLen += doc.length
		index.docs = append(index.docs, doc)
	}

	if len(index.docs) > 0 {
		index.avgDocLen = float64(totalLen) / float64(len(index.docs))
	}
	return index
}

func scoreMemohHelpChunk(doc memohHelpBM25Doc, queryLower string, terms []string, index memohHelpBM25Index) float64 {
	if len(index.docs) == 0 || index.avgDocLen <= 0 || doc.length == 0 {
		return 0
	}

	var score float64
	for _, term := range terms {
		tf := doc.tf[term]
		df := index.docFreq[term]
		if tf == 0 || df == 0 {
			continue
		}
		idf := math.Log(1 + (float64(len(index.docs)-df)+0.5)/(float64(df)+0.5))
		tf64 := float64(tf)
		docLen := float64(doc.length)
		denom := tf64 + memohHelpBM25K1*(1-memohHelpBM25B+memohHelpBM25B*docLen/index.avgDocLen)
		score += idf * (tf64 * (memohHelpBM25K1 + 1)) / denom
	}

	if queryLower != "" {
		switch {
		case strings.Contains(doc.titleLower, queryLower):
			score += 3
		case strings.Contains(doc.pathLower, queryLower):
			score += 2
		case strings.Contains(doc.textLower, queryLower):
			score++
		}
	}

	return score
}

func memohHelpQueryTokens(query string) []string {
	tokens := memohHelpTokens(query)
	filtered := tokens[:0]
	for _, token := range tokens {
		if isMemohHelpQueryStopTerm(token) {
			continue
		}
		filtered = append(filtered, token)
	}
	return filtered
}

func isMemohHelpQueryStopTerm(term string) bool {
	switch term {
	case "memoh", "是什", "什么", "怎么", "如何":
		return true
	default:
		return false
	}
}

// memohHelpTokens keeps Latin/digit runs as whole words and emits CJK (Han)
// runs as bigrams. Bigrams keep Chinese queries useful without letting common
// single characters match almost every document.
func memohHelpTokens(text string) []string {
	var tokens []string
	add := func(token string) {
		token = strings.ToLower(strings.TrimSpace(token))
		if token != "" {
			tokens = append(tokens, token)
		}
	}

	fields := strings.FieldsFunc(text, func(r rune) bool {
		if unicode.Is(unicode.Han, r) {
			return true
		}
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, field := range fields {
		add(field)
	}

	runes := []rune(text)
	runStart := -1
	flushRun := func(end int) {
		if runStart < 0 {
			return
		}
		run := runes[runStart:end]
		switch len(run) {
		case 0:
		case 1:
			add(string(run))
		default:
			for i := 0; i+1 < len(run); i++ {
				add(string(run[i : i+2]))
			}
		}
		runStart = -1
	}
	for i, r := range runes {
		if unicode.Is(unicode.Han, r) {
			if runStart < 0 {
				runStart = i
			}
			continue
		}
		flushRun(i)
	}
	flushRun(len(runes))
	return tokens
}
