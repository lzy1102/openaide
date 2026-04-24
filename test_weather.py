import json, sys, urllib.request

# Test 1: Create dialogue
data = json.dumps({"user_id": "test_user", "title": "Tool Test"}).encode()
req = urllib.request.Request("http://localhost:19375/api/dialogues", data=data, headers={"Content-Type": "application/json"})
resp = urllib.request.urlopen(req)
result = json.loads(resp.read())
dialogue_id = result.get("id", result.get("ID", ""))
print(f"Dialogue created: {dialogue_id}")

# Test 2: Send message via /api/chat/tools (with tool calling)
data = json.dumps({
    "model_id": "gpt-5-mini",
    "user_id": "test_user",
    "dialogue_id": dialogue_id,
    "content": "帮我查一下北京今天的天气怎么样"
}).encode()
req = urllib.request.Request("http://localhost:19375/api/chat/tools", data=data, headers={"Content-Type": "application/json"})
try:
    resp = urllib.request.urlopen(req, timeout=120)
    result = json.loads(resp.read())
    print(f"\n=== Weather Test Result ===")
    if isinstance(result, dict):
        content = result.get("content", result.get("Content", str(result)))
        print(f"Response: {content[:500]}")
    else:
        print(f"Response: {str(result)[:500]}")
except Exception as e:
    print(f"Error: {e}")
