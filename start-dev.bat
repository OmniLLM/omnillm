@echo off
echo Starting OmniModel with Golang backend + Frontend...
echo.
echo Backend (Go): http://localhost:5000
echo Frontend: http://localhost:5080
echo Admin UI: http://localhost:5080/admin/
echo.

REM Rebuild and start both services (stops, rebuilds, then starts)
echo Stopping existing services, rebuilding, and starting...
bun run omni:restart:rebuild -- --server-port 5000 --frontend-port 5080 --verbose

pause