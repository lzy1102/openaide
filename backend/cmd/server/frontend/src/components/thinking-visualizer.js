/**
 * 思考过程可视化组件
 * 用于展示 AI 的思考过程，包括思考步骤、状态和结果
 */

export class ThinkingVisualizer {
    constructor(container) {
        this.container = container;
        this.thoughts = [];
        this.currentThoughtId = null;
    }

    /**
     * 创建思考过程展示容器
     */
    render() {
        this.container.innerHTML = `
            <div class="thinking-visualizer">
                <div class="thinking-header">
                    <div class="thinking-title">
                        <svg class="thinking-icon" width="20" height="20" viewBox="0 0 20 20" fill="none">
                            <path d="M10 2C5.58172 2 2 5.58172 2 10C2 14.4183 5.58172 18 10 18C14.4183 18 18 14.4183 18 10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                            <path d="M18 6V10H14" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                            <circle cx="10" cy="10" r="3" stroke="currentColor" stroke-width="2"/>
                        </svg>
                        <span>Thinking Process</span>
                    </div>
                    <button class="btn btn-sm btn-secondary" id="toggle-thinking">
                        <span class="expand-icon">▼</span>
                    </button>
                </div>
                <div class="thinking-content" id="thinking-content">
                    <div class="thinking-timeline" id="thinking-timeline">
                        <div class="thinking-empty">
                            <p>No thinking process yet</p>
                        </div>
                    </div>
                    <div class="thinking-summary hidden" id="thinking-summary">
                        <!-- 思考总结将在这里显示 -->
                    </div>
                </div>
            </div>
        `;

        // 绑定事件
        this.container.querySelector('#toggle-thinking').addEventListener('click', () => {
            this.toggle();
        });

        return this;
    }

