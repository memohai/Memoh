#!/bin/sh
# 检测 TLS 证书，存在则启用 HTTPS + HTTP/2 入口（#822）。
# 以 15- 前缀在官方 20-envsubst-on-templates.sh 之前执行：把可选模板放入
# /etc/nginx/templates/，交给官方 envsubst 脚本统一渲染。
set -e

ME=$(basename "$0")

cert="${MEMOH_TLS_CERT:-/etc/nginx/certs/server.crt}"
key="${MEMOH_TLS_KEY:-/etc/nginx/certs/server.key}"
template="/etc/nginx/templates-available/https.conf.template"

if [ -f "$cert" ] && [ -f "$key" ]; then
    echo "$ME: found TLS certificate, enabling HTTPS + HTTP/2 on port ${MEMOH_WEB_HTTPS_PORT:-8443}"
    cp "$template" /etc/nginx/templates/https.conf.template
else
    echo "$ME: no TLS certificate at $cert / $key, serving plain HTTP only"
fi
