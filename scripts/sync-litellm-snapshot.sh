#!/usr/bin/env bash
# Refreshes the embedded LiteLLM capability snapshot consumed by
# internal/capabilities. It fetches the upstream LiteLLM registry, projects each
# record down to the handful of fields the matcher actually reads (see
# internal/capabilities/discover.go litellmEntry), sorts keys deterministically,
# and writes the gzipped result to internal/capabilities/litellm_snapshot.json.gz.
#
# The projection keeps explicit `false` values (only `null` fields are dropped)
# because the matcher distinguishes "registry is silent" (absent) from an
# explicit false.
#
# Note on determinism: the uncompressed payload is fully deterministic (jq -S),
# but gzip bytes can differ across gzip implementations (macOS vs GNU). The
# refresh workflow therefore gates on the DECOMPRESSED content, not raw bytes,
# so a re-compression alone never produces a spurious PR.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_FILE="$REPO_ROOT/internal/capabilities/litellm_snapshot.json.gz"
SOURCE_URL="${LITELLM_REGISTRY_URL:-https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json}"

# Anchor models that must always be present in a healthy registry. Picked to be
# stable (non date-pinned) and cross-vendor, so a truncated/garbage fetch fails
# the guard instead of silently shrinking the snapshot.
ANCHORS=(gpt-4o gpt-4o-mini claude-sonnet-4-5 gemini-2.0-flash)
# Fail if the new entry count drops below this percentage of the committed one.
MIN_RATIO_PERCENT=80

command -v jq >/dev/null 2>&1 || { echo "Error: jq is required but not installed" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "Error: curl is required but not installed" >&2; exit 1; }

echo "Fetching LiteLLM registry from $SOURCE_URL ..."
RAW="$(curl -sS --fail-with-body "$SOURCE_URL")"

if ! printf '%s' "$RAW" | jq -e 'type == "object"' >/dev/null 2>&1; then
  echo "Error: fetched payload is not a JSON object" >&2
  exit 1
fi

JQ_FILTER='
del(.sample_spec)
| with_entries(
    select(.value | type == "object")
    | .value |= ({
        mode,
        supports_reasoning,
        supports_adaptive_thinking,
        supports_none_reasoning_effort,
        supports_minimal_reasoning_effort,
        supports_low_reasoning_effort,
        supports_xhigh_reasoning_effort,
        supports_max_reasoning_effort,
        supports_vision,
        supports_function_calling,
        max_input_tokens
      } | with_entries(select(.value != null)))
  )
'

PRUNED="$(printf '%s' "$RAW" | jq -S "$JQ_FILTER")"
NEW_COUNT="$(printf '%s' "$PRUNED" | jq 'length')"
echo "Pruned snapshot has $NEW_COUNT model entries"

MISSING=()
for anchor in "${ANCHORS[@]}"; do
  if ! printf '%s' "$PRUNED" | jq -e --arg k "$anchor" 'has($k)' >/dev/null 2>&1; then
    MISSING+=("$anchor")
  fi
done
if [ "${#MISSING[@]}" -gt 0 ]; then
  echo "Error: anchor models missing from fetched registry: ${MISSING[*]}" >&2
  exit 1
fi

if [ -f "$OUTPUT_FILE" ]; then
  OLD_JSON="$(gzip -dc "$OUTPUT_FILE")"
  OLD_COUNT="$(printf '%s' "$OLD_JSON" | jq 'length')"
  if [ "$OLD_COUNT" -gt 0 ] && [ "$((NEW_COUNT * 100))" -lt "$((OLD_COUNT * MIN_RATIO_PERCENT))" ]; then
    echo "Error: entry count dropped sharply ($OLD_COUNT -> $NEW_COUNT, below ${MIN_RATIO_PERCENT}%)" >&2
    exit 1
  fi

  # Field-coverage guard: a rename/removal of a load-bearing field upstream
  # would pass the entry-count guard (keys unchanged) yet silently null out
  # capabilities after projection. So for each pillar field, the number of
  # entries carrying it must not collapse relative to the committed snapshot.
  for field in supports_reasoning max_input_tokens; do
    new_cov="$(printf '%s' "$PRUNED" | jq --arg f "$field" '[.[] | select(has($f))] | length')"
    old_cov="$(printf '%s' "$OLD_JSON" | jq --arg f "$field" '[.[] | select(has($f))] | length')"
    if [ "$old_cov" -gt 0 ] && [ "$((new_cov * 100))" -lt "$((old_cov * MIN_RATIO_PERCENT))" ]; then
      echo "Error: field '$field' coverage dropped sharply ($old_cov -> $new_cov, below ${MIN_RATIO_PERCENT}%); possible upstream field rename" >&2
      exit 1
    fi
  done
fi

# Write via a temp file + atomic rename so an interrupted run never leaves a
# truncated snapshot behind. -n omits the original name/timestamp from the gzip
# header (best-effort determinism); cross-implementation byte differences are
# tolerated because the workflow gates on decompressed content.
TMP_OUT="$(mktemp)"
trap 'rm -f "$TMP_OUT"' EXIT
printf '%s\n' "$PRUNED" | gzip -9 -n > "$TMP_OUT"
mv "$TMP_OUT" "$OUTPUT_FILE"
echo "Wrote $NEW_COUNT models to $OUTPUT_FILE"
