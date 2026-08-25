import json, urllib.request, sys, os

KEY = [l.split(':',1)[1].strip() for l in open(os.path.expanduser('~/.openaide/config.yaml'),encoding='utf8') if 'api_key' in l][0]
URL = 'https://opencode.ai/zen/go/v1/chat/completions'
Q = "单词 strawberry 里有几个字母 r？给出推理过程"

def ask(label, extra):
    body = {"model": "ox-alpha-free",
            "messages": [{"role": "user", "content": Q}],
            "max_tokens": 4000, **extra}
    req = urllib.request.Request(URL, data=json.dumps(body).encode(),
        headers={'authorization': f'Bearer {KEY}', 'content-type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=150) as r:
            d = json.loads(r.read())
        msg = d['choices'][0]['message']
        rt = d.get('usage', {}).get('completion_tokens_details', {}).get('reasoning_tokens')
        content = (msg.get('content') or '').replace('\n', ' ')
        correct = content.count('3') > 0 and 'r' in content.lower()
        print(f"[{label:6}] reasoning_tokens={rt:>4} | answer: {content[:90]}")
    except Exception as e:
        print(f"[{label:6}] FAILED: {e}")

ask('default', {})
ask('max', {'reasoning_effort': 'max'})
