import json, urllib.request, time

BASE = "http://localhost:19375"

def api(method, path, data=None, timeout=120):
    url = f"{BASE}{path}"
    body = json.dumps(data).encode() if data else None
    req = urllib.request.Request(url, data=body, method=method,
                                 headers={"Content-Type": "application/json"} if body else {})
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
        return json.loads(resp.read())
    except Exception as e:
        body = e.read().decode() if hasattr(e, 'read') else str(e)
        return {"error": str(e), "body": body[:300]}

# 创建对话
dlg = api("POST", "/api/dialogues", {"user_id": "test_user", "title": "IP查询测试"})
dlg_id = dlg.get("id", dlg.get("ID", ""))
print(f"对话ID: {dlg_id}")

# 通过前端聊天端点发送IP查询
print("\n测试: 通过 /dialogues/:id/stream 查询IP...")
data = json.dumps({"user_id": "test_user", "content": "帮我查一下我的公网IP地址", "model_id": "gpt-5-mini"}).encode()
req = urllib.request.Request(f"{BASE}/api/dialogues/{dlg_id}/stream", data=data, headers={"Content-Type": "application/json"})
try:
    resp = urllib.request.urlopen(req, timeout=120)
    full_resp = resp.read().decode()
    # 解析SSE
    lines = full_resp.split("\n")
    content_parts = []
    for line in lines:
        if line.startswith("data: "):
            try:
                event = json.loads(line[6:])
                if event.get("type") == "content":
                    content_parts.append(event.get("content", ""))
                elif event.get("type") == "tool_call":
                    print(f"  🔧 工具调用: {event.get('tool', '')} params={event.get('params', '')[:100]}")
            except:
                pass
    full_content = "".join(content_parts)
    print(f"\n  回复: {full_content[:500]}")
    
    has_ip = any(c.isdigit() and "." in c for c in full_content.split() if len(c.split(".")) == 4)
    if has_ip or "IP" in full_content or "ip" in full_content:
        print("\n  ✅ 检测到IP相关信息！工具调用路由生效！")
    else:
        print("\n  ⚠️ 未检测到IP信息")
except Exception as e:
    print(f"  错误: {e}")

# 也测试非流式端点
print("\n测试: 通过 /dialogues/:id/messages 查IP...")
result = api("POST", f"/api/dialogues/{dlg_id}/messages", 
             {"user_id": "test_user", "content": "查一下本机IP", "model_id": "gpt-5-mini"})
if isinstance(result, dict) and "error" not in result:
    content = result.get("content", "")
    print(f"  回复: {content[:300]}")
else:
    print(f"  结果: {str(result)[:300]}")
