/**
 * 纠错流程交互组件
 * 用于处理 AI 的纠错流程，包括显示错误、用户确认/拒绝等
 */

export class CorrectionPanel {
    constructor(container) {
        this.container = container;
        this.corrections = [];
        this.onResolve = null;
        this.onReject = null;
    }

    /**
     * 渲染纠错面板
     */
    render() {
        this.container.innerHTML = `
            <div class="correction-panel">
                <div class="correction-header">
                    <div class="correction-title">
                        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                            <path d="M10 2L12.5 7H18L13.5 10.5L15 16L10 12.5L5 16L6.5 10.5L2 7H7.5L10 2Z" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/>
                        </svg>
                        <span>Correction Queue</span>
                    </div>
                    <div class="correction-count">
                        <span class="badge badge-warning" id="correction-badge">0</span>
                    </div>
                </div>
                <div class="correction-list" id="correction-list">
                    <div class="correction-empty">
                        <svg width="48" height="48" viewBox="0 0 48 48" fill="none">
                            <path d="M24 4L29 14H40L31 21L34 32L24 25L14 32L17 21L8 14H19L24 4Z" stroke="currentColor" stroke-width="2" stroke-opacity="0.3"/>
                        </svg>
                        <p>No corrections needed</p>
                        <span class="text-secondary">AI corrections will appear here</span>
                    </div>
                </div>
            </div>
        `;

        return this;
    }

    /**
     * 添加纠错项
     */
    addCorrection(correction) {
        const correctionData = {
            id: correction.id || this.generateId(),
            type: correction.type || 'error', // error, suggestion, improvement
            severity: correction.severity || 'medium', // low, medium, high, critical
            title: correction.title,
            description: correction.description,
            originalValue: correction.originalValue,
            suggestedValue: correction.suggestedValue,
            context: correction.context,
            status: correction.status || 'pending', // pending, approved, rejected, dismissed
            timestamp: correction.timestamp || new Date().toISOString()
        };

        this.corrections.push(correctionData);
        this.updateList();
        this.updateBadge();

        return correctionData.id;
    }

    /**
     * 更新纠错状态
     */
    updateStatus(id, status) {
        const correction = this.corrections.find(c => c.id === id);
        if (correction) {
            correction.status = status;
            this.updateList();
            this.updateBadge();
        }
    }

    /**
     * 批准纠错
     */
    approve(id) {
        const correction = this.corrections.find(c => c.id === id);
        if (correction) {
            correction.status = 'approved';

            // 触发动画
            const item = this.container.querySelector(`[data-correction-id="${id}"]`);
            if (item) {
                item.classList.add('approving');
                setTimeout(() => {
                    this.updateList();
                    this.updateBadge();
                    if (this.onResolve) {
                        this.onResolve(correction);
                    }
                }, 300);
            }
        }
    }

    /**
     * 拒绝纠错
     */
    reject(id, reason = '') {
        const correction = this.corrections.find(c => c.id === id);
        if (correction) {
            correction.status = 'rejected';
            correction.rejectReason = reason;

            const item = this.container.querySelector(`[data-correction-id="${id}"]`);
            if (item) {
                item.classList.add('rejecting');
                setTimeout(() => {
                    this.updateList();
                    this.updateBadge();
                    if (this.onReject) {
                        this.onReject(correction);
                    }
                }, 300);
            }
        }
    }

    /**
     * 忽略纠错
     */
    dismiss(id) {
        const correction = this.corrections.find(c => c.id === id);
        if (correction) {
            correction.status = 'dismissed';
            this.updateList();
            this.updateBadge();
        }
    }

    /**
     * 批准所有纠错
     */
    approveAll() {
        const pendingCorrections = this.corrections.filter(c => c.status === 'pending');
        pendingCorrections.forEach(correction => {
            correction.status = 'approved';
        });
        this.updateList();
        this.updateBadge();
    }

    /**
     * 拒绝所有纠错
     */
    rejectAll(reason = '') {
        const pendingCorrections = this.corrections.filter(c => c.status === 'pending');
        pendingCorrections.forEach(correction => {
            correction.status = 'rejected';
            correction.rejectReason = reason;
        });
        this.updateList();
        this.updateBadge();
    }

