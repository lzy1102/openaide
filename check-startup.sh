#!/bin/bash
echo "=== Lines 100-200 of server.log ==="
sed -n '100,200p' /opt/openaide/server.log
echo ""
echo "=== Searching for Self-Evolution initialization ==="
grep -n "Self-Evolution" /opt/openaide/server.log
echo ""
echo "=== Searching for goroutine startups ==="
grep -n "go func\|Starting periodic\|RunPeriodic" /opt/openaide/server.log
echo ""
echo "=== Searching for panic or crash ==="
grep -n "panic\|fatal\|SIGSEGV\|nil pointer" /opt/openaide/server.log
