# OpenAIDE Development Team | OpenAIDE 开发团队

[English](#english) | [中文](#中文)

---

<a name="english"></a>
## English

### Quick Start | 快速启动

Start the team in Claude Code:

```
Read .claude/team.yaml configuration, start the OpenAIDE development team
```

### Team Members

#### Backend Development
| Member | Role | Task | Color |
|--------|------|------|-------|
| backend-llm | Backend Engineer | LLM Model Integration | 🔵 blue |
| backend-thinking | Backend Engineer | Thinking & Reasoning System | 🟡 yellow |
| backend-correction | Backend Engineer | Auto Correction System | 🟠 orange |
| backend-learning | Backend Engineer | Continuous Learning System | 🩷 pink |
| backend-workflow | Backend Engineer | Workflow Engine | 🟣 purple |

#### Frontend Development
| Member | Role | Task | Color |
|--------|------|------|-------|
| frontend-dev | Frontend Engineer | UI/UX Optimization | 🟢 green |

#### Knowledge Base Module
| Member | Role | Task | Color |
|--------|------|------|-------|
| knowledge-model | Backend Engineer | Knowledge Data Model | 🔵 cyan |
| knowledge-vector | Backend Engineer | Vector Storage & Retrieval | 🔴 red |
| knowledge-rag | Backend Engineer | RAG Service | 🔵 blue |
| knowledge-extract | Backend Engineer | Knowledge Extraction | 🟢 green |
| knowledge-doc | Backend Engineer | Document Import | 🟡 yellow |

### Completed Features

#### Core Capabilities ✅
- [x] LLM Multi-model Integration (OpenAI/Claude/GLM)
- [x] Chain-of-Thought Reasoning
- [x] Tree-of-Thought Reasoning
- [x] Auto Correction System
- [x] Continuous Learning System
- [x] Workflow Engine

#### Knowledge Base ✅
- [x] Knowledge Data Model
- [x] Vector Embedding Generation
- [x] Semantic Similarity Search
- [x] RAG Retrieval Augmented Generation
- [x] Auto Knowledge Extraction
- [x] Document Import & Chunking

#### Core Enhancement ✅
- [x] Smart Task Decomposition
- [x] Context Management
- [x] Team Coordination Service
- [x] Integration Testing

#### Frontend ✅
- [x] UI/UX Optimization
- [x] Dark Mode
- [x] Interaction Improvements

### Key Files

```
backend/src/
├── models/
│   └── knowledge.go          # Knowledge data model
├── services/
│   ├── llm/                  # LLM clients
│   │   ├── llm_client.go     # Unified interface
│   │   ├── openai_client.go
│   │   ├── anthropic_client.go
│   │   └── glm_client.go
│   ├── thinking_service.go   # Thinking & reasoning
│   ├── correction_service.go # Auto correction
│   ├── learning_service.go   # Continuous learning
│   ├── workflow_service.go   # Workflow engine
│   ├── knowledge_service.go  # Knowledge service
│   ├── embedding_service.go  # Vector generation
│   ├── rag_service.go        # RAG service
│   ├── knowledge_extraction_service.go # Knowledge extraction
│   ├── document_service.go   # Document import
│   ├── task_decompose_service.go # Task decomposition
│   ├── context_manager.go    # Context management
│   └── team_coordinator.go   # Team coordination

frontend/
├── index.html
├── styles.css
└── src/
    └── services/
        └── api.js
```

### Project Status

- ✅ All code compiled successfully
- ✅ Backend service architecture complete
- ✅ Frontend interface optimized
- ✅ Configuration persisted

---

<a name="中文"></a>
## 中文

### 快速启动

在 Claude Code 中说以下指令启动团队：

```
读取 .claude/team.yaml 配置，启动 OpenAIDE 开发团队
```

### 团队成员

#### 后端开发
| 成员 | 角色 | 任务 | 颜色 |
|------|------|------|------|
| backend-llm | 后端工程师 | LLM 模型接入 | 🔵 blue |
| backend-thinking | 后端工程师 | 思考推理系统 | 🟡 yellow |
| backend-correction | 后端工程师 | 自动纠错系统 | 🟠 orange |
| backend-learning | 后端工程师 | 持续学习系统 | 🩷 pink |
| backend-workflow | 后端工程师 | 工作流引擎 | 🟣 purple |

#### 前端开发
| 成员 | 角色 | 任务 | 颜色 |
|------|------|------|------|
| frontend-dev | 前端工程师 | 前端界面优化 | 🟢 green |

#### 知识库模块
| 成员 | 角色 | 任务 | 颜色 |
|------|------|------|------|
| knowledge-model | 后端工程师 | 知识库数据模型 | 🔵 cyan |
| knowledge-vector | 后端工程师 | 向量存储检索 | 🔴 red |
| knowledge-rag | 后端工程师 | RAG 服务 | 🔵 blue |
| knowledge-extract | 后端工程师 | 知识提取 | 🟢 green |
| knowledge-doc | 后端工程师 | 文档导入 | 🟡 yellow |

### 已完成的功能

#### 核心能力 ✅
- [x] LLM 多模型接入 (OpenAI/Claude/GLM)
- [x] Chain-of-Thought 思考推理
- [x] Tree-of-Thought 树状推理
- [x] 自动纠错系统
- [x] 持续学习系统
- [x] 工作流引擎

#### 知识库 ✅
- [x] 知识数据模型
- [x] 向量 Embedding 生成
- [x] 语义相似度搜索
- [x] RAG 检索增强生成
- [x] 知识自动提取
- [x] 文档导入与分块

#### 核心功能增强 ✅
- [x] 智能任务分解
- [x] 上下文管理
- [x] 团队协调服务
- [x] 集成测试

#### 前端 ✅
- [x] UI/UX 优化
- [x] 深色模式
- [x] 交互改进

### 关键文件

```
backend/src/
├── models/
│   └── knowledge.go          # 知识库数据模型
├── services/
│   ├── llm/                  # LLM 客户端
│   │   ├── llm_client.go     # 统一接口
│   │   ├── openai_client.go
│   │   ├── anthropic_client.go
│   │   └── glm_client.go
│   ├── thinking_service.go   # 思考推理
│   ├── correction_service.go # 自动纠错
│   ├── learning_service.go   # 持续学习
│   ├── workflow_service.go   # 工作流引擎
│   ├── knowledge_service.go  # 知识库服务
│   ├── embedding_service.go  # 向量生成
│   ├── rag_service.go        # RAG 服务
│   ├── knowledge_extraction_service.go # 知识提取
│   ├── document_service.go   # 文档导入
│   ├── task_decompose_service.go # 任务分解
│   ├── context_manager.go    # 上下文管理
│   └── team_coordinator.go   # 团队协调

frontend/
├── index.html
├── styles.css
└── src/
    └── services/
        └── api.js
```

### 项目状态

- ✅ 所有代码编译通过
- ✅ 后端服务架构完整
- ✅ 前端界面已完善
- ✅ 配置已持久化

---

## Compile Verification | 编译验证

```bash
cd backend
go build ./...
```
