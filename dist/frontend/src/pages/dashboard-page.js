/**
 * 使用量统计仪表板
 * 显示每日/每月使用量、成本统计和预算状态
 */

export class DashboardPage {
    constructor(container) {
        this.container = container;
        this.usageData = null;
        this.timeRange = 'month'; // day, week, month, year
    }

    /**
     * 渲染仪表板页面
     */
    async render() {
        this.container.innerHTML = `
            <div class="dashboard-page">
                <div class="page-header">
                    <div class="page-title">
                        <h1>Usage Dashboard</h1>
                        <p class="page-description">Monitor your API usage, costs, and budget</p>
                    </div>
                    <div class="time-range-selector">
                        <button class="filter-tab" data-range="day">Day</button>
                        <button class="filter-tab" data-range="week">Week</button>
                        <button class="filter-tab active" data-range="month">Month</button>
                        <button class="filter-tab" data-range="year">Year</button>
                    </div>
                </div>

                <!-- 统计卡片 -->
                <div class="stats-grid">
                    <div class="stat-card">
                        <div class="stat-card-header">
                            <div class="stat-card-icon stat-icon-primary">
                                <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
                                    <path d="M13 2L3 14H12L11 22L21 10H12L13 2Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                                </svg>
                            </div>
                            <span class="stat-card-label">Total Requests</span>
                        </div>
                        <div class="stat-card-value" id="total-requests">-</div>
                        <div class="stat-card-change positive" id="requests-change">-</div>
                    </div>

                    <div class="stat-card">
                        <div class="stat-card-header">
                            <div class="stat-card-icon stat-icon-success">
                                <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
                                    <path d="M12 2L2 7L12 12L22 7L12 2Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                                    <path d="M2 17L12 22L22 17" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                                    <path d="M2 12L12 17L22 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                                </svg>
                            </div>
                            <span class="stat-card-label">Total Tokens</span>
                        </div>
                        <div class="stat-card-value" id="total-tokens">-</div>
                        <div class="stat-card-change positive" id="tokens-change">-</div>
                    </div>

                    <div class="stat-card">
                        <div class="stat-card-header">
                            <div class="stat-card-icon stat-icon-warning">
                                <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
                                    <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="2"/>
                                    <path d="M12 8V12M12 16V16.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                                </svg>
                            </div>
                            <span class="stat-card-label">Total Cost</span>
                        </div>
                        <div class="stat-card-value" id="total-cost">-</div>
                        <div class="stat-card-change" id="cost-change">-</div>
                    </div>

                    <div class="stat-card">
                        <div class="stat-card-header">
                            <div class="stat-card-icon stat-icon-info">
                                <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
                                    <path d="M19 21V5C19 3.89543 18.1046 3 17 3H7C5.89543 3 5 3.89543 5 5V21" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                                    <path d="M9 7H15M9 11H15M9 15H15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                                </svg>
                            </div>
                            <span class="stat-card-label">Avg Cost/Request</span>
                        </div>
                        <div class="stat-card-value" id="avg-cost">-</div>
                        <div class="stat-card-change" id="avg-cost-change">-</div>
                    </div>
                </div>

                <!-- 图表和详细数据 -->
                <div class="dashboard-content">
                    <div class="dashboard-main">
                        <!-- 使用量图表 -->
                        <div class="chart-card">
                            <div class="chart-card-header">
                                <h3>Usage Over Time</h3>
                                <div class="chart-legend">
                                    <span class="legend-item">
                                        <span class="legend-color legend-primary"></span>
                                        Requests
                                    </span>
                                    <span class="legend-item">
                                        <span class="legend-color legend-success"></span>
                                        Tokens
                                    </span>
                                </div>
                            </div>
                            <div class="chart-container" id="usage-chart">
                                <canvas id="usage-chart-canvas"></canvas>
                            </div>
                        </div>

                        <!-- 模型使用分布 -->
                        <div class="chart-card">
                            <div class="chart-card-header">
                                <h3>Usage by Model</h3>
                            </div>
                            <div class="model-usage-list" id="model-usage-list">
                                <div class="loading">Loading...</div>
                            </div>
                        </div>
                    </div>

                    <!-- 侧边栏 -->
                    <div class="dashboard-sidebar">
                        <!-- 预算状态 -->
                        <div class="budget-card">
                            <div class="budget-card-header">
                                <h3>Budget Status</h3>
                            </div>
                            <div class="budget-content">
                                <div class="budget-progress">
                                    <div class="budget-labels">
                                        <span>Used</span>
                                        <span id="budget-remaining">Remaining</span>
                                    </div>
                                    <div class="progress">
                                        <div class="progress-bar" id="budget-progress-bar" style="width: 0%"></div>
                                    </div>
                                    <div class="budget-values">
                                        <span id="budget-used">$0</span>
                                        <span id="budget-total">of $0</span>
                                    </div>
                                </div>
                                <div class="budget-status" id="budget-status">
                                    <span class="status-indicator"></span>
                                    <span class="status-text">On track</span>
                                </div>
                            </div>
                        </div>

                        <!-- 预算设置 -->
                        <div class="card">
                            <div class="card-header">
                                <h3>Budget Settings</h3>
                            </div>
                            <div class="card-body">
                                <form id="budget-form">
                                    <div class="form-group">
                                        <label for="budget-limit">Monthly Budget Limit ($)</label>
                                        <input type="number" class="input" id="budget-limit" min="0" step="1" value="100">
                                    </div>
                                    <div class="form-group">
                                        <label for="budget-alert">Alert Threshold (%)</label>
                                        <input type="number" class="input" id="budget-alert" min="0" max="100" value="80">
                                    </div>
                                    <button type="submit" class="btn btn-primary btn-sm" style="width: 100%;">
                                        Update Budget
                                    </button>
                                </form>
                            </div>
                        </div>

                        <!-- 最近活动 -->
                        <div class="card">
                            <div class="card-header">
                                <h3>Recent Activity</h3>
                            </div>
                            <div class="card-body">
                                <div class="activity-list" id="activity-list">
                                    <div class="loading">Loading...</div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        `;

        // 绑定事件
        this.bindEvents();

        // 加载数据
        await this.loadData();

        return this;
    }

