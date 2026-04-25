import requests
import json
import sys
import re

BASE = 'http://127.0.0.1:19375'
PASS = 'PASS'
FAIL = 'FAIL'

def create_dialogue(title='test'):
    r = requests.post(f'{BASE}/api/dialogues', json={'title': title})
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

results = {}

print('=== Test 1: Context Memory ===')
did = create_dialogue('ctx test')
r1 = send_message(did, 'Remember: my name is Alice')
print(f'  Q1: Remember name=Alice -> {r1[:150]}')
r2 = send_message(did, 'What is my name?')
print(f'  Q2: What is my name? -> {r2[:150]}')
ok1 = 'Alice' in r2 or 'alice' in r2.lower()
print(f'  {PASS if ok1 else FAIL}: Context memory')
results['Context Memory'] = ok1

print('\n=== Test 2: Multi-turn Facts ===')
did2 = create_dialogue('multi-turn')
send_message(did2, 'I live in Shanghai and I work as a developer')
r3 = send_message(did2, 'Where do I live and what is my job?')
print(f'  Response: {r3[:200]}')
ok2 = ('shanghai' in r3.lower()) and ('developer' in r3.lower())
print(f'  {PASS if ok2 else FAIL}: Multi-turn facts')
results['Multi-turn Facts'] = ok2

print('\n=== Test 3: Tool Call (Time) ===')
did3 = create_dialogue('tool test')
r4 = send_message(did3, 'What time is it now?')
print(f'  Response: {r4[:200]}')
time_pat = re.compile(r'\d{1,2}[:]\d{2}')
ok3 = bool(time_pat.search(r4))
print(f'  {PASS if ok3 else FAIL}: Tool call (time)')
results['Tool Call'] = ok3

print('\n=== Test 4: Slash Commands API ===')
try:
    r5 = requests.get(f'{BASE}/api/slash/commands', timeout=5)
    ok4 = r5.status_code == 200
    if ok4:
        cmds = r5.json()
        print(f'  Commands: {json.dumps(cmds, ensure_ascii=False)[:200]}')
    else:
        print(f'  HTTP {r5.status_code}')
except Exception as e:
    ok4 = False
    print(f'  Error: {e}')
print(f'  {PASS if ok4 else FAIL}: Slash commands API')
results['Slash Commands'] = ok4

print('\n=== Test 5: Agent Routing API ===')
try:
    r6 = requests.get(f'{BASE}/api/agent-routing/routes', timeout=5)
    ok5 = r6.status_code == 200
    if ok5:
        print(f'  Routes: {r6.text[:200]}')
    else:
        print(f'  HTTP {r6.status_code}')
except Exception as e:
    ok5 = False
    print(f'  Error: {e}')
print(f'  {PASS if ok5 else FAIL}: Agent routing API')
results['Agent Routing'] = ok5

print('\n=== Test 6: Permission API ===')
try:
    r7 = requests.get(f'{BASE}/api/permissions/profiles', timeout=5)
    ok6 = r7.status_code == 200
    if ok6:
        print(f'  Profiles: {r7.text[:200]}')
    else:
        print(f'  HTTP {r7.status_code}')
except Exception as e:
    ok6 = False
    print(f'  Error: {e}')
print(f'  {PASS if ok6 else FAIL}: Permission API')
results['Permission API'] = ok6

print('\n' + '='*50)
print('SUMMARY')
print('='*50)
passed = 0
for name, ok in results.items():
    print(f'  {PASS if ok else FAIL} {name}')
    if ok:
        passed += 1
print(f'\nTotal: {passed}/{len(results)} passed')
if passed == len(results):
    print('\nAll tests PASSED!')
else:
    print(f'\n{len(results)-passed} test(s) FAILED')
