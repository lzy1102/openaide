// API服务，用于与后端API进行通信
// 后端 Go API server 默认运行在 localhost:8080

const API_BASE_URL = 'http://localhost:8080/api/v1';

// ============ 通用请求函数 ============

async function request(endpoint, options = {}) {
    const url = `${API_BASE_URL}${endpoint}`;
    const headers = { 'Content-Type': 'application/json' };
    const token = localStorage.getItem('openaide_token');
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }
    const mergedOptions = {
        ...options,
        headers: { ...headers, ...options.headers },
    };
    const response = await fetch(url, mergedOptions);
    if (!response.ok) {
        let errMsg = `HTTP ${response.status}`;
        try {
            const errBody = await response.json();
            errMsg = errBody.error || errBody.message || errMsg;
        } catch (_) {}
        throw new Error(errMsg);
    }
    return response.json();
}

// ============ 认证相关 API ============
// 后端: POST /auth/login, POST /auth/register (JWT auth 默认关闭)

export const authAPI = {
    login: (credentials) =>
        request('/auth/login', {
            method: 'POST',
            body: JSON.stringify(credentials),
        }).then((res) => {
            if (res.token) localStorage.setItem('openaide_token', res.token);
            return res;
        }),

    register: (data) =>
        request('/auth/register', {
            method: 'POST',
            body: JSON.stringify(data),
        }),

    logout: () => {
        localStorage.removeItem('openaide_token');
    },

    getUserInfo: () => {
        const token = localStorage.getItem('openaide_token');
        return { authenticated: !!token, token };
    },
};

// ============ 项目相关 API ============

export const projectAPI = {
    listProjects: () =>
        request('/projects'),

    createProject: (data) =>
        request('/projects', {
            method: 'POST',
            body: JSON.stringify(data),
        }),

    getProject: (id) =>
        request('/projects/' + id),

    updateProject: (id, data) =>
        request('/projects/' + id, {
            method: 'PUT',
            body: JSON.stringify(data),
        }),

    deleteProject: (id) =>
        request('/projects/' + id, { method: 'DELETE' }),
};

// ============ 对话/会话相关 API ============
// 后端: GET/POST /sessions, GET/DELETE /sessions/{id}
// 每次对话在前端生成唯一 ID 并作为 user_id 传给后端，后端据此关联消息

const STORAGE_KEY_DIALOGUES = 'openaide_dialogues';

function loadLocalDialogues() {
    try {
        const stored = localStorage.getItem(STORAGE_KEY_DIALOGUES);
        return stored ? JSON.parse(stored) : [];
    } catch { return []; }
}

function saveLocalDialogues(dialogues) {
    try {
        localStorage.setItem(STORAGE_KEY_DIALOGUES, JSON.stringify(dialogues));
    } catch {}
}

