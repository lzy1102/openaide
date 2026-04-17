# 代码审查示例

## Python 示例

### 输入
```python
def process_data(data):
    result = []
    for i in range(len(data)):
        if data[i] > 0:
            result.append(data[i] * 2)
    return result
```

### 审查结果
```markdown
## 审查结果

### 问题列表
| 严重程度 | 位置 | 问题描述 | 修复建议 |
|---------|------|---------|---------|
| 🟢 建议 | L3 | 使用range索引遍历 | 直接使用 `for item in data` |
| 🟢 建议 | L4-5 | 可用列表推导式 | `return [x * 2 for x in data if x > 0]` |

### 总体评分: B+
```

## Go 示例

### 输入
```go
func GetUser(db *sql.DB, id string) (*User, error) {
    query := "SELECT * FROM users WHERE id = '" + id + "'"
    rows, err := db.Query(query)
    // ...
}
```

### 审查结果
```markdown
## 审查结果

### 问题列表
| 严重程度 | 位置 | 问题描述 | 修复建议 |
|---------|------|---------|---------|
| 🔴 严重 | L2 | SQL注入风险 | 使用参数化查询 |
| 🟡 警告 | L1 | 未检查错误 | 添加错误处理 |

### 总体评分: D
**主要问题**: 存在严重安全漏洞
```
