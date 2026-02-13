package memory

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/blevesearch/bleve/v2/registry"

	_ "github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ar"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/bg"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ca"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ckb"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/da"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/de"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/el"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/en"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/es"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/eu"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/fa"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/fi"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/fr"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ga"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/gl"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hi"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hr"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hu"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hy"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/id"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/it"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/nl"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/no"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/pl"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/pt"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ro"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ru"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/sv"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/tr"
)

const (
	defaultBM25K1 = 1.2
	defaultBM25B  = 0.75
	sparseDimBits = 20
	sparseDimSize = 1 << sparseDimBits
	sparseDimMask = sparseDimSize - 1
)

type BM25Indexer struct {
	cache  *registry.Cache
	logger *slog.Logger
	k1     float64
	b      float64

	mu    sync.RWMutex
	stats map[string]*bm25Stats
}

type bm25Stats struct {
	DocCount  int
	AvgDocLen float64
	DocFreq   map[string]int
}

func NewBM25Indexer(log *slog.Logger) *BM25Indexer {
	if log == nil {
		log = slog.Default()
	}
	return &BM25Indexer{
		cache:  registry.NewCache(),
		logger: log.With(slog.String("indexer", "bm25")),
		k1:     defaultBM25K1,
		b:      defaultBM25B,
		stats:  map[string]*bm25Stats{},
	}
}

func (b *BM25Indexer) TermFrequencies(lang, text string) (map[string]int, int, error) {
	analyzerName, err := b.normalizeAnalyzer(lang)
	if err != nil {
		return nil, 0, err
	}
	analyzer, err := b.cache.AnalyzerNamed(analyzerName)
	if err != nil {
		return nil, 0, fmt.Errorf("bm25 analyzer %s: %w", analyzerName, err)
	}
	tokens := analyzer.Analyze([]byte(text))
	freq := map[string]int{}
	docLen := 0
	for _, token := range tokens {
		term := strings.TrimSpace(string(token.Term))
		if term == "" {
			continue
		}
		freq[term]++
		docLen++
	}
	return freq, docLen, nil
}

func (b *BM25Indexer) AddDocument(lang string, termFreq map[string]int, docLen int) (indices []uint32, values []float32) {
	b.mu.Lock()
	stats := b.ensureStatsLocked(lang)
	b.updateStatsAddLocked(stats, termFreq, docLen)
	indices, values = b.buildDocVectorLocked(stats, termFreq, docLen)
	b.mu.Unlock()
	return indices, values
}

func (b *BM25Indexer) RemoveDocument(lang string, termFreq map[string]int, docLen int) {
	b.mu.Lock()
	stats := b.ensureStatsLocked(lang)
	b.updateStatsRemoveLocked(stats, termFreq, docLen)
	b.mu.Unlock()
}

func (b *BM25Indexer) BuildQueryVector(lang string, termFreq map[string]int) (indices []uint32, values []float32) {
	b.mu.RLock()
	stats := b.ensureStatsLocked(lang)
	indices, values = b.buildQueryVectorLocked(stats, termFreq)
	b.mu.RUnlock()
	return indices, values
}

func (b *BM25Indexer) normalizeAnalyzer(lang string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(lang))
	switch normalized {
	case "":
		return "standard", nil
	case "in":
		normalized = "id"
	}
	return normalized, nil
}

func (b *BM25Indexer) ensureStatsLocked(lang string) *bm25Stats {
	name, _ := b.normalizeAnalyzer(lang)
	stats := b.stats[name]
	if stats == nil {
		stats = &bm25Stats{
			DocFreq: map[string]int{},
		}
		b.stats[name] = stats
	}
	return stats
}

func (b *BM25Indexer) updateStatsAddLocked(stats *bm25Stats, termFreq map[string]int, docLen int) {
	totalDocs := stats.DocCount
	stats.DocCount++
	totalLen := stats.AvgDocLen * float64(totalDocs)
	stats.AvgDocLen = (totalLen + float64(docLen)) / float64(stats.DocCount)
	for term := range termFreq {
		stats.DocFreq[term]++
	}
}

func (b *BM25Indexer) updateStatsRemoveLocked(stats *bm25Stats, termFreq map[string]int, docLen int) {
	if stats.DocCount <= 0 {
		return
	}
	totalDocs := stats.DocCount
	totalLen := stats.AvgDocLen * float64(totalDocs)
	stats.DocCount--
	if stats.DocCount > 0 {
		stats.AvgDocLen = (totalLen - float64(docLen)) / float64(stats.DocCount)
	} else {
		stats.AvgDocLen = 0
	}
	for term := range termFreq {
		if stats.DocFreq[term] > 1 {
			stats.DocFreq[term]--
		} else {
			delete(stats.DocFreq, term)
		}
	}
}

func (b *BM25Indexer) buildDocVectorLocked(stats *bm25Stats, termFreq map[string]int, docLen int) ([]uint32, []float32) {
	if stats.DocCount == 0 || docLen == 0 {
		return nil, nil
	}
	avgDocLen := stats.AvgDocLen
	if avgDocLen <= 0 {
		avgDocLen = 1
	}
	weights := map[uint32]float32{}
	for term, tf := range termFreq {
		df := stats.DocFreq[term]
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (float64(stats.DocCount)-float64(df)+0.5)/(float64(df)+0.5))
		numerator := float64(tf) * (b.k1 + 1)
		denominator := float64(tf) + b.k1*(1-b.b+b.b*float64(docLen)/avgDocLen)
		tfNorm := numerator / denominator
		weight := float32(tfNorm * idf)
		if weight == 0 {
			continue
		}
		index := termHash(term)
		weights[index] += weight
	}
	return sparseWeightsToVector(weights)
}

func (b *BM25Indexer) buildQueryVectorLocked(stats *bm25Stats, termFreq map[string]int) ([]uint32, []float32) {
	if stats.DocCount == 0 {
		return nil, nil
	}
	weights := map[uint32]float32{}
	for term, tf := range termFreq {
		if stats.DocFreq[term] == 0 {
			continue
		}
		weight := float32(tf)
		if weight == 0 {
			continue
		}
		index := termHash(term)
		weights[index] += weight
	}
	return sparseWeightsToVector(weights)
}

func sparseWeightsToVector(weights map[uint32]float32) ([]uint32, []float32) {
	if len(weights) == 0 {
		return nil, nil
	}
	indices := make([]uint32, 0, len(weights))
	for idx := range weights {
		indices = append(indices, idx)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })
	values := make([]float32, 0, len(indices))
	for _, idx := range indices {
		values = append(values, weights[idx])
	}
	return indices, values
}

func termHash(term string) uint32 {
	hasher := fnv.New32a()
	hasher.Write([]byte(term)) //nolint:errcheck // hash.Write never returns error
	return hasher.Sum32() & sparseDimMask
}
