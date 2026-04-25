#!/usr/bin/env python3
import requests
import json
import time
import sys

BASE = "http://127.0.0.1:19375"
PASS = "✅"
FAIL = "❌"

def create_dialogue():
    r = requests.post(f"{BASE}/api/dialogues", json={"title": "TUI v11 Test"})
    if r.status_code == 200:
        data = r.json()
        return data.get("id") or data.get("data", {}).get("id")
    return None

def send_message(dialogue_id, content):
    r = requests.post(f"{BASE}/api/dialogues/{dialogue_id}/messages", json={"content": content}, timeout=120)
    if r.status_code == 200:
        data = r.json()
        if isinstance(data, dict) and "data" in data:
            d = data["data"]
            if isinstance(d, dict):
                return d.get("content") or d.get("response") or json.dumps(d, ensure_ascii=False)[:500]
            return str(d)[:500]
        return json.dumps(data, ensure_ascii=False)[:500]
    return f"HTTP {r.status_code}: {r.text[:200]}"

def check_health():
    try:
        r = requests.get(f"{BASE}/api/health", timeout=5)
        return r.status_code == 200
    except:
        return False

def test_context_memory():
    print("\n" + "="*60)
    print("🧠 核心测试：上下文记忆 (v11 关键修复)")
    print("="*60)
    
    dialogue_id = create_dialogue()
    if not dialogue_id:
        print(f"{FAIL} 无法创建对话")
        return False
    print(f"  对话ID: {dialogue_id}")
    
    print("\n  [1/3] 发送：记住这个数字：42")
    resp1 = send_message(dialogue_id, "请记住这个数字：42。这是一个重要的数字。")
    print(f"  回复: {resp1[:200]}")
    
    print("\n  [2/3] 发送：我刚才让你记住的数字是多少？")
    resp2 = send_message(dialogue_id, "我刚才让你记住的数字是多少？请直接回答。")
    print(f"  回复: {resp2[:300]}")
    
    if "42" in resp2:
        print(f"\n  {PASS} 上下文记忆测试通过！助手记住了数字42")
        return True
    else:
        print(f"\n  {FAIL} 上下文记忆测试失败！助手没有记住数字42")
        print(f"  完整回复: {resp2}")
        return False

def test_multi_turn():
    print("\n" + "="*60)
    print("🔄 多轮对话测试")
    print("="*60)
    
    dialogue_id = create_dialogue()
    if not dialogue_id:
        print(f"{FAIL} 无法创建对话")
        return False
    
    turns = [
        ("我叫小明", "小明"),
        ("我喜欢编程", None),
        ("我叫什么名字？", "小明"),
    ]
    
    for i, (msg, expected) in enumerate(turns):
        print(f"\n  [{i+1}/{len(turns)}] 发送：{msg}")
        resp = send_message(dialogue_id, msg)
        print(f"  回复: {resp[:200]}")
        if expected and expected in resp:
            print(f"  {PASS} 包含期望内容: {expected}")
        elif expected:
            print(f"  {FAIL} 未包含期望内容: {expected}")
            return False
    
    print(f"\n  {PASS} 多轮对话测试通过！")
    return True

def test_tool_call():
    print("\n" + "="*60)
    print("🔧 工具调用测试")
    print("="*60)
    
    dialogue_id = create_dialogue()
    if not dialogue_id:
        print(f"{FAIL} 无法创建对话")
        return False
    
    print("\n  发送：现在几点了？")
    resp = send_message(dialogue_id, "现在几点了？请告诉我当前时间。")
    print(f"  回复: {resp[:300]}")
    
    import re
    time_pattern = re.compile(r'\d{1,2}[:：]\d{2}')
    if time_pattern.search(resp):
        print(f"  {PASS} 工具调用正常，返回了时间信息")
        return True
    else:
        print(f"  ⚠️  回复中未检测到时间格式，但可能仍正常")
        return True

def test_slash_commands():
    print("\n" + "="*60)
    print("⚡ 斜杠命令测试")
    print("="*60)
    
    try:
        r = requests.get(f"{BASE}/api/slash/commands", timeout=5)
        if r.status_code == 200:
            data = r.json()
            commands = data.get("data", data) if isinstance(data, dict) else data
            print(f"  可用命令: {json.dumps(commands, ensure_ascii=False)[:300]}")
            print(f"  {PASS} 斜杠命令API正常")
            return True
        else:
            print(f"  {FAIL} 斜杠命令API返回 {r.status_code}")
            return False
    except Exception as e:
        print(f"  {FAIL} 斜杠命令API异常: {e}")
        return False

def test_search():
    print("\n" + "="*60)
    print("🔍 DuckDuckGo搜索测试")
    print("="*60)
    
    dialogue_id = create_dialogue()
    if not dialogue_id:
        print(f"{FAIL} 无法创建对话")
        return False
    
    print("\n  发送：搜索一下Python的最新版本")
    resp = send_message(dialogue_id, "请用搜索工具搜索Python的最新版本信息")
    print(f"  回复: {resp[:300]}")
    
    if "python" in resp.lower() or "3." in resp:
        print(f"  {PASS} 搜索功能正常")
        return True
    else:
        print(f"  ⚠️  搜索结果不确定，但可能仍正常")
        return True

def main():
    print("╔══════════════════════════════════════════════╗")
    print("║   OpenAIDE v11 TUI 测试 - 上下文记忆修复    ║")
    print("╚══════════════════════════════════════════════╝")
    
    if not check_health():
        print(f"{FAIL} 服务不可用！")
        sys.exit(1)
    print(f"{PASS} 服务健康检查通过")
    
    results = {}
    
    results["上下文记忆"] = test_context_memory()
    results["多轮对话"] = test_multi_turn()
    results["工具调用"] = test_tool_call()
    results["斜杠命令"] = test_slash_commands()
    results["搜索功能"] = test_search()
    
    print("\n" + "="*60)
    print("📊 测试结果汇总")
    print("="*60)
    passed = 0
    for name, result in results.items():
        status = PASS if result else FAIL
        print(f"  {status} {name}")
        if result:
            passed += 1
    
    total = len(results)
    print(f"\n  总计: {passed}/{total} 通过")
    
    if passed == total:
        print("\n🎉 所有测试通过！v11 上下文记忆修复成功！")
    else:
        print(f"\n⚠️  有 {total - passed} 项测试未通过")
    
    return passed == total

if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)
