package memory

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	sparseHashVectorName  = "sparse_hash"
	sparseVocabVectorName = "sparse_vocab"
)

type QdrantStore struct {
	client            *qdrant.Client
	collection        string
	dimension         int
	baseURL           string
	apiKey            string
	timeout           time.Duration
	logger            *slog.Logger
	vectorNames       map[string]int
	usesNamedVectors  bool
	sparseVectorName  string
	usesSparseVectors bool
}

type qdrantPoint struct {
	ID               string         `json:"id"`
	Vector           []float32      `json:"vector"`
	VectorName       string         `json:"vector_name,omitempty"`
	SparseIndices    []uint32       `json:"sparse_indices,omitempty"`
	SparseValues     []float32      `json:"sparse_values,omitempty"`
	SparseVectorName string         `json:"sparse_vector_name,omitempty"`
	Payload          map[string]any `json:"payload,omitempty"`
}

func NewQdrantStore(log *slog.Logger, baseURL, apiKey, collection string, dimension int, sparseVectorName string, timeout time.Duration) (*QdrantStore, error) {
	host, port, useTLS, err := parseQdrantEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sparseVectorName) == "" {
		sparseVectorName = sparseHashVectorName
	}
	if collection == "" {
		collection = "memory"
	}
	if dimension <= 0 && strings.TrimSpace(sparseVectorName) == "" {
		return nil, fmt.Errorf("embedding dimension is required")
	}

	cfg := &qdrant.Config{
		Host:   host,
		Port:   port,
		APIKey: apiKey,
		UseTLS: useTLS,
	}
	client, err := qdrant.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	store := &QdrantStore{
		client:            client,
		collection:        collection,
		dimension:         dimension,
		baseURL:           baseURL,
		apiKey:            apiKey,
		timeout:           timeoutOrDefault(timeout),
		logger:            log.With(slog.String("store", "qdrant")),
		sparseVectorName:  strings.TrimSpace(sparseVectorName),
		usesSparseVectors: strings.TrimSpace(sparseVectorName) != "",
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutOrDefault(timeout))
	defer cancel()
	if err := store.ensureCollection(ctx, nil); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *QdrantStore) NewSibling(collection string, dimension int) (*QdrantStore, error) {
	return NewQdrantStore(s.logger, s.baseURL, s.apiKey, collection, dimension, s.sparseVectorName, s.timeout)
}

func NewQdrantStoreWithVectors(log *slog.Logger, baseURL, apiKey, collection string, vectors map[string]int, sparseVectorName string, timeout time.Duration) (*QdrantStore, error) {
	host, port, useTLS, err := parseQdrantEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sparseVectorName) == "" {
		sparseVectorName = sparseHashVectorName
	}
	if collection == "" {
		collection = "memory"
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("vectors map is required")
	}

	cfg := &qdrant.Config{
		Host:   host,
		Port:   port,
		APIKey: apiKey,
		UseTLS: useTLS,
	}
	client, err := qdrant.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	store := &QdrantStore{
		client:            client,
		collection:        collection,
		baseURL:           baseURL,
		apiKey:            apiKey,
		timeout:           timeoutOrDefault(timeout),
		logger:            log.With(slog.String("store", "qdrant")),
		vectorNames:       vectors,
		usesNamedVectors:  true,
		sparseVectorName:  strings.TrimSpace(sparseVectorName),
		usesSparseVectors: strings.TrimSpace(sparseVectorName) != "",
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutOrDefault(timeout))
	defer cancel()
	if err := store.ensureCollection(ctx, vectors); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *QdrantStore) Upsert(ctx context.Context, points []qdrantPoint) error {
	if len(points) == 0 {
		return nil
	}
	qPoints := make([]*qdrant.PointStruct, 0, len(points))
	for _, point := range points {
		payload, err := qdrant.TryValueMap(point.Payload)
		if err != nil {
			return err
		}
		var vectors *qdrant.Vectors
		vectorMap := map[string]*qdrant.Vector{}
		if len(point.Vector) > 0 {
			if point.VectorName != "" && s.usesNamedVectors {
				vectorMap[point.VectorName] = qdrant.NewVectorDense(point.Vector)
			} else if !s.usesNamedVectors && len(point.SparseIndices) == 0 {
				vectors = qdrant.NewVectorsDense(point.Vector)
			} else if point.VectorName != "" {
				vectorMap[point.VectorName] = qdrant.NewVectorDense(point.Vector)
			}
		}
		if len(point.SparseIndices) > 0 && len(point.SparseValues) > 0 {
			sparseName := strings.TrimSpace(point.SparseVectorName)
			if sparseName == "" {
				sparseName = s.sparseVectorName
			}
			if sparseName == "" {
				return fmt.Errorf("sparse vector name is required")
			}
			vectorMap[sparseName] = qdrant.NewVectorSparse(point.SparseIndices, point.SparseValues)
		}
		if vectors == nil {
			if len(vectorMap) == 0 {
				return fmt.Errorf("no vector data provided for point %s", point.ID)
			}
			vectors = qdrant.NewVectorsMap(vectorMap)
		}
		qPoints = append(qPoints, &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(point.ID),
			Vectors: vectors,
			Payload: payload,
		})
	}
	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.collection,
		Wait:           qdrant.PtrOf(true),
		Points:         qPoints,
	})
	return err
}

