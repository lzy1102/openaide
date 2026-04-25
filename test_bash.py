import requests
import json
import sys

BASE = 'http://127.0.0.1:19375'

def create_dialogue():
    r = requests.post(f'{BASE}/api/dialogues', json={'title': 'bash test'})
    if r.status_code == 200:
        data = r.json()
        return data.get('id') or data.get('data', {}).get('id')
    return None

def send_message(did, content):
    r = requests.post(f'{BASE}/api/dialogues/{did}/messages', json={'content': content}, timeout=120)
    if r.status_code == 200:
        data = r.json()
        if isinstance(data, dict) and 'data' in data:
            d = data['data']
            if isinstance(d, dict):
                return d.get('content') or d.get('response') or json.dumps(d, ensure_ascii=False)[:500]
            return str(d)[:500]
        return json.dumps(data, ensure_ascii=False)[:500]
    return f'HTTP {r.status_code}: {r.text[:200]}'

print('=== Bash Command Test ===')
did = create_dialogue()
if not did:
    print('FAIL: Cannot create dialogue')
    sys.exit(1)

print('\n[1] Testing: ls -la /tmp')
resp1 = send_message(did, 'Please execute command: ls -la /tmp and tell me the result')
print(f'Response: {resp1[:500]}')

print('\n[2] Testing: echo hello')
did2 = create_dialogue()
resp2 = send_message(did2, 'Please execute bash command: echo hello world')
print(f'Response: {resp2[:500]}')

print('\n[3] Testing: pwd')
did3 = create_dialogue()
resp3 = send_message(did3, 'Please tell me the current working directory (run pwd)')
print(f'Response: {resp3[:500]}')

print('\n=== Test Complete ===')
