#!/usr/bin/env python3
import urllib.request
import json

BASE_URL = "http://localhost:19375/api"

# 创建对话
req = urllib.request.Request(
    f"{BASE_URL}/dialogues",
    data=json.dumps({"user_id": "test", "title": "Stream Test"}).encode(),
    headers={"Content-Type": "application/json"}
)
resp = urllib.request.urlopen(req, timeout=5)
dialogue = json.loads(resp.read().decode())
dialogue_id = dialogue['id']
print(f"对话ID: {dialogue_id}")

# 流式请求
print("\n=== 流式测试 ===")
req = urllib.request.Request(
    f"{BASE_URL}/dialogues/{dialogue_id}/stream",
    data=json.dumps({"user_id": "test", "content": "Say hi", "model_id": ""}).encode(),
    headers={"Content-Type": "application/json"}
)
resp = urllib.request.urlopen(req, timeout=180)

for line in resp:
    line = line.decode().strip()
    if line:
        print(f"RAW: {line}")
