#!/bin/bash
if [ -f /opt/openaide/openaide.pid ]; then
    PID=$(cat /opt/openaide/openaide.pid)
    kill $PID 2>/dev/null && echo 'Stopped' || echo 'Not running'
    rm -f /opt/openaide/openaide.pid
fi
