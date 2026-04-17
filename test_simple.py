#!/usr/bin/env python3
import urllib.request
import json
import time

BASE_URL = "http://localhost:19375/api"

def test_models():
    """测试模型列表"""
    print("=== 1. 测试模型列表 ===")
    try:
        resp = urllib.request.urlopen(f"{BASE_URL}/models", timeout=5)
        data = json.loads(resp.read().decode())
        print(f"✓ 找到 {len(data)} 个模型")
        for m in data:
            if m.get('status') == 'enabled':
                print(f"  - {m['name']} ({m['provider']})")
        return True
    except Exception as e:
        print(f"✗ 错误: {e}")
        return False

def test_chat():
    """测试聊天功能"""
    print("\n=== 2. 测试聊天功能 ===")
    
    # 创建对话
    try:
        req = urllib.request.Request(
            f"{BASE_URL}/dialogues",
            data=json.dumps({"user_id": "test", "title": "Test"}).encode(),
            headers={"Content-Type": "application/json"}
        )
        resp = urllib.request.urlopen(req, timeout=5)
        dialogue = json.loads(resp.read().decode())
        dialogue_id = dialogue['id']
        print(f"✓ 创建对话: {dialogue_id[:8]}...")
    except Exception as e:
        print(f"✗ 创建对话失败: {e}")
        return False
    
    # 发送消息（非流式）
    print("\n发送消息: '你好'")
    try:
        req = urllib.request.Request(
            f"{BASE_URL}/dialogues/{dialogue_id}/messages",
            data=json.dumps({"user_id": "test", "content": "你好", "model_id": ""}).encode(),
            headers={"Content-Type": "application/json"}
        )
        start = time.time()
        resp = urllib.request.urlopen(req, timeout=180)
        result = json.loads(resp.read().decode())
        elapsed = time.time() - start
        content = result.get('content', '无内容')
        print(f"✓ 收到回复 ({elapsed:.1f}s): {content[:100]}...")
    except Exception as e:
        print(f"✗ 发送消息失败: {e}")
        return False
    
    # 流式请求
    print("\n流式请求: '1+1=?'")
    try:
        req = urllib.request.Request(
            f"{BASE_URL}/dialogues/{dialogue_id}/stream",
            data=json.dumps({"user_id": "test", "content": "1+1=?", "model_id": ""}).encode(),
            headers={"Content-Type": "application/json"}
        )
        start = time.time()
        resp = urllib.request.urlopen(req, timeout=180)
        
        events = []
        full_content = ""
        for line in resp:
            line = line.decode().strip()
            if line.startswith("data: "):
                data = line[6:]
                if data == "[DONE]":
                    break
                try:
                    event = json.loads(data)
                    events.append(event)
                    if event.get('type') == 'content':
                        full_content += event.get('content', '')
                        print(event.get('content', ''), end='', flush=True)
                except:
                    pass
        
        elapsed = time.time() - start
        print(f"\n✓ 流式完成 ({elapsed:.1f}s), {len(events)} 个事件")
        print(f"  完整内容: {full_content[:100]}...")
    except Exception as e:
        print(f"✗ 流式请求失败: {e}")
        return False
    
    return True

if __name__ == "__main__":
    print("开始测试 OpenAIDE...\n")
    test_models()
    test_chat()
    print("\n=== 测试完成 ===")
