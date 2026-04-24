#!/usr/bin/env python3
"""OpenAIDE 综合测试脚本 - 测试工具调用、任务分析、任务拆解"""
import json
import sys
import urllib.request
import urllib.error
import time

BASE_URL = "http://localhost:19375"
MODEL_ID = "gpt-5-mini"
USER_ID = "test_user_comprehensive"

def api_call(method, path, data=None, timeout=120):
    url = f"{BASE_URL}{path}"
    body = json.dumps(data).encode() if data else None
    req = urllib.request.Request(url, data=body, method=method,
                                 headers={"Content-Type": "application/json"} if body else {})
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
        return json.loads(resp.read())
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        return {"error": f"HTTP {e.code}", "detail": body[:500]}
    except Exception as e:
        return {"error": str(e)}

def print_section(title):
    print(f"\n{'='*60}")
    print(f"  {title}")
    print(f"{'='*60}")

def test_1_weather_query():
    """测试1：查询天气（工具调用链路）"""
    print_section("测试1：查询天气 - 工具调用链路测试")
    
    # 1.1 创建对话
    print("\n[1.1] 创建对话...")
    result = api_call("POST", "/api/dialogues", {"user_id": USER_ID, "title": "天气查询测试"})
    if "error" in result:
        print(f"  ❌ 创建对话失败: {result}")
        return False
    dialogue_id = result.get("id", result.get("ID", ""))
    print(f"  ✅ 对话创建成功: {dialogue_id}")
    
    # 1.2 查询工具定义
    print("\n[1.2] 查询可用工具定义...")
    tools = api_call("GET", "/api/tools/definitions")
    if isinstance(tools, list):
        tool_names = [t.get("name", t.get("function", {}).get("name", "?")) for t in tools]
        print(f"  ✅ 可用工具 ({len(tools)}个): {tool_names}")
        weather_tool = any("weather" in str(t).lower() for t in tools)
        command_tool = any("command" in str(t).lower() or "execute_command" in str(t).lower() for t in tools)
        print(f"  天气工具: {'✅ 存在' if weather_tool else '❌ 缺失'}")
        print(f"  命令工具: {'✅ 存在' if command_tool else '❌ 缺失'}")
    else:
        print(f"  ⚠️ 工具查询结果: {str(tools)[:200]}")
    
    # 1.3 通过 /api/chat/tools 发送天气查询
    print("\n[1.3] 发送天气查询请求 (POST /api/chat/tools)...")
    start_time = time.time()
    result = api_call("POST", "/api/chat/tools", {
        "model_id": MODEL_ID,
        "user_id": USER_ID,
        "dialogue_id": dialogue_id,
        "content": "帮我查一下北京今天的天气怎么样"
    }, timeout=120)
    elapsed = time.time() - start_time
    
    if "error" in result:
        print(f"  ❌ 天气查询失败: {result}")
        return False
    
    print(f"  ⏱️ 响应时间: {elapsed:.1f}s")
    
    # 分析响应结构
    if isinstance(result, dict):
        content = result.get("content", result.get("Content", ""))
        role = result.get("role", result.get("Role", ""))
        tool_calls = result.get("tool_calls", result.get("ToolCalls", []))
        
        print(f"  角色: {role}")
        print(f"  工具调用: {len(tool_calls) if tool_calls else 0}个")
        if tool_calls:
            for tc in tool_calls[:3]:
                func = tc.get("function", tc.get("Function", {}))
                print(f"    - {func.get('name', '?')}: {func.get('arguments', '')[:100]}")
        
        print(f"\n  📝 回复内容:")
        if isinstance(content, str) and len(content) > 0:
            print(f"  {content[:500]}")
        else:
            print(f"  {str(result)[:500]}")
        
        # 检查是否真正调用了天气工具
        has_weather_info = any(kw in str(content).lower() for kw in ["天气", "温度", "weather", "°", "℃", "晴", "雨", "阴", "cloud", "sun", "rain"])
        if has_weather_info:
            print(f"\n  ✅ 天气信息已获取！")
        else:
            print(f"\n  ⚠️ 未检测到天气信息，可能是LLM未调用工具而是直接回答")
    else:
        print(f"  响应: {str(result)[:500]}")
    
    return True

def test_2_task_analysis():
    """测试2：任务分析（Orchestration 分析端点）"""
    print_section("测试2：任务分析 - Orchestration 分析")
    
    print("\n[2.1] 发送任务分析请求 (POST /api/orchestration/analyze)...")
    start_time = time.time()
    result = api_call("POST", "/api/orchestration/analyze", {
        "user_message": "帮我开发一个博客系统，需要用户注册登录、文章发布、评论功能，使用Go语言和PostgreSQL",
        "user_id": USER_ID
    }, timeout=120)
    elapsed = time.time() - start_time
    
    print(f"  ⏱️ 响应时间: {elapsed:.1f}s")
    
    if isinstance(result, dict) and "error" not in result:
        print(f"  ✅ 任务分析完成")
        analysis = result.get("analysis", result)
        if isinstance(analysis, dict):
            print(f"  任务类型: {analysis.get('task_type', 'N/A')}")
            print(f"  复杂度: {analysis.get('complexity', 'N/A')}")
            print(f"  所需技能: {analysis.get('required_skills', 'N/A')}")
        print(f"\n  📝 分析结果:")
        print(f"  {json.dumps(result, ensure_ascii=False, indent=2)[:800]}")
    else:
        print(f"  ⚠️ 分析结果: {str(result)[:500]}")
    
    return True

