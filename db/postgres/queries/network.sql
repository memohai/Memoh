-- name: GetBotOverlayConfig :one
SELECT
  overlay_enabled,
  overlay_provider,
  overlay_config
FROM bots
WHERE tenant_id = app.current_tenant_id() AND id = $1;
