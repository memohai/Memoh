package models_test

import (
	"testing"

	"github.com/memohai/memoh/internal/models"
	"github.com/stretchr/testify/assert"
)

// This is an example test file demonstrating how to use the models service
// Actual tests would require database setup and mocking

func ExampleService_Create() {
	// Example usage - in real code, you would initialize with actual database connection
	// service := models.NewService(queries)

	// ctx := context.Background()
	// req := models.AddRequest{
	// 	ModelID:    "gpt-4",
	// 	Name:       "GPT-4",
	// 	LlmProviderID: "11111111-1111-1111-1111-111111111111",
	// 	Type:       models.ModelTypeChat,
	// }

	// resp, err := service.Create(ctx, req)
	// if err != nil {
	// 	// handle error
	// }
	// fmt.Printf("Created model with ID: %s\n", resp.ID)
}

func ExampleService_GetByModelID() {
	// Example usage
	// service := models.NewService(queries)

	// ctx := context.Background()
	// resp, err := service.GetByModelID(ctx, "gpt-4")
	// if err != nil {
	// 	// handle error
	// }
	// fmt.Printf("Model: %+v\n", resp.Model)
}

func ExampleService_List() {
	// Example usage
	// service := models.NewService(queries)

	// ctx := context.Background()
	// models, err := service.List(ctx)
	// if err != nil {
	// 	// handle error
	// }
	// for _, model := range models {
	// 	fmt.Printf("Model ID: %s, Type: %s\n", model.ModelID, model.Type)
	// }
}

func ExampleService_ListByType() {
	// Example usage
	// service := models.NewService(queries)

	// ctx := context.Background()
	// chatModels, err := service.ListByType(ctx, models.ModelTypeChat)
	// if err != nil {
	// 	// handle error
	// }
	// fmt.Printf("Found %d chat models\n", len(chatModels))
}

func ExampleService_UpdateByModelID() {
	// Example usage
	// service := models.NewService(queries)

	// ctx := context.Background()
	// req := models.UpdateRequest{
	// 	ModelID:    "gpt-4",
	// 	Name:       "GPT-4 Turbo",
	// 	LlmProviderID: "11111111-1111-1111-1111-111111111111",
	// 	Type:       models.ModelTypeChat,
	// }

	// resp, err := service.UpdateByModelID(ctx, "gpt-4", req)
	// if err != nil {
	// 	// handle error
	// }
	// fmt.Printf("Updated model: %s\n", resp.ModelID)
}

func ExampleService_DeleteByModelID() {
	// Example usage
	// service := models.NewService(queries)

	// ctx := context.Background()
	// err := service.DeleteByModelID(ctx, "gpt-4")
	// if err != nil {
	// 	// handle error
	// }
	// fmt.Println("Model deleted successfully")
}

func TestModel_Validate(t *testing.T) {
	tests := []struct {
		name    string
		model   models.Model
		wantErr bool
	}{
		{
			name: "valid chat model",
			model: models.Model{
				ModelID:       "gpt-4",
				Name:          "GPT-4",
				LlmProviderID: "11111111-1111-1111-1111-111111111111",
				Type:          models.ModelTypeChat,
			},
			wantErr: false,
		},
		{
			name: "valid chat model with modalities",
			model: models.Model{
				ModelID:         "gpt-4o",
				Name:            "GPT-4o",
				LlmProviderID:   "11111111-1111-1111-1111-111111111111",
				InputModalities: []string{"text", "image", "audio"},
				Type:            models.ModelTypeChat,
			},
			wantErr: false,
		},
		{
			name: "valid embedding model",
			model: models.Model{
				ModelID:       "text-embedding-ada-002",
				Name:          "Ada Embeddings",
				LlmProviderID: "11111111-1111-1111-1111-111111111111",
				Type:          models.ModelTypeEmbedding,
				Dimensions:    1536,
			},
			wantErr: false,
		},
		{
			name: "missing model_id",
			model: models.Model{
				LlmProviderID: "11111111-1111-1111-1111-111111111111",
				Type:          models.ModelTypeChat,
			},
			wantErr: true,
		},
		{
			name: "missing llm_provider_id",
			model: models.Model{
				ModelID: "gpt-4",
				Type:    models.ModelTypeChat,
			},
			wantErr: true,
		},
		{
			name: "invalid llm_provider_id",
			model: models.Model{
				ModelID:       "gpt-4",
				LlmProviderID: "not-a-uuid",
				Type:          models.ModelTypeChat,
			},
			wantErr: true,
		},
		{
			name: "invalid model type",
			model: models.Model{
				ModelID:       "gpt-4",
				LlmProviderID: "11111111-1111-1111-1111-111111111111",
				Type:          "invalid",
			},
			wantErr: true,
		},
		{
			name: "embedding model missing dimensions",
			model: models.Model{
				ModelID:       "text-embedding-ada-002",
				LlmProviderID: "11111111-1111-1111-1111-111111111111",
				Type:          models.ModelTypeEmbedding,
				Dimensions:    0,
			},
			wantErr: true,
		},
		{
			name: "invalid input modality",
			model: models.Model{
				ModelID:         "gpt-4",
				LlmProviderID:   "11111111-1111-1111-1111-111111111111",
				Type:            models.ModelTypeChat,
				InputModalities: []string{"text", "smell"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.model.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestModel_IsMultimodal(t *testing.T) {
	tests := []struct {
		name     string
		model    models.Model
		expected bool
	}{
		{
			name: "text only",
			model: models.Model{
				InputModalities: []string{"text"},
			},
			expected: false,
		},
		{
			name: "text and image",
			model: models.Model{
				InputModalities: []string{"text", "image"},
			},
			expected: true,
		},
		{
			name: "text image audio video",
			model: models.Model{
				InputModalities: []string{"text", "image", "audio", "video"},
			},
			expected: true,
		},
		{
			name:     "empty modalities",
			model:    models.Model{},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.model.IsMultimodal())
		})
	}
}

func TestModel_HasInputModality(t *testing.T) {
	m := models.Model{
		InputModalities: []string{"text", "image", "audio"},
	}
	assert.True(t, m.HasInputModality("text"))
	assert.True(t, m.HasInputModality("image"))
	assert.True(t, m.HasInputModality("audio"))
	assert.False(t, m.HasInputModality("video"))
	assert.False(t, m.HasInputModality("file"))
}

func TestModelTypes(t *testing.T) {
	t.Run("ModelType constants", func(t *testing.T) {
		assert.Equal(t, models.ModelType("chat"), models.ModelTypeChat)
		assert.Equal(t, models.ModelType("embedding"), models.ModelTypeEmbedding)
	})

	t.Run("ClientType constants", func(t *testing.T) {
		assert.Equal(t, models.ClientType("openai"), models.ClientTypeOpenAI)
		assert.Equal(t, models.ClientType("openai-compat"), models.ClientTypeOpenAICompat)
		assert.Equal(t, models.ClientType("anthropic"), models.ClientTypeAnthropic)
		assert.Equal(t, models.ClientType("google"), models.ClientTypeGoogle)
		assert.Equal(t, models.ClientType("azure"), models.ClientTypeAzure)
		assert.Equal(t, models.ClientType("bedrock"), models.ClientTypeBedrock)
		assert.Equal(t, models.ClientType("mistral"), models.ClientTypeMistral)
		assert.Equal(t, models.ClientType("xai"), models.ClientTypeXAI)
		assert.Equal(t, models.ClientType("ollama"), models.ClientTypeOllama)
		assert.Equal(t, models.ClientType("dashscope"), models.ClientTypeDashscope)
	})
}
