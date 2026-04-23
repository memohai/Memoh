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
COMPOSE_PROJECT_NAME="memoh"
SILENT=false

# Track whether the user explicitly set environment-backed options so upgrades
# can reuse prior install values by default.
if [ "${MEMOH_INSTALL_MODE+x}" = x ]; then
  INSTALL_MODE="$MEMOH_INSTALL_MODE"
else
  INSTALL_MODE="auto"
fi
if [ "${USE_CN_MIRROR+x}" = x ]; then
  USE_CN_MIRROR_SET=true
else
  USE_CN_MIRROR_SET=false
fi
if [ "${USE_SPARSE+x}" = x ]; then
  USE_SPARSE_SET=true
else
  USE_SPARSE_SET=false
fi
if [ "${BROWSER_CORE+x}" = x ]; then
  BROWSER_CORE_SET=true
else
  BROWSER_CORE_SET=false
fi

NETWORK_NAME="${COMPOSE_PROJECT_NAME}_memoh-network"
PROJECT_CONTAINERS="memoh-postgres memoh-migrate memoh-server memoh-web memoh-sparse memoh-qdrant memoh-browser"
PROJECT_VOLUMES="${COMPOSE_PROJECT_NAME}_postgres_data ${COMPOSE_PROJECT_NAME}_containerd_data ${COMPOSE_PROJECT_NAME}_memoh_data ${COMPOSE_PROJECT_NAME}_server_cni_state ${COMPOSE_PROJECT_NAME}_qdrant_data ${COMPOSE_PROJECT_NAME}_openviking_data"

EXISTING_CONFIG_SOURCE=""
EXISTING_ENV_SOURCE=""
EXISTING_INSTALL_STATE=false
EXISTING_DOCKER_STATE=false
EXISTING_DOCKER_VOLUMES=""
EXISTING_DOCKER_CONTAINERS=false
EXISTING_DOCKER_NETWORK=false
EXISTING_WORKSPACE_FILES=false
EXISTING_REPO_DIR=false

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
    --install-mode)
      shift
      INSTALL_MODE="$1"
      ;;
    --install-mode=*)
      INSTALL_MODE="${1#--install-mode=}"
      ;;
  esac
  shift
done

# Auto-silent if no TTY available
if [ "$SILENT" = false ] && ! [ -e /dev/tty ]; then
  SILENT=true
fi

echo "${PURPLE}Memoh One-Click Install${NC}"

read_env_file_value() {
  file="$1"
  key="$2"
  if [ ! -f "$file" ]; then
    return 1
  fi
  value=$(grep "^${key}=" "$file" 2>/dev/null | tail -n 1 | cut -d '=' -f 2-)
  if [ -z "$value" ]; then
    return 1
  fi
  case "$value" in
    \'*\')
      value=${value#\'}
      value=${value%\'}
      value=$(printf '%s' "$value" | sed "s/\\\\'/'/g")
      ;;
  esac
  printf '%s' "$value"
}

read_toml_value() {
  file="$1"
  section="$2"
  key="$3"
  if [ ! -f "$file" ]; then
    return 1
  fi
  value=$(awk -v target_section="[$section]" -v target_key="$key" '
    /^\[[^]]+\]/ {
      in_section = ($0 == target_section)
      next
    }
    in_section && $0 ~ "^[[:space:]]*" target_key "[[:space:]]*=" {
      value = substr($0, index($0, "=") + 1)
      sub(/^[[:space:]]*/, "", value)
      sub(/[[:space:]]*$/, "", value)
      if (value ~ /^".*"$/) {
        sub(/^"/, "", value)
        sub(/"$/, "", value)
      }
      print value
      exit
    }
  ' "$file")
  if [ -z "$value" ]; then
    return 1
  fi
  printf '%s' "$value" | sed 's/\\"/"/g; s/\\\\/\\/g'
}

browser_core_from_cores() {
  case "$1" in
    firefox) printf '%s' "firefox" ;;
    all|chromium,firefox|firefox,chromium) printf '%s' "all" ;;
    *) printf '%s' "chromium" ;;
  esac
}

escape_toml_string() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

set_toml_string_value() {
  file="$1"
  section="$2"
  key="$3"
  value=$(escape_toml_string "$4")
  tmp="${file}.tmp.$$"
  if TOML_VALUE="$value" awk -v target_section="[$section]" -v target_key="$key" '
    BEGIN {
      target_value = ENVIRON["TOML_VALUE"]
    }
    /^\[[^]]+\]/ {
      in_section = ($0 == target_section)
    }
    in_section && $0 ~ "^[[:space:]]*" target_key "[[:space:]]*=" {
      indent = $0
      sub(/[^[:space:]].*/, "", indent)
      print indent target_key " = \"" target_value "\""
      next
    }
    { print }
  ' "$file" > "$tmp"; then
    mv "$tmp" "$file"
  else
    rm -f "$tmp"
    return 1
  fi
}

