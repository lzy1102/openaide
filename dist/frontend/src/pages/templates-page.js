/**
 * 提示词模板管理页面
 * 用于管理提示词模板，支持列表、创建、编辑、删除和预览功能
 */

export class TemplatesPage {
    constructor(container) {
        this.container = container;
        this.templates = [];
        this.filteredTemplates = [];
        this.currentFilter = 'all';
        this.searchQuery = '';
        this.currentTemplate = null;
    }

    /**
     * 渲染模板管理页面
     */
    async render() {
        this.container.innerHTML = `
            <div class="templates-page">
                <div class="page-header">
                    <div class="page-title">
                        <h1>Prompt Templates</h1>
                        <p class="page-description">Manage and reuse prompt templates</p>
                    </div>
                    <button class="btn btn-primary" id="add-template-btn">
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                            <path d="M8 2V14M2 8H14" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                        </svg>
                        New Template
                    </button>
                </div>

                <div class="templates-controls">
                    <div class="search-box">
                        <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
                            <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="2"/>
                            <path d="M13 13L16 16" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                        </svg>
                        <input type="text" class="input" id="template-search" placeholder="Search templates...">
                    </div>
                    <div class="filter-tabs">
                        <button class="filter-tab active" data-filter="all">All</button>
                        <button class="filter-tab" data-filter="system">System</button>
                        <button class="filter-tab" data-filter="user">User</button>
                        <button class="filter-tab" data-filter="favorite">Favorites</button>
                    </div>
                </div>

                <div class="templates-content">
                    <div class="templates-list" id="templates-list">
                        <div class="loading">Loading templates...</div>
                    </div>
                </div>
            </div>

            <!-- 模板编辑模态框 -->
            <div class="modal hidden" id="template-modal">
                <div class="modal-overlay" id="modal-overlay"></div>
                <div class="modal-content modal-lg">
                    <div class="modal-header">
                        <h2 id="modal-title">New Template</h2>
                        <button class="btn-icon" id="modal-close">
                            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                                <path d="M5 5L15 15M15 5L5 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                            </svg>
                        </button>
                    </div>
                    <div class="modal-body">
                        <form id="template-form">
                            <div class="form-group">
                                <label for="template-name">Template Name</label>
                                <input type="text" class="input" id="template-name" name="name" required placeholder="e.g., Code Review Assistant">
                            </div>
                            <div class="form-group">
                                <label for="template-description">Description</label>
                                <input type="text" class="input" id="template-description" name="description" placeholder="Brief description of the template">
                            </div>
                            <div class="form-group">
                                <label for="template-category">Category</label>
                                <select class="input" id="template-category" name="category">
                                    <option value="general">General</option>
                                    <option value="coding">Coding</option>
                                    <option value="writing">Writing</option>
                                    <option value="analysis">Analysis</option>
                                    <option value="creative">Creative</option>
                                    <option value="custom">Custom</option>
                                </select>
                            </div>
                            <div class="form-group">
                                <label for="template-content">Template Content</label>
                                <textarea class="input template-content-area" id="template-content" name="content" rows="10" required placeholder="Enter your prompt template. Use {{variable}} for placeholders."></textarea>
                                <div class="template-hint">
                                    <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                                        <circle cx="7" cy="7" r="6" stroke="currentColor" stroke-width="1.5"/>
                                        <path d="M7 4V8M7 10V10.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
                                    </svg>
                                    <span>Use <code>{{variable_name}}</code> for dynamic placeholders</span>
                                </div>
                            </div>
                            <div class="form-group">
                                <label>Variables</label>
                                <div class="variables-list" id="variables-list">
                                    <div class="empty-state" style="padding: var(--space-sm); text-align: center; font-size: 12px; color: var(--text-tertiary);">
                                        No variables detected
                                    </div>
                                </div>
                            </div>
                            <div class="form-group">
                                <label class="checkbox-label">
                                    <input type="checkbox" id="template-favorite" name="favorite">
                                    <span>Add to favorites</span>
                                </label>
                            </div>
                        </form>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="modal-cancel">Cancel</button>
                        <button class="btn btn-secondary" id="modal-preview">Preview</button>
                        <button class="btn btn-primary" id="modal-save">Save Template</button>
                    </div>
                </div>
            </div>

            <!-- 预览模态框 -->
            <div class="modal hidden" id="preview-modal">
                <div class="modal-overlay" id="preview-overlay"></div>
                <div class="modal-content modal-lg">
                    <div class="modal-header">
                        <h2>Template Preview</h2>
                        <button class="btn-icon" id="preview-close">
                            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                                <path d="M5 5L15 15M15 5L5 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                            </svg>
                        </button>
                    </div>
                    <div class="modal-body">
                        <div id="preview-variables" class="preview-variables">
                            <h4 style="margin-bottom: var(--space-sm);">Fill in variables:</h4>
                            <div id="preview-variables-form"></div>
                        </div>
                        <div class="form-group" style="margin-top: var(--space-lg);">
                            <label>Preview</label>
                            <div class="preview-content" id="preview-content">
                                <pre id="preview-text"></pre>
                            </div>
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="preview-close-btn">Close</button>
                        <button class="btn btn-primary" id="preview-use">Use Template</button>
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
                        <p>Are you sure you want to delete this template? This action cannot be undone.</p>
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

        // 加载模板列表
        await this.loadTemplates();

        return this;
    }

    /**
     * 绑定事件处理
     */
    bindEvents() {
        // 搜索
        this.container.querySelector('#template-search').addEventListener('input', (e) => {
            this.searchQuery = e.target.value.toLowerCase();
            this.filterTemplates();
        });

        // 过滤标签
        this.container.querySelectorAll('.filter-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.container.querySelectorAll('.filter-tab').forEach(t => t.classList.remove('active'));
                e.target.classList.add('active');
                this.currentFilter = e.target.dataset.filter;
                this.filterTemplates();
            });
        });

        // 添加模板按钮
        this.container.querySelector('#add-template-btn').addEventListener('click', () => {
            this.openModal();
        });

        // 模态框事件
        const modal = this.container.querySelector('#template-modal');
        const modalOverlay = this.container.querySelector('#modal-overlay');
        const modalClose = this.container.querySelector('#modal-close');
        const modalCancel = this.container.querySelector('#modal-cancel');
        const modalSave = this.container.querySelector('#modal-save');
        const modalPreview = this.container.querySelector('#modal-preview');

        modalOverlay.addEventListener('click', () => this.closeModal());
        modalClose.addEventListener('click', () => this.closeModal());
        modalCancel.addEventListener('click', () => this.closeModal());
        modalSave.addEventListener('click', () => this.saveTemplate());
        modalPreview.addEventListener('click', () => this.showPreview());

        // 模板内容变化时更新变量列表
        this.container.querySelector('#template-content').addEventListener('input', (e) => {
            this.updateVariablesList(e.target.value);
        });

        // 预览模态框事件
        const previewModal = this.container.querySelector('#preview-modal');
        previewModal.querySelector('#preview-overlay').addEventListener('click', () => this.closePreviewModal());
        previewModal.querySelector('#preview-close').addEventListener('click', () => this.closePreviewModal());
        previewModal.querySelector('#preview-close-btn').addEventListener('click', () => this.closePreviewModal());
        previewModal.querySelector('#preview-use').addEventListener('click', () => this.useTemplate());

        // 删除模态框事件
        const deleteModal = this.container.querySelector('#delete-modal');
        deleteModal.querySelector('.modal-overlay').addEventListener('click', () => this.closeDeleteModal());
        deleteModal.querySelector('#delete-cancel').addEventListener('click', () => this.closeDeleteModal());
        deleteModal.querySelector('#delete-confirm').addEventListener('click', () => this.confirmDelete());
    }

    /**
     * 加载模板列表
     */
    async loadTemplates() {
        const listContainer = this.container.querySelector('#templates-list');
        listContainer.innerHTML = '<div class="loading">Loading templates...</div>';

        try {
            // 这里应该调用实际的 API
            // const response = await templateAPI.listTemplates();
            // this.templates = response.templates || [];

            // 使用模拟数据
            this.templates = this.getMockTemplates();
            this.filteredTemplates = [...this.templates];
            this.renderTemplates();
        } catch (error) {
            console.error('Error loading templates:', error);
            this.templates = this.getMockTemplates();
            this.filteredTemplates = [...this.templates];
            this.renderTemplates();
        }
    }

    /**
     * 获取模拟模板数据
     */
    getMockTemplates() {
        return [
            {
                id: '1',
                name: 'Code Review Assistant',
                description: 'Helps review code for best practices and potential issues',
                category: 'coding',
                content: 'You are a code review assistant. Please review the following code:\n\n```\n{{code}}\n```\n\nFocus on:\n- Code quality and readability\n- Potential bugs or issues\n- Performance considerations\n- Best practices adherence\n- Security concerns\n\nPlease provide constructive feedback and suggestions.',
                variables: ['code'],
                isFavorite: true,
                isSystem: false,
                createdAt: '2024-01-15T10:00:00Z',
                usageCount: 42
            },
            {
                id: '2',
                name: 'Blog Post Writer',
                description: 'Creates engaging blog posts on any topic',
                category: 'writing',
                content: 'Write a blog post about {{topic}} with the following requirements:\n- Target audience: {{audience}}\n- Tone: {{tone}}\n- Length: {{length}} words\n\nInclude an engaging introduction, informative body paragraphs, and a compelling conclusion.',
                variables: ['topic', 'audience', 'tone', 'length'],
                isFavorite: false,
                isSystem: false,
                createdAt: '2024-01-20T14:30:00Z',
                usageCount: 15
            },
            {
                id: '3',
                name: 'Data Analysis Helper',
                description: 'Assists with data analysis and visualization',
                category: 'analysis',
                content: 'Analyze the following data:\n\n{{data}}\n\nProvide insights on:\n1. Key trends and patterns\n2. Outliers or anomalies\n3. Statistical significance\n4. Recommendations for visualization',
                variables: ['data'],
                isFavorite: true,
                isSystem: false,
                createdAt: '2024-02-01T09:00:00Z',
                usageCount: 28
            },
            {
                id: '4',
                name: 'Story Generator',
                description: 'Generates creative stories with custom parameters',
                category: 'creative',
                content: 'Write a {{genre}} story about a {{character}} who {{plot}}.\n\nThe story should be approximately {{length}} words and written in a {{style}} style.',
                variables: ['genre', 'character', 'plot', 'length', 'style'],
                isFavorite: false,
                isSystem: false,
                createdAt: '2024-02-10T16:00:00Z',
                usageCount: 8
            }
        ];
    }

    /**
     * 过滤模板
     */
    filterTemplates() {
        this.filteredTemplates = this.templates.filter(template => {
            // 搜索过滤
            if (this.searchQuery) {
                const searchStr = `${template.name} ${template.description} ${template.category}`.toLowerCase();
                if (!searchStr.includes(this.searchQuery)) {
                    return false;
                }
            }

            // 分类过滤
            if (this.currentFilter === 'system' && !template.isSystem) return false;
            if (this.currentFilter === 'user' && template.isSystem) return false;
            if (this.currentFilter === 'favorite' && !template.isFavorite) return false;

            return true;
        });

        this.renderTemplates();
    }

    /**
     * 渲染模板列表
     */
    renderTemplates() {
        const listContainer = this.container.querySelector('#templates-list');

        if (this.filteredTemplates.length === 0) {
            listContainer.innerHTML = `
                <div class="empty-state">
                    <svg width="64" height="64" viewBox="0 0 64 64" fill="none">
                        <path d="M32 4L37 15H49L40 23L43 34L32 27L21 34L24 23L15 15H27L32 4Z" stroke="currentColor" stroke-width="2" stroke-opacity="0.2"/>
                    </svg>
                    <h3>No templates found</h3>
                    <p>Try adjusting your search or filter criteria, or create a new template</p>
                </div>
            `;
            return;
        }

        listContainer.innerHTML = `
            <div class="templates-grid">
                ${this.filteredTemplates.map(template => this.renderTemplateCard(template)).join('')}
            </div>
        `;

        // 绑定模板卡片事件
        this.bindTemplateCardEvents();
    }

    /**
     * 渲染单个模板卡片
     */
    renderTemplateCard(template) {
        const categoryColors = {
            general: 'primary',
            coding: 'success',
            writing: 'warning',
            analysis: 'info',
            creative: 'error',
            custom: 'secondary'
        };

        return `
            <div class="template-card" data-template-id="${template.id}">
                <div class="template-card-header">
                    <div class="template-category">
                        <span class="badge badge-${categoryColors[template.category] || 'primary'}">${template.category}</span>
                    </div>
                    <div class="template-actions">
                        ${template.isFavorite ? `
                            <svg class="favorite-icon" width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
                                <path d="M8 1L10 5H14L11 8L12 13L8 10L4 13L5 8L2 5H6L8 1Z"/>
                            </svg>
                        ` : ''}
                    </div>
                </div>
                <div class="template-card-body">
                    <h3 class="template-name">${this.escapeHtml(template.name)}</h3>
                    <p class="template-description">${this.escapeHtml(template.description)}</p>
                    <div class="template-meta">
                        <span class="template-variables-count">
                            <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                                <path d="M6 1L7.5 3.5H10.5L8.5 5.5L9.5 8.5L6 6.5L2.5 8.5L3.5 5.5L1.5 3.5H4.5L6 1Z" stroke="currentColor" stroke-width="1"/>
                            </svg>
                            ${template.variables?.length || 0} variables
                        </span>
                        <span class="template-usage-count">
                            <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                                <path d="M6 1C3.23858 1 1 3.23858 1 6C1 8.76142 3.23858 11 6 11C8.76142 11 11 8.76142 11 6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
                                <path d="M11 3V6H8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                            </svg>
                            ${template.usageCount || 0} uses
                        </span>
                    </div>
                </div>
                <div class="template-card-footer">
                    <button class="btn btn-sm btn-secondary template-preview" data-id="${template.id}">
                        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                            <path d="M1 7H13M1 7L5 3M1 7L5 11" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                        Preview
                    </button>
                    <button class="btn btn-sm btn-secondary template-edit" data-id="${template.id}">
                        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                            <path d="M2 12H12M2 12L5 5M2 12L8 2M12 12L9 5M12 12L6 2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                        Edit
                    </button>
                    <button class="btn btn-sm btn-secondary btn-icon template-delete" data-id="${template.id}">
                        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                            <path d="M3 4H11M4 4V12C4 12.5304 4.21071 13.0391 4.58579 13.4142C4.96086 13.7893 5.46957 14 6 14H8C8.53043 14 9.03914 13.7893 9.41421 13.4142C9.78929 13.0391 10 12.5304 10 12V4M5 4V3C5 2.46957 5.21071 1.96086 5.58579 1.58579C5.96086 1.21071 6.46957 1 7 1H7C7.53043 1 8.03914 1.21071 8.41421 1.58579C8.78929 1.96086 9 2.46957 9 3V4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * 绑定模板卡片事件
     */
    bindTemplateCardEvents() {
        // 预览按钮
        this.container.querySelectorAll('.template-preview').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                const template = this.templates.find(t => t.id === id);
                if (template) {
                    this.showPreviewModal(template);
                }
            });
        });

        // 编辑按钮
        this.container.querySelectorAll('.template-edit').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                const template = this.templates.find(t => t.id === id);
                if (template) {
                    this.openModal(template);
                }
            });
        });

        // 删除按钮
        this.container.querySelectorAll('.template-delete').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                this.openDeleteModal(id);
            });
        });
    }

    /**
     * 打开模态框
     */
    openModal(template = null) {
        const modal = this.container.querySelector('#template-modal');
        const title = this.container.querySelector('#modal-title');
        const form = this.container.querySelector('#template-form');

        title.textContent = template ? 'Edit Template' : 'New Template';

        if (template) {
            form.dataset.mode = 'edit';
            form.dataset.templateId = template.id;
            form.name.value = template.name;
            form.description.value = template.description || '';
            form.category.value = template.category || 'general';
            form.content.value = template.content || '';
            form.favorite.checked = template.isFavorite || false;
            this.updateVariablesList(template.content);
        } else {
            form.dataset.mode = 'create';
            delete form.dataset.templateId;
            form.reset();
            this.updateVariablesList('');
        }

        modal.classList.remove('hidden');
    }

    /**
     * 关闭模态框
     */
    closeModal() {
        const modal = this.container.querySelector('#template-modal');
        modal.classList.add('hidden');
    }

    /**
     * 更新变量列表
     */
    updateVariablesList(content) {
        const variablesList = this.container.querySelector('#variables-list');
        const variables = this.extractVariables(content);

        if (variables.length === 0) {
            variablesList.innerHTML = `
                <div class="empty-state" style="padding: var(--space-sm); text-align: center; font-size: 12px; color: var(--text-tertiary);">
                    No variables detected
                </div>
            `;
            return;
        }

        variablesList.innerHTML = `
            <div class="variables-tags">
                ${variables.map(v => `
                    <span class="variable-tag">
                        <code>{{${v}}}</code>
                    </span>
                `).join('')}
            </div>
        `;
    }

    /**
     * 提取模板中的变量
     */
    extractVariables(content) {
        const regex = /\{\{([^}]+)\}\}/g;
        const variables = [];
        let match;

        while ((match = regex.exec(content)) !== null) {
            if (!variables.includes(match[1])) {
                variables.push(match[1]);
            }
        }

        return variables;
    }

    /**
     * 显示预览
     */
    showPreview() {
        const form = this.container.querySelector('#template-form');
        const template = {
            name: form.name.value,
            content: form.content.value
        };

        this.showPreviewModal(template);
    }

    /**
     * 显示预览模态框
     */
    showPreviewModal(template) {
        this.currentTemplate = template;
        const modal = this.container.querySelector('#preview-modal');
        const variables = this.extractVariables(template.content);
        const formContainer = this.container.querySelector('#preview-variables-form');
        const previewText = this.container.querySelector('#preview-text');

        if (variables.length > 0) {
            this.container.querySelector('#preview-variables').classList.remove('hidden');
            formContainer.innerHTML = variables.map(v => `
                <div class="form-group">
                    <label for="var-${v}">${v}</label>
                    <input type="text" class="input" id="var-${v}" data-variable="${v}" placeholder="Enter ${v}">
                </div>
            `).join('');

            // 绑定输入事件以实时更新预览
            formContainer.querySelectorAll('input').forEach(input => {
                input.addEventListener('input', () => this.updatePreviewContent());
            });
        } else {
            this.container.querySelector('#preview-variables').classList.add('hidden');
        }

        previewText.textContent = template.content;
        modal.classList.remove('hidden');
    }

    /**
     * 更新预览内容
     */
    updatePreviewContent() {
        if (!this.currentTemplate) return;

        const previewText = this.container.querySelector('#preview-text');
        let content = this.currentTemplate.content;

        this.container.querySelectorAll('#preview-variables-form input').forEach(input => {
            const variable = input.dataset.variable;
            const value = input.value || `[${variable}]`;
            content = content.replace(new RegExp(`\\{\\{${variable}\\}\\}`, 'g'), value);
        });

        previewText.textContent = content;
    }

    /**
     * 关闭预览模态框
     */
    closePreviewModal() {
        const modal = this.container.querySelector('#preview-modal');
        modal.classList.add('hidden');
        this.currentTemplate = null;
    }

    /**
     * 使用模板
     */
    useTemplate() {
        // 这里可以实现将模板应用到聊天输入框的功能
        alert('Template applied to chat!');
        this.closePreviewModal();
    }

    /**
     * 保存模板
     */
    async saveTemplate() {
        const form = this.container.querySelector('#template-form');
        const mode = form.dataset.mode;
        const templateId = form.dataset.templateId;

        const templateData = {
            name: form.name.value,
            description: form.description.value,
            category: form.category.value,
            content: form.content.value,
            variables: this.extractVariables(form.content.value),
            isFavorite: form.favorite.checked
        };

        // 验证
        if (!templateData.name || !templateData.content) {
            alert('Please fill in all required fields');
            return;
        }

        try {
            if (mode === 'edit') {
                // await templateAPI.updateTemplate(templateId, templateData);
                const index = this.templates.findIndex(t => t.id === templateId);
                if (index !== -1) {
                    this.templates[index] = { ...this.templates[index], ...templateData };
                }
            } else {
                // const response = await templateAPI.createTemplate(templateData);
                templateData.id = this.generateId();
                templateData.createdAt = new Date().toISOString();
                templateData.usageCount = 0;
                templateData.isSystem = false;
                this.templates.unshift(templateData);
            }

            this.closeModal();
            this.filterTemplates();
        } catch (error) {
            console.error('Error saving template:', error);
            alert('Failed to save template: ' + error.message);
        }
    }

    /**
     * 打开删除确认模态框
     */
    openDeleteModal(id) {
        this.deleteTemplateId = id;
        const modal = this.container.querySelector('#delete-modal');
        modal.classList.remove('hidden');
    }

    /**
     * 关闭删除确认模态框
     */
    closeDeleteModal() {
        const modal = this.container.querySelector('#delete-modal');
        modal.classList.add('hidden');
        this.deleteTemplateId = null;
    }

    /**
     * 确认删除
     */
    async confirmDelete() {
        if (!this.deleteTemplateId) return;

        try {
            // await templateAPI.deleteTemplate(this.deleteTemplateId);
            this.templates = this.templates.filter(t => t.id !== this.deleteTemplateId);
            this.closeDeleteModal();
            this.filterTemplates();
        } catch (error) {
            console.error('Error deleting template:', error);
            alert('Failed to delete template: ' + error.message);
        }
    }

    /**
     * 生成唯一 ID
     */
    generateId() {
        return 'template-' + Date.now();
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
