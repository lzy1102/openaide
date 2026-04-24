#!/usr/bin/env python3
import requests
import json
import sys

BASE = "http://localhost:19375/api"

def test_create_dialogue():
    r = requests.post(f"{BASE}/dialogues", json={"user_id": "test", "title": "Hermes v8 Test"})
    print(f"Create dialogue: {r.status_code}")
    if r.status_code == 200:
        data = r.json()
        print(f"  ID: {data.get('id', 'N/A')}")
        return data.get('id')
    else:
        print(f"  Error: {r.text[:200]}")
    return None

def test_send_message(dialogue_id):
    if not dialogue_id:
        print("No dialogue ID, skip message test")
        return
    r = requests.post(f"{BASE}/dialogues/{dialogue_id}/messages", json={
        "user_id": "test",
        "content": "查一下当前服务器的IP地址",
        "model_id": ""
    }, timeout=120)
    print(f"Send message: {r.status_code}")
    if r.status_code == 200:
        data = r.json()
        content = data.get('content', '')[:500]
        print(f"  Response: {content}")
    else:
        print(f"  Error: {r.text[:500]}")

def test_send_simple(dialogue_id):
    if not dialogue_id:
        print("No dialogue ID, skip simple test")
        return
    r = requests.post(f"{BASE}/dialogues/{dialogue_id}/messages", json={
        "user_id": "test",
        "content": "现在几点了？",
        "model_id": ""
    }, timeout=60)
    print(f"Simple message: {r.status_code}")
    if r.status_code == 200:
        data = r.json()
        content = data.get('content', '')[:500]
        print(f"  Response: {content}")
    else:
        print(f"  Error: {r.text[:500]}")

if __name__ == "__main__":
    print("=== Hermes v8 Integration Test ===")
    print()
    
    print("1. Creating dialogue...")
    did = test_create_dialogue()
    print()
    
    print("2. Testing simple message (time query)...")
    test_send_simple(did)
    print()
    
    print("3. Testing tool-calling message (IP query)...")
    test_send_message(did)
    print()
    
    print("=== Test Complete ===")
