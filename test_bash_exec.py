#!/usr/bin/env python3
"""测试执行bash命令获取IP"""
import requests
import json
import time

BASE = "http://192.168.3.26:19375"

def test():
    print("=" * 70)
    print("测试执行bash命令获取IP")
    print("=" * 70)

    # 创建对话
    r = requests.post(f"{BASE}/api/dialogues", json={"user_id": "test", "title": "bash exec test"})
    did = r.json()["id"]
    print(f"\n创建对话: {did}")

    # 测试1: 直接要求执行curl命令
    print("\n--- 测试1: 执行 curl 获取IP ---")
    start = time.time()
    r = requests.post(f"{BASE}/api/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "执行这个命令获取公网IP: curl -s https://api.ipify.org",
        "model_id": ""
    })
    elapsed = time.time() - start
    msg = r.json()
    print(f"耗时: {elapsed:.2f}s")
    print(f"回复: {msg['content'][:500]}")

    # 测试2: 要求执行ifconfig
    print("\n--- 测试2: 执行 ifconfig ---")
    start = time.time()
    r = requests.post(f"{BASE}/api/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "执行 ifconfig 命令查看网络接口",
        "model_id": ""
    })
    elapsed = time.time() - start
    msg = r.json()
    print(f"耗时: {elapsed:.2f}s")
    print(f"回复: {msg['content'][:500]}")

    # 测试3: 查看消息历史
    print("\n--- 消息历史 ---")
    r = requests.get(f"{BASE}/api/dialogues/{did}/messages")
    messages = r.json()
    for i, m in enumerate(messages):
        print(f"[{i}] {m['sender']}: {m['content'][:100]}...")

    print("\n" + "=" * 70)
    print("测试完成!")
    print("=" * 70)

if __name__ == "__main__":
    test()