export const dialogueAPI = {
    // 获取所有对话 (本地 localStorage + 后端最佳同步)
    listDialogues: (projectId) => {
        // 尝试从后端同步(静默)
        request(
            `/sessions?user_id=user-001${projectId ? '&project_id=' + projectId : ''}`
        )
            .then((sessions) => {
                if (!Array.isArray(sessions)) return;
                const local = loadLocalDialogues();
                const localIds = new Set(local.map((d) => d.id));
                let changed = false;
                for (const s of sessions) {
                    if (!localIds.has(s.id)) {
                        local.push({
                            id: s.id,
                            title: s.id === 'new-session-id' ? 'New Chat' : s.id,
                            projectId: s.project_id,
                            messages: [],
                        });
                        changed = true;
                    }
                }
                if (changed) saveLocalDialogues(local);
            })
            .catch(() => {});

        return Promise.resolve(loadLocalDialogues());
    },

    // 创建新对话 (前端生成唯一ID，后端告知)
    createDialogue: async (userID, title, projectId) => {
        let realId = null;
        try {
            const session = await request('/sessions', {
                method: 'POST',
                body: JSON.stringify({ user_id: userID, project_id: projectId || '' }),
            });
            if (session && session.id) {
                realId = session.id;
            }
        } catch (e) {
            // 后端不可用时回退到本地 ID
        }
        const id = realId || ('session-' + Date.now() + '-' + Math.random().toString(36).slice(2, 8));
        const dialogue = {
            id,
            title: title || 'New Chat',
            projectId: projectId || '',
            messages: [],
        };
        const local = loadLocalDialogues();
        local.unshift(dialogue);
        saveLocalDialogues(local);
        return dialogue;
    },

    // 获取对话详情 (从后端加载历史消息)
    getDialogue: (id) =>
        request(`/sessions/${id}`)
            .then((data) => {
                const msgs = data.messages || [];
                return {
                    id,
                    title: (data.session && data.session.id) ? data.session.id.substring(0, 8) : 'Chat',
                    messages: msgs.map((m) => ({
                        sender: m.role === 'user' ? 'user' : 'assistant',
                        content: m.content || '',
                    })),
                };
            })
            .catch(() => ({
                id,
                title: 'Chat',
                messages: [],
            })),

    // 删除对话 (本地 + 后端)
    deleteDialogue: (id) => {
        const local = loadLocalDialogues();
        saveLocalDialogues(local.filter((d) => d.id !== id));
        return request(`/sessions/${id}`, { method: 'DELETE' }).catch(
            () => ({})
        );
    },

    // 更新对话 (后端不支持)
    updateDialogue: () => Promise.resolve(),

    // 保存流式消息内容 (后端在chat/stream中自动保存)
    saveStreamMessage: (dialogueId, content, thinkingText) => {
        const local = loadLocalDialogues();
        const dialogue = local.find(d => d.id === dialogueId);
        if (dialogue) {
            dialogue.messages.push({
                sender: 'assistant',
                content: content,
            });
            if (thinkingText) {
                dialogue.messages.push({
                    sender: 'assistant',
                    content: '[思考] ' + thinkingText,
                });
            }
            saveLocalDialogues(local);
        }
    },

    // 获取对话消息 (通过getDialogue统一获取)
    getMessages: () => Promise.resolve([]),

    // ============ 流式发送消息 (SSE) ============
    // 后端 POST /chat/stream 返回 SSE:
    //   data: {"type":"content","content":"..."}
    //   data: {"type":"thinking","content":"..."}
    //   data: {"type":"tool_call","tool_call_id":"...","tool_name":"..."}
    //   data: {"type":"tool_done","tool_call_id":"...","tool_name":"...","tool_result":...}
    //   data: {"type":"progress","round":1,"total_rounds":10}
    //   data: {"type":"done","tokens_used":123}
    //   data: {"type":"error","error":"..."}
    sendMessageStream: (dialogueId, userID, content, modelID, options = {}, onChunk) => {
        const url = `${API_BASE_URL}/chat/stream`;
        const headers = { 'Content-Type': 'application/json' };
        const token = localStorage.getItem('openaide_token');
        if (token) headers['Authorization'] = `Bearer ${token}`;

        const body = {
            message: content,
            user_id: dialogueId,
            model: modelID || undefined,
            working_dir: options.working_dir || undefined,
        };

        return fetch(url, {
            method: 'POST',
            headers,
            body: JSON.stringify(body),
        }).then((response) => {
            if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = '';

            return new Promise((resolve, reject) => {
                function read() {
                    reader
                        .read()
                        .then(({ done, value }) => {
                            if (done) {
                                if (buffer.trim()) processSSELines(buffer);
                                resolve();
                                return;
                            }
                            buffer += decoder.decode(value, { stream: true });
                            const lines = buffer.split('\n');
                            buffer = lines.pop() || '';
                            processSSELines(lines.join('\n'));
                            read();
                        })
                        .catch(reject);
                }

                function processSSELines(text) {
                    const lines = text.split('\n');
                    for (const line of lines) {
                        const trimmed = line.trim();
                        if (trimmed.startsWith('data:')) {
                            try {
                                const data = JSON.parse(trimmed.slice(5).trim());
								switch (data.type) {
									case 'content':
										onChunk({ content: data.content || '' });
										break;
									case 'thinking':
										onChunk({ thinking: data.content || '' });
										break;
									case 'tool_call':
										onChunk({
											toolCall: {
												tool: data.tool_name,
												params: data.tool_call_id,
												status: 'running',
											},
										});
										break;
									case 'tool_done':
										onChunk({
											toolDone: {
												tool: data.tool_name,
												result: data.tool_result,
											},
										});
										break;
									case 'progress':
										onChunk({
											progress: `Round ${data.round || '?'}/${data.total_rounds || '?'}`,
										});
										break;
									case 'done':
										onChunk({
											done: true,
											model: data.model,
											usage: data.tokens_used
												? { total_tokens: data.tokens_used }
												: undefined,
										});
										break;
									case 'error':
										onChunk({ error: data.error || 'Unknown error' });
										break;
								}
                            } catch (_) {
                                // skip malformed JSON
                            }
                        }
                    }
                }

                read();
            });
        });
    },
};

