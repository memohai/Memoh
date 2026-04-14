-- name: GetBotNetworkConfig :one
SELECT
  network_enabled,
  network_provider,
  network_config
FROM bots
WHERE id = $1;
