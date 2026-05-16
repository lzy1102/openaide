#!/bin/bash
if [ -f ~/.openaide/openaide.pid ]; then
    PID=$(cat ~/.openaide/openaide.pid)
    kill $PID 2>/dev/null && echo 'Stopped' || echo 'Not running'
    rm -f ~/.openaide/openaide.pid
fi