    /**
     * 更新纠错列表显示
     */
    updateList() {
        const list = this.container.querySelector('#correction-list');
        if (!list) return;

        const activeCorrections = this.corrections.filter(c =>
            c.status === 'pending' || c.status === 'approved' || c.status === 'rejected'
        );

        if (activeCorrections.length === 0) {
            list.innerHTML = `
                <div class="correction-empty">
                    <svg width="48" height="48" viewBox="0 0 48 48" fill="none">
                        <path d="M24 4L29 14H40L31 21L34 32L24 25L14 32L17 21L8 14H19L24 4Z" stroke="currentColor" stroke-width="2" stroke-opacity="0.3"/>
                    </svg>
                    <p>No corrections needed</p>
                    <span class="text-secondary">AI corrections will appear here</span>
                </div>
            `;
            return;
        }

        list.innerHTML = activeCorrections.map(correction => {
            const severityClass = correction.severity;
            const statusClass = correction.status;
            const isPending = correction.status === 'pending';

            return `
                <div class="correction-item ${severityClass} ${statusClass}" data-correction-id="${correction.id}">
                    <div class="correction-item-header">
                        <div class="correction-item-type">
                            <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                                ${this.getTypeIcon(correction.type)}
                            </svg>
                            <span class="correction-type-label">${this.formatType(correction.type)}</span>
                        </div>
                        <div class="correction-item-actions">
                            <span class="correction-severity ${correction.severity}">${correction.severity}</span>
                            ${isPending ? `
                                <button class="btn-icon btn-approve" title="Approve" data-id="${correction.id}">
                                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                                        <path d="M3 8L6 11L13 4" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                                    </svg>
                                </button>
                                <button class="btn-icon btn-reject" title="Reject" data-id="${correction.id}">
                                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                                        <path d="M4 4L12 12M12 4L4 12" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                                    </svg>
                                </button>
                            ` : `
                                <span class="correction-status ${correction.status}">
                                    ${this.getStatusIcon(correction.status)}
                                </span>
                            `}
                        </div>
                    </div>
                    <div class="correction-item-body">
                        <h4 class="correction-title">${this.escapeHtml(correction.title)}</h4>
                        <p class="correction-description">${this.escapeHtml(correction.description)}</p>
                        ${correction.context ? `
                            <div class="correction-context">
                                <span class="context-label">Context:</span>
                                <code class="context-code">${this.escapeHtml(correction.context)}</code>
                            </div>
                        ` : ''}
                        <div class="correction-diff">
                            <div class="diff-item diff-original">
                                <span class="diff-label">Original:</span>
                                <div class="diff-value removed">${this.escapeHtml(correction.originalValue)}</div>
                            </div>
                            <div class="diff-arrow">
                                <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                                    <path d="M3 8H13M13 8L9 4M13 8L9 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                                </svg>
                            </div>
                            <div class="diff-item diff-suggested">
                                <span class="diff-label">Suggested:</span>
                                <div class="diff-value added">${this.escapeHtml(correction.suggestedValue)}</div>
                            </div>
                        </div>
                        ${correction.rejectReason ? `
                            <div class="correction-reject-reason">
                                <span class="reject-label">Reject reason:</span>
                                <span>${this.escapeHtml(correction.rejectReason)}</span>
                            </div>
                        ` : ''}
                    </div>
                </div>
            `;
        }).join('');

        // 绑定事件
        this.bindEvents();
    }

    /**
     * 绑定事件处理
     */
    bindEvents() {
        // 批准按钮
        this.container.querySelectorAll('.btn-approve').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                this.approve(id);
            });
        });

        // 拒绝按钮
        this.container.querySelectorAll('.btn-reject').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                // 可以添加对话框让用户输入拒绝原因
                this.reject(id);
            });
        });
    }

    /**
     * 更新徽章计数
     */
    updateBadge() {
        const badge = this.container.querySelector('#correction-badge');
        if (badge) {
            const pendingCount = this.corrections.filter(c => c.status === 'pending').length;
            badge.textContent = pendingCount;

            if (pendingCount === 0) {
                badge.classList.remove('badge-warning');
                badge.classList.add('badge-success');
            } else {
                badge.classList.remove('badge-success');
                badge.classList.add('badge-warning');
            }
        }
    }

    /**
     * 获取类型图标
     */
    getTypeIcon(type) {
        const icons = {
            error: '<path d="M8 2C4.68629 2 2 4.68629 2 8C2 11.3137 4.68629 14 8 14C11.3137 14 14 11.3137 14 8C14 4.68629 11.3137 2 8 2Z" stroke="currentColor" stroke-width="2"/><path d="M8 5V9M8 11V11.01" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>',
            suggestion: '<path d="M8 2L9.5 5.5H13L10.5 7.5L11.5 11L8 8.5L4.5 11L5.5 7.5L3 5.5H6.5L8 2Z" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/>',
            improvement: '<path d="M3 8H13M13 8L9 4M13 8L9 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>'
        };
        return icons[type] || icons.suggestion;
    }

    /**
     * 获取状态图标
     */
    getStatusIcon(status) {
        const icons = {
            approved: '<svg width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M3 7L5 9L11 3" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>',
            rejected: '<svg width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M3 3L11 11M11 3L3 11" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>',
            dismissed: '<svg width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M3 7H11" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>'
        };
        return icons[status] || '';
    }

    /**
     * 格式化类型
     */
    formatType(type) {
        return type.charAt(0).toUpperCase() + type.slice(1);
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
        return 'correction-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);
    }

    /**
     * 清空纠错列表
     */
    clear() {
        this.corrections = [];
        this.updateList();
        this.updateBadge();
    }

    /**
     * 获取纠错数据
     */
    getData() {
        return {
            corrections: this.corrections
        };
    }

    /**
     * 设置回调函数
     */
    on(event, callback) {
        if (event === 'resolve') {
            this.onResolve = callback;
        } else if (event === 'reject') {
            this.onReject = callback;
        }
    }
}
