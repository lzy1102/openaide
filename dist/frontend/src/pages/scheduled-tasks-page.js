/**
 * 定时任务管理页面
 * 支持Cron表达式配置和任务执行历史显示
 */

export class ScheduledTasksPage {
    constructor(container) {
        this.container = container;
        this.tasks = [];
        this.filteredTasks = [];
        this.currentFilter = 'all';
        this.searchQuery = '';
        this.currentTask = null;
        this.executionHistory = {};
    }

    /**
     * 渲染定时任务管理页面
     */
    async render() {
        this.container.innerHTML = `
            <div class="scheduled-tasks-page">
                <div class="page-header">
                    <div class="page-title">
                        <h1>Scheduled Tasks</h1>
                        <p class="page-description">Manage and monitor scheduled automated tasks</p>
                    </div>
                    <button class="btn btn-primary" id="add-task-btn">
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                            <path d="M8 2V14M2 8H14" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                        </svg>
                        New Task
                    </button>
                </div>

                <div class="tasks-controls">
                    <div class="search-box">
                        <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
                            <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="2"/>
                            <path d="M13 13L16 16" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                        </svg>
                        <input type="text" class="input" id="task-search" placeholder="Search tasks...">
                    </div>
                    <div class="filter-tabs">
                        <button class="filter-tab active" data-filter="all">All</button>
                        <button class="filter-tab" data-filter="active">Active</button>
                        <button class="filter-tab" data-filter="paused">Paused</button>
                        <button class="filter-tab" data-filter="failed">Failed</button>
                    </div>
                </div>

                <div class="tasks-content">
                    <div class="tasks-list" id="tasks-list">
                        <div class="loading">Loading tasks...</div>
                    </div>
                </div>
            </div>

            <!-- 任务编辑模态框 -->
            <div class="modal hidden" id="task-modal">
                <div class="modal-overlay" id="modal-overlay"></div>
                <div class="modal-content modal-lg">
                    <div class="modal-header">
                        <h2 id="modal-title">New Scheduled Task</h2>
                        <button class="btn-icon" id="modal-close">
                            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                                <path d="M5 5L15 15M15 5L5 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                            </svg>
                        </button>
                    </div>
                    <div class="modal-body">
                        <form id="task-form">
                            <div class="form-group">
                                <label for="task-name">Task Name</label>
                                <input type="text" class="input" id="task-name" name="name" required placeholder="e.g., Daily Data Sync">
                            </div>
                            <div class="form-group">
                                <label for="task-description">Description</label>
                                <input type="text" class="input" id="task-description" name="description" placeholder="What does this task do?">
                            </div>
                            <div class="form-group">
                                <label for="task-type">Task Type</label>
                                <select class="input" id="task-type" name="type">
                                    <option value="api_call">API Call</option>
                                    <option value="script">Script Execution</option>
                                    <option value="workflow">Workflow Trigger</option>
                                    <option value="notification">Notification</option>
                                    <option value="custom">Custom</option>
                                </select>
                            </div>
                            <div class="form-group">
                                <label>Schedule</label>
                                <div class="schedule-options">
                                    <label class="radio-label">
                                        <input type="radio" name="schedule-type" value="interval" checked>
                                        <span>Interval</span>
                                    </label>
                                    <label class="radio-label">
                                        <input type="radio" name="schedule-type" value="cron">
                                        <span>Cron Expression</span>
                                    </label>
                                </div>
                            </div>
                            <div id="interval-schedule" class="schedule-inputs">
                                <div class="form-row">
                                    <div class="form-col">
                                        <label for="interval-value">Every</label>
                                        <input type="number" class="input" id="interval-value" name="interval_value" min="1" value="1">
                                    </div>
                                    <div class="form-col">
                                        <label for="interval-unit">Unit</label>
                                        <select class="input" id="interval-unit" name="interval_unit">
                                            <option value="minutes">Minutes</option>
                                            <option value="hours">Hours</option>
                                            <option value="days">Days</option>
                                            <option value="weeks">Weeks</option>
                                        </select>
                                    </div>
                                </div>
                            </div>
                            <div id="cron-schedule" class="schedule-inputs hidden">
                                <div class="form-group">
                                    <label for="cron-expression">Cron Expression</label>
                                    <input type="text" class="input" id="cron-expression" name="cron_expression" placeholder="* * * * *">
                                    <div class="cron-hint">
                                        <span>Format: minute hour day month weekday</span>
                                        <a href="#" class="cron-helper-link" id="cron-helper">Cron Helper</a>
                                    </div>
                                </div>
                                <div class="cron-preview" id="cron-preview">
                                    <span class="preview-label">Preview:</span>
                                    <span class="preview-text">Run every minute</span>
                                </div>
                            </div>
                            <div class="form-group">
                                <label for="task-payload">Task Payload (JSON)</label>
                                <textarea class="input" id="task-payload" name="payload" rows="5" placeholder='{"key": "value"}'></textarea>
                            </div>
                            <div class="form-row">
                                <div class="form-col">
                                    <div class="form-group">
                                        <label for="task-retry-max">Max Retries</label>
                                        <input type="number" class="input" id="task-retry-max" name="retry_max" min="0" value="3">
                                    </div>
                                </div>
                                <div class="form-col">
                                    <div class="form-group">
                                        <label for="task-timeout">Timeout (seconds)</label>
                                        <input type="number" class="input" id="task-timeout" name="timeout" min="1" value="300">
                                    </div>
                                </div>
                            </div>
                            <div class="form-group">
                                <label class="checkbox-label">
                                    <input type="checkbox" id="task-enabled" name="enabled" checked>
                                    <span>Enable this task</span>
                                </label>
                            </div>
                        </form>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="modal-cancel">Cancel</button>
                        <button class="btn btn-primary" id="modal-save">Save Task</button>
                    </div>
                </div>
            </div>

            <!-- Cron Helper 模态框 -->
            <div class="modal hidden" id="cron-helper-modal">
                <div class="modal-overlay" id="cron-helper-overlay"></div>
                <div class="modal-content">
                    <div class="modal-header">
                        <h2>Cron Expression Builder</h2>
                        <button class="btn-icon" id="cron-helper-close">
                            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                                <path d="M5 5L15 15M15 5L5 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                            </svg>
                        </button>
                    </div>
                    <div class="modal-body">
                        <div class="cron-builder">
                            <div class="cron-field">
                                <label>Minute</label>
                                <select class="input" id="cron-minute">
                                    ${this.generateCronOptions(59)}
                                </select>
                            </div>
                            <div class="cron-field">
                                <label>Hour</label>
                                <select class="input" id="cron-hour">
                                    ${this.generateCronOptions(23, '*')}
                                </select>
                            </div>
                            <div class="cron-field">
                                <label>Day</label>
                                <select class="input" id="cron-day">
                                    ${this.generateCronOptions(31, '*')}
                                </select>
                            </div>
                            <div class="cron-field">
                                <label>Month</label>
                                <select class="input" id="cron-month">
                                    ${this.generateCronOptions(12, '*', true)}
                                </select>
                            </div>
                            <div class="cron-field">
                                <label>Weekday</label>
                                <select class="input" id="cron-weekday">
                                    <option value="*">Every day</option>
                                    <option value="0">Sunday</option>
                                    <option value="1">Monday</option>
                                    <option value="2">Tuesday</option>
                                    <option value="3">Wednesday</option>
                                    <option value="4">Thursday</option>
                                    <option value="5">Friday</option>
                                    <option value="6">Saturday</option>
                                </select>
                            </div>
                            <div class="cron-result">
                                <label>Generated Expression</label>
                                <input type="text" class="input" id="cron-result-input" readonly value="* * * * *">
                            </div>
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="cron-helper-cancel">Cancel</button>
                        <button class="btn btn-primary" id="cron-helper-apply">Apply</button>
                    </div>
                </div>
            </div>

            <!-- 执行历史模态框 -->
            <div class="modal hidden" id="history-modal">
                <div class="modal-overlay" id="history-overlay"></div>
                <div class="modal-content modal-lg">
                    <div class="modal-header">
                        <h2 id="history-modal-title">Execution History</h2>
                        <button class="btn-icon" id="history-close">
                            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                                <path d="M5 5L15 15M15 5L5 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                            </svg>
                        </button>
                    </div>
                    <div class="modal-body">
                        <div class="execution-history-list" id="execution-history-list">
                            <div class="loading">Loading history...</div>
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="history-close-btn">Close</button>
                    </div>
                </div>
            </div>

            <!-- 删除确认模态框 -->
            <div class="modal hidden" id="delete-modal">
                <div class="modal-overlay"></div>
                <div class="modal-content modal-sm">
                    <div class="modal-header">
                        <h2>Confirm Delete</h2>
                    </div>
                    <div class="modal-body">
                        <p>Are you sure you want to delete this task? This action cannot be undone.</p>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="delete-cancel">Cancel</button>
                        <button class="btn btn-error" id="delete-confirm">Delete</button>
                    </div>
                </div>
            </div>
        `;

        // 绑定事件
        this.bindEvents();

        // 加载任务列表
        await this.loadTasks();

        return this;
    }