write_env_value() {
  key="$1"
  value=$(printf '%s' "$2" | sed "s/'/\\\\'/g")
  printf "%s='%s'\n" "$key" "$value" >> .env
}

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

detect_existing_installation() {
  EXISTING_CONFIG_SOURCE=""
  EXISTING_ENV_SOURCE=""
  EXISTING_INSTALL_STATE=false
  EXISTING_DOCKER_STATE=false
  EXISTING_DOCKER_VOLUMES=""
  EXISTING_DOCKER_CONTAINERS=false
  EXISTING_DOCKER_NETWORK=false
  EXISTING_WORKSPACE_FILES=false
  EXISTING_REPO_DIR=false

  if [ -d "$WORKSPACE/$DIR" ]; then
    EXISTING_REPO_DIR=true
    EXISTING_INSTALL_STATE=true
  fi

  if [ -f "$WORKSPACE/config.toml" ]; then
    EXISTING_CONFIG_SOURCE="$WORKSPACE/config.toml"
    EXISTING_WORKSPACE_FILES=true
    EXISTING_INSTALL_STATE=true
    if [ -f "$WORKSPACE/.env" ]; then
      EXISTING_ENV_SOURCE="$WORKSPACE/.env"
    fi
  elif [ -f "$WORKSPACE/$DIR/config.toml" ]; then
    EXISTING_CONFIG_SOURCE="$WORKSPACE/$DIR/config.toml"
    EXISTING_INSTALL_STATE=true
    if [ -f "$WORKSPACE/$DIR/.env" ]; then
      EXISTING_ENV_SOURCE="$WORKSPACE/$DIR/.env"
    fi
  fi

  if [ -f "$WORKSPACE/docker-compose.yml" ] || [ -f "$WORKSPACE/.env" ]; then
    EXISTING_WORKSPACE_FILES=true
    EXISTING_INSTALL_STATE=true
    if [ -z "$EXISTING_ENV_SOURCE" ] && [ -f "$WORKSPACE/.env" ]; then
      EXISTING_ENV_SOURCE="$WORKSPACE/.env"
    fi
  fi

  for volume in $PROJECT_VOLUMES; do
    if $DOCKER volume inspect "$volume" >/dev/null 2>&1; then
      EXISTING_DOCKER_STATE=true
      EXISTING_INSTALL_STATE=true
      EXISTING_DOCKER_VOLUMES="${EXISTING_DOCKER_VOLUMES} ${volume}"
    fi
  done

  for container in $PROJECT_CONTAINERS; do
    if $DOCKER container inspect "$container" >/dev/null 2>&1; then
      EXISTING_DOCKER_STATE=true
      EXISTING_DOCKER_CONTAINERS=true
      EXISTING_INSTALL_STATE=true
      break
    fi
  done

  if $DOCKER network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
    EXISTING_DOCKER_STATE=true
    EXISTING_DOCKER_NETWORK=true
    EXISTING_INSTALL_STATE=true
  fi
}

