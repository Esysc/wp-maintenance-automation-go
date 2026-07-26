(function () {
    const icons = {
        '/dashboard': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 13h8V3H3v10zm10 8h8V11h-8v10zM3 21h8v-6H3v6zm10-10h8V3h-8v8z"/></svg>',
        '/sites': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2l9 4.9v10.2L12 22l-9-4.9V6.9L12 2zm0 2.3L5 8v8.1l7 3.8 7-3.8V8l-7-3.7z"/></svg>',
        '/backups': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M19 8h-1V6a6 6 0 10-12 0v2H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2v-9a2 2 0 00-2-2zm-7 8a3 3 0 110-6 3 3 0 010 6zm4-8H8V6a4 4 0 118 0v2z"/></svg>',
        '/restore': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5V1L7 6l5 5V7a5 5 0 11-5 5H5a7 7 0 107-7z"/></svg>',
        '/upgrade': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M13 3l7 7h-4v7h-6v-7H6l7-7zm-9 16h16v2H4v-2z"/></svg>',
        '/snapshots': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3a9 9 0 00-9 9h2a7 7 0 1114 0h2a9 9 0 00-9-9zm-1 5h2v5h-2V8zm0 7h2v2h-2v-2z"/></svg>',
        '/healthcheck': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 13h4l2-4 3 8 2-4h7v-2h-6l-3 6-3-8-2 4H3v2z"/></svg>',
        '/users': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M16 11c1.7 0 3-1.3 3-3s-1.3-3-3-3-3 1.3-3 3 1.3 3 3 3zm-8 0c1.7 0 3-1.3 3-3S9.7 5 8 5 5 6.3 5 8s1.3 3 3 3zm0 2c-2.3 0-7 1.2-7 3.5V19h14v-2.5C15 14.2 10.3 13 8 13zm8 0c-.3 0-.7 0-1.1.1 1.2.8 2.1 1.9 2.1 3.4V19h6v-2.5c0-2.3-4.7-3.5-7-3.5z"/></svg>',
        '/tokens': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M21 7h-2.6A5 5 0 1013 13.9V17h-3v3H7v-3H3v-3h2v-3h3.1A5 5 0 1016 7h5V7zm-10 4a3 3 0 110-6 3 3 0 010 6z"/></svg>',
        '/system': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M19.4 13a7.7 7.7 0 000-2l2.1-1.6-2-3.5-2.5 1a7.2 7.2 0 00-1.7-1l-.4-2.7H9.1L8.7 6a7.2 7.2 0 00-1.7 1l-2.5-1-2 3.5L4.6 11a7.7 7.7 0 000 2l-2.1 1.6 2 3.5 2.5-1a7.2 7.2 0 001.7 1l.4 2.7h5.8l.4-2.7a7.2 7.2 0 001.7-1l2.5 1 2-3.5-2.1-1.6zM12 15.5a3.5 3.5 0 110-7 3.5 3.5 0 010 7z"/></svg>',
        '/api/docs': '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 2h9l5 5v15a2 2 0 01-2 2H6a2 2 0 01-2-2V4a2 2 0 012-2zm8 1.5V8h4.5"/></svg>'
    };

    const translations = Object.create(null);
    const LANGUAGE_STORAGE_KEY = 'wpma-language';
    const LANGUAGE_OPTIONS = [
        { code: 'en', label: 'English' },
        { code: 'fr', label: 'Français' },
        { code: 'it', label: 'Italiano' },
        { code: 'es', label: 'Español' },
        { code: 'pt', label: 'Português' },
        { code: 'zh', label: '中文' },
        { code: 'ja', label: '日本語' },
        { code: 'ko', label: '한국어' },
        { code: 'ru', label: 'Русский' }
    ];
    const state = { activeModal: null, currentLanguage: 'en' };

    function normalizeLanguage(lang) {
        const normalized = (lang || 'en').split('-')[0].toLowerCase();
        const valid = ['en', 'fr', 'it', 'es', 'pt', 'zh', 'ja', 'ko', 'ru'];
        return valid.includes(normalized) ? normalized : 'en';
    }

    function loadLocale(language) {
        const norm = normalizeLanguage(language);
        if (Object.prototype.hasOwnProperty.call(translations, norm) && translations[norm]) {
            return translations[norm];
        }
        try {
            const xhr = new XMLHttpRequest();
            xhr.open('GET', '/static/locales/' + norm + '.json', false);
            xhr.send(null);
            if (xhr.status >= 200 && xhr.status < 300) {
                translations[norm] = JSON.parse(xhr.responseText);
                return translations[norm];
            }
        } catch (_) {}
        translations[norm] = translations.en || null;
        return translations.en || null;
    }

    function getPreferredLanguage() {
        try {
            const stored = localStorage.getItem(LANGUAGE_STORAGE_KEY);
            if (stored) return normalizeLanguage(stored);
        } catch (_) {}
        return normalizeLanguage(navigator.language);
    }

    loadLocale('en');
    const preferredLanguage = getPreferredLanguage();
    if (preferredLanguage !== 'en') loadLocale(preferredLanguage);
    state.currentLanguage = preferredLanguage;

    function getTranslation(key) {
        const lang = state.currentLanguage || 'en';
        if (translations[lang] && translations[lang][key]) return translations[lang][key];
        if (translations.en && translations.en[key]) return translations.en[key];
        return key;
    }

    function t(key) {
        return getTranslation(key);
    }

    function applyTranslations(root) {
        const scope = root || document;
        scope.querySelectorAll('[data-i18n]').forEach((element) => {
            const key = element.getAttribute('data-i18n');
            if (!key) return;
            const value = getTranslation(key);
            const prefix = element.getAttribute('data-i18n-prefix') || '';
            const suffix = element.getAttribute('data-i18n-suffix') || '';
            const output = prefix + value + suffix;
            if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
                element.value = output;
            } else {
                element.textContent = output;
            }
        });

        scope.querySelectorAll('[data-i18n-placeholder]').forEach((element) => {
            const key = element.getAttribute('data-i18n-placeholder');
            if (!key) return;
            element.setAttribute('placeholder', getTranslation(key));
        });

        scope.querySelectorAll('[data-i18n-title]').forEach((element) => {
            const key = element.getAttribute('data-i18n-title');
            if (!key) return;
            element.setAttribute('title', getTranslation(key));
        });

        scope.querySelectorAll('[data-i18n-aria-label]').forEach((element) => {
            const key = element.getAttribute('data-i18n-aria-label');
            if (!key) return;
            element.setAttribute('aria-label', getTranslation(key));
        });
    }

    function populateLanguageSelectors(root) {
        const scope = root || document;
        scope.querySelectorAll('[data-language-selector]').forEach((select) => {
            if (!select.dataset.languageBound) {
                select.innerHTML = '';
                LANGUAGE_OPTIONS.forEach((option) => {
                    const optionEl = document.createElement('option');
                    optionEl.value = option.code;
                    optionEl.textContent = option.label;
                    select.appendChild(optionEl);
                });
                select.addEventListener('change', () => {
                    setLanguage(select.value);
                });
                select.dataset.languageBound = 'true';
            }
            select.value = state.currentLanguage;
            const label = select.closest('.language-selector')?.querySelector('[data-language-label]');
            if (label) label.textContent = getTranslation('form_language');
        });
    }

    function setLanguage(language, options) {
        const normalized = normalizeLanguage(language);
        loadLocale('en');
        if (normalized !== 'en') loadLocale(normalized);
        state.currentLanguage = normalized;
        document.documentElement.lang = normalized;

        if (!options || options.persist !== false) {
            try {
                localStorage.setItem(LANGUAGE_STORAGE_KEY, normalized);
            } catch (_) {}
        }

        applyTranslations();
        populateLanguageSelectors();
        document.dispatchEvent(new CustomEvent('app:languagechange', { detail: { language: normalized } }));
        return normalized;
    }

    function escapeHTML(value) {
        return String(value || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    }

    function getToken() {
        const match = document.cookie.match(/(?:^| )token=([^;]+)/);
        return match ? match[1] : '';
    }

    function ensureChrome() {
        const sidebar = document.querySelector('.sidebar');
        if (!sidebar) return;
        if (!document.querySelector('.sidebar-toggle')) {
            const toggle = document.createElement('button');
            toggle.type = 'button';
            toggle.className = 'sidebar-toggle';
            toggle.setAttribute('aria-label', getTranslation('nav_dashboard'));
            toggle.innerHTML = '<span></span><span></span><span></span>';
            toggle.addEventListener('click', () => { document.body.classList.toggle('sidebar-open'); });
            document.body.appendChild(toggle);
        }
        if (!document.querySelector('.sidebar-backdrop')) {
            const backdrop = document.createElement('button');
            backdrop.type = 'button';
            backdrop.className = 'sidebar-backdrop';
            backdrop.setAttribute('aria-label', getTranslation('nav_dashboard'));
            backdrop.addEventListener('click', () => { document.body.classList.remove('sidebar-open'); });
            document.body.appendChild(backdrop);
        }
        const navLinks = sidebar.querySelectorAll('.nav-list a');
        navLinks.forEach((a) => {
            if (a.querySelector('.nav-icon')) return;
            const icon = Object.keys(icons).find((prefix) => a.getAttribute('href').indexOf(prefix) === 0);
            const iconSpan = document.createElement('span');
            iconSpan.className = 'nav-icon';
            iconSpan.innerHTML = icons[icon] || icons['/system'];
            const textSpan = document.createElement('span');
            textSpan.className = 'nav-label';
            let href = a.getAttribute('href');
            let textKey = '';
            if (href === '/dashboard') textKey = 'nav_dashboard';
            else if (href === '/sites') textKey = 'nav_sites';
            else if (href === '/backups') textKey = 'nav_backups';
            else if (href === '/restore') textKey = 'nav_restore';
            else if (href === '/upgrade') textKey = 'nav_upgrade';
            else if (href === '/snapshots') textKey = 'nav_snapshots';
            else if (href === '/healthcheck') textKey = 'nav_healthcheck';
            else if (href === '/users') textKey = 'nav_users';
            else if (href === '/tokens') textKey = 'nav_tokens';
            else if (href === '/system') textKey = 'nav_system';
            else if (href === '/api/docs') textKey = 'nav_docs';
            textSpan.textContent = getTranslation(textKey) || a.textContent.trim();
            a.textContent = '';
            a.appendChild(iconSpan);
            a.appendChild(textSpan);
        });
    }

    function ensureToastStack() {
        let stack = document.querySelector('.toast-stack');
        if (!stack) {
            stack = document.createElement('div');
            stack.className = 'toast-stack';
            document.body.appendChild(stack);
        }
        return stack;
    }

    function toast(message, type, options) {
        const stack = ensureToastStack();
        const item = document.createElement('div');
        const duration = options && typeof options.duration === 'number' ? options.duration : 3200;
        const icon = type === 'success' ? 'ok' : type === 'error' ? '!' : type === 'warning' ? '!' : 'i';
        item.className = 'toast toast-' + (type || 'info');
        item.innerHTML = '<div class="toast-head"><span class="toast-icon" aria-hidden="true">' + icon + '</span><span class="toast-message">' + escapeHTML(message) + '</span><button type="button" class="toast-close" aria-label="Dismiss">x</button></div><span class="toast-progress"></span>';
        stack.appendChild(item);
        const closeBtn = item.querySelector('.toast-close');
        function closeToast() {
            item.classList.remove('show');
            setTimeout(() => item.remove(), 180);
        }
        closeBtn.addEventListener('click', closeToast);
        requestAnimationFrame(() => { item.classList.add('show'); });
        setTimeout(() => { closeToast(); }, duration);
    }

    function buildSkeleton(type) {
        if (type === 'table') {
            return '<div class="skeleton-table"><div class="skeleton-row"></div><div class="skeleton-row"></div><div class="skeleton-row"></div></div>';
        }
        return '<div class="skeleton-cards"><article class="skeleton-card"></article><article class="skeleton-card"></article><article class="skeleton-card"></article></div>';
    }

    function setLoading(containerId, isLoading, type) {
        const el = document.getElementById(containerId);
        if (!el) return;
        if (isLoading) {
            el.setAttribute('data-prev', el.innerHTML);
            el.innerHTML = buildSkeleton(type || 'cards');
            return;
        }
        if (el.getAttribute('data-prev') !== null && !el.innerHTML.trim()) {
            el.innerHTML = el.getAttribute('data-prev') || '';
        }
        el.removeAttribute('data-prev');
    }

    function applyRevealAnimations() {
        const revealTargets = document.querySelectorAll('.card, .stat-card, .action-card, .entity-card');
        revealTargets.forEach((el, index) => {
            el.style.setProperty('--reveal-delay', String(index * 35) + 'ms');
            el.classList.add('reveal');
        });
    }

    function initTopbar() {
        const main = document.querySelector('.main-content');
        const activeLink = document.querySelector('.nav-list a.active');
        if (!main || !activeLink || main.querySelector('.page-topbar')) return;
        const topbar = document.createElement('div');
        topbar.className = 'page-topbar';
        const now = new Date();
        const dateLabel = now.toLocaleDateString(state.currentLanguage || 'en', { weekday: 'short', month: 'short', day: 'numeric' });
        topbar.innerHTML = '<div class="crumbs"><span class="crumb-chip">' + getTranslation('header_control_panel') + '</span><span class="crumb-sep">/</span><span class="crumb-current">' + escapeHTML(getTranslation(activeLink.getAttribute('href') === '/dashboard' ? 'nav_dashboard' : activeLink.getAttribute('href') === '/sites' ? 'nav_sites' : activeLink.getAttribute('href') === '/backups' ? 'nav_backups' : activeLink.getAttribute('href') === '/restore' ? 'nav_restore' : activeLink.getAttribute('href') === '/upgrade' ? 'nav_upgrade' : activeLink.getAttribute('href') === '/snapshots' ? 'nav_snapshots' : activeLink.getAttribute('href') === '/healthcheck' ? 'nav_healthcheck' : activeLink.getAttribute('href') === '/users' ? 'nav_users' : activeLink.getAttribute('href') === '/tokens' ? 'nav_tokens' : activeLink.getAttribute('href') === '/system' ? 'nav_system' : activeLink.getAttribute('href') === '/api/docs' ? 'nav_docs' : 'nav_dashboard')) + '</span></div><div class="topbar-meta"><span class="meta-pill">' + getTranslation('header_live_api') + '</span><span class="meta-pill">' + escapeHTML(dateLabel) + '</span></div>';
        main.prepend(topbar);
    }

    function closeModal() {
        if (!state.activeModal) return;
        state.activeModal.classList.remove('show');
        state.activeModal.style.display = 'none';
        document.body.classList.remove('has-modal');
        state.activeModal = null;
    }

    function openModal(id) {
        const modal = document.getElementById(id);
        if (!modal) return;
        state.activeModal = modal;
        modal.style.display = 'block';
        modal.classList.add('show');
        modal.removeAttribute('aria-hidden');
        document.body.classList.add('has-modal');
    }

    function ensureConfirmModal() {
        let modal = document.getElementById('confirmModal');
        if (modal) return modal;
        modal = document.createElement('div');
        modal.id = 'confirmModal';
        modal.className = 'modal';
        modal.style.display = 'none';
        modal.innerHTML = '<div class="modal-backdrop" data-close-modal="confirmModal"></div><div class="modal-dialog modal-sm"><div class="modal-header"><h3 id="confirmModalTitle">' + getTranslation('modal_confirm_title') + '</h3><button type="button" class="icon-btn" data-close-modal="confirmModal">x</button></div><div class="modal-body"><p id="confirmModalBody"></p></div><div class="modal-actions"><button type="button" class="btn" id="confirmCancel">' + getTranslation('btn_cancel') + '</button><button type="button" class="btn btn-danger" id="confirmAccept">' + getTranslation('btn_confirm') + '</button></div></div>';
        document.body.appendChild(modal);
        return modal;
    }

    function confirmDialog(message, title) {
        const modal = ensureConfirmModal();
        const titleEl = document.getElementById('confirmModalTitle');
        const bodyEl = document.getElementById('confirmModalBody');
        const cancelBtn = document.getElementById('confirmCancel');
        const acceptBtn = document.getElementById('confirmAccept');
        titleEl.textContent = title || getTranslation('modal_confirm_title');
        bodyEl.textContent = message || getTranslation('modal_confirm_body');
        return new Promise((resolve) => {
            function cleanup(result) {
                cancelBtn.removeEventListener('click', onCancel);
                acceptBtn.removeEventListener('click', onAccept);
                resolve(result);
                closeModal();
            }
            function onCancel() { cleanup(false); }
            function onAccept() { cleanup(true); }
            cancelBtn.addEventListener('click', onCancel);
            acceptBtn.addEventListener('click', onAccept);
            openModal('confirmModal');
        });
    }

    function bindModalCloseEvents() {
        document.addEventListener('click', (event) => {
            const target = event.target;
            if (!(target instanceof Element)) return;
            const modalId = target.getAttribute('data-close-modal');
            if (!modalId) return;
            const modal = document.getElementById(modalId);
            if (modal) {
                modal.classList.remove('show');
                modal.style.display = 'none';
                if (state.activeModal === modal) {
                    state.activeModal = null;
                    document.body.classList.remove('has-modal');
                }
            }
        });
        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape') {
                closeModal();
                document.body.classList.remove('sidebar-open');
            }
        });
    }

    function updatePasswordStrength(password, indicatorId, textId) {
        const indicator = document.getElementById(indicatorId);
        const text = document.getElementById(textId);
        if (!indicator || !text) return;
        if (!password) {
            indicator.setAttribute('data-strength', '0');
            text.textContent = getTranslation('strength_enter_password');
            return;
        }
        let score = 0;
        if (password.length >= 8) score += 1;
        if (/[A-Z]/.test(password)) score += 1;
        if (/[a-z]/.test(password)) score += 1;
        if (/[0-9]/.test(password)) score += 1;
        if (/[^A-Za-z0-9]/.test(password)) score += 1;
        indicator.setAttribute('data-strength', String(score));
        const labels = [
            getTranslation('strength_very_weak'),
            getTranslation('strength_weak'),
            getTranslation('strength_fair'),
            getTranslation('strength_good'),
            getTranslation('strength_strong'),
            getTranslation('strength_excellent')
        ];
        text.textContent = labels[score] || getTranslation('strength_very_weak');
    }

    function copyText(value) {
        if (!navigator.clipboard) return Promise.reject(new Error('Clipboard API unavailable'));
        return navigator.clipboard.writeText(value);
    }

    function init() {
        document.documentElement.lang = state.currentLanguage;
        applyTranslations();
        populateLanguageSelectors();
        ensureChrome();
        bindModalCloseEvents();
        initTopbar();
        applyRevealAnimations();
    }

    window.App = {
        init, getToken, escapeHTML, toast, openModal, closeModal, confirm: confirmDialog, updatePasswordStrength, copyText, setLoading, reveal: applyRevealAnimations, getTranslation, t, setLanguage, applyTranslations, populateLanguageSelectors
    };

    document.addEventListener('DOMContentLoaded', init);
})();
