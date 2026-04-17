#!/usr/bin/env python3
import requests
import json
import sys

BASE_URL = "http://localhost:19375/api"

def test_chat():
    # 1. 创建对话
    dialogue_resp = requests.post(f"{BASE_URL}/dialogues", json={"title": "test"})
    print(f"Create dialogue: {dialogue_resp.status_code}")
    if dialogue_resp.status_code != 200:
        print(f"Error: {dialogue_resp.text}")
        return
    
    dialogue = dialogue_resp.json()
    dialogue_id = dialogue.get("id")
    print(f"Dialogue ID: {dialogue_id}")
    
    # 2. 发送消息
    message_data = {
        "dialogue_id": dialogue_id,
        "content": "介绍一下自己",
        "model_id": "openrouter-free"
    }
    
    print(f"Sending message: {message_data}")
    msg_resp = requests.post(
        f"{BASE_URL}/dialogues/{dialogue_id}/messages",
        json=message_data,
        stream=True
    )
    print(f"Message response: {msg_resp.status_code}")
    
    # 3. 读取流式响应
    print("Response content:")
    for line in msg_resp.iter_lines():
        if line:
            try:
                data = json.loads(line.decode('utf-8'))
                print(f"  {data}")
            except:
                print(f"  Raw: {line.decode('utf-8')}")

if __name__ == "__main__":
    test_chat()