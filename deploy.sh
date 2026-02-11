#!/bin/bash

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}   Memoh Docker Compose Deployment${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check Docker
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: Docker is not installed${NC}"
    echo "Please install Docker first: https://docs.docker.com/get-docker/"
    exit 1
fi

# Check Docker Compose
if ! docker compose version &> /dev/null; then
    echo -e "${RED}Error: Docker Compose is not installed or version is too old${NC}"
    echo "Please install Docker Compose v2.0+: https://docs.docker.com/compose/install/"
    exit 1
fi

echo -e "${GREEN}✓ Docker and Docker Compose are installed${NC}"
echo ""

# Check .env file
if [ ! -f .env ]; then
    echo -e "${YELLOW}⚠ .env file does not exist, creating...${NC}"
    cp .env.example .env
    
    # Generate random JWT secret
    JWT_SECRET=$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64)
    sed -i.bak "s|JWT_SECRET=.*|JWT_SECRET=$JWT_SECRET|g" .env
    rm -f .env.bak
    
    echo -e "${GREEN}✓ .env file created${NC}"
    echo -e "${YELLOW}⚠ Please edit .env file to change default passwords and configuration${NC}"
    echo ""
fi

# Check config.toml
if [ ! -f config.toml ]; then
    echo -e "${YELLOW}⚠ config.toml does not exist, creating...${NC}"
    cp config.docker.toml config.toml
    echo -e "${GREEN}✓ config.toml created${NC}"
    echo ""
fi

# Build MCP image
echo -e "${GREEN}Building MCP image...${NC}"
if docker build -f cmd/mcp/Dockerfile -t memoh-mcp:latest . > /dev/null 2>&1; then
    echo -e "${GREEN}✓ MCP image built successfully${NC}"
else
    echo -e "${YELLOW}⚠ MCP image build failed, will try to pull at runtime${NC}"
fi
echo ""

# Start services
echo -e "${GREEN}Starting services...${NC}"
docker compose up -d

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}   Deployment Complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Service URLs:"
echo "  - Web UI: http://localhost"
echo "  - API Service: http://localhost:8080"
echo "  - Agent Gateway: http://localhost:8081"
echo ""
echo "View service status:"
echo "  docker compose ps"
echo ""
echo "View logs:"
echo "  docker compose logs -f"
echo ""
echo "Stop services:"
echo "  docker compose down"
echo ""
echo -e "${YELLOW}⚠ First startup may take 1-2 minutes, please be patient${NC}"
echo ""
echo "View detailed documentation: DEPLOYMENT.md"