// ============ 工作流相关 API (后端无端点，本地存根) ============

export const workflowAPI = {
    listWorkflows: () => Promise.resolve([]),
    createWorkflow: () => Promise.resolve({ id: 'wf-' + Date.now() }),
    getWorkflow: () => Promise.resolve(null),
    updateWorkflow: () => Promise.resolve(),
    deleteWorkflow: () => Promise.resolve(),
    createWorkflowInstance: () => Promise.resolve({ id: 'wfi-' + Date.now() }),
    executeWorkflowInstance: () => Promise.resolve({}),
};

// ============ 技能相关 API (后端无端点，本地存根) ============

export const skillAPI = {
    listSkills: () => Promise.resolve([]),
    createSkill: () => Promise.resolve({ id: 'skill-' + Date.now() }),
    getSkill: () => Promise.resolve(null),
    updateSkill: () => Promise.resolve(),
    deleteSkill: () => Promise.resolve(),
    createSkillInstance: () => Promise.resolve({ id: 'ski-' + Date.now() }),
    executeSkillInstance: () => Promise.resolve({}),
};

// ============ 插件相关 API (后端无端点，本地存根) ============

export const pluginAPI = {
    listPlugins: () => Promise.resolve([]),
    createPlugin: () => Promise.resolve({ id: 'plugin-' + Date.now() }),
    getPlugin: () => Promise.resolve(null),
    updatePlugin: () => Promise.resolve(),
    deletePlugin: () => Promise.resolve(),
    installPlugin: () => Promise.resolve({}),
    enablePlugin: () => Promise.resolve(),
    disablePlugin: () => Promise.resolve(),
    createPluginInstance: () => Promise.resolve({ id: 'pli-' + Date.now() }),
    executePluginInstance: () => Promise.resolve({}),
};

// ============ 模型相关 API (后端无独立 CRUD 端点，可尝试从 stats 获取基础模型) ============

let _cachedModels = null;

export const modelAPI = {
    listModels: async () => {
        if (_cachedModels) return _cachedModels;
        try {
            const stats = await request('/stats');
            const model = stats?.model || stats?.models?.[0] || null;
            _cachedModels = model
                ? [{ id: model, name: model, status: 'enabled' }]
                : [{ id: 'default-model', name: 'Default Model', status: 'enabled' }];
        } catch {
            _cachedModels = [
                { id: 'default-model', name: 'Default Model', status: 'enabled' },
            ];
        }
        return _cachedModels;
    },

    createModel: () => Promise.resolve({ id: 'model-' + Date.now() }),
    getModel: () => Promise.resolve(null),
    updateModel: () => Promise.resolve(),
    deleteModel: () => Promise.resolve(),
    enableModel: () => Promise.resolve(),
    disableModel: () => Promise.resolve(),
    createModelInstance: () => Promise.resolve({ id: 'mi-' + Date.now() }),
    executeModelInstance: () => Promise.resolve({}),
};

// ============ 自动化相关 API (后端无端点，本地存根) ============

export const automationAPI = {
    listExecutions: () => Promise.resolve([]),
    createExecution: () => Promise.resolve({ id: 'exec-' + Date.now() }),
    getExecution: () => Promise.resolve(null),
    deleteExecution: () => Promise.resolve(),
    execute: () => Promise.resolve({}),
};

