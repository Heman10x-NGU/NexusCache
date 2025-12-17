@echo off
REM NexusCache Demo Script for Windows
REM This script demonstrates the distributed cache functionality

echo ==========================================
echo   NexusCache Distributed Cache Demo
echo ==========================================
echo.

echo Waiting for services to be ready (30 seconds)...
timeout /t 30 /nobreak > nul

echo.
echo === Demo 1: Basic SET/GET Operations ===
echo.

echo Setting key 'user1' with value 'John Doe', expiry 5 min...
curl -s -X POST "http://localhost:9999/api/set" -d "key=user1&value=JohnDoe&expire=5&hot=false"
echo.

echo Getting key 'user1'...
curl -s "http://localhost:9999/api/get?key=user1"
echo.

echo.
echo === Demo 2: Built-in Test Data ===
echo.

echo Getting pre-defined test data (Tom, Jack, Sam)...
echo Tom:
curl -s "http://localhost:9999/api/get?key=Tom"
echo.
echo Jack:
curl -s "http://localhost:9999/api/get?key=Jack"
echo.
echo Sam:
curl -s "http://localhost:9999/api/get?key=Sam"
echo.

echo.
echo === Demo 3: Hot Cache ===
echo.

echo Setting 'popular_item' as HOT data...
curl -s -X POST "http://localhost:9999/api/set" -d "key=popular_item&value=HotData&expire=5&hot=true"
echo.

echo Getting from Node 1:
curl -s "http://localhost:9999/api/get?key=popular_item"
echo.
echo Getting from Node 2:
curl -s "http://localhost:9998/api/get?key=popular_item"
echo.
echo Getting from Node 3:
curl -s "http://localhost:9997/api/get?key=popular_item"
echo.

echo.
echo ==========================================
echo   Demo Complete!
echo ==========================================
echo.
echo The cache cluster is still running. You can:
echo   - Set values: curl -X POST "http://localhost:9999/api/set" -d "key=X&value=Y&expire=5&hot=false"
echo   - Get values: curl "http://localhost:9999/api/get?key=X"
echo   - Stop cluster: docker-compose down
echo.

pause
