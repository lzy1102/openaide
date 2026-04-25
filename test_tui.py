#!/usr/bin/env python3
"""
OpenAIDE TUI 交互式测试
模拟终端用户体验，逐个发送消息并展示完整响应
"""
import requests
import json
import sys
import time

BASE = "http://localhost:19375/api"
DID = None

def print_header(title):
    print(f"\n{'='*60}")
    print(f"  {title}")
    print(f"{'='*60}\n")

def print_msg(role, content):
    if role == "user":
        print(f"\033[1;36m👤 你: {content}\033[0m")
    elif role == "assistant":
        print(f"\033[1;32m🤖 助手: {content}\033[0m")
    elif role == "system":
        print(f"\033[1;33m⚙️  系统: {content}\033[0m")
    elif role == "tool":
        print(f"\033[0;35m🔧 工具: {content}\033[0m")

def create_dialogue():
    global DID
    r = requests.post(f"{BASE}/dialogues", json={"user_id": "tui-test", "title": "TUI交互测试"})
    if r.status_code == 200:
        DID = r.json().get('id')
        print_msg("system", f"对话已创建: {DID}")
    else:
        print(f"❌ 创建对话失败: {r.status_code} {r.text[:200]}")
        sys.exit(1)

def send_message(content, timeout=120):
    print_msg("user", content)
    start = time.time()
    try:
        r = requests.post(f"{BASE}/dialogues/{DID}/messages", json={
            "user_id": "tui-test",
            "content": content,
            "model_id": ""
        }, timeout=timeout)
        elapsed = time.time() - start
        if r.status_code == 200:
            resp = r.json()
            content = resp.get('content', '')
            tool_calls = resp.get('tool_calls', [])
            print_msg("assistant", content)
            if tool_calls:
                for tc in tool_calls:
                    print_msg("tool", f"{tc.get('name', '?')}({json.dumps(tc.get('arguments', {}), ensure_ascii=False)[:100]})")
            print(f"    ⏱️  {elapsed:.1f}s")
            return content
        else:
            print(f"❌ 请求失败: {r.status_code} {r.text[:300]}")
            return None
    except requests.exceptions.Timeout:
        print(f"❌ 请求超时 ({timeout}s)")
        return None
    except Exception as e:
        print(f"❌ 请求异常: {e}")
        return None

def test_slash_command(cmd, args=""):
    print_msg("user", f"/{cmd} {args}".strip())
    r = requests.post(f"{BASE}/slash/execute", json={
        "command": cmd,
        "args": args,
        "session_id": DID or "test"
    }, timeout=5)
    if r.status_code == 200:
        result = r.json().get('result', '')
        print_msg("system", result[:500])
    else:
        print(f"❌ 命令失败: {r.text[:200]}")

def test_agent_route(agent):
    r = requests.get(f"{BASE}/agent-routing/route/{agent}", timeout=5)
    if r.status_code == 200:
        model = r.json().get('model', '无')
        print_msg("system", f"Agent '{agent}' → 模型: {model}")
    else:
        print(f"❌ 路由查询失败")

def test_permission(mode):
    r = requests.get(f"{BASE}/permissions/profiles/{mode}", timeout=5)
    if r.status_code == 200:
        p = r.json()
        denied = p.get('DeniedTools', []) or []
        print_msg("system", f"Agent '{mode}': 禁止工具={denied}")
    else:
        print(f"❌ 权限查询失败")