    /**
     * 绑定事件处理
     */
    bindEvents() {
        // 时间范围选择
        this.container.querySelectorAll('.time-range-selector .filter-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.container.querySelectorAll('.time-range-selector .filter-tab').forEach(t => t.classList.remove('active'));
                e.target.classList.add('active');
                this.timeRange = e.target.dataset.range;
                this.loadData();
            });
        });

        // 预算表单
        this.container.querySelector('#budget-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.updateBudget();
        });
    }

    /**
     * 加载数据
     */
    async loadData() {
        try {
            // 这里应该调用实际的 API
            // const response = await dashboardAPI.getUsageStats(this.timeRange);
            // this.usageData = response;

            // 使用模拟数据
            this.usageData = this.getMockData();
            this.renderStats();
            this.renderUsageChart();
            this.renderModelUsage();
            this.renderBudget();
            this.renderActivity();
        } catch (error) {
            console.error('Error loading dashboard data:', error);
            this.usageData = this.getMockData();
            this.renderStats();
            this.renderUsageChart();
            this.renderModelUsage();
            this.renderBudget();
            this.renderActivity();
        }
    }

    /**
     * 获取模拟数据
     */
    getMockData() {
        const now = new Date();
        const days = this.timeRange === 'day' ? 1 : this.timeRange === 'week' ? 7 : this.timeRange === 'month' ? 30 : 365;

        // 生成时间序列数据
        const timeSeries = [];
        for (let i = days - 1; i >= 0; i--) {
            const date = new Date(now);
            date.setDate(date.getDate() - i);
            timeSeries.push({
                date: date.toISOString().split('T')[0],
                requests: Math.floor(Math.random() * 500) + 100,
                tokens: Math.floor(Math.random() * 100000) + 20000
            });
        }

        return {
            timeRange: this.timeRange,
            totalRequests: timeSeries.reduce((sum, d) => sum + d.requests, 0),
            totalTokens: timeSeries.reduce((sum, d) => sum + d.tokens, 0),
            totalCost: timeSeries.reduce((sum, d) => sum + (d.tokens * 0.00002), 0),
            previousRequests: timeSeries.reduce((sum, d) => sum + d.requests, 0) * 0.9,
            previousTokens: timeSeries.reduce((sum, d) => sum + d.tokens, 0) * 0.85,
            previousCost: timeSeries.reduce((sum, d) => sum + (d.tokens * 0.00002), 0) * 0.8,
            timeSeries,
            modelUsage: [
                { model: 'GPT-4 Turbo', requests: 2340, tokens: 456000, cost: 9.12, percentage: 45 },
                { model: 'Claude 3 Opus', requests: 1890, tokens: 378000, cost: 11.34, percentage: 35 },
                { model: 'GPT-3.5 Turbo', requests: 3450, tokens: 234000, cost: 0.47, percentage: 15 },
                { model: 'Gemini Pro', requests: 890, tokens: 89000, cost: 0.89, percentage: 5 }
            ],
            budget: {
                limit: 50,
                used: 21.82,
                alertThreshold: 80
            },
            recentActivity: [
                { type: 'request', model: 'GPT-4 Turbo', time: new Date(now - 300000), cost: 0.05 },
                { type: 'request', model: 'Claude 3 Opus', time: new Date(now - 600000), cost: 0.08 },
                { type: 'error', model: 'GPT-3.5 Turbo', time: new Date(now - 900000), cost: 0 },
                { type: 'request', model: 'GPT-4 Turbo', time: new Date(now - 1200000), cost: 0.12 },
                { type: 'alert', model: null, time: new Date(now - 3600000), cost: 0, message: 'Budget at 80% capacity' }
            ]
        };
    }

    /**
     * 渲染统计卡片
     */
    renderStats() {
        const data = this.usageData;

        // Total Requests
        this.setElementValue('total-requests', this.formatNumber(data.totalRequests));
        this.setElementValue('requests-change', this.getChangeText(data.totalRequests, data.previousRequests));

        // Total Tokens
        this.setElementValue('total-tokens', this.formatNumber(data.totalTokens));
        this.setElementValue('tokens-change', this.getChangeText(data.totalTokens, data.previousTokens));

        // Total Cost
        this.setElementValue('total-cost', `$${data.totalCost.toFixed(2)}`);
        this.setElementValue('cost-change', this.getChangeText(data.totalCost, data.previousCost, true));

        // Avg Cost per Request
        const avgCost = data.totalRequests > 0 ? data.totalCost / data.totalRequests : 0;
        this.setElementValue('avg-cost', `$${avgCost.toFixed(4)}`);
    }

    /**
     * 渲染使用量图表
     */
    renderUsageChart() {
        const canvas = this.container.querySelector('#usage-chart-canvas');
        const ctx = canvas.getContext('2d');
        const data = this.usageData.timeSeries;

        // 设置画布尺寸
        const container = canvas.parentElement;
        canvas.width = container.offsetWidth;
        canvas.height = 250;

        const width = canvas.width;
        const height = canvas.height;
        const padding = { top: 20, right: 20, bottom: 40, left: 50 };
        const chartWidth = width - padding.left - padding.right;
        const chartHeight = height - padding.top - padding.bottom;

        // 清空画布
        ctx.clearRect(0, 0, width, height);

        // 计算最大值
        const maxRequests = Math.max(...data.map(d => d.requests)) * 1.2;
        const maxTokens = Math.max(...data.map(d => d.tokens)) * 1.2;

        // 绘制网格线
        ctx.strokeStyle = 'var(--border-color)';
        ctx.lineWidth = 1;
        for (let i = 0; i <= 4; i++) {
            const y = padding.top + (chartHeight / 4) * i;
            ctx.beginPath();
            ctx.moveTo(padding.left, y);
            ctx.lineTo(width - padding.right, y);
            ctx.stroke();
        }

        // 绘制请求数折线
        ctx.strokeStyle = '#3b82f6';
        ctx.lineWidth = 2;
        ctx.beginPath();
        data.forEach((d, i) => {
            const x = padding.left + (chartWidth / (data.length - 1)) * i;
            const y = padding.top + chartHeight - (d.requests / maxRequests) * chartHeight;
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        });
        ctx.stroke();

        // 绘制 Token 数折线（右轴）
        ctx.strokeStyle = '#10b981';
        ctx.beginPath();
        data.forEach((d, i) => {
            const x = padding.left + (chartWidth / (data.length - 1)) * i;
            const y = padding.top + chartHeight - (d.tokens / maxTokens) * chartHeight;
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        });
        ctx.stroke();

        // 绘制 X 轴标签
        ctx.fillStyle = 'var(--text-tertiary)';
        ctx.font = '11px sans-serif';
        ctx.textAlign = 'center';
        const labelInterval = Math.ceil(data.length / 6);
        data.forEach((d, i) => {
            if (i % labelInterval === 0) {
                const x = padding.left + (chartWidth / (data.length - 1)) * i;
                const label = new Date(d.date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
                ctx.fillText(label, x, height - 10);
            }
        });
    }

    /**
     * 渲染模型使用分布
     */
    renderModelUsage() {
        const container = this.container.querySelector('#model-usage-list');
        const data = this.usageData.modelUsage;

        container.innerHTML = data.map(model => `
            <div class="model-usage-item">
                <div class="model-usage-info">
                    <span class="model-usage-name">${this.escapeHtml(model.model)}</span>
                    <span class="model-usage-stats">${this.formatNumber(model.requests)} requests</span>
                </div>
                <div class="model-usage-bar">
                    <div class="model-usage-fill" style="width: ${model.percentage}%"></div>
                </div>
                <div class="model-usage-details">
                    <span>${this.formatNumber(model.tokens)} tokens</span>
                    <span>$${model.cost.toFixed(2)}</span>
                </div>
            </div>
        `).join('');
    }

    /**
     * 渲染预算状态
     */
    renderBudget() {
        const budget = this.usageData.budget;
        const percentage = (budget.used / budget.limit) * 100;

        this.setElementValue('budget-used', `$${budget.used.toFixed(2)}`);
        this.setElementValue('budget-total', `of $${budget.limit}`);

        const progressBar = this.container.querySelector('#budget-progress-bar');
        progressBar.style.width = `${Math.min(percentage, 100)}%`;

        // 设置颜色
        if (percentage >= 100) {
            progressBar.style.backgroundColor = 'var(--error)';
        } else if (percentage >= budget.alertThreshold) {
            progressBar.style.backgroundColor = 'var(--warning)';
        } else {
            progressBar.style.backgroundColor = 'var(--success)';
        }

        // 更新预算状态
        const statusText = this.container.querySelector('#budget-status .status-text');
        const statusIndicator = this.container.querySelector('#budget-status .status-indicator');

        if (percentage >= 100) {
            statusText.textContent = 'Budget exceeded';
            statusIndicator.style.backgroundColor = 'var(--error)';
        } else if (percentage >= budget.alertThreshold) {
            statusText.textContent = 'Approaching limit';
            statusIndicator.style.backgroundColor = 'var(--warning)';
        } else {
            statusText.textContent = 'On track';
            statusIndicator.style.backgroundColor = 'var(--success)';
        }
    }

    /**
     * 渲染最近活动
     */
    renderActivity() {
        const container = this.container.querySelector('#activity-list');
        const data = this.usageData.recentActivity;

        container.innerHTML = data.map(activity => `
            <div class="activity-item">
                <div class="activity-icon activity-${activity.type}">
                    ${this.getActivityIcon(activity.type)}
                </div>
                <div class="activity-details">
                    <span class="activity-text">${this.getActivityText(activity)}</span>
                    <span class="activity-time">${this.formatTime(activity.time)}</span>
                </div>
                ${activity.cost > 0 ? `<span class="activity-cost">$${activity.cost.toFixed(2)}</span>` : ''}
            </div>
        `).join('');
    }

    /**
     * 获取活动图标
     */
    getActivityIcon(type) {
        const icons = {
            request: `<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                <path d="M8 1L14 4.5V11.5L8 15L2 11.5V4.5L8 1Z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                <path d="M8 8V15M8 8L14 4.5M8 8L2 4.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>`,
            error: `<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/>
                <path d="M5 5L11 11M11 5L5 11" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
            </svg>`,
            alert: `<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                <path d="M8 1L1 14H15L8 1Z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                <path d="M8 6V9M8 11V11.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
            </svg>`
        };
        return icons[type] || icons.request;
    }

    /**
     * 获取活动文本
     */
    getActivityText(activity) {
        switch (activity.type) {
            case 'request':
                return `API call to ${activity.model}`;
            case 'error':
                return `Error with ${activity.model}`;
            case 'alert':
                return activity.message || 'Budget alert';
            default:
                return 'Unknown activity';
        }
    }

    /**
     * 更新预算
     */
    async updateBudget() {
        const limit = parseFloat(this.container.querySelector('#budget-limit').value);
        const alertThreshold = parseInt(this.container.querySelector('#budget-alert').value);

        try {
            // await budgetAPI.updateBudget({ limit, alertThreshold });
            this.usageData.budget.limit = limit;
            this.usageData.budget.alertThreshold = alertThreshold;
            this.renderBudget();
            alert('Budget updated successfully!');
        } catch (error) {
            console.error('Error updating budget:', error);
            alert('Failed to update budget');
        }
    }

    /**
     * 设置元素值
     */
    setElementValue(id, value) {
        const element = this.container.querySelector(`#${id}`);
        if (element) {
            element.textContent = value;
        }
    }

    /**
     * 获取变化文本
     */
    getChangeText(current, previous, isCost = false) {
        if (!previous || previous === 0) return '-';
        const change = ((current - previous) / previous) * 100;
        const isPositive = change >= 0;
        const className = isCost ? (isPositive ? 'negative' : 'positive') : (isPositive ? 'positive' : 'negative');
        const icon = isPositive ? '↑' : '↓';
        return `<span class="${className}">${icon} ${Math.abs(change).toFixed(1)}%</span>`;
    }

    /**
     * 格式化数字
     */
    formatNumber(num) {
        if (num >= 1000000) {
            return (num / 1000000).toFixed(1) + 'M';
        } else if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'K';
        }
        return num.toString();
    }

    /**
     * 格式化时间
     */
    formatTime(date) {
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
}