// ============ 代码执行相关 API (后端无端点，本地存根) ============

export const codeAPI = {
    listExecutions: () => Promise.resolve([]),
    createExecution: () => Promise.resolve({ id: 'code-exec-' + Date.now() }),
    getExecution: () => Promise.resolve(null),
    deleteExecution: () => Promise.resolve(),
    execute: () => Promise.resolve({}),
};

// ============ 确认相关 API (后端无端点，本地存根) ============

export const confirmationAPI = {
    listConfirmations: () => Promise.resolve([]),
    createConfirmation: () => Promise.resolve({ id: 'conf-' + Date.now() }),
    getConfirmation: () => Promise.resolve(null),
    deleteConfirmation: () => Promise.resolve(),
    confirm: () => Promise.resolve({}),
    reject: () => Promise.resolve({}),
};

// ============ 输入相关 API (后端无端点，本地存根) ============

export const inputAPI = {
    listActions: () => Promise.resolve([]),
    createAction: () => Promise.resolve({ id: 'action-' + Date.now() }),
    getAction: () => Promise.resolve(null),
    updateAction: () => Promise.resolve(),
    deleteAction: () => Promise.resolve(),
    disableAction: () => Promise.resolve(),
    enableAction: () => Promise.resolve(),
    createInstance: () => Promise.resolve({ id: 'inst-' + Date.now() }),
    executeAction: () => Promise.resolve({}),
};

// ============ 思考相关 API (后端无端点，本地存根) ============

export const thinkingAPI = {
    listThoughts: () => Promise.resolve([]),
    createThought: () => Promise.resolve({ id: 'thought-' + Date.now() }),
    getThought: () => Promise.resolve(null),
    deleteThought: () => Promise.resolve(),
    createCorrection: () => Promise.resolve({ id: 'corr-' + Date.now() }),
    listCorrections: () => Promise.resolve([]),
    resolveCorrection: () => Promise.resolve({}),
    deleteCorrection: () => Promise.resolve(),
};

// ============ 提示词模板相关 API (后端无端点，本地存根) ============

export const templateAPI = {
    listTemplates: () => Promise.resolve([]),
    createTemplate: () => Promise.resolve({ id: 'tpl-' + Date.now() }),
    getTemplate: () => Promise.resolve(null),
    updateTemplate: () => Promise.resolve(),
    deleteTemplate: () => Promise.resolve(),
    renderTemplate: () => Promise.resolve(''),
};

// ============ 使用量统计相关 API ============
// 后端: GET /stats 返回系统统计 (可能不包含前端指标，使用混合模式：后端 + 模拟回退)

export const dashboardAPI = {
    getUsageStats: (timeRange) =>
        request('/stats').catch(() => null),

    getCostStats: () => Promise.resolve(null),
    getBudgetStatus: () => Promise.resolve(null),
    updateBudget: () => Promise.resolve(),
    getRecentActivity: () => Promise.resolve([]),
    getModelUsage: () => Promise.resolve([]),
};

// ============ 定时任务相关 API (后端无端点，本地存根) ============

export const scheduledTaskAPI = {
    listTasks: () => Promise.resolve([]),
    createTask: () => Promise.resolve({ id: 'task-' + Date.now() }),
    getTask: () => Promise.resolve(null),
    updateTask: () => Promise.resolve(),
    deleteTask: () => Promise.resolve(),
    executeTask: () => Promise.resolve({}),
    pauseTask: () => Promise.resolve(),
    resumeTask: () => Promise.resolve(),
    getExecutionHistory: () => Promise.resolve([]),
    validateCron: () => Promise.resolve({ valid: true }),
};

// ============ 工具调用相关 API ============
// 后端: GET /tools 返回工具统计数据

export const toolCallAPI = {
    listToolCalls: () =>
        request('/tools').catch(() => []),

    getToolCall: () => Promise.resolve(null),
    executeTool: () => Promise.resolve({}),
};
