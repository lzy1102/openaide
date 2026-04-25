#!/usr/bin/env python3
"""测试查IP功能 - 观察Agent思考、工具调用和命令执行"""
import requests
import json
import time

BASE = "http://192.168.3.26:19375"

def test_ip_query():
    print("=" * 70)
    print("测试查IP - 观察Agent思考、工具调用和命令执行")
    print("=" * 70)

    # 1. 创建对话
    r = requests.post(f"{BASE}/api/dialogues", json={"user_id": "test", "title": "IP查询测试"})
    dialogue = r.json()
    did = dialogue["id"]
    print(f"\n[1] 创建对话: {did}")

    # 2. 发送查IP消息 (非流式，观察完整响应)
    print(f"\n[2] 发送消息: '查一下我的公网IP'")
    start = time.time()
    r = requests.post(f"{BASE}/api/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "查一下我的公网IP",
        "model_id": ""
    })
    elapsed = time.time() - start
    msg = r.json()
    print(f"    耗时: {elapsed:.2f}s")
    print(f"    回复ID: {msg['id']}")
    print(f"    回复内容: {msg['content'][:200]}...")
    if msg.get('reasoning_content'):
        print(f"    思考过程: {msg['reasoning_content'][:200]}...")

    # 3. 获取完整消息历史
    print(f"\n[3] 获取消息历史:")
    r = requests.get(f"{BASE}/api/dialogues/{did}/messages")
    messages = r.json()
    for i, m in enumerate(messages):
        print(f"    [{i}] {m['sender']}: {m['content'][:80]}...")
        if m.get('reasoning_content'):
            print(f"         思考: {m['reasoning_content'][:80]}...")

    # 4. 测试带思考的复杂查询
    print(f"\n[4] 发送消息: '用curl命令获取我的公网IP并解释结果'")
    start = time.time()
    r = requests.post(f"{BASE}/api/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "用curl命令获取我的公网IP并解释结果",
        "model_id": ""
    })
    elapsed = time.time() - start
    msg = r.json()
    print(f"    耗时: {elapsed:.2f}s")
    print(f"    回复内容: {msg['content'][:300]}...")
    if msg.get('reasoning_content'):
        print(f"    思考过程:\n    {msg['reasoning_content'][:500]}...")

    # 5. 查看系统状态
    print(f"\n[5] 查看系统状态:")
    try:
        r = requests.get(f"{BASE}/api/status", timeout=5)
        if r.status_code == 200:
            print(f"    状态: {r.json()}")
    except:
        pass

    # 6. 查看工具列表
    print(f"\n[6] 查看可用工具:")
    try:
        r = requests.get(f"{BASE}/api/tools", timeout=5)
        if r.status_code == 200:
            tools = r.json()
            if isinstance(tools, list):
                for t in tools[:5]:
                    name = t.get('name', t.get('function', {}).get('name', 'unknown'))
                    print(f"    - {name}")
    except Exception as e:
        print(f"    获取工具列表失败: {e}")

    print("\n" + "=" * 70)
    print("测试完成!")
    print("=" * 70)

if __name__ == "__main__":
    test_ip_query()
