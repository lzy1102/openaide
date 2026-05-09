#!/bin/bash
set -e

echo "1. Stopping old server..."
pkill -f openaide-server || true
sleep 2

echo "2. Replacing binaries..."
cp /opt/openaide/openaide-server.new /opt/openaide/openaide-server
cp /opt/openaide/openaide-cli.new /opt/openaide/openaide-cli
chmod +x /opt/openaide/openaide-server /opt/openaide/openaide-cli

echo "3. Starting new server..."
cd /opt/openaide
nohup ./openaide-server > /opt/openaide/logs/server.log 2>&1 &

echo "4. Waiting 3 seconds..."
sleep 3

if pgrep -f openaide-server > /dev/null; then
    echo "✅ Deployment successful! Server is running."
else
    echo "❌ Server failed to start, check logs at /opt/openaide/logs/server.log"
    exit 1
fi