    /**
     * 生成 Cron 选项
     */
    generateCronOptions(max, wildcard = null, month = false) {
        let options = wildcard !== null ? `<option value="*">*</option>` : '';
        for (let i = month ? 1 : 0; i <= max; i++) {
            const value = month ? i : (wildcard !== null ? i : '*');
            const label = month ? new Date(0, i - 1).toLocaleDateString('en-US', { month: 'short' }) : i;
            options += `<option value="${value}">${label}</option>`;
        }
        return options;
    }

    /**
     * 绑定事件处理
     */
    bindEvents() {
        // 搜索
        this.container.querySelector('#task-search').addEventListener('input', (e) => {
            this.searchQuery = e.target.value.toLowerCase();
            this.filterTasks();
        });

        // 过滤标签
        this.container.querySelectorAll('.filter-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.container.querySelectorAll('.filter-tab').forEach(t => t.classList.remove('active'));
                e.target.classList.add('active');
                this.currentFilter = e.target.dataset.filter;
                this.filterTasks();
            });
        });

        // 添加任务按钮
        this.container.querySelector('#add-task-btn').addEventListener('click', () => {
            this.openModal();
        });

        // 模态框事件
        const modal = this.container.querySelector('#task-modal');
        const modalOverlay = this.container.querySelector('#modal-overlay');
        const modalClose = this.container.querySelector('#modal-close');
        const modalCancel = this.container.querySelector('#modal-cancel');
        const modalSave = this.container.querySelector('#modal-save');

        modalOverlay.addEventListener('click', () => this.closeModal());
        modalClose.addEventListener('click', () => this.closeModal());
        modalCancel.addEventListener('click', () => this.closeModal());
        modalSave.addEventListener('click', () => this.saveTask());

        // 调度类型切换
        this.container.querySelectorAll('input[name="schedule-type"]').forEach(radio => {
            radio.addEventListener('change', (e) => {
                const intervalDiv = this.container.querySelector('#interval-schedule');
                const cronDiv = this.container.querySelector('#cron-schedule');
                if (e.target.value === 'cron') {
                    intervalDiv.classList.add('hidden');
                    cronDiv.classList.remove('hidden');
                } else {
                    intervalDiv.classList.remove('hidden');
                    cronDiv.classList.add('hidden');
                }
            });
        });

        // Cron Helper
        this.container.querySelector('#cron-helper').addEventListener('click', (e) => {
            e.preventDefault();
            this.openCronHelper();
        });

        // Cron Helper 模态框
        const cronHelperModal = this.container.querySelector('#cron-helper-modal');
        cronHelperModal.querySelector('#cron-helper-overlay').addEventListener('click', () => this.closeCronHelper());
        cronHelperModal.querySelector('#cron-helper-close').addEventListener('click', () => this.closeCronHelper());
        cronHelperModal.querySelector('#cron-helper-cancel').addEventListener('click', () => this.closeCronHelper());
        cronHelperModal.querySelector('#cron-helper-apply').addEventListener('click', () => this.applyCronExpression());

        // Cron 构建器选项
        ['cron-minute', 'cron-hour', 'cron-day', 'cron-month', 'cron-weekday'].forEach(id => {
            this.container.querySelector(`#${id}`).addEventListener('change', () => this.updateCronPreview());
        });

        // 执行历史模态框
        const historyModal = this.container.querySelector('#history-modal');
        historyModal.querySelector('#history-overlay').addEventListener('click', () => this.closeHistoryModal());
        historyModal.querySelector('#history-close').addEventListener('click', () => this.closeHistoryModal());
        historyModal.querySelector('#history-close-btn').addEventListener('click', () => this.closeHistoryModal());

        // 删除模态框
        const deleteModal = this.container.querySelector('#delete-modal');
        deleteModal.querySelector('.modal-overlay').addEventListener('click', () => this.closeDeleteModal());
        deleteModal.querySelector('#delete-cancel').addEventListener('click', () => this.closeDeleteModal());
        deleteModal.querySelector('#delete-confirm').addEventListener('click', () => this.confirmDelete());
    }

    /**
     * 加载任务列表
     */
    async loadTasks() {
        const listContainer = this.container.querySelector('#tasks-list');
        listContainer.innerHTML = '<div class="loading">Loading tasks...</div>';

        try {
            // 使用模拟数据
            this.tasks = this.getMockTasks();
            this.filteredTasks = [...this.tasks];
            this.renderTasks();
        } catch (error) {
            console.error('Error loading tasks:', error);
            this.tasks = this.getMockTasks();
            this.filteredTasks = [...this.tasks];
            this.renderTasks();
        }
    }

    /**
     * 获取模拟任务数据
     */
    getMockTasks() {
        return [
            {
                id: '1',
                name: 'Daily Data Sync',
                description: 'Sync data with external API daily at 2 AM',
                type: 'api_call',
                schedule: 'cron',
                cronExpression: '0 2 * * *',
                nextRun: new Date(Date.now() + 3600000 * 6).toISOString(),
                enabled: true,
                status: 'active',
                lastRun: new Date(Date.now() - 3600000 * 2).toISOString(),
                lastStatus: 'success',
                retryMax: 3,
                timeout: 300,
                executionCount: 145,
                lastExecutionDuration: 23.5
            },
            {
                id: '2',
                name: 'Hourly Report Generation',
                description: 'Generate hourly analytics report',
                type: 'workflow',
                schedule: 'interval',
                intervalValue: 1,
                intervalUnit: 'hours',
                nextRun: new Date(Date.now() + 1800000).toISOString(),
                enabled: true,
                status: 'active',
                lastRun: new Date(Date.now() - 1800000).toISOString(),
                lastStatus: 'success',
                retryMax: 2,
                timeout: 600,
                executionCount: 432,
                lastExecutionDuration: 45.2
            },
            {
                id: '3',
                name: 'Weekly Backup',
                description: 'Create weekly database backup',
                type: 'script',
                schedule: 'cron',
                cronExpression: '0 3 * * 0',
                nextRun: new Date(Date.now() + 3600000 * 48).toISOString(),
                enabled: true,
                status: 'active',
                lastRun: new Date(Date.now() - 3600000 * 24 * 5).toISOString(),
                lastStatus: 'success',
                retryMax: 1,
                timeout: 1800,
                executionCount: 23,
                lastExecutionDuration: 156.8
            },
            {
                id: '4',
                name: 'Failed Notification Task',
                description: 'Send notifications on failures',
                type: 'notification',
                schedule: 'cron',
                cronExpression: '*/30 * * * *',
                nextRun: new Date(Date.now() + 900000).toISOString(),
                enabled: false,
                status: 'paused',
                lastRun: new Date(Date.now() - 3600000).toISOString(),
                lastStatus: 'failed',
                retryMax: 3,
                timeout: 120,
                executionCount: 89,
                lastExecutionDuration: 0,
                lastError: 'Connection timeout'
            }
        ];
    }

    /**
     * 过滤任务
     */
    filterTasks() {
        this.filteredTasks = this.tasks.filter(task => {
            // 搜索过滤
            if (this.searchQuery) {
                const searchStr = `${task.name} ${task.description}`.toLowerCase();
                if (!searchStr.includes(this.searchQuery)) {
                    return false;
                }
            }

            // 状态过滤
            if (this.currentFilter === 'active' && !task.enabled) return false;
            if (this.currentFilter === 'paused' && task.enabled) return false;
            if (this.currentFilter === 'failed' && task.lastStatus !== 'failed') return false;

            return true;
        });

        this.renderTasks();
    }

    /**
     * 渲染任务列表
     */
    renderTasks() {
        const listContainer = this.container.querySelector('#tasks-list');

        if (this.filteredTasks.length === 0) {
            listContainer.innerHTML = `
                <div class="empty-state">
                    <svg width="64" height="64" viewBox="0 0 64 64" fill="none">
                        <path d="M32 8L40 24H56L44 36L48 52L32 42L16 52L20 36L8 24H24L32 8Z" stroke="currentColor" stroke-width="2" stroke-opacity="0.2"/>
                    </svg>
                    <h3>No tasks found</h3>
                    <p>Create a new scheduled task to get started</p>
                </div>
            `;
            return;
        }

        listContainer.innerHTML = `
            <div class="tasks-grid">
                ${this.filteredTasks.map(task => this.renderTaskCard(task)).join('')}
            </div>
        `;

        // 绑定任务卡片事件
        this.bindTaskCardEvents();
    }

    /**
     * 渲染单个任务卡片
     */
    renderTaskCard(task) {
        const typeIcons = {
            api_call: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M8 2L3 6V10L8 14L13 10V6L8 2Z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
            script: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M4 4H12M4 7H10M4 10H8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
            workflow: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M2 8H6L8 4L10 8H14L12 12L10 16" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
            notification: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M8 2L10 5H13L11 8L12 12L8 10L4 12L5 8L3 5H6L8 2Z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
            custom: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/></svg>'
        };

        const statusBadge = task.enabled
            ? (task.lastStatus === 'failed' ? '<span class="badge badge-error">Failed</span>' : '<span class="badge badge-success">Active</span>')
            : '<span class="badge badge-secondary">Paused</span>';

        const scheduleText = task.schedule === 'cron'
            ? `Cron: ${task.cronExpression}`
            : `Every ${task.intervalValue} ${task.intervalUnit}`;

        return `
            <div class="task-card" data-task-id="${task.id}">
                <div class="task-card-header">
                    <div class="task-type">
                        ${typeIcons[task.type] || typeIcons.custom}
                    </div>
                    <div class="task-status">
                        <span class="status-indicator ${task.enabled ? 'enabled' : 'disabled'}"></span>
                    </div>
                </div>
                <div class="task-card-body">
                    <h3 class="task-name">${this.escapeHtml(task.name)}</h3>
                    <p class="task-description">${this.escapeHtml(task.description)}</p>
                    <div class="task-meta">
                        <span class="task-schedule">
                            <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                                <circle cx="6" cy="6" r="5" stroke="currentColor" stroke-width="1"/>
                                <path d="M6 3V6L8 8" stroke="currentColor" stroke-width="1" stroke-linecap="round"/>
                            </svg>
                            ${scheduleText}
                        </span>
                        ${statusBadge}
                    </div>
                    <div class="task-stats">
                        <span class="task-stat">
                            <span class="stat-label">Runs</span>
                            <span class="stat-value">${task.executionCount}</span>
                        </span>
                        ${task.lastExecutionDuration > 0 ? `
                            <span class="task-stat">
                                <span class="stat-label">Last</span>
                                <span class="stat-value">${task.lastExecutionDuration}s</span>
                            </span>
                        ` : ''}
                    </div>
                </div>
                <div class="task-card-footer">
                    <button class="btn btn-sm btn-secondary task-history" data-id="${task.id}">
                        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                            <path d="M2 7H12M2 7L5 4M2 7L5 10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                        History
                    </button>
                    <button class="btn btn-sm ${task.enabled ? 'btn-warning' : 'btn-success'} task-toggle" data-id="${task.id}">
                        ${task.enabled ? 'Pause' : 'Resume'}
                    </button>
                    <button class="btn btn-sm btn-secondary task-edit" data-id="${task.id}">
                        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                            <path d="M2 12H12M2 12L5 5M2 12L8 2M12 12L9 5M12 12L6 2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                    </button>
                    <button class="btn btn-sm btn-secondary btn-icon task-delete" data-id="${task.id}">
                        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                            <path d="M3 4H11M4 4V12C4 12.5304 4.21071 13.0391 4.58579 13.4142C4.96086 13.7893 5.46957 14 6 14H8C8.53043 14 9.03914 13.7893 9.41421 13.4142C9.78929 13.0391 10 12.5304 10 12V4M5 4V3C5 2.46957 5.21071 1.96086 5.58579 1.58579C5.96086 1.21071 6.46957 1 7 1H7C7.53043 1 8.03914 1.21071 8.41421 1.58579C8.78929 1.96086 9 2.46957 9 3V4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * 绑定任务卡片事件
     */
    bindTaskCardEvents() {
        // 历史按钮
        this.container.querySelectorAll('.task-history').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                this.showExecutionHistory(id);
            });
        });

        // 切换按钮
        this.container.querySelectorAll('.task-toggle').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                this.toggleTask(id);
            });
        });

        // 编辑按钮
        this.container.querySelectorAll('.task-edit').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                const task = this.tasks.find(t => t.id === id);
                if (task) {
                    this.openModal(task);
                }
            });
        });

        // 删除按钮
        this.container.querySelectorAll('.task-delete').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                this.openDeleteModal(id);
            });
        });
    }

    /**
     * 打开模态框
     */
    openModal(task = null) {
        const modal = this.container.querySelector('#task-modal');
        const title = this.container.querySelector('#modal-title');
        const form = this.container.querySelector('#task-form');

        title.textContent = task ? 'Edit Task' : 'New Scheduled Task';

        if (task) {
            form.dataset.mode = 'edit';
            form.dataset.taskId = task.id;
            form.name.value = task.name;
            form.description.value = task.description || '';
            form.type.value = task.type;
            form.retry_max.value = task.retryMax;
            form.timeout.value = task.timeout;
            form.enabled.checked = task.enabled;

            if (task.schedule === 'cron') {
                form.querySelector('input[name="schedule-type"][value="cron"]').checked = true;
                this.container.querySelector('#interval-schedule').classList.add('hidden');
                this.container.querySelector('#cron-schedule').classList.remove('hidden');
                form.cron_expression.value = task.cronExpression;
            } else {
                form.querySelector('input[name="schedule-type"][value="interval"]').checked = true;
                this.container.querySelector('#interval-schedule').classList.remove('hidden');
                this.container.querySelector('#cron-schedule').classList.add('hidden');
                form.interval_value.value = task.intervalValue;
                form.interval_unit.value = task.intervalUnit;
            }
        } else {
            form.dataset.mode = 'create';
            delete form.dataset.taskId;
            form.reset();
        }

        modal.classList.remove('hidden');
    }

    /**
     * 关闭模态框
     */
    closeModal() {
        const modal = this.container.querySelector('#task-modal');
        modal.classList.add('hidden');
    }

    /**
     * 打开 Cron Helper
     */
    openCronHelper() {
        const modal = this.container.querySelector('#cron-helper-modal');
        modal.classList.remove('hidden');
    }

    /**
     * 关闭 Cron Helper
     */
    closeCronHelper() {
        const modal = this.container.querySelector('#cron-helper-modal');
        modal.classList.add('hidden');
    }

    /**
     * 更新 Cron 预览
     */
    updateCronPreview() {
        const minute = this.container.querySelector('#cron-minute').value;
        const hour = this.container.querySelector('#cron-hour').value;
        const day = this.container.querySelector('#cron-day').value;
        const month = this.container.querySelector('#cron-month').value;
        const weekday = this.container.querySelector('#cron-weekday').value;

        const expression = `${minute} ${hour} ${day} ${month} ${weekday}`;
        this.container.querySelector('#cron-result-input').value = expression;
    }

    /**
     * 应用 Cron 表达式
     */
    applyCronExpression() {
        const expression = this.container.querySelector('#cron-result-input').value;
        this.container.querySelector('#cron-expression').value = expression;
        this.closeCronHelper();
    }

    /**
     * 显示执行历史
     */
    showExecutionHistory(taskId) {
        const task = this.tasks.find(t => t.id === taskId);
        if (!task) return;

        const modal = this.container.querySelector('#history-modal');
        const title = this.container.querySelector('#history-modal-title');
        title.textContent = `Execution History - ${task.name}`;

        const listContainer = this.container.querySelector('#execution-history-list');
        listContainer.innerHTML = this.renderExecutionHistory(taskId);

        modal.classList.remove('hidden');
    }

    /**
     * 渲染执行历史
     */
    renderExecutionHistory(taskId) {
        const history = this.getMockExecutionHistory(taskId);

        return `
            <div class="history-list">
                ${history.map(item => `
                    <div class="history-item ${item.status}">
                        <div class="history-item-header">
                            <div class="history-status">
                                ${item.status === 'success' ?
                                    '<svg class="status-icon success" width="16" height="16" viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="7" fill="currentColor"/><path d="M5 8L7 10L11 6" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>' :
                                    '<svg class="status-icon error" width="16" height="16" viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="7" fill="currentColor"/><path d="M5 5L11 11M11 5L5 11" stroke="white" stroke-width="2" stroke-linecap="round"/></svg>'
                                }
                            </div>
                            <div class="history-time">${this.formatDateTime(item.startTime)}</div>
                        </div>
                        <div class="history-details">
                            <span class="history-duration">${item.duration}ms</span>
                            ${item.error ? `<span class="history-error">${this.escapeHtml(item.error)}</span>` : ''}
                        </div>
                    </div>
                `).join('')}
            </div>
        `;
    }

    /**
     * 获取模拟执行历史
     */
    getMockExecutionHistory(taskId) {
        const now = Date.now();
        return [
            { startTime: new Date(now - 3600000), status: 'success', duration: 23400, error: null },
            { startTime: new Date(now - 7200000), status: 'success', duration: 22100, error: null },
            { startTime: new Date(now - 10800000), status: 'failed', duration: 5000, error: 'Connection timeout' },
            { startTime: new Date(now - 14400000), status: 'success', duration: 24500, error: null },
            { startTime: new Date(now - 18000000), status: 'success', duration: 22800, error: null }
        ];
    }

    /**
     * 关闭历史模态框
     */
    closeHistoryModal() {
        const modal = this.container.querySelector('#history-modal');
        modal.classList.add('hidden');
    }

    /**
     * 切换任务状态
     */
    async toggleTask(id) {
        const task = this.tasks.find(t => t.id === id);
        if (task) {
            task.enabled = !task.enabled;
            task.status = task.enabled ? 'active' : 'paused';
            this.filterTasks();
        }
    }

    /**
     * 保存任务
     */
    async saveTask() {
        const form = this.container.querySelector('#task-form');
        const mode = form.dataset.mode;
        const taskId = form.dataset.taskId;

        const scheduleType = form.querySelector('input[name="schedule-type"]:checked').value;
        const schedule = scheduleType === 'cron'
            ? { type: 'cron', cronExpression: form.cron_expression.value }
            : { type: 'interval', intervalValue: parseInt(form.interval_value.value), intervalUnit: form.interval_unit.value };

        const taskData = {
            name: form.name.value,
            description: form.description.value,
            type: form.type.value,
            schedule: schedule.type,
            cronExpression: schedule.cronExpression || null,
            intervalValue: schedule.intervalValue || null,
            intervalUnit: schedule.intervalUnit || null,
            retryMax: parseInt(form.retry_max.value),
            timeout: parseInt(form.timeout.value),
            enabled: form.enabled.checked
        };

        // 验证
        if (!taskData.name) {
            alert('Please enter a task name');
            return;
        }

        if (schedule.type === 'cron' && !taskData.cronExpression) {
            alert('Please enter a cron expression');
            return;
        }

        try {
            if (mode === 'edit') {
                const index = this.tasks.findIndex(t => t.id === taskId);
                if (index !== -1) {
                    this.tasks[index] = { ...this.tasks[index], ...taskData };
                }
            } else {
                taskData.id = this.generateId();
                taskData.status = taskData.enabled ? 'active' : 'paused';
                taskData.executionCount = 0;
                taskData.lastStatus = 'pending';
                this.tasks.unshift(taskData);
            }

            this.closeModal();
            this.filterTasks();
        } catch (error) {
            console.error('Error saving task:', error);
            alert('Failed to save task: ' + error.message);
        }
    }

    /**
     * 打开删除确认模态框
     */
    openDeleteModal(id) {
        this.deleteTaskId = id;
        const modal = this.container.querySelector('#delete-modal');
        modal.classList.remove('hidden');
    }

    /**
     * 关闭删除确认模态框
     */
    closeDeleteModal() {
        const modal = this.container.querySelector('#delete-modal');
        modal.classList.add('hidden');
        this.deleteTaskId = null;
    }

    /**
     * 确认删除
     */
    async confirmDelete() {
        if (!this.deleteTaskId) return;

        try {
            this.tasks = this.tasks.filter(t => t.id !== this.deleteTaskId);
            this.closeDeleteModal();
            this.filterTasks();
        } catch (error) {
            console.error('Error deleting task:', error);
            alert('Failed to delete task: ' + error.message);
        }
    }

    /**
     * 生成唯一 ID
     */
    generateId() {
        return 'task-' + Date.now();
    }

    /**
     * 格式化日期时间
     */
    formatDateTime(date) {
        return new Date(date).toLocaleString();
    }

    /**
     * 转义 HTML
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}
