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
    
    # 2. 发送消息 - 使用流式 API
    print(f"\nSending streaming request...")
    stream_resp = requests.post(
        f"{BASE_URL}/dialogues/{dialogue_id}/stream",
        json={"content": "你好", "model_id": "openrouter-free"},
        stream=True
    )
    print(f"Stream response: {stream_resp.status_code}")
    
    # 3. 读取流式响应
    print("\nResponse content:")
    for line in stream_resp.iter_lines():
        if line:
            line_str = line.decode('utf-8')
            if line_str.startswith('data: '):
                data_str = line_str[6:]  # 去掉 'data: ' 前缀
                if data_str == '[DONE]':
                    break
                try:
                    data = json.loads(data_str)
                    if 'choices' in data and len(data['choices']) > 0:
                        delta = data['choices'][0].get('delta', {})
                        if 'content' in delta:
                            print(delta['content'], end='', flush=True)
                except Exception as e:
                    print(f"\n[Parse error: {e}] {line_str}")
    print("\n")

if __name__ == "__main__":
    test_chat()