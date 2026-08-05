// 国际化 (i18n) 模块

const translations = {
    'zh-CN': {
        appName: 'OpenAIDE',
        title: 'OpenAIDE',
        nav: { chat: '对话', templates: '模板', dashboard: '仪表盘', tasks: '任务', models: '模型' },
        chat: {
            conversations: '对话列表', newChat: '新建对话', newConversation: '新对话', previousChat: '历史对话',
            placeholder: '输入你的消息...', thinking: '思考中...',
            welcomeMessage: '你好！我是 OpenAIDE。有什么可以帮助你的吗？',
            sendError: '抱歉，处理消息时出错。错误：'
        },
        thinking: { title: '思考过程' },
        workflows: { title: '工作流', comingSoon: '工作流管理功能即将上线...' },
        skills: { title: '技能', comingSoon: '技能管理功能即将上线...' },
        common: { language: '语言', theme: '主题', settings: '设置' },
        theme: { toggle: '切换主题' },
        language: { zhCN: '简体中文', en: 'English', ja: '日本語', ko: '한국어' }
    },
    'en': {
        appName: 'OpenAIDE',
        title: 'OpenAIDE',
        nav: { chat: 'Chat', templates: 'Templates', dashboard: 'Dashboard', tasks: 'Tasks', models: 'Models' },
        chat: {
            conversations: 'Conversations', newChat: 'New Chat', newConversation: 'New Conversation', previousChat: 'Previous Chat',
            placeholder: 'Type your message...', thinking: 'Thinking...',
            welcomeMessage: 'Hello! I am OpenAIDE. How can I help you today?',
            sendError: 'Sorry, there was an error processing your message. Error: '
        },
        thinking: { title: 'Thinking Process' },
        workflows: { title: 'Workflows', comingSoon: 'Workflow management coming soon...' },
        skills: { title: 'Skills', comingSoon: 'Skill management coming soon...' },
        common: { language: 'Language', theme: 'Theme', settings: 'Settings' },
        theme: { toggle: 'Toggle Theme' },
        language: { zhCN: '简体中文', en: 'English', ja: '日本語', ko: '한국어' }
    },
    'ja': {
        appName: 'OpenAIDE',
        title: 'OpenAIDE',
        nav: { chat: 'チャット', templates: 'テンプレート', dashboard: 'ダッシュボード', tasks: 'タスク', models: 'モデル' },
        chat: {
            conversations: '会話一覧', newChat: '新規チャット', newConversation: '新規会話', previousChat: '過去の会話',
            placeholder: 'メッセージを入力...', thinking: '考え中...',
            welcomeMessage: 'こんにちは！OpenAIDE です。何かお手伝いできることはありますか？',
            sendError: '申し訳ありませんが、メッセージの処理中にエラーが発生しました。エラー：'
        },
        thinking: { title: '思考プロセス' },
        workflows: { title: 'ワークフロー', comingSoon: 'ワークフロー管理機能は近日公開予定...' },
        skills: { title: 'スキル', comingSoon: 'スキル管理機能は近日公開予定...' },
        common: { language: '言語', theme: 'テーマ', settings: '設定' },
        theme: { toggle: 'テーマ切替' },
        language: { zhCN: '简体中文', en: 'English', ja: '日本語', ko: '한국어' }
    },
    'ko': {
        appName: 'OpenAIDE',
        title: 'OpenAIDE',
        nav: { chat: '채팅', templates: '템플릿', dashboard: '대시보드', tasks: '태스크', models: '모델' },
        chat: {
            conversations: '대화 목록', newChat: '새 채팅', newConversation: '새 대화', previousChat: '이전 대화',
            placeholder: '메시지를 입력하세요...', thinking: '생각 중...',
            welcomeMessage: '안녕하세요! OpenAIDE 입니다. 무엇을 도와드릴까요?',
            sendError: '메시지 처리 중 오류가 발생했습니다. 오류: '
        },
        thinking: { title: '생각 과정' },
        workflows: { title: '워크플로우', comingSoon: '워크플로우 관리 기능이 곧 출시됩니다...' },
        skills: { title: '스킬', comingSoon: '스킬 관리 기능이 곧 출시됩니다...' },
        common: { language: '언어', theme: '테마', settings: '설정' },
        theme: { toggle: '테마 전환' },
        language: { zhCN: '简体中文', en: 'English', ja: '日本語', ko: '한국어' }
    }
};

class I18n {
    constructor() {
        this.currentLang = localStorage.getItem('language') || 'zh-CN';
        this.listeners = [];
    }

    getLanguage() {
        return this.currentLang;
    }

    setLanguage(lang) {
        if (translations[lang]) {
            this.currentLang = lang;
            localStorage.setItem('language', lang);
            document.documentElement.lang = lang;
            this.notifyListeners();
            this.updatePageContent();
        }
    }

    t(key) {
        const keys = key.split('.');
        let value = translations[this.currentLang];
        for (const k of keys) {
            if (value && value[k] !== undefined) {
                value = value[k];
            } else {
                value = translations['en'];
                for (const k2 of keys) {
                    if (value && value[k2] !== undefined) {
                        value = value[k2];
                    } else {
                        return key;
                    }
                }
                return value;
            }
        }
        return value;
    }

    addListener(callback) {
        this.listeners.push(callback);
    }

    notifyListeners() {
        this.listeners.forEach(callback => callback(this.currentLang));
    }

    updatePageContent() {
        document.title = this.t('title');
        document.querySelectorAll('[data-i18n]').forEach(el => {
            const key = el.getAttribute('data-i18n');
            const text = this.t(key);
            if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
                el.placeholder = text;
            } else {
                el.textContent = text;
            }
        });
        document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
            const key = el.getAttribute('data-i18n-placeholder');
            el.placeholder = this.t(key);
        });
        document.querySelectorAll('[data-i18n-title]').forEach(el => {
            const key = el.getAttribute('data-i18n-title');
            el.title = this.t(key);
        });
    }

    getSupportedLanguages() {
        return Object.keys(translations);
    }
}

const i18n = new I18n();

function t(key) {
    return i18n.t(key);
}

function setLanguage(lang) {
    i18n.setLanguage(lang);
}

function getLanguage() {
    return i18n.getLanguage();
}

export { i18n, t, setLanguage, getLanguage };
