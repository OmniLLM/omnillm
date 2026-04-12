#!/bin/bash
# Comprehensive OmniModel Development Launcher
# Stop, rebuild, and start both Golang backend and frontend development servers

echo "🚀 Starting OmniModel Development Environment with Golang Backend"
echo ""
echo "Services will be available at:"
echo "  🔥 Golang Backend: http://localhost:5000"
echo "  🌐 Frontend: http://localhost:5080"
echo "  📱 Admin UI: http://localhost:5080/admin/"
echo ""

# Check if bun is available
if ! command -v bun &> /dev/null; then
    echo "❌ Bun is not installed. Please install Bun first: https://bun.sh"
    exit 1
fi

# Stop, rebuild, and start both services
echo "🔨 Stopping existing services, rebuilding, and starting..."
exec bun run omni:restart:rebuild -- --server-port 5000 --frontend-port 5080 --verbose