func (s *QdrantStore) Search(ctx context.Context, vector []float32, limit int, filters map[string]any, vectorName string) ([]qdrantPoint, []float64, error) {
	if limit <= 0 {
		limit = 10
	}
	filter := buildQdrantFilter(filters)
	var using *string
	if vectorName != "" && s.usesNamedVectors {
		using = qdrant.PtrOf(vectorName)
	}
	results, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: s.collection,
		Query:          qdrant.NewQueryDense(vector),
		Using:          using,
		Limit:          qdrant.PtrOf(uint64(limit)),
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, nil, err
	}

	points := make([]qdrantPoint, 0, len(results))
	scores := make([]float64, 0, len(results))
	for _, scored := range results {
		points = append(points, qdrantPoint{
			ID:      pointIDToString(scored.GetId()),
			Payload: valueMapToInterface(scored.GetPayload()),
		})
		scores = append(scores, float64(scored.GetScore()))
	}
	return points, scores, nil
}

func (s *QdrantStore) SearchSparse(ctx context.Context, indices []uint32, values []float32, limit int, filters map[string]any) ([]qdrantPoint, []float64, error) {
	if limit <= 0 {
		limit = 10
	}
	if len(indices) == 0 || len(values) == 0 {
		return nil, nil, nil
	}
	if s.sparseVectorName == "" {
		return nil, nil, fmt.Errorf("sparse vector name not configured")
	}
	filter := buildQdrantFilter(filters)
	using := qdrant.PtrOf(s.sparseVectorName)
	results, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: s.collection,
		Query:          qdrant.NewQuerySparse(indices, values),
		Using:          using,
		Limit:          qdrant.PtrOf(uint64(limit)),
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, nil, err
	}
	points := make([]qdrantPoint, 0, len(results))
	scores := make([]float64, 0, len(results))
	for _, scored := range results {
		points = append(points, qdrantPoint{
			ID:      pointIDToString(scored.GetId()),
			Payload: valueMapToInterface(scored.GetPayload()),
		})
		scores = append(scores, float64(scored.GetScore()))
	}
	return points, scores, nil
}

func (s *QdrantStore) SearchBySources(ctx context.Context, vector []float32, limit int, filters map[string]any, sources []string, vectorName string) (map[string][]qdrantPoint, map[string][]float64, error) {
	pointsBySource := make(map[string][]qdrantPoint, len(sources))
	scoresBySource := make(map[string][]float64, len(sources))
	if len(sources) == 0 {
		return pointsBySource, scoresBySource, nil
	}
	for _, source := range sources {
		merged := cloneFilters(filters)
		if source != "" {
			merged["source"] = source
		}
		points, scores, err := s.Search(ctx, vector, limit, merged, vectorName)
		if err != nil {
			return nil, nil, err
		}
		pointsBySource[source] = points
		scoresBySource[source] = scores
	}
	return pointsBySource, scoresBySource, nil
}

