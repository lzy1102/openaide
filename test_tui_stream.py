#!/usr/bin/env python3
"""测试TUI流式输出功能"""
import subprocess
import sys

def test_tui_stream():
    """测试TUI是否能正确处理流式输出"""
    print("=== TUI 流式输出测试 ===")
    print("命令: echo 'hello' | .\\terminal\\openaide-tui.exe chat")
    print()

    # 使用管道输入测试TUI
    proc = subprocess.Popen(
        ["terminal\\openaide-tui.exe", "chat"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True
    )

    # 发送测试消息
    stdout, stderr = proc.communicate(input="你好\n", timeout=200)

    print("STDOUT:")
    print(stdout)
    print()
    print("STDERR:")
    print(stderr)
    print()
    print(f"返回码: {proc.returncode}")

if __name__ == "__main__":
    test_tui_stream()
