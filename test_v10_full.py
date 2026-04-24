#!/usr/bin/env python3
import requests
import json
import sys

BASE = "http://localhost:19375/api"
PASS = "✅"
FAIL = "❌"

results = []

def record(name, passed, detail=""):
    status = PASS if passed else FAIL
    results.append((name, passed, detail))
    print(f"  {status} {name}" + (f" — {detail}" if detail else ""))

def test_agent_routing():
    print("\n=== 1. Agent 路由测试 ===")
    
    r = requests.get(f"{BASE}/agent-routing/config", timeout=5)
    record("GET /agent-routing/config", r.status_code == 200, f"status={r.status_code}")
    if r.status_code == 200:
        config = r.json()
        models = config.get('agentModels', {})
        routing = config.get('agentRouting', {})
        record("agentRouting 配置存在", len(routing) > 0, f"routes={routing}")
    
    r = requests.get(f"{BASE}/agent-routing/routes", timeout=5)
    record("GET /agent-routing/routes", r.status_code == 200)
    if r.status_code == 200:
        routes = r.json()
        record("路由表有内容", len(routes) > 0, f"routes={json.dumps(routes, ensure_ascii=False)[:200]}")
    
    for agent in ["build", "plan", "explore", "general", "default"]:
        r = requests.get(f"{BASE}/agent-routing/route/{agent}", timeout=5)
        record(f"Route {agent}", r.status_code == 200, f"response={r.text[:100]}")
    
    r = requests.get(f"{BASE}/agent-routing/route/nonexistent", timeout=5)
    record("未知Agent返回空模型", r.status_code == 200)

def test_slash_commands():
    print("\n=== 2. Slash 命令测试 ===")
    
    r = requests.get(f"{BASE}/slash/commands", timeout=5)
    record("GET /slash/commands", r.status_code == 200)
    if r.status_code == 200:
        commands = r.json()
        record("8个内置命令", len(commands) >= 8, f"count={len(commands)}")
        names = [c.get('name', '') for c in commands]
        for expected in ['compact', 'model', 'clear', 'agent', 'tools', 'help', 'routes', 'status']:
            record(f"命令 /{expected}", expected in names, f"available={names}")
    
    test_commands = [
        ("help", "", "帮助信息"),
        ("agent", "", "Agent模式列表"),
        ("agent", "build", "切换到build"),
        ("tools", "", "工具列表"),
        ("compact", "", "上下文压缩"),
        ("clear", "", "清除历史"),
        ("model", "", "模型信息"),
        ("routes", "", "路由配置"),
        ("status", "", "会话状态"),
    ]
    
    for cmd, args, desc in test_commands:
        r = requests.post(f"{BASE}/slash/execute", json={
            "command": cmd,
            "args": args,
            "session_id": "test-session"
        }, timeout=5)
        record(f"/{cmd} {args}".strip(), r.status_code == 200, 
               f"desc={desc}, response={r.text[:80]}")
    
    r = requests.post(f"{BASE}/slash/execute", json={
        "command": "nonexistent",
        "args": "",
        "session_id": "test-session"
    }, timeout=5)
    record("不存在命令返回错误", r.status_code == 400)

def test_existing_features():
    print("\n=== 3. 已有功能回归测试 ===")
    
    r = requests.get(f"{BASE}/permissions/profiles", timeout=5)
    record("权限系统正常", r.status_code == 200)
    
    r = requests.post(f"{BASE}/dialogues", json={"user_id": "test", "title": "V10 Test"}, timeout=5)
    record("创建对话", r.status_code == 200)
    if r.status_code != 200:
        return
    did = r.json().get('id')
    
    r = requests.post(f"{BASE}/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "现在几点了？",
        "model_id": ""
    }, timeout=60)
    record("工具调用正常", r.status_code == 200, 
           r.json().get('content', '')[:80] if r.status_code == 200 else f"error={r.text[:100]}")

def print_summary():
    print("\n" + "=" * 60)
    print("测试结果汇总")
    print("=" * 60)
    total = len(results)
    passed = sum(1 for _, p, _ in results if p)
    failed = total - passed
    print(f"\n总计: {total} 项 | {PASS} 通过: {passed} | {FAIL} 失败: {failed}")
    if failed > 0:
        print(f"\n失败项:")
        for name, passed, detail in results:
            if not passed:
                print(f"  {FAIL} {name} — {detail}")
    print()
    return failed == 0

if __name__ == "__main__":
    print("╔══════════════════════════════════════════════╗")
    print("║   OpenAIDE v10 测试 (OpenClaude架构改进)    ║")
    print("╚══════════════════════════════════════════════╝")
    
    test_agent_routing()
    test_slash_commands()
    test_existing_features()
    
    all_passed = print_summary()
    sys.exit(0 if all_passed else 1)