func (s *QdrantStore) SearchSparseBySources(ctx context.Context, indices []uint32, values []float32, limit int, filters map[string]any, sources []string) (map[string][]qdrantPoint, map[string][]float64, error) {
	pointsBySource := make(map[string][]qdrantPoint, len(sources))
	scoresBySource := make(map[string][]float64, len(sources))
	if len(sources) == 0 {
		return pointsBySource, scoresBySource, nil
	}
	for _, source := range sources {
		merged := cloneFilters(filters)
		if source != "" {
			merged["source"] = source
		}
		points, scores, err := s.SearchSparse(ctx, indices, values, limit, merged)
		if err != nil {
			return nil, nil, err
		}
		pointsBySource[source] = points
		scoresBySource[source] = scores
	}
	return pointsBySource, scoresBySource, nil
}

func (s *QdrantStore) Get(ctx context.Context, id string) (*qdrantPoint, error) {
	result, err := s.client.Get(ctx, &qdrant.GetPoints{
		CollectionName: s.collection,
		Ids:            []*qdrant.PointId{qdrant.NewIDUUID(id)},
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}
	point := result[0]
	return &qdrantPoint{
		ID:      pointIDToString(point.GetId()),
		Payload: valueMapToInterface(point.GetPayload()),
	}, nil
}

func (s *QdrantStore) Delete(ctx context.Context, id string) error {
	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collection,
		Wait:           qdrant.PtrOf(true),
		Points:         qdrant.NewPointsSelectorIDs([]*qdrant.PointId{qdrant.NewIDUUID(id)}),
	})
	return err
}

func (s *QdrantStore) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	pointIDs := make([]*qdrant.PointId, 0, len(ids))
	for _, id := range ids {
		pointIDs = append(pointIDs, qdrant.NewIDUUID(id))
	}
	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collection,
		Wait:           qdrant.PtrOf(true),
		Points:         qdrant.NewPointsSelectorIDs(pointIDs),
	})
	return err
}

func (s *QdrantStore) List(ctx context.Context, limit int, filters map[string]any) ([]qdrantPoint, error) {
	if limit <= 0 {
		limit = 100
	}
	filter := buildQdrantFilter(filters)
	points, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collection,
		Limit:          qdrant.PtrOf(uint32(limit)),
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	result := make([]qdrantPoint, 0, len(points))
	for _, point := range points {
		result = append(result, qdrantPoint{
			ID:      pointIDToString(point.GetId()),
			Payload: valueMapToInterface(point.GetPayload()),
		})
	}
	return result, nil
}

func (s *QdrantStore) Scroll(ctx context.Context, limit int, filters map[string]any, offset *qdrant.PointId) ([]qdrantPoint, *qdrant.PointId, error) {
	if limit <= 0 {
		limit = 100
	}
	filter := buildQdrantFilter(filters)
	points, nextOffset, err := s.client.ScrollAndOffset(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collection,
		Limit:          qdrant.PtrOf(uint32(limit)),
		Filter:         filter,
		Offset:         offset,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, nil, err
	}
	result := make([]qdrantPoint, 0, len(points))
	for _, point := range points {
		result = append(result, qdrantPoint{
			ID:      pointIDToString(point.GetId()),
			Payload: valueMapToInterface(point.GetPayload()),
		})
	}
	return result, nextOffset, nil
}

func (s *QdrantStore) Count(ctx context.Context, filters map[string]any) (uint64, error) {
	filter := buildQdrantFilter(filters)
	result, err := s.client.Count(ctx, &qdrant.CountPoints{
		CollectionName: s.collection,
		Filter:         filter,
		Exact:          qdrant.PtrOf(true),
	})
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (s *QdrantStore) DeleteAll(ctx context.Context, filters map[string]any) error {
	filter := buildQdrantFilter(filters)
	if filter == nil {
		return fmt.Errorf("delete all requires filters")
	}
	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collection,
		Wait:           qdrant.PtrOf(true),
		Points:         qdrant.NewPointsSelectorFilter(filter),
	})
	return err
}

