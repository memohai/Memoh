#!/bin/sh
set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
PURPLE='\033[0;35m'
RED='\033[0;31m'
NC='\033[0m'

GITHUB_REPO="memohai/Memoh"
REPO="https://github.com/${GITHUB_REPO}.git"
DIR="Memoh"
SILENT=false

# Parse flags
while [ $# -gt 0 ]; do
  case "$1" in
    -y|--yes) SILENT=true ;;
    --version)
      shift
      MEMOH_VERSION="$1"
      ;;
    --version=*)
      MEMOH_VERSION="${1#--version=}"
      ;;
  esac
  shift
done

# Auto-silent if no TTY available
if [ "$SILENT" = false ] && ! [ -e /dev/tty ]; then
  SILENT=true
fi

echo "${PURPLE}Memoh One-Click Install${NC}"

# Check Docker and determine if sudo is needed
DOCKER="docker"
if ! command -v docker >/dev/null 2>&1; then
    echo "${RED}Error: Docker is not installed${NC}"
    echo "Install Docker first: https://docs.docker.com/get-docker/"
    exit 1
fi
if ! docker info >/dev/null 2>&1; then
    if sudo docker info >/dev/null 2>&1; then
        DOCKER="sudo docker"
    else
        echo "${RED}Error: Cannot connect to Docker daemon${NC}"
        echo "Try: sudo usermod -aG docker \$USER && newgrp docker"
        exit 1
    fi
fi
if ! $DOCKER compose version >/dev/null 2>&1; then
    echo "${RED}Error: Docker Compose v2 is required${NC}"
    echo "Install: https://docs.docker.com/compose/install/"
    exit 1
fi
echo "${GREEN}✓ Docker and Docker Compose detected${NC}"

# Resolve version: use MEMOH_VERSION env if set, otherwise fetch latest release
if [ -n "$MEMOH_VERSION" ]; then
    echo "${GREEN}✓ Using specified version: ${MEMOH_VERSION}${NC}"
else
    fetch_latest_version() {
      if command -v curl >/dev/null 2>&1; then
        curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null
      elif command -v wget >/dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null
      else
        echo "${RED}Error: curl or wget is required${NC}" >&2
        exit 1
      fi
    }
    MEMOH_VERSION=$(fetch_latest_version | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    if [ -n "$MEMOH_VERSION" ]; then
        echo "${GREEN}✓ Latest release: ${MEMOH_VERSION}${NC}"
    else
        echo "${YELLOW}Warning: Failed to fetch latest release tag, falling back to main branch${NC}"
    fi
fi

# Docker image tag: strip leading "v", fall back to "latest" only when version is unknown
if [ -n "$MEMOH_VERSION" ]; then
    MEMOH_DOCKER_VERSION=$(echo "$MEMOH_VERSION" | sed 's/^v//')
else
    MEMOH_DOCKER_VERSION="latest"
fi
echo "${GREEN}✓ Docker image version: ${MEMOH_DOCKER_VERSION}${NC}"

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
USE_CN_MIRROR="${USE_CN_MIRROR:-false}"
USE_SPARSE="${USE_SPARSE:-false}"
BROWSER_CORES="${BROWSER_CORES:-chromium,firefox}"

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

  printf "  Data directory (bind mount for server container data) [%s]: " "$WORKSPACE/data" > /dev/tty
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

  printf "  Enable sparse memory service? [y/N]: " > /dev/tty
  read -r input < /dev/tty || true
  case "$input" in
    y|Y|yes|YES) USE_SPARSE=true ;;
  esac

  echo "" > /dev/tty
  echo "  Browser core selection:" > /dev/tty
  echo "    1) Chromium only (smaller image)" > /dev/tty
  echo "    2) Firefox only" > /dev/tty
  echo "    3) Both Chromium and Firefox (default)" > /dev/tty
  printf "  Browser core [3]: " > /dev/tty
  read -r input < /dev/tty || true
  case "$input" in
    1) BROWSER_CORES="chromium" ;;
    2) BROWSER_CORES="firefox" ;;
    *) BROWSER_CORES="chromium,firefox" ;;
  esac

  echo "" > /dev/tty
fi

# Enter workspace (all operations run here)
mkdir -p "$WORKSPACE"
cd "$WORKSPACE"

# Clone or update
if [ -d "$DIR" ]; then
    echo "Updating existing installation in $WORKSPACE..."
    cd "$DIR"
    if [ -n "$MEMOH_VERSION" ]; then
        git fetch --depth 1 origin tag "$MEMOH_VERSION"
        git checkout "$MEMOH_VERSION"
    else
        git fetch --depth 1 origin main
        git checkout main 2>/dev/null || git checkout -b main --track origin/main
        git reset --hard origin/main
    fi
