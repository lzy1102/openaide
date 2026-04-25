#!/usr/bin/env python3
"""测试 reasoning_content 修复"""
import requests
import json
import sys

BASE = "http://192.168.3.26:19375"

def test_api():
    print("=" * 60)
    print("测试 reasoning_content 修复")
    print("=" * 60)

    # 1. 创建对话
    r = requests.post(f"{BASE}/api/dialogues", json={"user_id": "test", "title": "reasoning test"})
    if r.status_code != 200:
        print(f"[FAIL] 创建对话失败: {r.status_code} {r.text}")
        return False
    dialogue = r.json()
    did = dialogue["id"]
    print(f"[OK] 创建对话: {did}")

    # 2. 发送普通消息 (非流式)
    r = requests.post(f"{BASE}/api/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "你好，请简单介绍一下自己",
        "model_id": ""
    })
    if r.status_code != 200:
        print(f"[FAIL] 发送消息失败: {r.status_code} {r.text}")
        return False
    msg = r.json()
    print(f"[OK] 发送消息成功, 回复ID: {msg['id']}")
    if msg.get("reasoning_content"):
        print(f"     reasoning_content: {msg['reasoning_content'][:100]}...")
    else:
        print(f"     无 reasoning_content (模型可能不支持)")

    # 3. 获取消息列表，验证 reasoning_content 在响应中
    r = requests.get(f"{BASE}/api/dialogues/{did}/messages")
    if r.status_code != 200:
        print(f"[FAIL] 获取消息失败: {r.status_code}")
        return False
    messages = r.json()
    print(f"[OK] 获取消息列表: {len(messages)} 条")
    for m in messages:
        has_reasoning = "reasoning_content" in m
        print(f"     - {m['sender']}: reasoning_content字段存在={has_reasoning}, 内容={'有' if m.get('reasoning_content') else '无'}")

    # 4. 测试保存流式消息 API
    r = requests.post(f"{BASE}/api/dialogues/{did}/save-stream", json={
        "content": "这是流式测试回复",
        "reasoning_content": "这是思考过程内容"
    })
    if r.status_code != 200:
        print(f"[FAIL] 保存流式消息失败: {r.status_code} {r.text}")
        return False
    saved = r.json()
    print(f"[OK] 保存流式消息成功, ID: {saved['id']}")
    if saved.get("reasoning_content") == "这是思考过程内容":
        print(f"     [OK] reasoning_content 正确保存")
    else:
        print(f"     [FAIL] reasoning_content 保存错误: {saved.get('reasoning_content')}")
        return False

    # 5. 再次获取消息列表验证
    r = requests.get(f"{BASE}/api/dialogues/{did}/messages")
    messages = r.json()
    reasoning_msgs = [m for m in messages if m.get("reasoning_content")]
    print(f"[OK] 最终消息列表中 {len(reasoning_msgs)} 条包含 reasoning_content")

    print("\n" + "=" * 60)
    print("所有测试通过!")
    print("=" * 60)
    return True

if __name__ == "__main__":
    try:
        ok = test_api()
        sys.exit(0 if ok else 1)
    except Exception as e:
        print(f"[FAIL] 测试异常: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
