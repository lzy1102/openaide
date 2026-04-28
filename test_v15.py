#!/usr/bin/env python3
"""OpenAIDE v15 测试脚本 - ThinkingService API + CodeService"""

import requests
import json

BASE_URL = "http://192.168.3.26:19375"
HEADERS = {"Authorization": "Bearer sk-openaide-test-key-2024", "Content-Type": "application/json"}

def test_health():
    r = requests.get(f"{BASE_URL}/health", timeout=10)
    print("Health:", r.status_code, r.json().get("status"))
    return r.status_code == 200

def test_thinking_list():
    r = requests.get(f"{BASE_URL}/api/thinking/thoughts", headers=HEADERS, timeout=10)
    print("Thinking list:", r.status_code, f"count={r.json().get('count')}")
    return r.status_code == 200

def test_code_execute():
    r = requests.post(f"{BASE_URL}/api/code/execute", headers=HEADERS, json={
        "language": "python",
        "code": "print('Hello from OpenAIDE Code Service!')\nprint(2+2)"
    }, timeout=15)
    print("Code execute:", r.status_code)
    if r.status_code == 200:
        data = r.json()
        print(f"  Status: {data.get('status')}, Time: {data.get('execution_time'):.3f}s")
        print(f"  Output: {data.get('output', '').strip()}")
    else:
        print(f"  Error: {r.text[:200]}")
    return r.status_code == 200

def test_code_execute_bash():
    r = requests.post(f"{BASE_URL}/api/code/execute", headers=HEADERS, json={
        "language": "bash",
        "code": "echo 'Current dir:' && pwd && echo 'Date:' && date"
    }, timeout=15)
    print("Code bash:", r.status_code)
    if r.status_code == 200:
        data = r.json()
        print(f"  Output: {data.get('output', '').strip()}")
    return r.status_code == 200

def main():
    print("=" * 50)
    print("OpenAIDE v15 Test")
    print("=" * 50)
    results = []
    results.append(("Health", test_health()))
    results.append(("Thinking List", test_thinking_list()))
    results.append(("Code Execute Python", test_code_execute()))
    results.append(("Code Execute Bash", test_code_execute_bash()))
    print("=" * 50)
    passed = sum(1 for _, r in results if r)
    print(f"Results: {passed}/{len(results)} passed")
    for name, ok in results:
        print(f"  {name}: {'PASS' if ok else 'FAIL'}")

if __name__ == "__main__":
    main()
