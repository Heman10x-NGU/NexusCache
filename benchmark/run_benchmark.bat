@echo off
REM NexusCache Benchmark Script for Windows
REM Run this after docker-compose up

echo ==========================================
echo   NexusCache Performance Benchmark
echo ==========================================
echo.

REM Check if services are running
echo Checking if services are ready...
curl -s "http://localhost:9999/api/get?key=test" > nul 2>&1
if errorlevel 1 (
    echo.
    echo ERROR: Cache service not responding on localhost:9999
    echo Please run 'docker-compose up -d' first and wait for services to start.
    echo.
    pause
    exit /b 1
)

echo Services are ready!
echo.

REM Change to benchmark directory
cd /d "%~dp0"

echo Running benchmark with default settings...
echo   - Duration: 30 seconds
echo   - Concurrency: 50 workers
echo   - Key Space: 100 keys
echo   - Read Ratio: 80%% reads, 20%% writes
echo.

go run load_test.go -duration=30s -concurrency=50 -keys=100

echo.
echo Benchmark complete!
echo.
pause