def test_3_task_decomposition():
    """测试3：任务拆解（结构化规划）"""
    print_section("测试3：任务拆解 - 结构化规划")
    
    # 3.1 通过 /api/chat/plan 端点测试
    print("\n[3.1] 创建对话用于规划...")
    dlg = api_call("POST", "/api/dialogues", {"user_id": USER_ID, "title": "任务拆解测试"})
    if "error" in dlg:
        print(f"  ❌ 创建对话失败: {dlg}")
        dialogue_id = "test_plan_dlg"
    else:
        dialogue_id = dlg.get("id", dlg.get("ID", "test_plan_dlg"))
    print(f"  对话ID: {dialogue_id}")
    
    print("\n[3.2] 发送规划请求 (POST /api/chat/plan)...")
    start_time = time.time()
    result = api_call("POST", "/api/chat/plan", {
        "user_id": USER_ID,
        "dialogue_id": dialogue_id,
        "content": "帮我开发一个博客系统，需要用户注册登录、文章发布、评论功能"
    }, timeout=180)
    elapsed = time.time() - start_time
    
    print(f"  ⏱️ 响应时间: {elapsed:.1f}s")
    
    if isinstance(result, dict) and "error" not in result:
        print(f"  ✅ 规划完成")
        print(f"\n  📝 规划结果:")
        formatted = json.dumps(result, ensure_ascii=False, indent=2)
        print(f"  {formatted[:1500]}")
        if len(formatted) > 1500:
            print(f"  ... (截断，总长度 {len(formatted)} 字符)")
        
        # 分析规划结构
        phases = result.get("phases", [])
        if phases:
            print(f"\n  📊 规划结构分析:")
            print(f"  阶段数: {len(phases)}")
            total_subtasks = 0
            for phase in phases:
                subtasks = phase.get("subtasks", [])
                total_subtasks += len(subtasks)
                print(f"    阶段 [{phase.get('name', '?')}]: {len(subtasks)}个子任务")
                for st in subtasks[:3]:
                    print(f"      - {st.get('id', '?')}: {st.get('title', '?')} ({st.get('type', '?')})")
            print(f"  子任务总数: {total_subtasks}")
            
            deps = result.get("dependencies", [])
            print(f"  依赖关系: {len(deps)}个")
            for dep in deps[:5]:
                print(f"    {dep.get('from', '?')} → {dep.get('to', '?')} ({dep.get('type', '?')})")
            
            risks = result.get("risk_assessment", {})
            if risks:
                print(f"  整体风险: {risks.get('overall_risk', 'N/A')}")
                print(f"  风险项: {len(risks.get('top_risks', []))}个")
    else:
        print(f"  ⚠️ 规划结果: {str(result)[:500]}")
    
    # 3.2 通过 Orchestration process 端点测试完整流程
    print("\n[3.3] 测试完整编排流程 (POST /api/orchestration/process)...")
    start_time = time.time()
    result2 = api_call("POST", "/api/orchestration/process", {
        "user_message": "帮我开发一个博客系统，需要用户注册登录、文章发布、评论功能",
        "user_id": USER_ID
    }, timeout=180)
    elapsed2 = time.time() - start_time
    
    print(f"  ⏱️ 响应时间: {elapsed2:.1f}s")
    
    if isinstance(result2, dict) and "error" not in result2:
        print(f"  ✅ 编排完成")
        session_id = result2.get("session_id", "N/A")
        status = result2.get("status", "N/A")
        print(f"  会话ID: {session_id}")
        print(f"  状态: {status}")
        
        proposal = result2.get("proposal", {})
        if proposal:
            team = proposal.get("team", [])
            plan = proposal.get("plan", [])
            print(f"  团队成员: {len(team)}个")
            print(f"  计划步骤: {len(plan)}个")
        
        print(f"\n  📝 编排结果:")
        formatted2 = json.dumps(result2, ensure_ascii=False, indent=2)
        print(f"  {formatted2[:1000]}")
    else:
        print(f"  ⚠️ 编排结果: {str(result2)[:500]}")
    
    return True