load_existing_settings() {
  if [ -n "$EXISTING_CONFIG_SOURCE" ]; then
    value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "admin" "username" || true)
    [ -n "$value" ] && ADMIN_USER="$value"

    value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "admin" "password" || true)
    [ -n "$value" ] && ADMIN_PASS="$value"

    value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "auth" "jwt_secret" || true)
    [ -n "$value" ] && JWT_SECRET="$value"

    value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "postgres" "password" || true)
    [ -n "$value" ] && PG_PASS="$value"

    if [ "$USE_CN_MIRROR_SET" = false ]; then
      value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "workspace" "registry" || true)
      if [ "$value" = "memoh.cn" ]; then
        USE_CN_MIRROR=true
      fi
    fi
  fi

  if [ -n "$EXISTING_ENV_SOURCE" ]; then
    if [ "$USE_SPARSE_SET" = false ]; then
      value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "USE_SPARSE" || true)
      [ -n "$value" ] && USE_SPARSE="$value"
    fi

    if [ "$BROWSER_CORE_SET" = false ]; then
      value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "BROWSER_CORES" || true)
      [ -n "$value" ] && BROWSER_CORE=$(browser_core_from_cores "$value")
    fi

    value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "POSTGRES_PASSWORD" || true)
    [ -n "$value" ] && PG_PASS="$value"

    value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "MEMOH_DATA_DIR" || true)
    [ -n "$value" ] && MEMOH_DATA_DIR="$value"
  fi
}

prompt_install_mode() {
  if [ "$SILENT" = true ]; then
    if [ "$INSTALL_MODE" = "auto" ]; then
      if [ -n "$EXISTING_CONFIG_SOURCE" ]; then
        INSTALL_MODE="upgrade"
        echo "${YELLOW}ℹ Existing Memoh installation detected. Reusing existing configuration in silent mode.${NC}"
      elif [ "$EXISTING_DOCKER_STATE" = true ]; then
        echo "${RED}Error: Existing Memoh Docker state was detected but no reusable config.toml was found.${NC}"
        echo "Run again with MEMOH_INSTALL_MODE=reinstall to wipe Docker data, or restore the previous config.toml."
        exit 1
      else
        INSTALL_MODE="fresh"
        if [ "$EXISTING_INSTALL_STATE" = true ]; then
          echo "${YELLOW}ℹ Existing Memoh files were detected, but no Docker state or reusable config.toml was found. Proceeding with a fresh install in silent mode.${NC}"
        fi
      fi
    fi
    return
  fi

  if [ "$INSTALL_MODE" != "auto" ]; then
    return
  fi

  if [ "$EXISTING_INSTALL_STATE" = false ]; then
    INSTALL_MODE="fresh"
    return
  fi

  echo "${YELLOW}Detected existing Memoh installation state:${NC}" > /dev/tty
  if [ -n "$EXISTING_CONFIG_SOURCE" ]; then
    echo "  - Config: ${EXISTING_CONFIG_SOURCE}" > /dev/tty
  fi
  if [ -n "$EXISTING_ENV_SOURCE" ]; then
    echo "  - Env: ${EXISTING_ENV_SOURCE}" > /dev/tty
  fi
  if [ "$EXISTING_REPO_DIR" = true ]; then
    echo "  - Repository checkout: ${WORKSPACE}/${DIR}" > /dev/tty
  fi
  if [ -n "$EXISTING_DOCKER_VOLUMES" ]; then
    echo "  - Docker volumes:${EXISTING_DOCKER_VOLUMES}" > /dev/tty
  fi
  if [ "$EXISTING_DOCKER_CONTAINERS" = true ]; then
    echo "  - Existing Memoh containers" > /dev/tty
  fi
  if [ "$EXISTING_DOCKER_NETWORK" = true ]; then
    echo "  - Docker network: ${NETWORK_NAME}" > /dev/tty
  fi
  echo "" > /dev/tty

  if [ -n "$EXISTING_CONFIG_SOURCE" ]; then
    echo "Choose install mode:" > /dev/tty
    echo "  1) Upgrade existing installation (recommended, reuses config and DB password)" > /dev/tty
    echo "  2) Reinstall from scratch (removes Memoh Docker data)" > /dev/tty
    echo "  3) Abort" > /dev/tty
    printf "  Install mode [1]: " > /dev/tty
    read -r input < /dev/tty || true
    case "$input" in
      2) INSTALL_MODE="reinstall" ;;
      3) INSTALL_MODE="abort" ;;
      *) INSTALL_MODE="upgrade" ;;
    esac
  elif [ "$EXISTING_DOCKER_STATE" = true ]; then
    echo "No reusable config.toml was found for a safe upgrade." > /dev/tty
    echo "Choose install mode:" > /dev/tty
    echo "  1) Reinstall from scratch (removes Memoh Docker data)" > /dev/tty
    echo "  2) Abort" > /dev/tty
    printf "  Install mode [2]: " > /dev/tty
    read -r input < /dev/tty || true
    case "$input" in
      1) INSTALL_MODE="reinstall" ;;
      *) INSTALL_MODE="abort" ;;
    esac
  else
    echo "No reusable config.toml or Docker state was found." > /dev/tty
    echo "Choose install mode:" > /dev/tty
    echo "  1) Continue fresh install (recommended)" > /dev/tty
    echo "  2) Abort" > /dev/tty
    printf "  Install mode [1]: " > /dev/tty
    read -r input < /dev/tty || true
    case "$input" in
      2) INSTALL_MODE="abort" ;;
      *) INSTALL_MODE="fresh" ;;
    esac
  fi
}

