#!/bin/bash
cd ~/.openaide
nohup ~/.openaide/bin/openaide-server -config ~/.openaide/config.yaml > ~/.openaide/logs/server.log 2>&1 &
echo "OpenAIDE started, PID: $!"
echo "$!" > ~/.openaide/openaide.pid