def test_4_command_execution():
    """测试4：命令执行"""
    print_section("测试4：命令执行 - 工具直接调用")
    
    # 4.1 直接执行工具
    print("\n[4.1] 直接执行命令工具 (POST /api/tools/execute)...")
    start_time = time.time()
    result = api_call("POST", "/api/tools/execute", {
        "name": "execute_command",
        "arguments": {"command": "echo 'Hello from OpenAIDE!' && date && whoami"}
    }, timeout=30)
    elapsed = time.time() - start_time
    
    print(f"  ⏱️ 响应时间: {elapsed:.1f}s")
    if isinstance(result, dict):
        if "error" in result:
            print(f"  ⚠️ 执行结果: {result}")
        else:
            output = result.get("output", result.get("result", str(result)))
            print(f"  ✅ 命令执行成功")
            print(f"  输出: {str(output)[:300]}")
    else:
        print(f"  结果: {str(result)[:300]}")
    
    # 4.2 通过 chat/tools 执行命令
    print("\n[4.2] 通过对话执行命令 (POST /api/chat/tools)...")
    dlg = api_call("POST", "/api/dialogues", {"user_id": USER_ID, "title": "命令执行测试"})
    dialogue_id = dlg.get("id", dlg.get("ID", "")) if isinstance(dlg, dict) and "error" not in dlg else "cmd_test"
    
    start_time = time.time()
    result2 = api_call("POST", "/api/chat/tools", {
        "model_id": MODEL_ID,
        "user_id": USER_ID,
        "dialogue_id": dialogue_id,
        "content": "请执行命令 curl -s ifconfig.me 查询我的公网IP地址"
    }, timeout=120)
    elapsed2 = time.time() - start_time
    
    print(f"  ⏱️ 响应时间: {elapsed2:.1f}s")
    if isinstance(result2, dict) and "error" not in result2:
        content = result2.get("content", result2.get("Content", ""))
        print(f"  📝 回复: {str(content)[:500]}")
    else:
        print(f"  ⚠️ 结果: {str(result2)[:300]}")
    
    # 4.3 测试危险命令确认机制
    print("\n[4.3] 测试危险命令确认机制 (rm -rf)...")
    result3 = api_call("POST", "/api/tools/execute", {
        "name": "execute_command",
        "arguments": {"command": "rm -rf /tmp/test_dangerous"}
    }, timeout=30)
    
    if isinstance(result3, dict):
        output = str(result3.get("output", result3.get("result", "")))
        has_warning = "确认" in output or "confirm" in output.lower() or "⚠️" in output or "危险" in output or "风险" in output
        if has_warning:
            print(f"  ✅ 危险命令被拦截，需要确认")
            print(f"  提示: {output[:200]}")
        else:
            print(f"  ⚠️ 危险命令未被拦截")
            print(f"  结果: {str(result3)[:200]}")
    
    return True

def test_5_http_request_tool():
    """测试5：HTTP请求工具"""
    print_section("测试5：HTTP请求工具")
    
    print("\n[5.1] 执行HTTP请求工具 (POST /api/tools/execute)...")
    start_time = time.time()
    result = api_call("POST", "/api/tools/execute", {
        "name": "http_request",
        "arguments": {
            "url": "https://wttr.in/Beijing?format=3",
            "method": "GET"
        }
    }, timeout=30)
    elapsed = time.time() - start_time
    
    print(f"  ⏱️ 响应时间: {elapsed:.1f}s")
    if isinstance(result, dict):
        output = result.get("output", result.get("result", str(result)))
        print(f"  📝 结果: {str(output)[:300]}")
    else:
        print(f"  结果: {str(result)[:300]}")
    
    return True

def main():
    print("🚀 OpenAIDE 综合测试开始")
    print(f"服务器: {BASE_URL}")
    print(f"模型: {MODEL_ID}")
    print(f"时间: {time.strftime('%Y-%m-%d %H:%M:%S')}")
    
    # 健康检查
    print("\n📋 健康检查...")
    models = api_call("GET", "/api/models", timeout=10)
    if isinstance(models, list):
        print(f"  ✅ 服务正常，可用模型: {len(models)}个")
        for m in models:
            print(f"    - {m.get('name', '?')} ({m.get('status', '?')})")
    else:
        print(f"  ❌ 服务异常: {models}")
        sys.exit(1)
    
    results = {}
    
    # 执行所有测试
    try:
        results["weather"] = test_1_weather_query()
    except Exception as e:
        print(f"\n  ❌ 测试1异常: {e}")
        results["weather"] = False
    
    try:
        results["analysis"] = test_2_task_analysis()
    except Exception as e:
        print(f"\n  ❌ 测试2异常: {e}")
        results["analysis"] = False
    
    try:
        results["decomposition"] = test_3_task_decomposition()
    except Exception as e:
        print(f"\n  ❌ 测试3异常: {e}")
        results["decomposition"] = False
    
    try:
        results["command"] = test_4_command_execution()
    except Exception as e:
        print(f"\n  ❌ 测试4异常: {e}")
        results["command"] = False
    
    try:
        results["http"] = test_5_http_request_tool()
    except Exception as e:
        print(f"\n  ❌ 测试5异常: {e}")
        results["http"] = False
    
    # 测试总结
    print_section("测试总结")
    for name, passed in results.items():
        status = "✅ 通过" if passed else "❌ 失败"
        print(f"  {name}: {status}")
    
    total = len(results)
    passed = sum(1 for v in results.values() if v)
    print(f"\n  总计: {passed}/{total} 通过")
    
    return 0 if passed == total else 1

if __name__ == "__main__":
    sys.exit(main())
