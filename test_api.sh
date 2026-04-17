#!/bin/bash
# 测试 OpenAIDE API

echo "=== 1. 检查服务器状态 ==="
curl -s http://localhost:19375/api/models | python3 -c "import sys,json; data=json.load(sys.stdin); print(f'可用模型: {len(data)}个'); [print(f'  - {m[\"name\"]} ({m[\"provider\"]})') for m in data if m.get('status')=='enabled']"

echo ""
echo "=== 2. 创建对话 ==="
DIALOGUE=$(curl -s -X POST http://localhost:19375/api/dialogues \
  -H "Content-Type: application/json" \
  -d '{"user_id":"test","title":"Test"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "对话ID: $DIALOGUE"

echo ""
echo "=== 3. 非流式请求测试 ==="
echo "发送: 1+1=?"
START=$(date +%s)
RESPONSE=$(curl -s -X POST "http://localhost:19375/api/dialogues/$DIALOGUE/messages" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"test","content":"1+1=?","model_id":""}' \
  -m 180)
END=$(date +%s)
echo "耗时: $((END-START))秒"
echo "响应: $(echo $RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['content'][:100])")"

echo ""
echo "=== 4. 流式请求测试 ==="
echo "发送: hi"
START=$(date +%s)
curl -s -X POST "http://localhost:19375/api/dialogues/$DIALOGUE/stream" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"test","content":"hi","model_id":""}' \
  -m 180 | while read line; do
    if [[ $line == data:* ]]; then
        DATA="${line:6}"
        if [ "$DATA" != "[DONE]" ]; then
            echo "$DATA" | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'[{d.get(\"type\",\"?\")}] {d.get(\"content\",\"\")[:50]}')" 2>/dev/null
        fi
    fi
done
END=$(date +%s)
echo "耗时: $((END-START))秒"

echo ""
echo "=== 测试完成 ==="
