/**
 * 工具调用显示组件
 * 用于在对话界面展示工具调用结果，包括工具名称、参数和执行结果
 */

export class ToolCallDisplay {
    constructor(container) {
        this.container = container;
        this.toolCalls = [];
    }

    /**
     * 渲染工具调用显示容器
     */
    render() {
        this.container.innerHTML = `
            <div class="tool-call-display">
                <div class="tool-call-header">
                    <div class="tool-call-title">
                        <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
                            <path d="M9 2L11 6H15L12 9L13 14L9 11L5 14L6 9L3 6H7L9 2Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                            <path d="M6 13L3 16M12 13L15 16" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                        </svg>
                        <span>Tool Calls</span>
                    </div>
                    <button class="btn btn-sm btn-secondary" id="toggle-tool-calls">
                        <span class="expand-icon">▼</span>
                    </button>
                </div>
                <div class="tool-call-content" id="tool-call-content">
                    <div class="tool-call-list" id="tool-call-list">
                        <div class="tool-call-empty">
                            <p>No tool calls yet</p>
                        </div>
                    </div>
                </div>
            </div>
        `;

        // 绑定事件
        this.container.querySelector('#toggle-tool-calls').addEventListener('click', () => {
            this.toggle();
        });

        return this;
    }

    /**
     * 切换展开/收起状态
     */
    toggle() {
        const content = this.container.querySelector('#tool-call-content');
        const icon = this.container.querySelector('.expand-icon');
        const isExpanded = !content.classList.contains('collapsed');

        if (isExpanded) {
            content.classList.add('collapsed');
            icon.textContent = '◀';
        } else {
            content.classList.remove('collapsed');
            icon.textContent = '▼';
        }
    }

    /**
     * 添加工具调用记录
     */
    addToolCall(toolCall) {
        const toolCallData = {
            id: toolCall.id || this.generateId(),
            toolName: toolCall.toolName || toolCall.name,
            parameters: toolCall.parameters || toolCall.args || {},
            result: toolCall.result || null,
            status: toolCall.status || 'pending', // pending, running, completed, error
            startTime: toolCall.startTime || new Date().toISOString(),
            endTime: toolCall.endTime || null,
            duration: toolCall.duration || null,
            error: toolCall.error || null
        };

        this.toolCalls.push(toolCallData);
        this.updateToolCallList();

        return toolCallData.id;
    }

    /**
     * 更新工具调用状态
     */
    updateToolCallStatus(id, status, result = null) {
        const toolCall = this.toolCalls.find(tc => tc.id === id);
        if (toolCall) {
            toolCall.status = status;
            if (result) {
                toolCall.result = result;
            }
            if (status === 'completed' || status === 'error') {
                toolCall.endTime = new Date().toISOString();
                toolCall.duration = new Date(toolCall.endTime) - new Date(toolCall.startTime);
            }
            if (status === 'error' && result) {
                toolCall.error = result;
            }
            this.updateToolCallList();
        }
    }

    /**
     * 更新工具调用列表显示
     */
    updateToolCallList() {
        const listContainer = this.container.querySelector('#tool-call-list');

        if (this.toolCalls.length === 0) {
            listContainer.innerHTML = `
                <div class="tool-call-empty">
                    <p>No tool calls yet</p>
                </div>
            `;
            return;
        }

        listContainer.innerHTML = this.toolCalls.map(toolCall => this.renderToolCallItem(toolCall)).join('');
    }

    /**
     * 渲染单个工具调用项
     */
    renderToolCallItem(toolCall) {
        const statusIcon = this.getStatusIcon(toolCall.status);
        const statusBadge = this.getStatusBadge(toolCall.status);

        return `
            <div class="tool-call-item ${toolCall.status}" data-id="${toolCall.id}">
                <div class="tool-call-item-header">
                    <div class="tool-call-info">
                        <span class="tool-call-icon">${statusIcon}</span>
                        <div class="tool-call-details">
                            <span class="tool-call-name">${this.escapeHtml(toolCall.toolName)}</span>
                            <div class="tool-call-meta">
                                ${statusBadge}
                                ${toolCall.duration ? `<span class="tool-call-duration">${toolCall.duration}ms</span>` : ''}
                            </div>
                        </div>
                    </div>
                    <button class="btn-icon tool-call-expand" data-id="${toolCall.id}">
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                            <path d="M4 6L8 10L12 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                    </button>
                </div>
                <div class="tool-call-item-body hidden" data-body-id="${toolCall.id}">
                    ${toolCall.parameters && Object.keys(toolCall.parameters).length > 0 ? `
                        <div class="tool-call-section">
                            <span class="tool-call-section-title">Parameters</span>
                            <pre class="tool-call-code">${this.escapeHtml(JSON.stringify(toolCall.parameters, null, 2))}</pre>
                        </div>
                    ` : ''}
                    ${toolCall.result ? `
                        <div class="tool-call-section">
                            <span class="tool-call-section-title">Result</span>
                            <pre class="tool-call-code tool-call-result">${this.escapeHtml(JSON.stringify(toolCall.result, null, 2))}</pre>
                        </div>
                    ` : ''}
                    ${toolCall.error ? `
                        <div class="tool-call-section tool-call-error-section">
                            <span class="tool-call-section-title">Error</span>
                            <pre class="tool-call-code tool-call-error">${this.escapeHtml(toolCall.error)}</pre>
                        </div>
                    ` : ''}
                </div>
            </div>
        `;
    }

    /**
     * 获取状态图标
     */
    getStatusIcon(status) {
        const icons = {
            pending: `<svg class="status-icon pending" width="16" height="16" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="2" stroke-dasharray="2 2"/>
            </svg>`,
            running: `<svg class="status-icon running" width="16" height="16" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="2"/>
                <path d="M8 2A6 6 0 0 1 14 8" stroke="currentColor" stroke-width="2" stroke-linecap="round" class="spin"/>
            </svg>`,
            completed: `<svg class="status-icon completed" width="16" height="16" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="8" r="7" fill="currentColor"/>
                <path d="M5 8L7 10L11 6" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>`,
            error: `<svg class="status-icon error" width="16" height="16" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="8" r="7" fill="currentColor"/>
                <path d="M5 5L11 11M11 5L5 11" stroke="white" stroke-width="2" stroke-linecap="round"/>
            </svg>`
        };
        return icons[status] || icons.pending;
    }

    /**
     * 获取状态徽章
     */
    getStatusBadge(status) {
        const badges = {
            pending: '<span class="badge badge-secondary">Pending</span>',
            running: '<span class="badge badge-primary">Running</span>',
            completed: '<span class="badge badge-success">Completed</span>',
            error: '<span class="badge badge-error">Error</span>'
        };
        return badges[status] || badges.pending;
    }

    /**
     * 转义 HTML
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        if (typeof text === 'object') {
            div.textContent = JSON.stringify(text);
        } else {
            div.textContent = text;
        }
        return div.innerHTML;
    }

    /**
     * 生成唯一 ID
     */
    generateId() {
        return 'tool-call-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);
    }

    /**
     * 清空工具调用记录
     */
    clear() {
        this.toolCalls = [];
        this.updateToolCallList();
    }

    /**
     * 获取工具调用数据
     */
    getData() {
        return {
            toolCalls: this.toolCalls
        };
    }

    /**
     * 绑定展开/收起事件
     */
    bindExpandEvents() {
        this.container.querySelectorAll('.tool-call-expand').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                const body = this.container.querySelector(`[data-body-id="${id}"]`);
                if (body) {
                    body.classList.toggle('hidden');
                }
            });
        });
    }
}