func (s *QdrantStore) ensureCollection(ctx context.Context, vectors map[string]int) error {
	exists, err := s.client.CollectionExists(ctx, s.collection)
	if err != nil {
		return err
	}
	if exists {
		if err := s.refreshCollectionSchema(ctx, vectors); err != nil {
			return err
		}
		return s.ensurePayloadIndexes(ctx)
	}
	var vectorsConfig *qdrant.VectorsConfig
	if len(vectors) > 0 {
		params := make(map[string]*qdrant.VectorParams, len(vectors))
		for name, dim := range vectors {
			params[name] = &qdrant.VectorParams{
				Size:     uint64(dim),
				Distance: qdrant.Distance_Cosine,
			}
		}
		vectorsConfig = qdrant.NewVectorsConfigMap(params)
	} else if s.dimension > 0 {
		vectorsConfig = qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(s.dimension),
			Distance: qdrant.Distance_Cosine,
		})
	}
	var sparseConfig *qdrant.SparseVectorConfig
	if s.sparseVectorName != "" {
		sparseConfig = qdrant.NewSparseVectorsConfig(map[string]*qdrant.SparseVectorParams{
			s.sparseVectorName:    {Modifier: qdrant.PtrOf(qdrant.Modifier_None)},
			sparseVocabVectorName: {Modifier: qdrant.PtrOf(qdrant.Modifier_None)},
		})
	}
	if err := s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName:      s.collection,
		VectorsConfig:       vectorsConfig,
		SparseVectorsConfig: sparseConfig,
	}); err != nil {
		return err
	}
	return s.ensurePayloadIndexes(ctx)
}

func (s *QdrantStore) refreshCollectionSchema(ctx context.Context, vectors map[string]int) error {
	info, err := s.client.GetCollectionInfo(ctx, s.collection)
	if err != nil {
		return err
	}
	config := info.GetConfig()
	if config == nil || config.GetParams() == nil {
		return nil
	}
	params := config.GetParams()
	vectorsConfig := params.GetVectorsConfig()
	if vectorsConfig != nil && vectorsConfig.GetParamsMap() != nil {
		s.usesNamedVectors = true
		s.vectorNames = map[string]int{}
		for name, vec := range vectorsConfig.GetParamsMap().GetMap() {
			if vec != nil {
				s.vectorNames[name] = int(vec.GetSize())
			}
		}
		if len(vectors) > 0 {
			for name, dim := range vectors {
				if existing, ok := s.vectorNames[name]; ok && existing == dim {
					continue
				}
				return fmt.Errorf("collection missing vector %s (dim %d); migration required", name, dim)
			}
		}
	} else {
		s.usesNamedVectors = false
		s.vectorNames = nil
	}

	sparseConfig := params.GetSparseVectorsConfig()
	if s.sparseVectorName != "" {
		needsUpdate := false
		if sparseConfig == nil || len(sparseConfig.GetMap()) == 0 {
			needsUpdate = true
		} else {
			if _, ok := sparseConfig.GetMap()[s.sparseVectorName]; !ok {
				needsUpdate = true
			}
			if _, ok := sparseConfig.GetMap()[sparseVocabVectorName]; !ok {
				needsUpdate = true
			}
		}
		if needsUpdate {
			if err := s.ensureSparseVectors(ctx); err != nil {
				return err
			}
		}
		s.usesSparseVectors = true
		return nil
	}
	if sparseConfig != nil && len(sparseConfig.GetMap()) > 0 {
		s.usesSparseVectors = true
		for name := range sparseConfig.GetMap() {
			s.sparseVectorName = name
			break
		}
	}
	return nil
}

func (s *QdrantStore) ensurePayloadIndexes(ctx context.Context) error {
	if s.client == nil {
		return nil
	}
	fields := []string{"bot_id", "run_id"}
	wait := true
	for _, field := range fields {
		_, err := s.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
			CollectionName: s.collection,
			FieldName:      field,
			FieldType:      qdrant.FieldType_FieldTypeKeyword.Enum(),
			Wait:           &wait,
		})
		if err == nil {
			continue
		}
		if status.Code(err) == codes.AlreadyExists {
			continue
		}
		// Fall back to string match when the backend wraps the error.
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			continue
		}
		return err
	}
	return nil
}

func (s *QdrantStore) ensureSparseVectors(ctx context.Context) error {
	if s.sparseVectorName == "" {
		return nil
	}
	err := s.client.UpdateCollection(ctx, &qdrant.UpdateCollection{
		CollectionName: s.collection,
		SparseVectorsConfig: qdrant.NewSparseVectorsConfig(map[string]*qdrant.SparseVectorParams{
			s.sparseVectorName:    {Modifier: qdrant.PtrOf(qdrant.Modifier_None)},
			sparseVocabVectorName: {Modifier: qdrant.PtrOf(qdrant.Modifier_None)},
		}),
	})
	return err
}

