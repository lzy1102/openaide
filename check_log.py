#!/usr/bin/env python3
import requests
import json

# 1. 首先检查服务器是否响应
print("Checking server health...")
try:
    resp = requests.get("http://localhost:19375/api/models", timeout=5)
    print(f"Models API: {resp.status_code}")
    if resp.status_code == 200:
        models = resp.json()
        print(f"Found {len(models)} models")
        for m in models:
            print(f"  - {m['name']}: {m['config'].get('model', 'N/A')}")
except Exception as e:
    print(f"Error: {e}")

# 2. 创建对话并发送消息测试
print("\nTesting chat...")
try:
    # 创建对话
    dialogue_resp = requests.post("http://localhost:19375/api/dialogues", 
                                   json={"title": "test"}, timeout=5)
    print(f"Create dialogue: {dialogue_resp.status_code}")
    
    if dialogue_resp.status_code == 200:
        dialogue = dialogue_resp.json()
        dialogue_id = dialogue.get("id")
        print(f"Dialogue ID: {dialogue_id}")
        
        # 发送流式请求
        print("\nSending streaming request...")
        stream_resp = requests.post(
            f"http://localhost:19375/api/dialogues/{dialogue_id}/stream",
            json={"content": "你好", "model_id": "openrouter-free"},
            stream=True,
            timeout=30
        )
        print(f"Stream response: {stream_resp.status_code}")
        print(f"Headers: {dict(stream_resp.headers)}")
        
        # 读取响应
        print("\nResponse content (first 500 chars):")
        content = ""
        for i, line in enumerate(stream_resp.iter_lines()):
            if i > 20:  # 只读前20行
                break
            if line:
                line_str = line.decode('utf-8', errors='replace')
                content += line_str + "\n"
        print(content[:500])
        
except Exception as e:
    print(f"Error: {e}")
    import traceback
    traceback.print_exc()