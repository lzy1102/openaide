#!/usr/bin/env python3
import requests
import json
import sys
import time

BASE = "http://localhost:19375/api"
PASS = "✅"
FAIL = "❌"

results = []

def record(name, passed, detail=""):
    status = PASS if passed else FAIL
    results.append((name, passed, detail))
    print(f"  {status} {name}" + (f" — {detail}" if detail else ""))

def test_health():
    print("\n=== 1. 健康检查 ===")
    try:
        r = requests.get(f"{BASE}/health", timeout=5)
        record("服务运行", r.status_code == 200)
    except Exception as e:
        record("服务运行", False, str(e))

def test_permission_profiles():
    print("\n=== 2. 权限系统测试 ===")
    
    # 2.1 获取所有 profiles
    r = requests.get(f"{BASE}/permissions/profiles", timeout=5)
    record("GET /permissions/profiles", r.status_code == 200)
    if r.status_code != 200:
        return
    
    profiles = r.json()
    record("4个Agent Profile", len(profiles) == 4, f"got {len(profiles)}")
    
    # 2.2 Build profile - 全权
    r = requests.get(f"{BASE}/permissions/profiles/build", timeout=5)
    record("Build Profile 可获取", r.status_code == 200)
    if r.status_code == 200:
        p = r.json()
        denied = p.get('DeniedTools') or []
        record("Build 无禁止工具", len(denied) == 0, f"denied={denied}")
    
    # 2.3 Plan profile - 只读
    r = requests.get(f"{BASE}/permissions/profiles/plan", timeout=5)
    record("Plan Profile 可获取", r.status_code == 200)
    if r.status_code == 200:
        p = r.json()
        denied = p.get('DeniedTools') or []
        record("Plan 禁止写入工具", 'write_file' in denied, f"denied={denied}")
        record("Plan 禁止执行命令", 'execute_command' in denied, f"denied={denied}")
    
    # 2.4 Explore profile - 只读探索
    r = requests.get(f"{BASE}/permissions/profiles/explore", timeout=5)
    record("Explore Profile 可获取", r.status_code == 200)
    if r.status_code == 200:
        p = r.json()
        denied = p.get('DeniedTools') or []
        record("Explore 禁止写入+执行", 'write_file' in denied and 'execute_command' in denied)
    
    # 2.5 General profile
    r = requests.get(f"{BASE}/permissions/profiles/general", timeout=5)
    record("General Profile 可获取", r.status_code == 200)
    
    # 2.6 不存在的 profile
    r = requests.get(f"{BASE}/permissions/profiles/nonexistent", timeout=5)
    record("不存在Profile返回404", r.status_code == 404)
    
    # 2.7 更新全局规则
    test_rules = [
        {"permission": "bash", "pattern": "rm *", "action": "deny"},
        {"permission": "bash", "pattern": "git *", "action": "allow"},
    ]
    r = requests.post(f"{BASE}/permissions/rules", json=test_rules, timeout=5)
    record("POST /permissions/rules", r.status_code == 200)
    
    # 2.8 清除会话审批
    r = requests.delete(f"{BASE}/permissions/session/test-session-123", timeout=5)
    record("DELETE /permissions/session/:id", r.status_code == 200)

def test_tool_calling():
    print("\n=== 3. 工具调用测试（ReAct循环）===")
    
    # 创建对话
    r = requests.post(f"{BASE}/dialogues", json={"user_id": "test", "title": "V9 Full Test"}, timeout=5)
    record("创建对话", r.status_code == 200)
    if r.status_code != 200:
        return
    did = r.json().get('id')
    
    # 3.1 简单工具调用 - 时间查询
    r = requests.post(f"{BASE}/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "现在几点了？",
        "model_id": ""
    }, timeout=60)
    passed = r.status_code == 200
    detail = ""
    if passed:
        content = r.json().get('content', '')
        has_time = any(kw in content for kw in ['时', '间', 'time', '点', '分', '秒', 'UTC', 'CST', ':'])
        passed = has_time
        detail = content[:100]
    record("时间查询(工具调用)", passed, detail)
    
    # 3.2 命令执行 - IP查询
    r = requests.post(f"{BASE}/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "查一下服务器的IP地址",
        "model_id": ""
    }, timeout=120)
    passed = r.status_code == 200
    detail = ""
    if passed:
        content = r.json().get('content', '')
        has_ip = any(kw in content for kw in ['192.168', '172.', '127.0', 'IP', 'ip', '地址'])
        passed = has_ip
        detail = content[:150]
    record("IP查询(命令执行)", passed, detail)

def test_task_tool():
    print("\n=== 4. Task工具测试（子代理委托）===")
    
    # 创建新对话
    r = requests.post(f"{BASE}/dialogues", json={"user_id": "test", "title": "Task Tool Test"}, timeout=5)
    record("创建Task测试对话", r.status_code == 200)
    if r.status_code != 200:
        return
    did = r.json().get('id')
    
    # 4.1 触发子代理 - 要求搜索/探索
    r = requests.post(f"{BASE}/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "帮我搜索一下当前目录下有哪些Go源代码文件",
        "model_id": ""
    }, timeout=180)
    passed = r.status_code == 200
    detail = ""
    if passed:
        content = r.json().get('content', '')
        has_result = len(content) > 20
        detail = content[:200]
    else:
        detail = f"status={r.status_code}, body={r.text[:200]}"
    record("Task工具调用", passed, detail)

def test_multi_turn():
    print("\n=== 5. 多轮对话测试 ===")
    
    r = requests.post(f"{BASE}/dialogues", json={"user_id": "test", "title": "Multi-turn Test"}, timeout=5)
    if r.status_code != 200:
        record("多轮对话", False, "创建对话失败")
        return
    did = r.json().get('id')
    
    turns = [
        ("你好，我是测试用户", "问候"),
        ("帮我算一下 123 * 456", "计算"),
        ("结果是多少？", "上下文记忆"),
    ]
    
    for msg, desc in turns:
        r = requests.post(f"{BASE}/dialogues/{did}/messages", json={
            "user_id": "test",
            "content": msg,
            "model_id": ""
        }, timeout=120)
        passed = r.status_code == 200
        detail = ""
        if passed:
            content = r.json().get('content', '')[:100]
            detail = content
        record(f"多轮-{desc}", passed, detail)

def test_stability():
    print("\n=== 6. 稳定性测试 ===")
    
    # 快速连续请求
    errors = 0
    for i in range(5):
        try:
            r = requests.get(f"{BASE}/permissions/profiles", timeout=5)
            if r.status_code != 200:
                errors += 1
        except:
            errors += 1
    record("5次快速请求", errors == 0, f"errors={errors}/5")

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
    print("║   OpenAIDE v9 全面测试 (OpenCode架构改进)   ║")
    print("╚══════════════════════════════════════════════╝")
    
    test_health()
    test_permission_profiles()
    test_tool_calling()
    test_task_tool()
    test_multi_turn()
    test_stability()
    
    all_passed = print_summary()
    sys.exit(0 if all_passed else 1)
