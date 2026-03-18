---
name: twilight-ai
description: Assist with development in the Twilight AI Go SDK. Use when working in this repository, adding or updating providers, embeddings, tool calling, streaming, examples, or docs for Twilight AI.
---

# Twilight AI

## When To Use

Use this skill when the task involves `twilight-ai`, especially:

- implementing or refactoring SDK APIs in `sdk/`
- adding or updating providers under `provider/`
- working on `GenerateText`, `GenerateTextResult`, `StreamText`, `Embed`, or `EmbedMany`
- adding tool-calling, streaming, reasoning, or embedding support
- writing examples, docs, or usage guidance for this library

## Project Snapshot

Twilight AI is a lightweight Go AI SDK with a provider-agnostic core API.

- Text generation: `sdk.GenerateText`, `sdk.GenerateTextResult`, `sdk.StreamText`
- Embeddings: `sdk.Embed`, `sdk.EmbedMany`
- Tool calling: `sdk.Tool`, `sdk.NewTool[T]`, `WithMaxSteps`, approval flow
- Streaming: typed `StreamPart` events over Go channels
- Current providers:
  - `provider/openai/completions`
  - `provider/openai/responses`
  - `provider/anthropic/messages`
  - `provider/google/generativeai`
  - `provider/openai/embedding`
  - `provider/google/embedding`

## Default Mental Model

Prefer the high-level SDK API first, then drop to provider details only when needed.

- `sdk.Model` binds a chat model to a `sdk.Provider`
- `sdk.EmbeddingModel` binds an embedding model to an `sdk.EmbeddingProvider`
- The client orchestrates tool loops, callbacks, approvals, and streaming lifecycle
- Providers handle backend-specific HTTP, request mapping, response parsing, and SSE translation

## Core API Guidance

Choose the narrowest API that matches the task:

- Need only final text: use `sdk.GenerateText`
- Need usage, finish reason, steps, sources, files, or tool details: use `sdk.GenerateTextResult`
- Need live output: use `sdk.StreamText`
- Need one vector: use `sdk.Embed`
- Need multiple vectors or embedding token usage: use `sdk.EmbedMany`

If the task introduces examples or docs, prefer simple end-to-end snippets that start with:

1. construct provider
2. get model
3. call SDK API
4. handle error

## Provider Selection Rules

- Use `openai/completions` for broad OpenAI-compatible support such as DeepSeek, Groq, Ollama, Azure-style compatible endpoints, and generic `/chat/completions` backends.
- Use `openai/responses` when the task needs OpenAI Responses API features such as first-class reasoning models, reasoning summaries, URL citation annotations, or flat input mapping.
- Use `anthropic/messages` for Claude and Anthropic extended thinking via `WithThinking`.
- Use `google/generativeai` for Gemini chat, tool calling, vision, streaming, and Gemini reasoning.
- Use `openai/embedding` or `google/embedding` for embeddings. Keep embedding-provider work separate from chat-provider work.

## Implementation Rules

### Chat Providers

If adding or changing a chat provider, preserve the `sdk.Provider` contract:

- `Name()`
- `ListModels(ctx)`
- `Test(ctx)`
- `TestModel(ctx, modelID)`
- `DoGenerate(ctx, params)`
- `DoStream(ctx, params)`

Keep provider responsibilities focused:

- translate SDK messages/options into backend request format
- parse backend responses into `sdk.GenerateResult`
- map backend streaming events into typed `sdk.StreamPart` values
- report usage, finish reasons, reasoning, tool calls, sources, and files when supported

### Embedding Providers

Embedding providers are separate from chat providers. Use `sdk.EmbeddingProvider` and return an `sdk.EmbeddingModel` via `EmbeddingModel(id)`.

When updating embeddings:

- keep `sdk.Embed` for single-string convenience
- keep `sdk.EmbedMany` for batched requests
- preserve `Usage.Tokens`
- only expose dimensions/task-type behavior when the backend supports it

### Tool Calling

Prefer `sdk.NewTool[T]` for new tool examples and integrations. It gives typed input and inferred JSON Schema.

Use these defaults unless the task requires something else:

- `WithToolChoice("auto")` for normal use
- `WithMaxSteps(0)` for inspection-only tool calls
- `WithMaxSteps(N)` for automatic execution loops
- `RequireApproval: true` only for sensitive side effects

When streaming with tools, ensure the implementation can emit:

- tool input construction parts
- tool execution parts
- progress updates
- denial/error events when applicable

### Streaming

Twilight AI streaming is channel-first and type-safe. Prefer type switches over loosely typed event parsing.

Important expectations:

- `StreamText` returns `*sdk.StreamResult`
- `sr.Stream` must be consumed before relying on `sr.Steps` or `sr.Messages`
- `Text()` and `ToResult()` are the convenience paths when callers do not want manual event handling

### Messages And Results

Preserve the SDK message model and avoid backend-specific shapes leaking into public usage.

- user, assistant, system, and tool messages should stay in SDK types
- support rich parts where relevant: text, image, file, reasoning, tool call, tool result
- keep finish reason mapping aligned with SDK constants such as `stop`, `length`, `content-filter`, and `tool-calls`

## Common Task Patterns

### Add A New Usage Example

Use this structure:

1. pick the correct provider package
2. create provider with explicit options
3. create model via `ChatModel` or `EmbeddingModel`
4. call the top-level `sdk` function
5. show minimal but idiomatic result handling

### Add Or Update A Provider Feature

Check all affected layers:

1. request mapping
2. non-streaming response mapping
3. streaming event mapping
4. finish-reason and usage mapping
5. reasoning/tool/source/file support if the backend exposes them
6. model discovery and provider health checks if endpoints exist

### Add A Custom Provider

Use the built-in providers as the template. A custom provider should feel identical to existing ones from the caller's perspective.

Minimum behavior:

1. return a provider-bound model from `ChatModel`
2. implement discovery and health-check methods
3. support `DoGenerate`
4. support `DoStream` with correct lifecycle parts

## Documentation Rules

When writing Twilight AI docs or README content:

- prefer provider-agnostic phrasing first, provider-specific details second
- use Go examples, not pseudocode, unless explaining an interface contract
- keep examples small and runnable in spirit
- mention exact package paths for imports
- explain when to choose Completions vs Responses when OpenAI is involved
- keep embeddings, tool calling, and streaming as separate concerns unless the example truly combines them

## Terminology

Use these terms consistently:

- Provider: backend implementation for chat generation
- Embedding provider: backend implementation for embeddings
- Model: provider-bound chat model
- Embedding model: provider-bound embedding model
- Tool calling: model requests a tool invocation
- Multi-step execution: automatic tool loop controlled by `WithMaxSteps`
- Stream part: a typed event from `StreamText`

## Quick Checklist

Before finishing work in this repo, verify:

- the chosen provider package matches the intended backend capabilities
- chat and embedding concerns are not mixed accidentally
- public examples use top-level `sdk` APIs unless lower-level behavior is the point
- streaming logic uses typed `StreamPart` handling
- tool-calling changes cover both inspection mode and multi-step mode when relevant
- provider work includes health checks or model discovery behavior if the backend supports them

## Additional Resources

- For exported APIs, signatures, provider options, and stream/event types, see [reference.md](reference.md)