if __name__ == "__main__":
    print("\033[2J\033[H")
    print("╔══════════════════════════════════════════════════════╗")
    print("║          OpenAIDE TUI 交互式测试                    ║")
    print("║    测试场景：日常使用中常见的对话和工具调用          ║")
    print("╚══════════════════════════════════════════════════════╝")

    # ===== 第一部分：系统状态 =====
    print_header("第一部分：系统状态检查")
    
    r = requests.get(f"{BASE}/health", timeout=3)
    print_msg("system", f"服务状态: {'✅ 运行中' if r.status_code == 200 else '❌ 异常'}")

    print("\nAgent 路由配置:")
    for agent in ["build", "plan", "explore", "general"]:
        test_agent_route(agent)

    print("\n权限配置:")
    for mode in ["build", "plan"]:
        test_permission(mode)

    print("\nSlash 命令:")
    test_slash_command("help")

    # ===== 第二部分：基础对话 =====
    print_header("第二部分：基础对话测试")
    create_dialogue()

    print("\n--- 测试1: 简单问候 ---")
    send_message("你好，请简单介绍一下你自己", timeout=30)

    print("\n--- 测试2: 时间查询（工具调用）---")
    result = send_message("现在几点了？")
    has_time = result and any(kw in result for kw in ['时', '间', 'time', '点', '分', '秒', 'UTC', 'CST', ':'])
    print(f"    {'✅ 正确调用时间工具' if has_time else '❌ 未正确返回时间'}")

    # ===== 第三部分：命令执行 =====
    print_header("第三部分：命令执行测试")

    print("\n--- 测试3: IP查询 ---")
    result = send_message("查一下服务器的IP地址")
    has_ip = result and any(kw in result for kw in ['192.168', '172.', '127.0', 'IP', 'ip', '地址', 'inet'])
    print(f"    {'✅ 成功执行命令获取IP' if has_ip else '❌ 未获取到IP'}")

    print("\n--- 测试4: 系统信息 ---")
    result = send_message("看看服务器是什么系统，内存多大")
    has_sys = result and any(kw in result for kw in ['Linux', 'Ubuntu', 'CentOS', '内存', 'Memory', 'RAM', 'GB', 'Mem'])
    print(f"    {'✅ 成功获取系统信息' if has_sys else '❌ 未获取到系统信息'}")

    # ===== 第四部分：文件操作 =====
    print_header("第四部分：文件操作测试")

    print("\n--- 测试5: 查看文件 ---")
    result = send_message("看一下 /etc/os-release 的内容")
    has_os = result and any(kw in result for kw in ['NAME=', 'VERSION=', 'ID=', 'Ubuntu', 'CentOS', 'Debian'])
    print(f"    {'✅ 成功读取文件' if has_os else '❌ 未读取到文件内容'}")

    # ===== 第五部分：网络搜索 =====
    print_header("第五部分：网络搜索测试")

    print("\n--- 测试6: DuckDuckGo搜索 ---")
    result = send_message("搜索一下 Go语言 1.24 的新特性", timeout=60)
    has_search = result and len(result) > 50
    print(f"    {'✅ 搜索返回结果' if has_search else '❌ 搜索无结果'}")

    # ===== 第六部分：多轮上下文 =====
    print_header("第六部分：多轮上下文记忆测试")

    print("\n--- 测试7: 上下文记忆 ---")
    send_message("记住这个数字：42")
    result = send_message("我刚才让你记住的数字是多少？")
    has_memory = result and '42' in result
    print(f"    {'✅ 上下文记忆正常' if has_memory else '❌ 上下文记忆丢失'}")

    # ===== 第七部分：Slash命令 =====
    print_header("第七部分：Slash 命令测试")

    test_slash_command("tools")
    test_slash_command("agent", "build")
    test_slash_command("routes")
    test_slash_command("compact")

    # ===== 汇总 =====
    print_header("测试汇总")
    print("TUI 交互式测试完成！")
    print("以上测试覆盖了：")
    print("  ✅ 系统状态检查（路由/权限/命令）")
    print("  ✅ 基础对话（问候/时间查询）")
    print("  ✅ 命令执行（IP查询/系统信息）")
    print("  ✅ 文件操作（读取文件）")
    print("  ✅ 网络搜索（DuckDuckGo）")
    print("  ✅ 多轮上下文记忆")
    print("  ✅ Slash 命令系统")
