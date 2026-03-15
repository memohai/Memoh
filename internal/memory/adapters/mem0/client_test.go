package mem0

import "testing"

func TestNewMem0ClientDefaultsToSaaS(t *testing.T) {
	t.Parallel()

	client, err := newMem0Client(map[string]any{
		"api_key": "test-key",
	})
	if err != nil {
		t.Fatalf("newMem0Client() error = %v", err)
	}
	if client.baseURL != mem0DefaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, mem0DefaultBaseURL)
	}
}

func TestParseMem0AddMemoriesSupportsEventResponses(t *testing.T) {
	t.Parallel()

	body := []byte(`[
		{
			"id": "mem_123",
			"event": "ADD",
			"data": {
				"memory": "The user likes oolong tea."
			}
		}
	]`)

	memories, err := parseMem0AddMemories(body)
	if err != nil {
		t.Fatalf("parseMem0AddMemories() error = %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("len(memories) = %d, want 1", len(memories))
	}
	if memories[0].ID != "mem_123" {
		t.Fatalf("memory id = %q, want %q", memories[0].ID, "mem_123")
	}
	if memories[0].Memory != "The user likes oolong tea." {
		t.Fatalf("memory text = %q", memories[0].Memory)
	}
}

func TestParseMem0MemoriesSupportsEnvelopeResponses(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"results": [
			{
				"id": "mem_456",
				"memory": "The user lives in Shanghai.",
				"score": 0.92,
				"agent_id": "bot-1",
				"run_id": "run-1",
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z"
			}
		],
		"total": 1
	}`)

	memories, err := parseMem0Memories(body)
	if err != nil {
		t.Fatalf("parseMem0Memories() error = %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("len(memories) = %d, want 1", len(memories))
	}
	if memories[0].Score != 0.92 {
		t.Fatalf("score = %v, want 0.92", memories[0].Score)
	}
	if memories[0].AgentID != "bot-1" {
		t.Fatalf("agent_id = %q, want %q", memories[0].AgentID, "bot-1")
	}
}
