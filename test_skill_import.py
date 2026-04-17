#!/usr/bin/env python3
"""测试 SKILL.md 导入功能"""
import json
import urllib.request

BASE_URL = "http://localhost:19375/api"

def test_validate_skill_md():
    """测试验证 SKILL.md"""
    print("=== 1. 测试验证 SKILL.md ===")
    
    skill_content = """
---
name: test-skill
description: |
  这是一个测试技能。
  用于验证 SKILL.md 格式。

model-preference: code

triggers:
  - "测试"
  - "test"

parameters:
  - name: input
    type: string
    description: 输入内容
    required: true

metadata:
  author: "Test"
  version: "1.0.0"
  category: "test"
---

# Test Skill

这是一个测试技能的说明文档。
"""
    
    req = urllib.request.Request(
        f"{BASE_URL}/skills/validate",
        data=json.dumps({"content": skill_content}).encode(),
        headers={"Content-Type": "application/json"}
    )
    
    try:
        resp = urllib.request.urlopen(req, timeout=10)
        result = json.loads(resp.read().decode())
        print(f"✓ 验证结果: {result}")
        return True
    except urllib.error.HTTPError as e:
        result = json.loads(e.read().decode())
        print(f"✗ 验证失败: {result}")
        return False

def test_import_skill():
    """测试导入技能"""
    print("\n=== 2. 测试导入技能 ===")
    
    skill_content = """
---
name: sql-optimizer
description: |
  优化SQL查询性能，提供索引建议和查询重写。
  当用户提交慢查询或请求SQL优化时触发。

model-preference: code

triggers:
  - "优化SQL"
  - "SQL优化"
  - "慢查询"

parameters:
  - name: database_type
    type: enum
    description: 数据库类型
    required: false
    default: "mysql"
    values:
      - mysql
      - postgres
      - sqlite

metadata:
  author: "OpenAIDE"
  version: "1.0.0"
  category: "database"
  tags:
    - sql
    - performance
---

# SQL Optimizer

优化SQL查询性能，识别瓶颈并提供优化方案。

## 工作流程
1. 使用 `EXPLAIN` 分析查询计划
2. 识别全表扫描、缺少索引等问题
3. 提供索引建议
4. 重写查询（如可能）
"""
    
    req = urllib.request.Request(
        f"{BASE_URL}/skills/import",
        data=json.dumps({"content": skill_content}).encode(),
        headers={"Content-Type": "application/json"}
    )
    
    try:
        resp = urllib.request.urlopen(req, timeout=10)
        result = json.loads(resp.read().decode())
        print(f"✓ 导入成功: {result.get('message')}")
        skill = result.get('skill', {})
        print(f"  - ID: {skill.get('id', 'N/A')[:8]}...")
        print(f"  - Name: {skill.get('name')}")
        print(f"  - Parameters: {len(result.get('parameters', []))}")
        return skill.get('id')
    except urllib.error.HTTPError as e:
        result = json.loads(e.read().decode())
        print(f"✗ 导入失败: {result}")
        return None

def test_export_skill(skill_id):
    """测试导出技能"""
    print(f"\n=== 3. 测试导出技能 ===")
    
    if not skill_id:
        print("✗ 跳过（没有技能ID）")
        return
    
    try:
        resp = urllib.request.urlopen(f"{BASE_URL}/skills/{skill_id}/export", timeout=10)
        result = json.loads(resp.read().decode())
        if result.get('success'):
            print(f"✓ 导出成功")
            print("--- 导出内容预览 ---")
            content = result.get('content', '')
            print(content[:500] + "..." if len(content) > 500 else content)
        else:
            print(f"✗ 导出失败: {result}")
    except urllib.error.HTTPError as e:
        result = json.loads(e.read().decode())
        print(f"✗ 导出失败: {result}")

def test_list_skills():
    """测试列出技能"""
    print("\n=== 4. 测试列出技能 ===")
    
    try:
        resp = urllib.request.urlopen(f"{BASE_URL}/skills", timeout=10)
        result = json.loads(resp.read().decode())
        skills = result.get('data', [])
        print(f"✓ 找到 {len(skills)} 个技能")
        for skill in skills[-3:]:  # 显示最后3个
            print(f"  - {skill.get('name')} ({skill.get('category', 'N/A')})")
    except Exception as e:
        print(f"✗ 列出失败: {e}")

if __name__ == "__main__":
    print("开始测试 SKILL.md 导入功能...\n")
    
    # 测试验证
    test_validate_skill_md()
    
    # 测试导入
    skill_id = test_import_skill()
    
    # 测试导出
    test_export_skill(skill_id)
    
    # 测试列出
    test_list_skills()
    
    print("\n=== 测试完成 ===")