cleanup_existing_installation() {
  echo "${YELLOW}Removing existing Memoh Docker containers, volumes, and network...${NC}"
  for container in $PROJECT_CONTAINERS; do
    $DOCKER rm -f "$container" >/dev/null 2>&1 || true
  done
  for volume in $PROJECT_VOLUMES; do
    $DOCKER volume rm -f "$volume" >/dev/null 2>&1 || true
  done
  $DOCKER network rm "$NETWORK_NAME" >/dev/null 2>&1 || true
}

show_failure_logs() {
  echo ""
  echo "${RED}Startup failed. Recent PostgreSQL and migration logs:${NC}"
  $DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES logs --no-color --tail=200 postgres migrate || true
}

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
BROWSER_CORE="${BROWSER_CORE:-chromium}"

if [ "$SILENT" = false ]; then
  echo "Configure Memoh (press Enter to use defaults):" > /dev/tty
  echo "" > /dev/tty

  printf "  Workspace (install and clone here) [%s]: " "~/memoh" > /dev/tty
  read -r input < /dev/tty || true
  if [ -n "$input" ]; then
    case "$input" in
      "~") WORKSPACE="${HOME:-/tmp}" ;;
      "~"/*) WORKSPACE="${HOME:-/tmp}${input#\~}" ;;
      *) WORKSPACE="$input" ;;
    esac
  fi
fi

mkdir -p "$WORKSPACE"
WORKSPACE=$(cd "$WORKSPACE" && pwd)

detect_existing_installation
load_existing_settings
prompt_install_mode

case "$INSTALL_MODE" in
  auto) INSTALL_MODE="fresh" ;;
  fresh|upgrade|reinstall) ;;
  abort)
    echo "Installation aborted."
    exit 0
    ;;
  *)
    echo "${RED}Error: Unknown install mode '${INSTALL_MODE}'. Use fresh, upgrade, reinstall, or auto.${NC}"
    exit 1
    ;;
esac

if [ "$INSTALL_MODE" = "upgrade" ] && [ -z "$EXISTING_CONFIG_SOURCE" ]; then
  echo "${RED}Error: Upgrade mode requires an existing config.toml to reuse.${NC}"
  exit 1
fi

if [ "$INSTALL_MODE" = "fresh" ] && [ "$EXISTING_DOCKER_STATE" = true ]; then
  echo "${RED}Error: Existing Memoh Docker state was detected. Use upgrade or reinstall instead of fresh.${NC}"
  exit 1
fi

if [ "$SILENT" = false ] && [ "$INSTALL_MODE" != "upgrade" ]; then
  printf "  Data directory (reserved for future bind-mount support) [%s]: " "$MEMOH_DATA_DIR" > /dev/tty
  read -r input < /dev/tty || true
  if [ -n "$input" ]; then
    case "$input" in
      "~") MEMOH_DATA_DIR="${HOME:-/tmp}" ;;
      "~"/*) MEMOH_DATA_DIR="${HOME:-/tmp}${input#\~}" ;;
      *) MEMOH_DATA_DIR="$input" ;;
    esac
  fi

  printf "  Admin username [%s]: " "$ADMIN_USER" > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && ADMIN_USER="$input"

  printf "  Admin password [%s]: " "$ADMIN_PASS" > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && ADMIN_PASS="$input"

  printf "  JWT secret [current/default value retained]: " > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && JWT_SECRET="$input"

  printf "  Postgres password [%s]: " "$PG_PASS" > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && PG_PASS="$input"

  printf "  Enable sparse memory service? [%s]: " "$( [ "$USE_SPARSE" = true ] && printf 'Y/n' || printf 'y/N' )" > /dev/tty
  read -r input < /dev/tty || true
  case "$input" in
    y|Y|yes|YES) USE_SPARSE=true ;;
    n|N|no|NO) USE_SPARSE=false ;;
  esac

  echo "" > /dev/tty
  echo "  Browser core selection:" > /dev/tty
  echo "    1) Chromium only (default, smaller image)" > /dev/tty
  echo "    2) Firefox only" > /dev/tty
  echo "    3) Both Chromium and Firefox" > /dev/tty
  case "$BROWSER_CORE" in
    firefox) browser_default="2" ;;
    all) browser_default="3" ;;
    *) browser_default="1" ;;
  esac
  printf "  Browser core [%s]: " "$browser_default" > /dev/tty
  read -r input < /dev/tty || true
  case "$input" in
    2) BROWSER_CORE="firefox" ;;
    3) BROWSER_CORE="all" ;;
    "") 
      case "$browser_default" in
        2) BROWSER_CORE="firefox" ;;
        3) BROWSER_CORE="all" ;;
        *) BROWSER_CORE="chromium" ;;
      esac
      ;;
    *) BROWSER_CORE="chromium" ;;
  esac

  echo "" > /dev/tty
elif [ "$INSTALL_MODE" = "upgrade" ]; then
  echo "${GREEN}✓ Upgrade mode: reusing existing configuration and database credentials${NC}"
fi

# Enter workspace (all operations run here)
cd "$WORKSPACE"

# Clone or update
CLONED_FRESH=false
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
    CLONED_FRESH=true
fi

# Pin Docker image versions in docker-compose.yml
if [ "$MEMOH_DOCKER_VERSION" != "latest" ]; then
    sed -i.bak "s|memohai/server:latest|memohai/server:${MEMOH_DOCKER_VERSION}|g" docker-compose.yml
    sed -i.bak "s|memohai/agent:latest|memohai/agent:${MEMOH_DOCKER_VERSION}|g" docker-compose.yml
    sed -i.bak "s|memohai/web:latest|memohai/web:${MEMOH_DOCKER_VERSION}|g" docker-compose.yml
    sed -i.bak "s|memohai/sparse:latest|memohai/sparse:${MEMOH_DOCKER_VERSION}|g" docker-compose.yml
    rm -f docker-compose.yml.bak
    echo "${GREEN}✓ Docker images pinned to ${MEMOH_DOCKER_VERSION}${NC}"
fi

if [ "$INSTALL_MODE" = "upgrade" ]; then
  if [ "$EXISTING_CONFIG_SOURCE" != "$PWD/config.toml" ]; then
    cp "$EXISTING_CONFIG_SOURCE" ./config.toml
  fi
else
  cp conf/app.docker.toml config.toml
  set_toml_string_value config.toml "admin" "username" "$ADMIN_USER"
  set_toml_string_value config.toml "admin" "password" "$ADMIN_PASS"
  set_toml_string_value config.toml "auth" "jwt_secret" "$JWT_SECRET"
  set_toml_string_value config.toml "postgres" "password" "$PG_PASS"
  if [ "$USE_CN_MIRROR" = true ]; then
    sed -i.bak 's|# registry = "memoh.cn"|registry = "memoh.cn"|' config.toml
  fi
  rm -f config.toml.bak
fi

INSTALL_DIR="$(pwd)"
mkdir -p "$MEMOH_DATA_DIR"
MEMOH_DATA_DIR=$(cd "$MEMOH_DATA_DIR" && pwd)
export MEMOH_CONFIG=./config.toml
export MEMOH_DATA_DIR
export POSTGRES_PASSWORD="${PG_PASS}"

# Resolve browser tag and cores from BROWSER_CORE selection
case "$BROWSER_CORE" in
  firefox)
    BROWSER_TAG_VARIANT="firefox"
    BROWSER_CORES="firefox"
    ;;
  all)
    BROWSER_TAG_VARIANT=""
    BROWSER_CORES="chromium,firefox"
    ;;
  *)
    BROWSER_TAG_VARIANT="chromium"
    BROWSER_CORES="chromium"
    ;;
esac

if [ -n "$BROWSER_TAG_VARIANT" ]; then
  if [ "$MEMOH_DOCKER_VERSION" != "latest" ]; then
    BROWSER_TAG="${MEMOH_DOCKER_VERSION}-${BROWSER_TAG_VARIANT}"
  else
    BROWSER_TAG="${BROWSER_TAG_VARIANT}-latest"
  fi
else
  BROWSER_TAG="${MEMOH_DOCKER_VERSION}"
fi

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

: > .env
write_env_value "POSTGRES_PASSWORD" "$PG_PASS"
write_env_value "MEMOH_CONFIG" "./config.toml"
write_env_value "MEMOH_DATA_DIR" "$MEMOH_DATA_DIR"
write_env_value "USE_SPARSE" "$USE_SPARSE"
write_env_value "BROWSER_TAG" "$BROWSER_TAG"
write_env_value "BROWSER_CORES" "$BROWSER_CORES"
echo "${GREEN}✓ Browser: ${BROWSER_CORE} (image tag: ${BROWSER_TAG})${NC}"

if [ "$INSTALL_MODE" = "reinstall" ]; then
  cleanup_existing_installation
fi

echo ""
echo "${GREEN}Pulling Docker images...${NC}"
$DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES pull

echo ""
echo "${GREEN}Starting services (first startup may take a few minutes)...${NC}"
if ! $DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES up -d; then
  show_failure_logs
  exit 1
fi

# After fresh clone: copy minimal files to workspace and remove clone directory
if [ "$CLONED_FRESH" = true ]; then
  echo ""
  echo "${GREEN}Cleaning up clone directory...${NC}"
  cp docker-compose.yml config.toml .env "$WORKSPACE/"
  mkdir -p "$WORKSPACE/conf"
  cp -r conf/providers "$WORKSPACE/conf/"
  if [ "$USE_CN_MIRROR" = true ]; then
    mkdir -p "$WORKSPACE/docker"
    cp docker/docker-compose.cn.yml "$WORKSPACE/docker/"
  fi
  cd "$WORKSPACE"
  rm -rf "$WORKSPACE/$DIR"
  INSTALL_DIR="$WORKSPACE"
  echo "${GREEN}✓ Clone directory removed, minimal install at ${INSTALL_DIR}${NC}"
fi

echo ""
echo "${GREEN}✅ Memoh is running!${NC}"
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
if [ "$INSTALL_MODE" != "fresh" ]; then
  echo "  cd ${INSTALL_DIR} && ${COMPOSE_CMD} down -v   # Remove containers and Docker data"
fi
echo ""
echo "${YELLOW}⏳ First startup may take 1-2 minutes, please be patient.${NC}"
