// API服务，用于与后端API进行通信

const API_BASE_URL = 'http://localhost:8080/api';

// 通用请求函数
async function request(endpoint, options = {}) {
    const url = `${API_BASE_URL}${endpoint}`;
    
    const defaultOptions = {
        headers: {
            'Content-Type': 'application/json',
        },
    };
    
    const mergedOptions = {
        ...defaultOptions,
        ...options,
        headers: {
            ...defaultOptions.headers,
            ...options.headers,
        },
    };
    
    try {
        const response = await fetch(url, mergedOptions);
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        return await response.json();
    } catch (error) {
        console.error('API request error:', error);
        throw error;
    }
}

// 对话相关API
export const dialogueAPI = {
    // 获取所有对话
    listDialogues: () => request('/dialogues'),
    
    // 创建新对话
    createDialogue: (userID, title) => request('/dialogues', {
        method: 'POST',
        body: JSON.stringify({ user_id: userID, title }),
    }),
    
    // 获取对话详情
    getDialogue: (id) => request(`/dialogues/${id}`),
    
    // 更新对话
    updateDialogue: (id, title) => request(`/dialogues/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ title }),
    }),
};

// 工作流相关API
export const workflowAPI = {
    listWorkflows: () => request('/workflows'),
    createWorkflow: (data) => request('/workflows', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getWorkflow: (id) => request(`/workflows/${id}`),
    updateWorkflow: (id, data) => request(`/workflows/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    }),
    deleteWorkflow: (id) => request(`/workflows/${id}`, {
        method: 'DELETE',
    }),
    createWorkflowInstance: (id) => request(`/workflows/${id}/instances`, {
        method: 'POST',
    }),
    executeWorkflowInstance: (id) => request(`/workflows/instances/${id}/execute`, {
        method: 'POST',
    }),
};

