/**
 * 模型配置管理页面
 * 用于管理 AI 模型的配置、启用/禁用等
 */

import { modelAPI } from '../services/api.js';

export class ModelsPage {
    constructor(container) {
        this.container = container;
        this.models = [];
        this.filteredModels = [];
        this.currentFilter = 'all';
        this.searchQuery = '';
    }

    /**
     * 渲染模型管理页面
     */
    async render() {
        this.container.innerHTML = `
            <div class="models-page">
                <div class="page-header">
                    <div class="page-title">
                        <h1>Model Configuration</h1>
                        <p class="page-description">Manage AI models and their configurations</p>
                    </div>
                    <button class="btn btn-primary" id="add-model-btn">
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                            <path d="M8 2V14M2 8H14" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                        </svg>
                        Add Model
                    </button>
                </div>

                <div class="models-controls">
                    <div class="search-box">
                        <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
                            <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="2"/>
                            <path d="M13 13L16 16" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                        </svg>
                        <input type="text" class="input" id="model-search" placeholder="Search models...">
                    </div>
                    <div class="filter-tabs">
                        <button class="filter-tab active" data-filter="all">All</button>
                        <button class="filter-tab" data-filter="enabled">Enabled</button>
                        <button class="filter-tab" data-filter="disabled">Disabled</button>
                        <button class="filter-tab" data-filter="llm">LLM</button>
                        <button class="filter-tab" data-filter="embedding">Embedding</button>
                    </div>
                </div>

                <div class="models-content">
                    <div class="models-list" id="models-list">
                        <div class="loading">Loading models...</div>
                    </div>
                </div>
            </div>

            <!-- 模型编辑模态框 -->
            <div class="modal hidden" id="model-modal">
                <div class="modal-overlay" id="modal-overlay"></div>
                <div class="modal-content">
                    <div class="modal-header">
                        <h2 id="modal-title">Add Model</h2>
                        <button class="btn-icon" id="modal-close">
                            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                                <path d="M5 5L15 15M15 5L5 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                            </svg>
                        </button>
                    </div>
                    <div class="modal-body">
                        <form id="model-form">
                            <div class="form-group">
                                <label for="model-name">Model Name</label>
                                <input type="text" class="input" id="model-name" name="name" required>
                            </div>
                            <div class="form-group">
                                <label for="model-provider">Provider</label>
                                <select class="input" id="model-provider" name="provider" required>
                                    <option value="">Select provider</option>
                                    <option value="openai">OpenAI</option>
                                    <option value="anthropic">Anthropic</option>
                                    <option value="google">Google</option>
                                    <option value="azure">Azure</option>
                                    <option value="local">Local</option>
                                    <option value="custom">Custom</option>
                                </select>
                            </div>
                            <div class="form-group">
                                <label for="model-type">Type</label>
                                <select class="input" id="model-type" name="type" required>
                                    <option value="llm">LLM</option>
                                    <option value="embedding">Embedding</option>
                                    <option value="vision">Vision</option>
                                    <option value="audio">Audio</option>
                                </select>
                            </div>
                            <div class="form-group">
                                <label for="model-version">Version</label>
                                <input type="text" class="input" id="model-version" name="version" placeholder="e.g., gpt-4, claude-3-opus">
                            </div>
                            <div class="form-group">
                                <label for="model-api-key">API Key</label>
                                <input type="password" class="input" id="model-api-key" name="api_key">
                            </div>
                            <div class="form-group">
                                <label for="model-api-base">API Base URL</label>
                                <input type="url" class="input" id="model-api-base" name="api_base" placeholder="https://api.example.com/v1">
                            </div>
                            <div class="form-group">
                                <label>Parameters</label>
                                <div class="form-row">
                                    <div class="form-col">
                                        <label for="model-temperature">Temperature</label>
                                        <input type="number" class="input" id="model-temperature" name="temperature" min="0" max="2" step="0.1" value="0.7">
                                    </div>
                                    <div class="form-col">
                                        <label for="model-max-tokens">Max Tokens</label>
                                        <input type="number" class="input" id="model-max-tokens" name="max_tokens" min="1" value="4096">
                                    </div>
                                </div>
                            </div>
                            <div class="form-group">
                                <label class="checkbox-label">
                                    <input type="checkbox" id="model-enabled" name="enabled" checked>
                                    <span>Enable this model</span>
                                </label>
                            </div>
                        </form>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="modal-cancel">Cancel</button>
                        <button class="btn btn-primary" id="modal-save">Save Model</button>
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
                        <p>Are you sure you want to delete this model? This action cannot be undone.</p>
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

        // 加载模型列表
        await this.loadModels();

        return this;
    }

    /**
     * 绑定事件处理
     */
    bindEvents() {
        // 搜索
        this.container.querySelector('#model-search').addEventListener('input', (e) => {
            this.searchQuery = e.target.value.toLowerCase();
            this.filterModels();
        });

        // 过滤标签
        this.container.querySelectorAll('.filter-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.container.querySelectorAll('.filter-tab').forEach(t => t.classList.remove('active'));
                e.target.classList.add('active');
                this.currentFilter = e.target.dataset.filter;
                this.filterModels();
            });
        });

        // 添加模型按钮
        this.container.querySelector('#add-model-btn').addEventListener('click', () => {
            this.openModal();
        });

        // 模态框事件
        const modal = this.container.querySelector('#model-modal');
        const modalOverlay = this.container.querySelector('#modal-overlay');
        const modalClose = this.container.querySelector('#modal-close');
        const modalCancel = this.container.querySelector('#modal-cancel');
        const modalSave = this.container.querySelector('#modal-save');

        modalOverlay.addEventListener('click', () => this.closeModal());
        modalClose.addEventListener('click', () => this.closeModal());
        modalCancel.addEventListener('click', () => this.closeModal());
        modalSave.addEventListener('click', () => this.saveModel());

        // 删除模态框事件
        const deleteModal = this.container.querySelector('#delete-modal');
        const deleteCancel = this.container.querySelector('#delete-cancel');
        const deleteConfirm = this.container.querySelector('#delete-confirm');

        deleteModal.querySelector('.modal-overlay').addEventListener('click', () => this.closeDeleteModal());
        deleteCancel.addEventListener('click', () => this.closeDeleteModal());
        deleteConfirm.addEventListener('click', () => this.confirmDelete());
    }

    /**
     * 加载模型列表
     */
    async loadModels() {
        const listContainer = this.container.querySelector('#models-list');
        listContainer.innerHTML = '<div class="loading">Loading models...</div>';

        try {
            const response = await modelAPI.listModels();
            this.models = response.models || response || [];
            this.filteredModels = [...this.models];
            this.renderModels();
        } catch (error) {
            console.error('Error loading models:', error);
            // 使用模拟数据
            this.models = this.getMockModels();
            this.filteredModels = [...this.models];
            this.renderModels();
        }
    }

    /**
     * 获取模拟模型数据
     */
    getMockModels() {
        return [
            {
                id: '1',
                name: 'GPT-4 Turbo',
                provider: 'openai',
                type: 'llm',
                version: 'gpt-4-turbo-preview',
                enabled: true,
                parameters: {
                    temperature: 0.7,
                    max_tokens: 4096
                }
            },
            {
                id: '2',
                name: 'Claude 3 Opus',
                provider: 'anthropic',
                type: 'llm',
                version: 'claude-3-opus-20240229',
                enabled: true,
                parameters: {
                    temperature: 0.7,
                    max_tokens: 4096
                }
            },
            {
                id: '3',
                name: 'Text Embedding ADA',
                provider: 'openai',
                type: 'embedding',
                version: 'text-embedding-ada-002',
                enabled: true,
                parameters: {}
            },
            {
                id: '4',
                name: 'Gemini Pro',
                provider: 'google',
                type: 'llm',
                version: 'gemini-pro',
                enabled: false,
                parameters: {
                    temperature: 0.7,
                    max_tokens: 2048
                }
            }
        ];
    }

    /**
     * 过滤模型
     */
    filterModels() {
        this.filteredModels = this.models.filter(model => {
            // 搜索过滤
            if (this.searchQuery) {
                const searchStr = `${model.name} ${model.provider} ${model.version}`.toLowerCase();
                if (!searchStr.includes(this.searchQuery)) {
                    return false;
                }
            }

            // 状态过滤
            if (this.currentFilter === 'enabled' && !model.enabled) return false;
            if (this.currentFilter === 'disabled' && model.enabled) return false;
            if (this.currentFilter === 'llm' && model.type !== 'llm') return false;
            if (this.currentFilter === 'embedding' && model.type !== 'embedding') return false;

            return true;
        });

        this.renderModels();
    }

    /**
     * 渲染模型列表
     */
    renderModels() {
        const listContainer = this.container.querySelector('#models-list');

        if (this.filteredModels.length === 0) {
            listContainer.innerHTML = `
                <div class="empty-state">
                    <svg width="64" height="64" viewBox="0 0 64 64" fill="none">
                        <path d="M32 4L37 15H49L40 23L43 34L32 27L21 34L24 23L15 15H27L32 4Z" stroke="currentColor" stroke-width="2" stroke-opacity="0.2"/>
                    </svg>
                    <h3>No models found</h3>
                    <p>Try adjusting your search or filter criteria</p>
                </div>
            `;
            return;
        }

        listContainer.innerHTML = `
            <div class="models-grid">
                ${this.filteredModels.map(model => this.renderModelCard(model)).join('')}
            </div>
        `;

        // 绑定模型卡片事件
        this.bindModelCardEvents();
    }

    /**
     * 渲染单个模型卡片
     */
    renderModelCard(model) {
        const typeColors = {
            llm: 'primary',
            embedding: 'success',
            vision: 'warning',
            audio: 'info'
        };

        const providerIcons = {
            openai: '<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M14.5 8a6.5 6.5 0 1 0-6.5 6.5V8H14.5z"/></svg>',
            anthropic: '<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 2L6 6H2L4 10L2 14L6 12L8 16L10 12L14 14L12 10L14 6H10L8 2z"/></svg>',
            google: '<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 2C4.686 2 2 4.686 2 8C2 11.314 4.686 14 8 14C11.314 14 14 11.314 14 8C14 4.686 11.314 2 8 2Z"/></svg>',
            azure: '<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M1 8L8 1L15 8L8 15L1 8Z"/></svg>',
            local: '<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M4 4H12V12H4V4Z"/></svg>',
            custom: '<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 2L10 6H14L11 9L12 14L8 11L4 14L5 9L2 6H6L8 2Z"/></svg>'
        };

        return `
            <div class="model-card" data-model-id="${model.id}">
                <div class="model-card-header">
                    <div class="model-provider">
                        ${providerIcons[model.provider] || providerIcons.custom}
                        <span class="provider-name">${model.provider}</span>
                    </div>
                    <div class="model-status">
                        <span class="status-indicator ${model.enabled ? 'enabled' : 'disabled'}"></span>
                    </div>
                </div>
                <div class="model-card-body">
                    <h3 class="model-name">${model.name}</h3>
                    <p class="model-version">${model.version}</p>
                    <div class="model-tags">
                        <span class="badge badge-${typeColors[model.type] || 'primary'}">${model.type.toUpperCase()}</span>
                        ${model.enabled ? '<span class="badge badge-success">Active</span>' : '<span class="badge badge-secondary">Inactive</span>'}
                    </div>
                    ${model.parameters ? `
                        <div class="model-params">
                            ${model.parameters.temperature !== undefined ? `<span>Temp: ${model.parameters.temperature}</span>` : ''}
                            ${model.parameters.max_tokens ? `<span>Max: ${model.parameters.max_tokens}</span>` : ''}
                        </div>
                    ` : ''}
                </div>
                <div class="model-card-footer">
                    <button class="btn btn-sm btn-secondary model-edit" data-id="${model.id}">
                        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                            <path d="M2 12H12M2 12L5 5M2 12L8 2M12 12L9 5M12 12L6 2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
                        </svg>
                        Edit
                    </button>
                    <button class="btn btn-sm ${model.enabled ? 'btn-error' : 'btn-success'} model-toggle" data-id="${model.id}">
                        ${model.enabled ? 'Disable' : 'Enable'}
                    </button>
                    <button class="btn btn-sm btn-secondary btn-icon model-delete" data-id="${model.id}">
                        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                            <path d="M3 4H11M4 4V12C4 12.5304 4.21071 13.0391 4.58579 13.4142C4.96086 13.7893 5.46957 14 6 14H8C8.53043 14 9.03914 13.7893 9.41421 13.4142C9.78929 13.0391 10 12.5304 10 12V4M5 4V3C5 2.46957 5.21071 1.96086 5.58579 1.58579C5.96086 1.21071 6.46957 1 7 1H7C7.53043 1 8.03914 1.21071 8.41421 1.58579C8.78929 1.96086 9 2.46957 9 3V4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * 绑定模型卡片事件
     */
    bindModelCardEvents() {
        // 编辑按钮
        this.container.querySelectorAll('.model-edit').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                const model = this.models.find(m => m.id === id);
                if (model) {
                    this.openModal(model);
                }
            });
        });

        // 启用/禁用按钮
        this.container.querySelectorAll('.model-toggle').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                const id = e.currentTarget.dataset.id;
                const model = this.models.find(m => m.id === id);
                if (model) {
                    await this.toggleModel(model);
                }
            });
        });

        // 删除按钮
        this.container.querySelectorAll('.model-delete').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                this.openDeleteModal(id);
            });
        });
    }

    /**
     * 打开模态框
     */
    openModal(model = null) {
        const modal = this.container.querySelector('#model-modal');
        const title = this.container.querySelector('#modal-title');
        const form = this.container.querySelector('#model-form');

        title.textContent = model ? 'Edit Model' : 'Add Model';

        if (model) {
            form.dataset.mode = 'edit';
            form.dataset.modelId = model.id;
            form.name.value = model.name;
            form.provider.value = model.provider;
            form.type.value = model.type;
            form.version.value = model.version || '';
            form.temperature.value = model.parameters?.temperature || 0.7;
            form.max_tokens.value = model.parameters?.max_tokens || 4096;
            form.enabled.checked = model.enabled;
        } else {
            form.dataset.mode = 'create';
            delete form.dataset.modelId;
            form.reset();
        }

        modal.classList.remove('hidden');
    }

    /**
     * 关闭模态框
     */
    closeModal() {
        const modal = this.container.querySelector('#model-modal');
        modal.classList.add('hidden');
    }

    /**
     * 保存模型
     */
    async saveModel() {
        const form = this.container.querySelector('#model-form');
        const mode = form.dataset.mode;
        const modelId = form.dataset.modelId;

        const modelData = {
            name: form.name.value,
            provider: form.provider.value,
            type: form.type.value,
            version: form.version.value,
            parameters: {
                temperature: parseFloat(form.temperature.value),
                max_tokens: parseInt(form.max_tokens.value)
            },
            enabled: form.enabled.checked
        };

        try {
            if (mode === 'edit') {
                await modelAPI.updateModel(modelId, modelData);
                // 更新本地数据
                const index = this.models.findIndex(m => m.id === modelId);
                if (index !== -1) {
                    this.models[index] = { ...this.models[index], ...modelData };
                }
            } else {
                const response = await modelAPI.createModel(modelData);
                this.models.push({ ...modelData, id: response.id || this.generateId() });
            }

            this.closeModal();
            this.filterModels();
        } catch (error) {
            console.error('Error saving model:', error);
            // 在实际项目中，应该显示错误提示
            alert('Failed to save model: ' + error.message);
        }
    }

    /**
     * 切换模型状态
     */
    async toggleModel(model) {
        const newStatus = !model.enabled;

        try {
            if (newStatus) {
                await modelAPI.enableModel(model.id);
            } else {
                await modelAPI.disableModel(model.id);
            }

            model.enabled = newStatus;
            this.filterModels();
        } catch (error) {
            console.error('Error toggling model:', error);
            alert('Failed to update model status: ' + error.message);
        }
    }

    /**
     * 打开删除确认模态框
     */
    openDeleteModal(id) {
        this.deleteModelId = id;
        const modal = this.container.querySelector('#delete-modal');
        modal.classList.remove('hidden');
    }

    /**
     * 关闭删除确认模态框
     */
    closeDeleteModal() {
        const modal = this.container.querySelector('#delete-modal');
        modal.classList.add('hidden');
        this.deleteModelId = null;
    }

    /**
     * 确认删除
     */
    async confirmDelete() {
        if (!this.deleteModelId) return;

        try {
            await modelAPI.deleteModel(this.deleteModelId);
            this.models = this.models.filter(m => m.id !== this.deleteModelId);
            this.closeDeleteModal();
            this.filterModels();
        } catch (error) {
            console.error('Error deleting model:', error);
            alert('Failed to delete model: ' + error.message);
        }
    }

    /**
     * 生成唯一 ID
     */
    generateId() {
        return 'model-' + Date.now();
    }
}
