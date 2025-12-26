#!/bin/bash
# Development environment setup script

set -e

echo "=========================================="
echo "NVR System - Development Setup"
echo "=========================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check prerequisites
echo -e "\n${YELLOW}Checking prerequisites...${NC}"

check_command() {
    if command -v $1 &> /dev/null; then
        echo -e "${GREEN}✓${NC} $1 found: $($1 --version 2>&1 | head -n1)"
        return 0
    else
        echo -e "${RED}✗${NC} $1 not found"
        return 1
    fi
}

MISSING=0
check_command go || MISSING=1
check_command node || MISSING=1
check_command npm || MISSING=1
check_command python3 || MISSING=1
check_command docker || MISSING=1

if [ $MISSING -eq 1 ]; then
    echo -e "\n${RED}Some prerequisites are missing. Please install them first.${NC}"
    exit 1
fi

# Project root directory
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$PROJECT_ROOT"

echo -e "\n${YELLOW}Project root: ${PROJECT_ROOT}${NC}"

# Create config from example
echo -e "\n${YELLOW}Setting up configuration...${NC}"
if [ ! -f config/config.yaml ]; then
    cp config/config.example.yaml config/config.yaml
    echo -e "${GREEN}✓${NC} Created config/config.yaml from example"
else
    echo -e "${YELLOW}⚠${NC} config/config.yaml already exists, skipping"
fi

if [ ! -f config/go2rtc.yaml ]; then
    cp config/go2rtc.example.yaml config/go2rtc.yaml
    echo -e "${GREEN}✓${NC} Created config/go2rtc.yaml from example"
else
    echo -e "${YELLOW}⚠${NC} config/go2rtc.yaml already exists, skipping"
fi

# Create data directory
mkdir -p data
echo -e "${GREEN}✓${NC} Created data directory"

# Install Go dependencies
echo -e "\n${YELLOW}Installing Go dependencies...${NC}"
go mod download
echo -e "${GREEN}✓${NC} Go dependencies installed"

# Install air for hot reload (if not present)
if ! command -v air &> /dev/null; then
    echo -e "\n${YELLOW}Installing air for hot reload...${NC}"
    go install github.com/air-verse/air@latest
    echo -e "${GREEN}✓${NC} Air installed"
fi

# Setup web UI
echo -e "\n${YELLOW}Setting up web UI...${NC}"
cd web-ui
if [ -f package.json ]; then
    npm install
    echo -e "${GREEN}✓${NC} Web UI dependencies installed"
else
    echo -e "${YELLOW}⚠${NC} web-ui/package.json not found - run 'npm create vite@latest' first"
fi
cd "$PROJECT_ROOT"

# Setup AI detection service
echo -e "\n${YELLOW}Setting up AI detection service...${NC}"
cd services/ai-detection
if [ ! -d venv ]; then
    python3 -m venv venv
    echo -e "${GREEN}✓${NC} Python virtual environment created"
fi
source venv/bin/activate
pip install -q -r requirements.txt
deactivate
echo -e "${GREEN}✓${NC} AI detection dependencies installed"
cd "$PROJECT_ROOT"

# Initialize database
echo -e "\n${YELLOW}Initializing database...${NC}"
if [ ! -f data/nvr.db ]; then
    sqlite3 data/nvr.db < migrations/001_initial_schema.sql
    echo -e "${GREEN}✓${NC} Database initialized with schema"
else
    echo -e "${YELLOW}⚠${NC} Database already exists, skipping initialization"
fi

echo -e "\n${GREEN}=========================================="
echo "Setup complete!"
echo "==========================================${NC}"
echo ""
echo "To start development:"
echo "  1. Start go2rtc:        docker-compose -f docker-compose.dev.yml up go2rtc"
echo "  2. Start Go backend:    air -c .air.toml"
echo "  3. Start React UI:      cd web-ui && npm run dev"
echo "  4. Start AI service:    cd services/ai-detection && source venv/bin/activate && python main.py"
echo ""
echo "Or start everything with Docker:"
echo "  docker-compose -f docker-compose.dev.yml up"
echo ""
