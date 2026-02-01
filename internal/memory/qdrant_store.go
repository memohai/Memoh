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
)

type QdrantStore struct {
	client           *qdrant.Client
	collection       string
	dimension        int
	baseURL          string
	apiKey           string
	timeout          time.Duration
	logger           *slog.Logger
	vectorNames      map[string]int
	usesNamedVectors bool
}

type qdrantPoint struct {
	ID         string                 `json:"id"`
	Vector     []float32              `json:"vector"`
	VectorName string                 `json:"vector_name,omitempty"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
}

func NewQdrantStore(log *slog.Logger, baseURL, apiKey, collection string, dimension int, timeout time.Duration) (*QdrantStore, error) {
	host, port, useTLS, err := parseQdrantEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	if collection == "" {
		collection = "memory"
	}
	if dimension <= 0 {
		dimension = 1536
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
		client:     client,
		collection: collection,
		dimension:  dimension,
		baseURL:    baseURL,
		apiKey:     apiKey,
		timeout:    timeoutOrDefault(timeout),
		logger:     log.With(slog.String("store", "qdrant")),
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutOrDefault(timeout))
	defer cancel()
	if err := store.ensureCollection(ctx, nil); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *QdrantStore) NewSibling(collection string, dimension int) (*QdrantStore, error) {
	return NewQdrantStore(s.logger, s.baseURL, s.apiKey, collection, dimension, s.timeout)
}

func NewQdrantStoreWithVectors(log *slog.Logger, baseURL, apiKey, collection string, vectors map[string]int, timeout time.Duration) (*QdrantStore, error) {
	host, port, useTLS, err := parseQdrantEndpoint(baseURL)
	if err != nil {
		return nil, err
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
		client:           client,
		collection:       collection,
		baseURL:          baseURL,
		apiKey:           apiKey,
		timeout:          timeoutOrDefault(timeout),
		logger:           log.With(slog.String("store", "qdrant")),
		vectorNames:      vectors,
		usesNamedVectors: true,
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
		if point.VectorName != "" && s.usesNamedVectors {
			vectors = qdrant.NewVectorsMap(map[string]*qdrant.Vector{
				point.VectorName: qdrant.NewVectorDense(point.Vector),
			})
		} else {
			vectors = qdrant.NewVectorsDense(point.Vector)
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

func (s *QdrantStore) Search(ctx context.Context, vector []float32, limit int, filters map[string]interface{}, vectorName string) ([]qdrantPoint, []float64, error) {
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

func (s *QdrantStore) SearchBySources(ctx context.Context, vector []float32, limit int, filters map[string]interface{}, sources []string, vectorName string) (map[string][]qdrantPoint, map[string][]float64, error) {
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

func (s *QdrantStore) List(ctx context.Context, limit int, filters map[string]interface{}) ([]qdrantPoint, error) {
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

func (s *QdrantStore) DeleteAll(ctx context.Context, filters map[string]interface{}) error {
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
		return s.refreshCollectionSchema(ctx, vectors)
	}
	if len(vectors) > 0 {
		params := make(map[string]*qdrant.VectorParams, len(vectors))
		for name, dim := range vectors {
			params[name] = &qdrant.VectorParams{
				Size:     uint64(dim),
				Distance: qdrant.Distance_Cosine,
			}
		}
		return s.client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: s.collection,
			VectorsConfig:  qdrant.NewVectorsConfigMap(params),
		})
	}
	return s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: s.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(s.dimension),
			Distance: qdrant.Distance_Cosine,
		}),
	})
}

func (s *QdrantStore) refreshCollectionSchema(ctx context.Context, vectors map[string]int) error {
	info, err := s.client.GetCollectionInfo(ctx, s.collection)
	if err != nil {
		return err
	}
	config := info.GetConfig()
	if config == nil || config.GetParams() == nil || config.GetParams().GetVectorsConfig() == nil {
		return nil
	}
	vectorsConfig := config.GetParams().GetVectorsConfig()
	if vectorsConfig.GetParamsMap() != nil {
		s.usesNamedVectors = true
		s.vectorNames = map[string]int{}
		for name, vec := range vectorsConfig.GetParamsMap().GetMap() {
			if vec != nil {
				s.vectorNames[name] = int(vec.GetSize())
			}
		}
		if len(vectors) == 0 {
			return nil
		}
		for name, dim := range vectors {
			if existing, ok := s.vectorNames[name]; ok && existing == dim {
				continue
			}
			return fmt.Errorf("collection missing vector %s (dim %d); migration required", name, dim)
		}
		return nil
	}
	s.usesNamedVectors = false
	s.vectorNames = nil
	return nil
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

func buildQdrantFilter(filters map[string]interface{}) *qdrant.Filter {
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

func cloneFilters(filters map[string]interface{}) map[string]interface{} {
	if len(filters) == 0 {
		return map[string]interface{}{}
	}
	clone := make(map[string]interface{}, len(filters))
	for key, value := range filters {
		clone[key] = value
	}
	return clone
}

func buildQdrantCondition(key string, value interface{}) *qdrant.Condition {
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
	case map[string]interface{}:
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

func toFloat(value interface{}) (float64, bool) {
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

func valueMapToInterface(values map[string]*qdrant.Value) map[string]interface{} {
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = valueToInterface(value)
	}
	return result
}

func valueToInterface(value *qdrant.Value) interface{} {
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
		items := make([]interface{}, 0, len(kind.ListValue.GetValues()))
		for _, item := range kind.ListValue.GetValues() {
			items = append(items, valueToInterface(item))
		}
		return items
	default:
		return nil
	}
}
