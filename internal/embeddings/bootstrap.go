package embeddings

import (
	"context"

	"github.com/memohai/memoh/internal/models"
)

// ResolverTextEmbedder adapts Resolver to the Embedder interface for text embeddings.
type ResolverTextEmbedder struct {
	Resolver *Resolver
	ModelID  string
	Dims     int
}

func (e *ResolverTextEmbedder) Embed(ctx context.Context, input string) ([]float32, error) {
	result, err := e.Resolver.Embed(ctx, Request{
		Type:  TypeText,
		Model: e.ModelID,
		Input: Input{Text: input},
	})
	if err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

func (e *ResolverTextEmbedder) Dimensions() int {
	return e.Dims
}

// CollectEmbeddingVectors gathers embedding model dimensions and defaults.
func CollectEmbeddingVectors(ctx context.Context, service *models.Service) (map[string]int, models.GetResponse, models.GetResponse, bool, error) {
	candidates, err := service.ListByType(ctx, models.ModelTypeEmbedding)
	if err != nil {
		return nil, models.GetResponse{}, models.GetResponse{}, false, err
	}
	vectors := map[string]int{}
	var textModel models.GetResponse
	var multimodalModel models.GetResponse
	for _, model := range candidates {
		if model.Dimensions > 0 && model.ModelID != "" {
			vectors[model.ModelID] = model.Dimensions
		}
		if model.IsMultimodal() {
			if multimodalModel.ModelID == "" {
				multimodalModel = model
			}
			continue
		}
		if textModel.ModelID == "" {
			textModel = model
		}
	}

	hasTextModel := textModel.ModelID != ""
	hasMultimodalModel := multimodalModel.ModelID != ""
	hasAnyModel := hasTextModel || hasMultimodalModel

	return vectors, textModel, multimodalModel, hasAnyModel, nil
}
