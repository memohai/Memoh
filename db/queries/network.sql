-- name: GetBotOverlayConfig :one
SELECT
  overlay_enabled,
  overlay_provider,
  overlay_config
FROM bots
WHERE id = $1;