// 技能相关API
export const skillAPI = {
    listSkills: () => request('/skills'),
    createSkill: (data) => request('/skills', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getSkill: (id) => request(`/skills/${id}`),
    updateSkill: (id, data) => request(`/skills/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    }),
    deleteSkill: (id) => request(`/skills/${id}`, {
        method: 'DELETE',
    }),
    createSkillInstance: (id) => request(`/skills/${id}/instances`, {
        method: 'POST',
    }),
    executeSkillInstance: (id) => request(`/skills/instances/${id}/execute`, {
        method: 'POST',
    }),
};

// 插件相关API
export const pluginAPI = {
    listPlugins: () => request('/plugins'),
    createPlugin: (data) => request('/plugins', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getPlugin: (id) => request(`/plugins/${id}`),
    updatePlugin: (id, data) => request(`/plugins/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    }),
    deletePlugin: (id) => request(`/plugins/${id}`, {
        method: 'DELETE',
    }),
    installPlugin: (data) => request('/plugins/install', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    enablePlugin: (id) => request(`/plugins/${id}/enable`, {
        method: 'POST',
    }),
    disablePlugin: (id) => request(`/plugins/${id}/disable`, {
        method: 'POST',
    }),
    createPluginInstance: (id) => request(`/plugins/${id}/instances`, {
        method: 'POST',
    }),
    executePluginInstance: (id) => request(`/plugins/instances/${id}/execute`, {
        method: 'POST',
    }),
};

// 模型相关API
export const modelAPI = {
    listModels: () => request('/models'),
    createModel: (data) => request('/models', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getModel: (id) => request(`/models/${id}`),
    updateModel: (id, data) => request(`/models/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    }),
    deleteModel: (id) => request(`/models/${id}`, {
        method: 'DELETE',
    }),
    enableModel: (id) => request(`/models/${id}/enable`, {
        method: 'POST',
    }),
    disableModel: (id) => request(`/models/${id}/disable`, {
        method: 'POST',
    }),
    createModelInstance: (id) => request(`/models/${id}/instances`, {
        method: 'POST',
    }),
    executeModelInstance: (id) => request(`/models/instances/${id}/execute`, {
        method: 'POST',
    }),
};

// 自动化相关API
export const automationAPI = {
    listExecutions: () => request('/automation/executions'),
    createExecution: (data) => request('/automation/executions', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getExecution: (id) => request(`/automation/executions/${id}`),
    deleteExecution: (id) => request(`/automation/executions/${id}`, {
        method: 'DELETE',
    }),
    execute: (id) => request(`/automation/executions/${id}/execute`, {
        method: 'POST',
    }),
};

// 代码执行相关API
export const codeAPI = {
    listExecutions: () => request('/code/executions'),
    createExecution: (data) => request('/code/executions', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getExecution: (id) => request(`/code/executions/${id}`),
    deleteExecution: (id) => request(`/code/executions/${id}`, {
        method: 'DELETE',
    }),
    execute: (id) => request(`/code/executions/${id}/execute`, {
        method: 'POST',
    }),
};

// 确认相关API
export const confirmationAPI = {
    listConfirmations: () => request('/confirmations'),
    createConfirmation: (data) => request('/confirmations', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getConfirmation: (id) => request(`/confirmations/${id}`),
    deleteConfirmation: (id) => request(`/confirmations/${id}`, {
        method: 'DELETE',
    }),
    confirm: (id) => request(`/confirmations/${id}/confirm`, {
        method: 'POST',
    }),
    reject: (id) => request(`/confirmations/${id}/reject`, {
        method: 'POST',
    }),
};

// 输入相关API
export const inputAPI = {
    listActions: () => request('/input/actions'),
    createAction: (data) => request('/input/actions', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getAction: (id) => request(`/input/actions/${id}`),
    updateAction: (id, data) => request(`/input/actions/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    }),
    deleteAction: (id) => request(`/input/actions/${id}`, {
        method: 'DELETE',
    }),
    disableAction: (id) => request(`/input/actions/${id}/disable`, {
        method: 'POST',
    }),
    enableAction: (id) => request(`/input/actions/${id}/enable`, {
        method: 'POST',
    }),
    createInstance: (id) => request(`/input/actions/${id}/instances`, {
        method: 'POST',
    }),
    executeAction: (id) => request(`/input/instances/${id}/execute`, {
        method: 'POST',
    }),
};

// 思考相关API
export const thinkingAPI = {
    listThoughts: () => request('/thinking/thoughts'),
    createThought: (data) => request('/thinking/thoughts', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getThought: (id) => request(`/thinking/thoughts/${id}`),
    deleteThought: (id) => request(`/thinking/thoughts/${id}`, {
        method: 'DELETE',
    }),
    createCorrection: (id, data) => request(`/thinking/thoughts/${id}/corrections`, {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    listCorrections: (id) => request(`/thinking/thoughts/${id}/corrections`),
    resolveCorrection: (id) => request(`/thinking/corrections/${id}/resolve`, {
        method: 'POST',
    }),
    deleteCorrection: (id) => request(`/thinking/corrections/${id}`, {
        method: 'DELETE',
    }),
};

// 提示词模板相关API
export const templateAPI = {
    listTemplates: () => request('/templates'),
    createTemplate: (data) => request('/templates', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getTemplate: (id) => request(`/templates/${id}`),
    updateTemplate: (id, data) => request(`/templates/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    }),
    deleteTemplate: (id) => request(`/templates/${id}`, {
        method: 'DELETE',
    }),
    renderTemplate: (id, variables) => request(`/templates/${id}/render`, {
        method: 'POST',
        body: JSON.stringify({ variables }),
    }),
};

// 使用量统计相关API
export const dashboardAPI = {
    getUsageStats: (timeRange) => request(`/dashboard/usage?range=${timeRange}`),
    getCostStats: (timeRange) => request(`/dashboard/costs?range=${timeRange}`),
    getBudgetStatus: () => request('/dashboard/budget'),
    updateBudget: (data) => request('/dashboard/budget', {
        method: 'PUT',
        body: JSON.stringify(data),
    }),
    getRecentActivity: (limit = 10) => request(`/dashboard/activity?limit=${limit}`),
    getModelUsage: () => request('/dashboard/models'),
};

// 定时任务相关API
export const scheduledTaskAPI = {
    listTasks: () => request('/scheduled-tasks'),
    createTask: (data) => request('/scheduled-tasks', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getTask: (id) => request(`/scheduled-tasks/${id}`),
    updateTask: (id, data) => request(`/scheduled-tasks/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    }),
    deleteTask: (id) => request(`/scheduled-tasks/${id}`, {
        method: 'DELETE',
    }),
    executeTask: (id) => request(`/scheduled-tasks/${id}/execute`, {
        method: 'POST',
    }),
    pauseTask: (id) => request(`/scheduled-tasks/${id}/pause`, {
        method: 'POST',
    }),
    resumeTask: (id) => request(`/scheduled-tasks/${id}/resume`, {
        method: 'POST',
    }),
    getExecutionHistory: (id, limit = 20) => request(`/scheduled-tasks/${id}/history?limit=${limit}`),
    validateCron: (expression) => request('/scheduled-tasks/validate-cron', {
        method: 'POST',
        body: JSON.stringify({ expression }),
    }),
};

// 工具调用相关API
export const toolCallAPI = {
    listToolCalls: () => request('/tool-calls'),
    getToolCall: (id) => request(`/tool-calls/${id}`),
    executeTool: (data) => request('/tool-calls/execute', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
};
