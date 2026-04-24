import json, urllib.request
data = json.dumps({"name": "execute_command", "arguments": {"command": "echo hello"}}).encode()
req = urllib.request.Request("http://localhost:19375/api/tools/execute", data=data, headers={"Content-Type": "application/json"})
try:
    resp = urllib.request.urlopen(req, timeout=30)
    print(resp.read().decode()[:500])
except Exception as e:
    body = e.read().decode() if hasattr(e, 'read') else str(e)
    print(f"Error: {e}")
    print(f"Body: {body[:500]}")