    /**
     * 切换展开/收起状态
     */
    toggle() {
        const content = this.container.querySelector('#thinking-content');
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
     * 添加新的思考步骤
     */
    addThought(thought) {
        const thoughtData = {
            id: thought.id || this.generateId(),
            type: thought.type || 'step',
            content: thought.content,
            status: thought.status || 'pending', // pending, processing, completed, error
            timestamp: thought.timestamp || new Date().toISOString(),
            metadata: thought.metadata || {}
        };

        this.thoughts.push(thoughtData);
        this.currentThoughtId = thoughtData.id;

        this.updateTimeline();
        this.updateSummary();

        return thoughtData.id;
    }

    /**
     * 更新思考步骤状态
     */
    updateThoughtStatus(id, status, result = null) {
        const thought = this.thoughts.find(t => t.id === id);
        if (thought) {
            thought.status = status;
            if (result) {
                thought.result = result;
            }
            this.updateTimeline();
            this.updateSummary();
        }
    }

    /**
     * 添加纠错记录
     */
    addCorrection(correction) {
        const correctionData = {
            id: correction.id || this.generateId(),
            thoughtId: correction.thoughtId,
            originalContent: correction.originalContent,
            correctedContent: correction.correctedContent,
            reason: correction.reason,
            timestamp: correction.timestamp || new Date().toISOString(),
            status: correction.status || 'pending' // pending, resolved, dismissed
        };

        // 找到对应的思考步骤并添加纠错信息
        const thought = this.thoughts.find(t => t.id === correctionData.thoughtId);
        if (thought) {
            if (!thought.corrections) {
                thought.corrections = [];
            }
            thought.corrections.push(correctionData);
            thought.status = 'corrected';

            this.updateTimeline();
            this.updateSummary();
        }

        return correctionData.id;
    }

    /**
     * 更新时间线显示
     */
    updateTimeline() {
        const timeline = this.container.querySelector('#thinking-timeline');

        if (this.thoughts.length === 0) {
            timeline.innerHTML = `
                <div class="thinking-empty">
                    <p>No thinking process yet</p>
                </div>
            `;
            return;
        }

        timeline.innerHTML = this.thoughts.map((thought, index) => {
            const statusIcon = this.getStatusIcon(thought.status);
            const isLast = index === this.thoughts.length - 1;

            return `
                <div class="thinking-step ${thought.status} ${isLast ? 'last' : ''}" data-id="${thought.id}">
                    <div class="thinking-step-marker">
                        <div class="thinking-step-icon">${statusIcon}</div>
                        ${!isLast ? '<div class="thinking-step-line"></div>' : ''}
                    </div>
                    <div class="thinking-step-content">
                        <div class="thinking-step-header">
                            <span class="thinking-step-type">${this.formatType(thought.type)}</span>
                            <span class="thinking-step-time">${this.formatTime(thought.timestamp)}</span>
                        </div>
                        <div class="thinking-step-text">${this.escapeHtml(thought.content)}</div>
                        ${thought.result ? `
                            <div class="thinking-step-result">
                                <span class="result-label">Result:</span>
                                <pre>${this.escapeHtml(JSON.stringify(thought.result, null, 2))}</pre>
                            </div>
                        ` : ''}
                        ${thought.corrections && thought.corrections.length > 0 ? `
                            <div class="thinking-corrections">
                                <div class="corrections-header">
                                    <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                                        <path d="M7 1L8.5 5H13L9.5 7.5L11 12L7 9L3 12L4.5 7.5L1 5H5.5L7 1Z" fill="currentColor"/>
                                    </svg>
                                    <span>Corrections (${thought.corrections.length})</span>
                                </div>
                                ${thought.corrections.map(correction => `
                                    <div class="correction-item ${correction.status}">
                                        <div class="correction-reason">
                                            <strong>Reason:</strong> ${this.escapeHtml(correction.reason)}
                                        </div>
                                        <div class="correction-changes">
                                            <div class="correction-original">
                                                <span class="change-label">Original:</span>
                                                <span class="change-text removed">${this.escapeHtml(correction.originalContent)}</span>
                                            </div>
                                            <div class="correction-corrected">
                                                <span class="change-label">Corrected:</span>
                                                <span class="change-text added">${this.escapeHtml(correction.correctedContent)}</span>
                                            </div>
                                        </div>
                                    </div>
                                `).join('')}
                            </div>
                        ` : ''}
                    </div>
                </div>
            `;
        }).join('');
    }

    /**
     * 更新总结显示
     */
    updateSummary() {
        const summary = this.container.querySelector('#thinking-summary');
        if (!summary) return;

        const completedThoughts = this.thoughts.filter(t => t.status === 'completed' || t.status === 'corrected');
        const errorThoughts = this.thoughts.filter(t => t.status === 'error');
        const totalCorrections = this.thoughts.reduce((sum, t) => sum + (t.corrections?.length || 0), 0);

        if (completedThoughts.length > 0 || errorThoughts.length > 0) {
            summary.classList.remove('hidden');
            summary.innerHTML = `
                <div class="thinking-summary-content">
                    <div class="summary-stats">
                        <div class="stat-item">
                            <span class="stat-label">Steps</span>
                            <span class="stat-value">${completedThoughts.length}/${this.thoughts.length}</span>
                        </div>
                        ${totalCorrections > 0 ? `
                            <div class="stat-item">
                                <span class="stat-label">Corrections</span>
                                <span class="stat-value">${totalCorrections}</span>
                            </div>
                        ` : ''}
                        ${errorThoughts.length > 0 ? `
                            <div class="stat-item stat-error">
                                <span class="stat-label">Errors</span>
                                <span class="stat-value">${errorThoughts.length}</span>
                            </div>
                        ` : ''}
                    </div>
                </div>
            `;
        } else {
            summary.classList.add('hidden');
        }
    }

    /**
     * 获取状态图标
     */
    getStatusIcon(status) {
        const icons = {
            pending: `<svg class="status-icon pending" width="16" height="16" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="2" stroke-dasharray="2 2"/>
            </svg>`,
            processing: `<svg class="status-icon processing" width="16" height="16" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="2"/>
                <path d="M8 2A6 6 0 0 1 14 8" stroke="currentColor" stroke-width="2" stroke-linecap="round" class="spin"/>
            </svg>`,
            completed: `<svg class="status-icon completed" width="16" height="16" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="8" r="7" fill="currentColor"/>
                <path d="M5 8L7 10L11 6" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>`,
            corrected: `<svg class="status-icon corrected" width="16" height="16" viewBox="0 0 16 16" fill="none">
                <path d="M8 1L10 5H14L11 8L12 13L8 10L4 13L5 8L2 5H6L8 1Z" fill="currentColor"/>
            </svg>`,
            error: `<svg class="status-icon error" width="16" height="16" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="8" r="7" fill="currentColor"/>
                <path d="M5 5L11 11M11 5L5 11" stroke="white" stroke-width="2" stroke-linecap="round"/>
            </svg>`
        };
        return icons[status] || icons.pending;
    }

    /**
     * 格式化思考类型
     */
    formatType(type) {
        const types = {
            step: 'Step',
            analysis: 'Analysis',
            decision: 'Decision',
            action: 'Action',
            correction: 'Correction'
        };
        return types[type] || type;
    }

    /**
     * 格式化时间
     */
    formatTime(timestamp) {
        const date = new Date(timestamp);
        const now = new Date();
        const diff = now - date;

        if (diff < 60000) {
            return 'Just now';
        } else if (diff < 3600000) {
            return `${Math.floor(diff / 60000)}m ago`;
        } else if (diff < 86400000) {
            return `${Math.floor(diff / 3600000)}h ago`;
        } else {
            return date.toLocaleDateString();
        }
    }

    /**
     * 转义 HTML
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    /**
     * 生成唯一 ID
     */
    generateId() {
        return 'thought-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);
    }

    /**
     * 清空思考过程
     */
    clear() {
        this.thoughts = [];
        this.currentThoughtId = null;
        this.updateTimeline();
        this.updateSummary();
    }

    /**
     * 获取思考数据
     */
    getData() {
        return {
            thoughts: this.thoughts,
            currentThoughtId: this.currentThoughtId
        };
    }

    /**
     * 加载思考数据
     */
    loadData(data) {
        if (data && data.thoughts) {
            this.thoughts = data.thoughts;
            this.currentThoughtId = data.currentThoughtId;
            this.updateTimeline();
            this.updateSummary();
        }
    }
}
