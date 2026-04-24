#!/usr/bin/env python3
import requests
import json
import sys

BASE = "http://localhost:19375/api"

def test_health():
    r = requests.get(f"{BASE}/health")
    print(f"Health: {r.status_code} - {r.text[:100]}")

def test_permission_profiles():
    r = requests.get(f"{BASE}/permissions/profiles")
    print(f"Permission Profiles: {r.status_code}")
    if r.status_code == 200:
        profiles = r.json()
        for p in profiles:
            print(f"  Agent: {p.get('Mode', 'N/A')} - {p.get('Description', 'N/A')}")
            print(f"    Denied tools: {p.get('DeniedTools', [])}")
    else:
        print(f"  Error: {r.text[:200]}")

def test_permission_profile_build():
    r = requests.get(f"{BASE}/permissions/profiles/build")
    print(f"Build Profile: {r.status_code}")
    if r.status_code == 200:
        profile = r.json()
        rules = profile.get('Rules', [])
        print(f"  Rules count: {len(rules)}")
        for rule in rules[:5]:
            print(f"    {rule.get('Permission')} {rule.get('Pattern')} -> {rule.get('Action')}")
    else:
        print(f"  Error: {r.text[:200]}")

def test_permission_profile_plan():
    r = requests.get(f"{BASE}/permissions/profiles/plan")
    print(f"Plan Profile: {r.status_code}")
    if r.status_code == 200:
        profile = r.json()
        denied = profile.get('DeniedTools', [])
        print(f"  Denied tools: {denied}")
    else:
        print(f"  Error: {r.text[:200]}")

def test_chat_with_tool():
    r = requests.post(f"{BASE}/dialogues", json={"user_id": "test", "title": "OpenCode v9 Test"})
    print(f"Create dialogue: {r.status_code}")
    if r.status_code != 200:
        print(f"  Error: {r.text[:200]}")
        return
    
    did = r.json().get('id')
    print(f"  Dialogue ID: {did}")

    r = requests.post(f"{BASE}/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "查一下当前服务器IP地址",
        "model_id": ""
    }, timeout=120)
    print(f"Send message: {r.status_code}")
    if r.status_code == 200:
        data = r.json()
        content = data.get('content', '')[:500]
        print(f"  Response: {content}")
    else:
        print(f"  Error: {r.text[:500]}")

if __name__ == "__main__":
    print("=== OpenCode v9 Integration Test ===")
    print()
    
    print("1. Health check...")
    test_health()
    print()
    
    print("2. Permission profiles...")
    test_permission_profiles()
    print()
    
    print("3. Build agent profile...")
    test_permission_profile_build()
    print()
    
    print("4. Plan agent profile...")
    test_permission_profile_plan()
    print()
    
    print("5. Chat with tool calling...")
    test_chat_with_tool()
    print()
    
    print("=== Test Complete ===")
