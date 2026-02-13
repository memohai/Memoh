#!/bin/sh
set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

REPO="https://github.com/memohai/Memoh.git"
BRANCH="main"
DIR="Memoh"
SILENT=false

# Parse flags
for arg in "$@"; do
  case "$arg" in
    -y|--yes) SILENT=true ;;
  esac
done

# Auto-silent if no TTY available
if [ "$SILENT" = false ] && ! [ -e /dev/tty ]; then
  SILENT=true
fi

echo "${GREEN}========================================${NC}"
echo "${GREEN}   Memoh One-Click Install${NC}"
echo "${GREEN}========================================${NC}"
echo ""

# Check Docker
if ! command -v docker >/dev/null 2>&1; then
    echo "${RED}Error: Docker is not installed${NC}"
    echo "Install Docker first: https://docs.docker.com/get-docker/"
    exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
    echo "${RED}Error: Docker Compose v2 is required${NC}"
    echo "Install: https://docs.docker.com/compose/install/"
    exit 1
fi
echo "${GREEN}✓ Docker and Docker Compose detected${NC}"
echo ""

# Generate random JWT secret
gen_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32
  else
    head -c 32 /dev/urandom | base64 | tr -d '\n'
  fi
}

# Configuration defaults (expand ~ for paths)
WORKSPACE_DEFAULT="${HOME:-/tmp}/memoh"
MEMOH_DATA_DIR_DEFAULT="${HOME:-/tmp}/memoh/data"
ADMIN_USER="admin"
ADMIN_PASS="admin123"
JWT_SECRET="$(gen_secret)"
PG_PASS="memoh123"
WORKSPACE="$WORKSPACE_DEFAULT"
MEMOH_DATA_DIR="$MEMOH_DATA_DIR_DEFAULT"

if [ "$SILENT" = false ]; then
  echo "Configure Memoh (press Enter to use defaults):" > /dev/tty
  echo "" > /dev/tty

  printf "  Workspace (install and clone here) [%s]: " "~/memoh" > /dev/tty
  read -r input < /dev/tty || true
  if [ -n "$input" ]; then
    case "$input" in
      ~) WORKSPACE="${HOME:-/tmp}" ;;
      ~/*) WORKSPACE="${HOME:-/tmp}${input#\~}" ;;
      *) WORKSPACE="$input" ;;
    esac
  fi

  printf "  Data directory (bind mount for containerd/memoh data) [%s]: " "$WORKSPACE/data" > /dev/tty
  read -r input < /dev/tty || true
  if [ -n "$input" ]; then
    case "$input" in
      ~) MEMOH_DATA_DIR="${HOME:-/tmp}" ;;
      ~/*) MEMOH_DATA_DIR="${HOME:-/tmp}${input#\~}" ;;
      *) MEMOH_DATA_DIR="$input" ;;
    esac
  else
    MEMOH_DATA_DIR="$WORKSPACE/data"
  fi

  printf "  Admin username [%s]: " "$ADMIN_USER" > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && ADMIN_USER="$input"

  printf "  Admin password [%s]: " "$ADMIN_PASS" > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && ADMIN_PASS="$input"

  printf "  JWT secret [auto-generated]: " > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && JWT_SECRET="$input"

  printf "  Postgres password [%s]: " "$PG_PASS" > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && PG_PASS="$input"

  echo "" > /dev/tty
fi

# Enter workspace (all operations run here)
mkdir -p "$WORKSPACE"
cd "$WORKSPACE"

# Clone or update
if [ -d "$DIR" ]; then
    echo "Updating existing installation in $WORKSPACE..."
    cd "$DIR"
    git pull --ff-only 2>/dev/null || true
else
    echo "Cloning Memoh into $WORKSPACE..."
    git clone --depth 1 -b "$BRANCH" "$REPO" "$DIR"
    cd "$DIR"
fi

# Generate config.toml from template
cp docker/config/config.docker.toml config.toml
sed -i.bak "s|username = \"admin\"|username = \"${ADMIN_USER}\"|" config.toml
sed -i.bak "s|password = \"admin123\"|password = \"${ADMIN_PASS}\"|" config.toml
sed -i.bak "s|jwt_secret = \".*\"|jwt_secret = \"${JWT_SECRET}\"|" config.toml
sed -i.bak "s|password = \"memoh123\"|password = \"${PG_PASS}\"|" config.toml
export POSTGRES_PASSWORD="${PG_PASS}"
rm -f config.toml.bak

# Use generated config and data dir
INSTALL_DIR="$(pwd)"
export MEMOH_CONFIG=./config.toml
export MEMOH_DATA_DIR
mkdir -p "$MEMOH_DATA_DIR"

echo ""
echo "${GREEN}Starting services (first build may take a few minutes)...${NC}"
docker compose up -d --build

echo ""
echo "${GREEN}========================================${NC}"
echo "${GREEN}   Memoh is running!${NC}"
echo "${GREEN}========================================${NC}"
echo ""
echo "  Web UI:          http://localhost"
echo "  API:             http://localhost:8080"
echo "  Agent Gateway:   http://localhost:8081"
echo ""
echo "  Admin login:     ${ADMIN_USER} / ${ADMIN_PASS}"
echo ""
echo "Commands:"
echo "  cd ${INSTALL_DIR} && docker compose ps       # Status"
echo "  cd ${INSTALL_DIR} && docker compose logs -f   # Logs"
echo "  cd ${INSTALL_DIR} && docker compose down      # Stop"
echo ""
echo "${YELLOW}First startup may take 1-2 minutes, please be patient.${NC}"