func parseQdrantEndpoint(endpoint string) (string, int, bool, error) {
	if endpoint == "" {
		return "127.0.0.1", 6334, false, nil
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", 0, false, err
	}
	host := parsed.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6334
	if parsed.Port() != "" {
		parsedPort, err := strconv.Atoi(parsed.Port())
		if err != nil {
			return "", 0, false, err
		}
		port = parsedPort
	}
	useTLS := parsed.Scheme == "https"
	return host, port, useTLS, nil
}

func timeoutOrDefault(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 10 * time.Second
	}
	return timeout
}

func buildQdrantFilter(filters map[string]any) *qdrant.Filter {
	if len(filters) == 0 {
		return nil
	}
	conditions := make([]*qdrant.Condition, 0, len(filters))
	for key, value := range filters {
		if condition := buildQdrantCondition(key, value); condition != nil {
			conditions = append(conditions, condition)
		}
	}
	if len(conditions) == 0 {
		return nil
	}
	return &qdrant.Filter{
		Must: conditions,
	}
}

func cloneFilters(filters map[string]any) map[string]any {
	if len(filters) == 0 {
		return map[string]any{}
	}
	clone := make(map[string]any, len(filters))
	for key, value := range filters {
		clone[key] = value
	}
	return clone
}

func buildQdrantCondition(key string, value any) *qdrant.Condition {
	switch typed := value.(type) {
	case string:
		return qdrant.NewMatch(key, typed)
	case bool:
		return qdrant.NewMatchBool(key, typed)
	case int:
		return qdrant.NewMatchInt(key, int64(typed))
	case int64:
		return qdrant.NewMatchInt(key, typed)
	case float32:
		v := float64(typed)
		return qdrant.NewRange(key, &qdrant.Range{Gte: &v, Lte: &v})
	case float64:
		return qdrant.NewRange(key, &qdrant.Range{Gte: &typed, Lte: &typed})
	case map[string]any:
		rangeValue := &qdrant.Range{}
		for _, op := range []string{"gte", "gt", "lte", "lt"} {
			if raw, ok := typed[op]; ok {
				val, ok := toFloat(raw)
				if !ok {
					continue
				}
				switch op {
				case "gte":
					rangeValue.Gte = &val
				case "gt":
					rangeValue.Gt = &val
				case "lte":
					rangeValue.Lte = &val
				case "lt":
					rangeValue.Lt = &val
				}
			}
		}
		if rangeValue.Gte != nil || rangeValue.Gt != nil || rangeValue.Lte != nil || rangeValue.Lt != nil {
			return qdrant.NewRange(key, rangeValue)
		}
	}
	return qdrant.NewMatch(key, fmt.Sprint(value))
}

func toFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func pointIDToString(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	if uuid := id.GetUuid(); uuid != "" {
		return uuid
	}
	if num := id.GetNum(); num != 0 {
		return fmt.Sprintf("%d", num)
	}
	return ""
}

func valueMapToInterface(values map[string]*qdrant.Value) map[string]any {
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = valueToInterface(value)
	}
	return result
}

func valueToInterface(value *qdrant.Value) any {
	if value == nil {
		return nil
	}
	switch kind := value.GetKind().(type) {
	case *qdrant.Value_NullValue:
		return nil
	case *qdrant.Value_BoolValue:
		return kind.BoolValue
	case *qdrant.Value_IntegerValue:
		return kind.IntegerValue
	case *qdrant.Value_DoubleValue:
		return kind.DoubleValue
	case *qdrant.Value_StringValue:
		return kind.StringValue
	case *qdrant.Value_StructValue:
		return valueMapToInterface(kind.StructValue.GetFields())
	case *qdrant.Value_ListValue:
		items := make([]any, 0, len(kind.ListValue.GetValues()))
		for _, item := range kind.ListValue.GetValues() {
			items = append(items, valueToInterface(item))
		}
		return items
	default:
		return nil
	}
}
