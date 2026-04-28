#!/usr/bin/env python3
import requests
import json
import time

BASE = "http://192.168.3.26:19375"

def safe_json(r):
    try:
        return r.json()
    except:
        return {"error": "not json", "text": r.text[:200], "status": r.status_code}

def test_react():
    print("=" * 70)
    print("Test ReAct State Machine and Session Recording")
    print("=" * 70)

    # 1. Check metrics
    print("\n[1] Check initial metrics:")
    r = requests.get(f"{BASE}/api/react/metrics")
    print(f"    Status: {r.status_code}")
    print(f"    Content-Type: {r.headers.get('content-type', 'unknown')}")
    data = safe_json(r)
    print(f"    Data: {json.dumps(data, indent=2)[:500]}")

    # 2. Check sessions
    print("\n[2] Check initial sessions:")
    r = requests.get(f"{BASE}/api/react/sessions")
    print(f"    Status: {r.status_code}")
    data = safe_json(r)
    print(f"    Data: {json.dumps(data, indent=2)[:500]}")

    # 3. Create dialogue
    print("\n[3] Create dialogue:")
    r = requests.post(f"{BASE}/api/dialogues", json={"user_id": "test", "title": "react test"})
    did = r.json()["id"]
    print(f"    Dialogue ID: {did}")

    # 4. Send message to trigger ReAct
    print("\n[4] Send message to trigger ReAct:")
    start = time.time()
    r = requests.post(f"{BASE}/api/dialogues/{did}/messages", json={
        "user_id": "test",
        "content": "查一下我的公网IP",
        "model_id": ""
    })
    elapsed = time.time() - start
    msg = r.json()
    print(f"    Elapsed: {elapsed:.2f}s")
    print(f"    Reply: {msg['content'][:200]}...")

    # 5. Check metrics again
    print("\n[5] Check updated metrics:")
    r = requests.get(f"{BASE}/api/react/metrics")
    data = safe_json(r)
    print(f"    Data: {json.dumps(data, indent=2)[:500]}")

    # 6. Check sessions
    print("\n[6] Check sessions:")
    r = requests.get(f"{BASE}/api/react/sessions")
    data = safe_json(r)
    sessions = data.get('sessions', []) if isinstance(data, dict) else []
    print(f"    Session count: {len(sessions)}")

    # 7. Export session if available
    if sessions:
        sid = sessions[0]
        print(f"\n[7] Export session {sid[:16]}...:")
        r = requests.get(f"{BASE}/api/react/sessions/{sid}/export")
        data = safe_json(r)
        if isinstance(data, dict) and 'error' not in data:
            print(f"    FinalState: {data.get('final_state', '')}")
            print(f"    TotalRounds: {data.get('total_rounds', 0)}")
            print(f"    TotalTokens: {data.get('total_tokens', 0)}")
            print(f"    Steps: {len(data.get('steps', []))}")
        else:
            print(f"    Error: {json.dumps(data, indent=2)[:300]}")

    print("\n" + "=" * 70)
    print("Test completed!")
    print("=" * 70)

if __name__ == "__main__":
    test_react()