else
    echo "Cloning Memoh into $WORKSPACE..."
    if [ -n "$MEMOH_VERSION" ]; then
        git clone --depth 1 --branch "$MEMOH_VERSION" "$REPO" "$DIR"
    else
        git clone --depth 1 "$REPO" "$DIR"
    fi
    cd "$DIR"
fi

# Pin Docker image versions in docker-compose.yml
if [ "$MEMOH_DOCKER_VERSION" != "latest" ]; then
    sed -i.bak "s|memohai/server:latest|memohai/server:${MEMOH_DOCKER_VERSION}|g" docker-compose.yml
    sed -i.bak "s|memohai/agent:latest|memohai/agent:${MEMOH_DOCKER_VERSION}|g" docker-compose.yml
    sed -i.bak "s|memohai/web:latest|memohai/web:${MEMOH_DOCKER_VERSION}|g" docker-compose.yml
    sed -i.bak "s|memohai/browser:latest|memohai/browser:${MEMOH_DOCKER_VERSION}|g" docker-compose.yml
    sed -i.bak "s|memohai/sparse:latest|memohai/sparse:${MEMOH_DOCKER_VERSION}|g" docker-compose.yml
    rm -f docker-compose.yml.bak
    echo "${GREEN}✓ Docker images pinned to ${MEMOH_DOCKER_VERSION}${NC}"
fi

# Generate config.toml from template
cp conf/app.docker.toml config.toml
sed -i.bak "s|username = \"admin\"|username = \"${ADMIN_USER}\"|" config.toml
sed -i.bak "s|password = \"admin123\"|password = \"${ADMIN_PASS}\"|" config.toml
sed -i.bak "s|jwt_secret = \".*\"|jwt_secret = \"${JWT_SECRET}\"|" config.toml
sed -i.bak "s|password = \"memoh123\"|password = \"${PG_PASS}\"|" config.toml
export POSTGRES_PASSWORD="${PG_PASS}"
if [ "$USE_CN_MIRROR" = true ]; then
  sed -i.bak 's|# registry = "memoh.cn"|registry = "memoh.cn"|' config.toml
fi
rm -f config.toml.bak

# Use generated config and data dir
INSTALL_DIR="$(pwd)"
export MEMOH_CONFIG=./config.toml
export MEMOH_DATA_DIR
mkdir -p "$MEMOH_DATA_DIR"

COMPOSE_FILES="-f docker-compose.yml"
COMPOSE_PROFILES="--profile qdrant --profile browser"
if [ "$USE_SPARSE" = true ]; then
  COMPOSE_PROFILES="$COMPOSE_PROFILES --profile sparse"
  echo "${GREEN}✓ Sparse memory service enabled${NC}"
else
  echo "${YELLOW}ℹ Sparse memory service disabled${NC}"
fi
if [ "$USE_CN_MIRROR" = true ]; then
  COMPOSE_FILES="$COMPOSE_FILES -f docker/docker-compose.cn.yml"
  echo "${GREEN}✓ Using China mainland mirror (memoh.cn)${NC}"
fi

echo POSTGRES_PASSWORD="${PG_PASS}" >> .env
echo MEMOH_CONFIG=./config.toml >> .env
echo MEMOH_DATA_DIR="{$MEMOH_DATA_DIR}" >> .env
echo BROWSER_CORES="${BROWSER_CORES}" >> .env
echo USE_SPARSE="${USE_SPARSE}" >> .env
echo "${GREEN}✓ Browser cores: ${BROWSER_CORES}${NC}"

echo ""
echo "${GREEN}Pulling latest Docker images...${NC}"
$DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES pull --ignore-buildable

echo ""
echo "${GREEN}Building browser image (cores: ${BROWSER_CORES})...${NC}"
$DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES build browser

echo ""
echo "${GREEN}Starting services (first startup may take a few minutes)...${NC}"
$DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES up -d

echo ""
echo "${GREEN}✅ Memoh is running!${NC}${NC}"
echo ""
echo "  🌐 Web UI:            http://localhost:8082"
echo "  🔌 API:               http://localhost:8080"
echo "  🤖 Agent Gateway:     http://localhost:8081"
echo "  🌍 Browser Gateway:   http://localhost:8083"
echo ""
echo "  🔑 Admin login:       ${ADMIN_USER} / ${ADMIN_PASS}"
echo ""
COMPOSE_CMD="$DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES"
echo "📋 Commands:"
echo "  cd ${INSTALL_DIR} && ${COMPOSE_CMD} ps       # Status"
echo "  cd ${INSTALL_DIR} && ${COMPOSE_CMD} logs -f   # Logs"
echo "  cd ${INSTALL_DIR} && ${COMPOSE_CMD} down      # Stop"
echo ""
echo "${YELLOW}⏳ First startup may take 1-2 minutes, please be patient.${NC}"
