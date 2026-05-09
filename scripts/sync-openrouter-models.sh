#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_FILE="$REPO_ROOT/conf/providers/openrouter.yaml"

command -v jq >/dev/null 2>&1 || { echo "Error: jq is required but not installed" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "Error: curl is required but not installed" >&2; exit 1; }

echo "Fetching models from OpenRouter API..."

curl_args=(-sS --fail-with-body)
if [ -n "${OPENROUTER_API_KEY:-}" ]; then
  curl_args+=(-H "Authorization: Bearer $OPENROUTER_API_KEY")
fi
curl_args+=("https://openrouter.ai/api/v1/models")

RESPONSE=$(curl "${curl_args[@]}")

if [ -z "$RESPONSE" ] || ! echo "$RESPONSE" | jq -e '.data' >/dev/null 2>&1; then
  echo "Error: failed to fetch models or unexpected response format" >&2
  echo "$RESPONSE" >&2
  exit 1
fi

TOTAL=$(echo "$RESPONSE" | jq '.data | length')
echo "Fetched $TOTAL models total from API"

# jq filter: keep text-output chat models, derive compatibilities, emit YAML lines
JQ_FILTER='
[.data[]
  | select(
      .context_length != null
      and .context_length > 0
      and (.architecture.output_modalities // [] | index("text"))
      and ((.architecture.output_modalities // [] | index("embeddings")) | not)
    )
  | {
      id,
      name,
      context_length,
      compats: (
        [
          (if (.architecture.input_modalities // [] | index("image")) then "vision" else empty end),
          (if (.supported_parameters // [] | index("tools"))          then "tool-call" else empty end),
          (if (.architecture.output_modalities // [] | index("image")) then "image-output" else empty end),
          (if ((.supported_parameters // [] | index("reasoning")) or (.supported_parameters // [] | index("include_reasoning"))) then "reasoning" else empty end)
        ]
      )
    }
]
| sort_by(.id)
| .[]
| "  - model_id: " + (.id | @json) + "\n" +
  "    name: " + (.name | @json) + "\n" +
  "    type: chat\n" +
  "    config:\n" +
  (if (.compats | length) > 0
   then "      compatibilities: [" + (.compats | join(", ")) + "]\n"
   else ""
   end) +
  "      context_window: " + (.context_length | tostring)
'

MODELS_YAML=$(echo "$RESPONSE" | jq -r "$JQ_FILTER")
MODEL_COUNT=$(echo "$MODELS_YAML" | grep -c '^ *- model_id:' || true)

cat > "$OUTPUT_FILE" <<HEADER
name: OpenRouter
client_type: openai-completions
icon: openrouter
base_url: https://openrouter.ai/api/v1

models:
  - model_id: openrouter/auto
    name: Auto (best for prompt)
    type: chat
    config:
      context_window: 2000000

HEADER

echo "$MODELS_YAML" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

echo "Wrote $MODEL_COUNT models (+ openrouter/auto) to $OUTPUT_FILE"
