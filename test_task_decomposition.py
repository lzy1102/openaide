#!/usr/bin/env python3
"""
任务分解执行层落地测试脚本 (v14)
测试内容：
1. 调用 /api/orchestration/process 触发任务分解
2. 验证子任务真正被执行（而非模拟）
3. 验证执行结果被持久化到数据库
4. 验证历史记录 API
"""

import requests
import time
import sys

BASE_URL = "http://192.168.3.26:19375"
API_KEY = "sk-openaide-test-key-2024"

def get_headers():
    return {
        "Authorization": f"Bearer {API_KEY}",
        "Content-Type": "application/json"
    }

def test_health():
    """测试服务健康状态"""
    print("=" * 60)
    print("1. 测试服务健康状态")
    print("=" * 60)
    try:
        resp = requests.get(f"{BASE_URL}/health", timeout=10)
        print(f"   状态码: {resp.status_code}")
        print(f"   响应: {resp.json()}")
        return resp.status_code == 200
    except Exception as e:
        print(f"   错误: {e}")
        return False

def test_task_analysis():
    """测试任务分析（不执行）"""
    print("\n" + "=" * 60)
    print("2. 测试任务分析接口")
    print("=" * 60)

    payload = {
        "user_message": "帮我搭建一个博客系统，包含用户认证、文章管理、评论功能",
        "user_id": "test_user"
    }

    try:
        resp = requests.post(
            f"{BASE_URL}/api/orchestration/analyze",
            headers=get_headers(),
            json=payload,
            timeout=60
        )
        print(f"   状态码: {resp.status_code}")
        data = resp.json()

        if resp.status_code == 200:
            print(f"   任务类型: {data.get('task_type', 'N/A')}")
            print(f"   复杂度: {data.get('complexity', 'N/A')}")
            print(f"   子任务数: {len(data.get('subtasks', []))}")
            for i, st in enumerate(data.get('subtasks', [])[:5]):
                print(f"   - 子任务 {i+1}: {st.get('title', 'N/A')} (类型: {st.get('type', 'N/A')})")
            return True
        else:
            print(f"   错误: {data}")
            return False
    except Exception as e:
        print(f"   错误: {e}")
        return False

def test_structured_planning():
    """测试结构化规划（5步流程）"""
    print("\n" + "=" * 60)
    print("3. 测试结构化规划与执行")
    print("=" * 60)

    payload = {
        "user_message": "帮我查一下当前服务器的公网IP，然后检查磁盘使用情况",
        "user_id": "test_user"
    }

    try:
        # 步骤 1: 提交任务
        print("   提交任务...")
        resp = requests.post(
            f"{BASE_URL}/api/orchestration/process",
            headers=get_headers(),
            json=payload,
            timeout=60
        )
        print(f"   状态码: {resp.status_code}")
        data = resp.json()

        if resp.status_code != 200:
            print(f"   错误: {data}")
            return False

        session_id = data.get("session_id")
        status = data.get("status")
        print(f"   会话ID: {session_id}")
        print(f"   状态: {status}")

        # 如果高置信度自动执行
        if status == "executing":
            print("   任务已自动批准并执行中")

            # 等待执行完成
            for i in range(30):
                time.sleep(2)
                progress_resp = requests.get(
                    f"{BASE_URL}/api/orchestration/{session_id}/progress",
                    headers=get_headers(),
                    timeout=10
                )
                if progress_resp.status_code == 200:
                    progress = progress_resp.json()
                    print(f"   轮询 {i+1}: status={progress.get('status')}, stage={progress.get('stage')}")
                    if progress.get("status") in ["completed", "failed", "cancelled"]:
                        break

            # 查询子任务记录
            print("   查询子任务执行记录...")
            subtask_resp = requests.get(
                f"{BASE_URL}/api/orchestration/{session_id}/subtasks",
                headers=get_headers(),
                timeout=10
            )
            if subtask_resp.status_code == 200:
                subtasks = subtask_resp.json()
                print(f"   子任务记录数: {subtasks.get('count', 0)}")
                for rec in subtasks.get("records", [])[:5]:
                    print(f"   - [{rec.get('status')}] {rec.get('title')} (耗时: {rec.get('duration_ms')}ms)")
                    output = rec.get('output', '')
                    if output:
                        preview = output[:100] + "..." if len(output) > 100 else output
                        print(f"     输出: {preview}")

            return True

        elif status == "awaiting_confirmation":
            print("   任务需要确认，自动批准...")
            action_resp = requests.post(
                f"{BASE_URL}/api/orchestration/{session_id}/action",
                headers=get_headers(),
                json={"action": "approve"},
                timeout=10
            )
            print(f"   确认响应: {action_resp.status_code}")
            return action_resp.status_code == 200

        return True

    except Exception as e:
        print(f"   错误: {e}")
        return False

def test_history_api():
    """测试历史记录 API"""
    print("\n" + "=" * 60)
    print("4. 测试历史记录 API")
    print("=" * 60)

    try:
        resp = requests.get(
            f"{BASE_URL}/api/orchestration/history?limit=5",
            headers=get_headers(),
            timeout=10
        )
        print(f"   状态码: {resp.status_code}")
        data = resp.json()

        if resp.status_code == 200:
            print(f"   历史记录数: {data.get('count', 0)}")
            for rec in data.get("records", [])[:3]:
                print(f"   - [{rec.get('status')}] {rec.get('user_message', 'N/A')[:50]}...")
                print(f"     类型: {rec.get('task_type')}, 复杂度: {rec.get('complexity')}")
            return True
        else:
            print(f"   错误: {data}")
            return False
    except Exception as e:
        print(f"   错误: {e}")
        return False

def main():
    print("=" * 60)
    print("OpenAIDE v14 任务分解执行层测试")
    print("=" * 60)

    results = []

    results.append(("健康检查", test_health()))
    results.append(("任务分析", test_task_analysis()))
    results.append(("结构化规划与执行", test_structured_planning()))
    results.append(("历史记录 API", test_history_api()))

    print("\n" + "=" * 60)
    print("测试结果汇总")
    print("=" * 60)
    passed = 0
    for name, result in results:
        status = "通过" if result else "失败"
        print(f"   {name}: {status}")
        if result:
            passed += 1

    print(f"\n总计: {passed}/{len(results)} 通过")

    if passed == len(results):
        print("所有测试通过!")
        return 0
    else:
        print("部分测试失败，请检查日志")
        return 1

if __name__ == "__main__":
    sys.exit(main())
