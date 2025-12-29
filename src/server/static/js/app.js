/**
 * Search Application - Consolidated JavaScript
 * Per AI.md PART 17: Single app.js with event delegation (no inline handlers)
 */
(function() {
    'use strict';

    // ========================================================================
    // THEME MANAGEMENT
    // ========================================================================
    const THEME_KEY = 'search-theme';
    const THEMES = ['dark', 'light'];

    function getPreferredTheme() {
        const saved = localStorage.getItem(THEME_KEY);
        if (saved && THEMES.includes(saved)) {
            return saved;
        }
        return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
    }

    function setTheme(theme) {
        document.documentElement.setAttribute('data-theme', theme);
        localStorage.setItem(THEME_KEY, theme);
        updateThemeIcon(theme);
    }

    function updateThemeIcon(theme) {
        const darkIcon = document.querySelector('.theme-icon-dark');
        const lightIcon = document.querySelector('.theme-icon-light');
        if (darkIcon && lightIcon) {
            if (theme === 'dark') {
                darkIcon.style.display = 'block';
                lightIcon.style.display = 'none';
            } else {
                darkIcon.style.display = 'none';
                lightIcon.style.display = 'block';
            }
        }
    }

    function toggleTheme() {
        const current = document.documentElement.getAttribute('data-theme') || 'dark';
        const next = current === 'dark' ? 'light' : 'dark';
        setTheme(next);
    }

    // ========================================================================
    // MOBILE NAVIGATION
    // ========================================================================
    function toggleNav() {
        const header = document.querySelector('.site-header');
        const navLinks = document.querySelector('.nav-links');
        if (header && navLinks) {
            header.classList.toggle('nav-open');
            navLinks.classList.toggle('active');
        }
    }

    function closeNav() {
        const header = document.querySelector('.site-header');
        const navLinks = document.querySelector('.nav-links');
        if (header) {
            header.classList.remove('nav-open');
        }
        if (navLinks) {
            navLinks.classList.remove('active');
        }
    }

    // ========================================================================
    // FLASH MESSAGES
    // ========================================================================
    function initFlashMessages() {
        const flashes = document.querySelectorAll('.flash');
        flashes.forEach(function(flash) {
            setTimeout(function() {
                flash.classList.add('flash-fade');
                setTimeout(function() {
                    flash.remove();
                }, 300);
            }, 5000);
        });
    }

    function closeFlash(button) {
        const flash = button.closest('.flash');
        if (flash) {
            flash.classList.add('flash-fade');
            setTimeout(function() {
                flash.remove();
            }, 300);
        }
    }

    // ========================================================================
    // MODAL MANAGEMENT
    // ========================================================================
    function closeModal(element) {
        const dialog = element.closest('dialog');
        if (dialog) {
            dialog.close();
        }
    }

    function closeModalBackdrop(element) {
        const modal = element.closest('.modal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    // ========================================================================
    // COOKIE CONSENT
    // ========================================================================
    function acceptCookies() {
        document.cookie = 'cookie_consent=accepted; path=/; max-age=31536000; SameSite=Lax';
        const banner = document.querySelector('.cookie-consent-banner');
        if (banner) {
            banner.remove();
        }
    }

    function declineCookies() {
        document.cookie = 'cookie_consent=declined; path=/; max-age=31536000; SameSite=Lax';
        const banner = document.querySelector('.cookie-consent-banner');
        if (banner) {
            banner.remove();
        }
    }

    // ========================================================================
    // ANNOUNCEMENTS
    // ========================================================================
    function dismissAnnouncement(id) {
        const dismissed = JSON.parse(localStorage.getItem('dismissed_announcements') || '[]');
        if (!dismissed.includes(id)) {
            dismissed.push(id);
            localStorage.setItem('dismissed_announcements', JSON.stringify(dismissed));
        }
        const announcement = document.querySelector('[data-announcement-id="' + id + '"]');
        if (announcement) {
            announcement.remove();
        }
    }

    // ========================================================================
    // CLIPBOARD
    // ========================================================================
    function copyToClipboard(elementId, button) {
        const element = document.getElementById(elementId);
        if (!element) return;

        const text = element.textContent || element.value;
        navigator.clipboard.writeText(text).then(function() {
            if (button) {
                const originalText = button.textContent;
                button.textContent = 'Copied!';
                button.classList.add('copied');
                setTimeout(function() {
                    button.textContent = originalText;
                    button.classList.remove('copied');
                }, 2000);
            }
        }).catch(function(err) {
            console.error('Failed to copy:', err);
        });
    }

    function copyToken() {
        const tokenDisplay = document.getElementById('token-display');
        if (!tokenDisplay) return;

        const text = tokenDisplay.textContent;
        navigator.clipboard.writeText(text).then(function() {
            const btn = document.querySelector('[data-action="copy-token"]');
            if (btn) {
                const originalText = btn.textContent;
                btn.textContent = 'Copied!';
                setTimeout(function() {
                    btn.textContent = originalText;
                }, 2000);
            }
        });
    }

    // ========================================================================
    // TOKEN MANAGEMENT
    // ========================================================================
    function handleRevokeToken(form) {
        return window.showConfirm('Are you sure you want to revoke this token? This action cannot be undone.', {
            title: 'Revoke Token',
            confirmText: 'Revoke',
            danger: true
        }).then(function(confirmed) {
            if (confirmed) {
                form.submit();
            }
            return false;
        });
    }

    function openTokenModal() {
        const modal = document.getElementById('token-modal');
        if (modal) {
            modal.style.display = 'flex';
        }
    }

    function closeTokenModal() {
        const modal = document.getElementById('token-modal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    // ========================================================================
    // SEARCH AUTOCOMPLETE
    // ========================================================================
    function initSearchAutocomplete() {
        const searchInputs = document.querySelectorAll('.search-input, .header-search-input');

        searchInputs.forEach(function(input) {
            let debounceTimer;
            input.addEventListener('input', function() {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(function() {
                    // Autocomplete logic placeholder
                }, 300);
            });

            input.addEventListener('keydown', function(e) {
                if (e.key === 'Escape') {
                    this.value = '';
                    this.focus();
                }
            });
        });
    }

    // ========================================================================
    // KEYBOARD SHORTCUTS
    // ========================================================================
    function initKeyboardShortcuts() {
        document.addEventListener('keydown', function(e) {
            if (e.key === '/' && !isInputFocused()) {
                e.preventDefault();
                const searchInput = document.querySelector('.search-input, .header-search-input');
                if (searchInput) {
                    searchInput.focus();
                    searchInput.select();
                }
            }

            if (e.key === 't' && !isInputFocused()) {
                toggleTheme();
            }

            if (e.key === 'Escape') {
                closeNav();
            }
        });
    }

    function isInputFocused() {
        const activeElement = document.activeElement;
        return activeElement && (
            activeElement.tagName === 'INPUT' ||
            activeElement.tagName === 'TEXTAREA' ||
            activeElement.isContentEditable
        );
    }

    // ========================================================================
    // LAZY LOADING
    // ========================================================================
    function initLazyLoading() {
        if ('loading' in HTMLImageElement.prototype) {
            document.querySelectorAll('img[data-src]').forEach(function(img) {
                img.src = img.dataset.src;
            });
        } else {
            const lazyImages = document.querySelectorAll('img[data-src]');

            if ('IntersectionObserver' in window) {
                const imageObserver = new IntersectionObserver(function(entries, observer) {
                    entries.forEach(function(entry) {
                        if (entry.isIntersecting) {
                            const img = entry.target;
                            img.src = img.dataset.src;
                            img.removeAttribute('data-src');
                            observer.unobserve(img);
                        }
                    });
                });

                lazyImages.forEach(function(img) {
                    imageObserver.observe(img);
                });
            } else {
                lazyImages.forEach(function(img) {
                    img.src = img.dataset.src;
                });
            }
        }
    }

    // ========================================================================
    // IMAGE VIEWER
    // ========================================================================
    function initImageViewer() {
        document.addEventListener('click', function(e) {
            const imageResult = e.target.closest('.image-result');
            if (imageResult && !e.target.closest('a')) {
                const fullUrl = imageResult.dataset.fullUrl;
                if (fullUrl) {
                    window.open(fullUrl, '_blank', 'noopener,noreferrer');
                }
            }
        });
    }

    // ========================================================================
    // INFINITE SCROLL
    // ========================================================================
    function initInfiniteScroll() {
        const resultsContainer = document.querySelector('.search-results');
        if (!resultsContainer) return;

        const loadMoreTrigger = document.querySelector('.load-more-trigger');
        if (!loadMoreTrigger) return;

        if ('IntersectionObserver' in window) {
            const observer = new IntersectionObserver(function(entries) {
                entries.forEach(function(entry) {
                    if (entry.isIntersecting) {
                        const event = new CustomEvent('loadmore');
                        document.dispatchEvent(event);
                    }
                });
            }, { rootMargin: '200px' });

            observer.observe(loadMoreTrigger);
        }
    }

    // ========================================================================
    // TOAST NOTIFICATIONS (per AI.md PART 17)
    // ========================================================================
    function getToastContainer() {
        let container = document.getElementById('toast-container');
        if (!container) {
            container = document.createElement('div');
            container.id = 'toast-container';
            container.setAttribute('role', 'alert');
            container.setAttribute('aria-live', 'polite');
            container.setAttribute('aria-atomic', 'true');
            document.body.appendChild(container);
        }
        return container;
    }

    function showToast(message, type) {
        type = type || 'info';
        const container = getToastContainer();

        const toast = document.createElement('div');
        toast.className = 'toast toast-' + type;
        toast.setAttribute('role', 'status');

        const icons = {
            success: '\u2713',
            error: '\u2717',
            warning: '\u26A0',
            info: '\u2139'
        };

        toast.innerHTML = '<span class="toast-icon">' + (icons[type] || icons.info) + '</span>' +
                         '<span class="toast-message">' + escapeHtml(message) + '</span>' +
                         '<button class="toast-close" data-action="close-toast" aria-label="Dismiss">&times;</button>';

        container.appendChild(toast);

        const delays = {
            success: 3000,
            error: 0,
            warning: 5000,
            info: 3000
        };

        const delay = delays[type] || 3000;
        if (delay > 0) {
            setTimeout(function() {
                dismissToast(toast);
            }, delay);
        }

        requestAnimationFrame(function() {
            toast.classList.add('toast-visible');
        });

        return toast;
    }

    function dismissToast(toast) {
        toast.classList.remove('toast-visible');
        toast.classList.add('toast-hiding');
        setTimeout(function() {
            if (toast.parentNode) {
                toast.parentNode.removeChild(toast);
            }
        }, 300);
    }

    // ========================================================================
    // CONFIRMATION DIALOG (per AI.md PART 17)
    // ========================================================================
    function getConfirmDialog() {
        let dialog = document.getElementById('confirm-dialog');
        if (!dialog) {
            dialog = document.createElement('dialog');
            dialog.id = 'confirm-dialog';
            dialog.setAttribute('role', 'alertdialog');
            dialog.setAttribute('aria-modal', 'true');
            dialog.setAttribute('aria-labelledby', 'confirm-dialog-title');
            dialog.innerHTML =
                '<header>' +
                    '<h2 id="confirm-dialog-title">Confirm</h2>' +
                '</header>' +
                '<main id="confirm-dialog-message"></main>' +
                '<footer>' +
                    '<button type="button" class="btn btn-secondary" data-action="cancel">Cancel</button>' +
                    '<button type="button" class="btn btn-danger" data-action="confirm">Confirm</button>' +
                '</footer>';
            document.body.appendChild(dialog);
        }
        return dialog;
    }

    function showConfirm(message, options) {
        options = options || {};
        return new Promise(function(resolve) {
            const dialog = getConfirmDialog();
            const titleEl = dialog.querySelector('#confirm-dialog-title');
            const messageEl = dialog.querySelector('#confirm-dialog-message');
            const confirmBtn = dialog.querySelector('[data-action="confirm"]');
            const cancelBtn = dialog.querySelector('[data-action="cancel"]');

            titleEl.textContent = options.title || 'Confirm Action';
            messageEl.textContent = message;
            confirmBtn.textContent = options.confirmText || 'Confirm';
            cancelBtn.textContent = options.cancelText || 'Cancel';

            if (options.danger) {
                confirmBtn.className = 'btn btn-danger';
            } else {
                confirmBtn.className = 'btn btn-primary';
            }

            function cleanup(result) {
                dialog.close();
                confirmBtn.removeEventListener('click', handleConfirm);
                cancelBtn.removeEventListener('click', handleCancel);
                dialog.removeEventListener('close', handleClose);
                resolve(result);
            }

            function handleConfirm() { cleanup(true); }
            function handleCancel() { cleanup(false); }
            function handleClose() { cleanup(false); }

            confirmBtn.addEventListener('click', handleConfirm);
            cancelBtn.addEventListener('click', handleCancel);
            dialog.addEventListener('close', handleClose);

            dialog.showModal();
            confirmBtn.focus();
        });
    }

    // ========================================================================
    // PROMPT DIALOG (per AI.md PART 17 - no JavaScript prompt())
    // ========================================================================
    function showPrompt(message, defaultValue) {
        return new Promise(function(resolve) {
            let dialog = document.getElementById('prompt-dialog');
            if (!dialog) {
                dialog = document.createElement('dialog');
                dialog.id = 'prompt-dialog';
                dialog.setAttribute('role', 'dialog');
                dialog.setAttribute('aria-modal', 'true');
                dialog.innerHTML =
                    '<form method="dialog">' +
                        '<label id="prompt-dialog-label"></label>' +
                        '<input type="text" id="prompt-dialog-input" class="form-control">' +
                        '<footer>' +
                            '<button type="button" class="btn btn-secondary" data-action="cancel">Cancel</button>' +
                            '<button type="submit" class="btn btn-primary">OK</button>' +
                        '</footer>' +
                    '</form>';
                document.body.appendChild(dialog);
            }

            const label = dialog.querySelector('#prompt-dialog-label');
            const input = dialog.querySelector('#prompt-dialog-input');
            const cancelBtn = dialog.querySelector('[data-action="cancel"]');

            label.textContent = message;
            input.value = defaultValue || '';

            function cleanup(result) {
                dialog.close();
                resolve(result);
            }

            cancelBtn.onclick = function() { cleanup(null); };
            dialog.querySelector('form').onsubmit = function(e) {
                e.preventDefault();
                cleanup(input.value);
            };
            dialog.onclose = function() { cleanup(null); };

            dialog.showModal();
            input.focus();
            input.select();
        });
    }

    // ========================================================================
    // ALERT DIALOG (per AI.md PART 17 - no JavaScript alert())
    // ========================================================================
    function showAlert(message) {
        return new Promise(function(resolve) {
            let dialog = document.getElementById('alert-dialog');
            if (!dialog) {
                dialog = document.createElement('dialog');
                dialog.id = 'alert-dialog';
                dialog.setAttribute('role', 'alertdialog');
                dialog.setAttribute('aria-modal', 'true');
                dialog.innerHTML =
                    '<main id="alert-dialog-message"></main>' +
                    '<footer>' +
                        '<button type="button" class="btn btn-primary" data-action="ok">OK</button>' +
                    '</footer>';
                document.body.appendChild(dialog);
            }

            const messageEl = dialog.querySelector('#alert-dialog-message');
            const okBtn = dialog.querySelector('[data-action="ok"]');

            messageEl.textContent = message;

            function cleanup() {
                dialog.close();
                resolve();
            }

            okBtn.onclick = cleanup;
            dialog.onclose = cleanup;

            dialog.showModal();
            okBtn.focus();
        });
    }

    // ========================================================================
    // FORM DOUBLE-SUBMIT PREVENTION
    // ========================================================================
    function initFormProtection() {
        const forms = document.querySelectorAll('form');

        forms.forEach(function(form) {
            form.addEventListener('submit', function(e) {
                if (form.dataset.submitting === 'true') {
                    e.preventDefault();
                    return false;
                }

                form.dataset.submitting = 'true';

                const buttons = form.querySelectorAll('button[type="submit"], input[type="submit"], .search-btn');
                buttons.forEach(function(btn) {
                    btn.disabled = true;
                    btn.classList.add('submitting');
                });

                setTimeout(function() {
                    form.dataset.submitting = 'false';
                    buttons.forEach(function(btn) {
                        btn.disabled = false;
                        btn.classList.remove('submitting');
                    });
                }, 5000);
            });
        });
    }

    // ========================================================================
    // SERVICE WORKER
    // ========================================================================
    function initServiceWorker() {
        if ('serviceWorker' in navigator) {
            window.addEventListener('load', function() {
                navigator.serviceWorker.register('/sw.js').catch(function() {
                    // Service worker registration failed
                });
            });
        }
    }

    // ========================================================================
    // EVENT DELEGATION (per AI.md PART 17 - no inline handlers)
    // ========================================================================
    function initEventDelegation() {
        document.addEventListener('click', function(e) {
            const target = e.target;

            // Flash close button
            if (target.matches('.flash-close') || target.closest('.flash-close')) {
                const btn = target.closest('.flash-close') || target;
                closeFlash(btn);
                return;
            }

            // Modal close button
            if (target.matches('.modal-close') || target.closest('.modal-close')) {
                const btn = target.closest('.modal-close') || target;
                closeModal(btn);
                return;
            }

            // Modal backdrop
            if (target.matches('.modal-backdrop')) {
                closeModalBackdrop(target);
                return;
            }

            // Modal cancel button in dialog
            if (target.matches('[data-action="modal-cancel"]')) {
                closeModal(target);
                return;
            }

            // Theme toggle
            if (target.matches('.theme-toggle') || target.closest('.theme-toggle')) {
                toggleTheme();
                return;
            }

            // Nav toggle
            if (target.matches('.nav-toggle') || target.closest('.nav-toggle')) {
                toggleNav();
                return;
            }

            // Nav overlay (close nav)
            if (target.matches('.nav-overlay')) {
                closeNav();
                return;
            }

            // Cookie consent accept
            if (target.matches('.cookie-consent-accept')) {
                acceptCookies();
                return;
            }

            // Cookie consent decline
            if (target.matches('.cookie-consent-decline')) {
                declineCookies();
                return;
            }

            // Announcement dismiss
            if (target.matches('.announcement-dismiss') || target.closest('.announcement-dismiss')) {
                const btn = target.closest('.announcement-dismiss') || target;
                const id = btn.dataset.announcementId;
                if (id) {
                    dismissAnnouncement(id);
                }
                return;
            }

            // Copy button
            if (target.matches('.btn-copy') || target.closest('.btn-copy')) {
                const btn = target.closest('.btn-copy') || target;
                const elementId = btn.dataset.copyTarget;
                if (elementId) {
                    copyToClipboard(elementId, btn);
                }
                return;
            }

            // Copy token button
            if (target.matches('[data-action="copy-token"]')) {
                copyToken();
                return;
            }

            // Go back button
            if (target.matches('[data-action="go-back"]')) {
                history.back();
                return;
            }

            // Close token modal
            if (target.matches('[data-action="close-token-modal"]')) {
                closeTokenModal();
                return;
            }

            // Toast close
            if (target.matches('.toast-close') || target.matches('[data-action="close-toast"]')) {
                const toast = target.closest('.toast');
                if (toast) {
                    dismissToast(toast);
                }
                return;
            }

            // Close nav on outside click
            const header = document.querySelector('.site-header');
            if (header && header.classList.contains('nav-open')) {
                if (!target.closest('.nav-links') && !target.closest('.nav-toggle')) {
                    closeNav();
                }
            }
        });

        // Form submit handlers for token revoke
        document.addEventListener('submit', function(e) {
            const form = e.target;

            if (form.matches('[data-confirm-revoke]')) {
                e.preventDefault();
                handleRevokeToken(form);
            }
        });
    }

    // ========================================================================
    // UTILITY FUNCTIONS
    // ========================================================================
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ========================================================================
    // INITIALIZATION
    // ========================================================================
    function init() {
        setTheme(getPreferredTheme());
        initFlashMessages();
        initSearchAutocomplete();
        initKeyboardShortcuts();
        initLazyLoading();
        initImageViewer();
        initInfiniteScroll();
        initFormProtection();
        initServiceWorker();
        initEventDelegation();
    }

    // Listen for system theme changes
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function(e) {
        if (!localStorage.getItem(THEME_KEY)) {
            setTheme(e.matches ? 'dark' : 'light');
        }
    });

    // Initialize on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    // ========================================================================
    // GLOBAL EXPORTS
    // ========================================================================
    window.showToast = showToast;
    window.showConfirm = showConfirm;
    window.showPrompt = showPrompt;
    window.showAlert = showAlert;
    window.toggleTheme = toggleTheme;
    window.toggleNav = toggleNav;
    window.closeNav = closeNav;
    window.copyToClipboard = copyToClipboard;
    window.dismissAnnouncement = dismissAnnouncement;

})();


// ============================================================================
// WIDGET MANAGER (separate IIFE to maintain encapsulation)
// ============================================================================
(function() {
    'use strict';

    const WIDGETS_KEY = 'search_widgets';
    const WIDGET_SETTINGS_PREFIX = 'search_widget_';

    // Widget definitions
    const WIDGETS = {
        clock: {
            type: 'clock',
            name: 'Clock',
            icon: 'clock',
            category: 'tool',
            refreshInterval: 1000,
            render: renderClockWidget
        },
        weather: {
            type: 'weather',
            name: 'Weather',
            icon: 'cloud-sun',
            category: 'data',
            refreshInterval: 900000,
            render: renderWeatherWidget
        },
        quicklinks: {
            type: 'quicklinks',
            name: 'Quick Links',
            icon: 'link',
            category: 'user',
            render: renderQuickLinksWidget
        },
        calculator: {
            type: 'calculator',
            name: 'Calculator',
            icon: 'calculator',
            category: 'tool',
            render: renderCalculatorWidget
        },
        notes: {
            type: 'notes',
            name: 'Notes',
            icon: 'sticky-note',
            category: 'user',
            render: renderNotesWidget
        },
        calendar: {
            type: 'calendar',
            name: 'Calendar',
            icon: 'calendar',
            category: 'tool',
            render: renderCalendarWidget
        },
        converter: {
            type: 'converter',
            name: 'Unit Converter',
            icon: 'exchange-alt',
            category: 'tool',
            render: renderConverterWidget
        },
        news: {
            type: 'news',
            name: 'News',
            icon: 'newspaper',
            category: 'data',
            refreshInterval: 1800000,
            render: renderNewsWidget
        },
        stocks: {
            type: 'stocks',
            name: 'Stocks',
            icon: 'chart-line',
            category: 'data',
            refreshInterval: 300000,
            render: renderStocksWidget
        },
        crypto: {
            type: 'crypto',
            name: 'Crypto',
            icon: 'bitcoin',
            category: 'data',
            refreshInterval: 300000,
            render: renderCryptoWidget
        },
        sports: {
            type: 'sports',
            name: 'Sports',
            icon: 'futbol',
            category: 'data',
            refreshInterval: 300000,
            render: renderSportsWidget
        },
        rss: {
            type: 'rss',
            name: 'RSS Feeds',
            icon: 'rss',
            category: 'user',
            refreshInterval: 1800000,
            render: renderRSSWidget
        }
    };

    function getEnabledWidgets() {
        try {
            const saved = localStorage.getItem(WIDGETS_KEY);
            if (saved) {
                return JSON.parse(saved);
            }
        } catch (e) {
            console.error('Failed to load widget preferences:', e);
        }
        return null;
    }

    function saveEnabledWidgets(widgets) {
        localStorage.setItem(WIDGETS_KEY, JSON.stringify(widgets));
    }

    function getWidgetSettings(widgetType) {
        try {
            const key = WIDGET_SETTINGS_PREFIX + widgetType;
            return JSON.parse(localStorage.getItem(key) || '{}');
        } catch (e) {
            return {};
        }
    }

    function saveWidgetSettings(widgetType, settings) {
        const key = WIDGET_SETTINGS_PREFIX + widgetType;
        localStorage.setItem(key, JSON.stringify(settings));
    }

    async function fetchWidgetData(widgetType, params) {
        params = params || {};
        const queryString = new URLSearchParams(params).toString();
        const url = '/api/v1/widgets/' + widgetType + (queryString ? '?' + queryString : '');

        try {
            const response = await fetch(url);
            const result = await response.json();
            if (result.success) {
                return result.data;
            }
            throw new Error(result.error?.message || 'Unknown error');
        } catch (e) {
            console.error('Failed to fetch ' + widgetType + ' widget:', e);
            return null;
        }
    }

    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }

    // Widget render functions
    function renderClockWidget(container, data, settings) {
        const timezone = settings.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone;
        const format = settings.format || '24h';

        function update() {
            const now = new Date();
            const timeOptions = {
                timeZone: timezone,
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
                hour12: format === '12h'
            };
            const dateOptions = {
                timeZone: timezone,
                weekday: 'long',
                year: 'numeric',
                month: 'long',
                day: 'numeric'
            };

            const timeStr = now.toLocaleTimeString(undefined, timeOptions);
            const dateStr = now.toLocaleDateString(undefined, dateOptions);

            container.innerHTML =
                '<div class="clock-widget">' +
                    '<div class="clock-time">' + timeStr + '</div>' +
                    '<div class="clock-date">' + dateStr + '</div>' +
                '</div>';
        }

        update();
        return setInterval(update, 1000);
    }

    function renderWeatherWidget(container, data, settings) {
        if (!data || data.error) {
            container.innerHTML =
                '<div class="widget-placeholder">' +
                    '<div class="widget-placeholder-icon">&#9925;</div>' +
                    '<div class="widget-placeholder-text">Set your city in widget settings</div>' +
                    '<button class="widget-settings-btn" data-widget-settings="weather">Configure</button>' +
                '</div>';
            return;
        }

        var iconMap = {
            'clear': '&#9728;',
            'sunny': '&#9728;',
            'cloudy': '&#9729;',
            'partly-cloudy': '&#9925;',
            'rain': '&#127783;',
            'snow': '&#127784;',
            'thunderstorm': '&#9928;',
            'fog': '&#127787;'
        };

        var icon = iconMap[data.condition] || '&#9925;';
        var temp = Math.round(data.temperature);
        var feelsLike = Math.round(data.feels_like || data.temperature);

        container.innerHTML =
            '<div class="weather-widget">' +
                '<div class="weather-main">' +
                    '<span class="weather-icon">' + icon + '</span>' +
                    '<span class="weather-temp">' + temp + '&deg;</span>' +
                '</div>' +
                '<div class="weather-details">' +
                    '<div class="weather-location">' + escapeHtml(data.location) + '</div>' +
                    '<div class="weather-description">' + escapeHtml(data.description) + '</div>' +
                    '<div class="weather-extra">Feels like ' + feelsLike + '&deg; &middot; ' + data.humidity + '% humidity</div>' +
                '</div>' +
            '</div>';
    }

    function renderQuickLinksWidget(container, data, settings) {
        var links = settings.links || [];

        if (links.length === 0) {
            container.innerHTML =
                '<div class="widget-placeholder">' +
                    '<div class="widget-placeholder-text">No links yet</div>' +
                    '<button class="widget-add-btn" data-widget-action="add-quicklink">+ Add Link</button>' +
                '</div>';
            return;
        }

        var html = '<div class="quicklinks-widget">';
        links.forEach(function(link) {
            var hostname = '';
            try { hostname = new URL(link.url).hostname; } catch(e) {}
            html += '<a href="' + escapeHtml(link.url) + '" class="quicklink-item" title="' + escapeHtml(link.name) + '">' +
                '<img src="https://www.google.com/s2/favicons?domain=' + encodeURIComponent(hostname) + '&sz=32" alt="" class="quicklink-favicon" onerror="this.style.display=\'none\'">' +
                '<span class="quicklink-name">' + escapeHtml(link.name) + '</span>' +
            '</a>';
        });
        html += '<button class="quicklink-add" data-widget-action="add-quicklink" title="Add link">+</button></div>';
        container.innerHTML = html;
    }

    function renderCalculatorWidget(container) {
        container.innerHTML =
            '<div class="calculator-widget">' +
                '<input type="text" class="calc-display" id="calc-display" readonly placeholder="0">' +
                '<div class="calc-buttons">' +
                    '<button data-calc="7">7</button><button data-calc="8">8</button><button data-calc="9">9</button><button data-calc="/" class="calc-op">/</button>' +
                    '<button data-calc="4">4</button><button data-calc="5">5</button><button data-calc="6">6</button><button data-calc="*" class="calc-op">*</button>' +
                    '<button data-calc="1">1</button><button data-calc="2">2</button><button data-calc="3">3</button><button data-calc="-" class="calc-op">-</button>' +
                    '<button data-calc="0">0</button><button data-calc=".">.</button><button data-calc="=" class="calc-eq">=</button><button data-calc="+" class="calc-op">+</button>' +
                    '<button data-calc="C" class="calc-clear">C</button>' +
                '</div>' +
            '</div>';
    }

    function renderNotesWidget(container, data, settings) {
        var notes = settings.notes || '';
        container.innerHTML =
            '<div class="notes-widget">' +
                '<textarea class="notes-textarea" data-widget-notes placeholder="Type your notes here...">' + escapeHtml(notes) + '</textarea>' +
            '</div>';
    }

    function renderCalendarWidget(container) {
        var now = new Date();
        var month = now.getMonth();
        var year = now.getFullYear();
        var today = now.getDate();

        var firstDay = new Date(year, month, 1).getDay();
        var daysInMonth = new Date(year, month + 1, 0).getDate();

        var monthNames = ['January', 'February', 'March', 'April', 'May', 'June',
                         'July', 'August', 'September', 'October', 'November', 'December'];

        var days = '';
        for (var i = 0; i < firstDay; i++) {
            days += '<span class="calendar-day empty"></span>';
        }
        for (var d = 1; d <= daysInMonth; d++) {
            var isToday = d === today ? ' today' : '';
            days += '<span class="calendar-day' + isToday + '">' + d + '</span>';
        }

        container.innerHTML =
            '<div class="calendar-widget">' +
                '<div class="calendar-header">' + monthNames[month] + ' ' + year + '</div>' +
                '<div class="calendar-weekdays"><span>Su</span><span>Mo</span><span>Tu</span><span>We</span><span>Th</span><span>Fr</span><span>Sa</span></div>' +
                '<div class="calendar-days">' + days + '</div>' +
            '</div>';
    }

    function renderConverterWidget(container, data, settings) {
        var category = settings.defaultCategory || 'length';
        container.innerHTML =
            '<div class="converter-widget">' +
                '<select id="converter-category" data-converter-category>' +
                    '<option value="length"' + (category === 'length' ? ' selected' : '') + '>Length</option>' +
                    '<option value="weight"' + (category === 'weight' ? ' selected' : '') + '>Weight</option>' +
                    '<option value="temperature"' + (category === 'temperature' ? ' selected' : '') + '>Temperature</option>' +
                    '<option value="volume"' + (category === 'volume' ? ' selected' : '') + '>Volume</option>' +
                '</select>' +
                '<div class="converter-row">' +
                    '<input type="number" id="converter-from" data-converter-from placeholder="0">' +
                    '<select id="converter-from-unit" data-converter-from-unit></select>' +
                '</div>' +
                '<div class="converter-equals">=</div>' +
                '<div class="converter-row">' +
                    '<input type="number" id="converter-to" readonly placeholder="0">' +
                    '<select id="converter-to-unit" data-converter-to-unit></select>' +
                '</div>' +
            '</div>';
        WidgetManager.updateConverterUnits();
    }

    function renderNewsWidget(container, data) {
        if (!data || !data.items || data.items.length === 0) {
            container.innerHTML = '<div class="widget-placeholder"><div class="widget-placeholder-text">No news available</div></div>';
            return;
        }

        var html = '<div class="news-widget">';
        data.items.slice(0, 5).forEach(function(item) {
            html += '<a href="' + escapeHtml(item.url) + '" class="news-item" target="_blank" rel="noopener">' +
                '<div class="news-title">' + escapeHtml(item.title) + '</div>' +
                '<div class="news-source">' + escapeHtml(item.source) + '</div>' +
            '</a>';
        });
        html += '</div>';
        container.innerHTML = html;
    }

    function renderStocksWidget(container, data, settings) {
        if (!data || !data.symbols || data.symbols.length === 0) {
            container.innerHTML =
                '<div class="widget-placeholder">' +
                    '<div class="widget-placeholder-text">Configure stock symbols</div>' +
                    '<button class="widget-settings-btn" data-widget-settings="stocks">Configure</button>' +
                '</div>';
            return;
        }

        var html = '<div class="stocks-widget">';
        data.symbols.forEach(function(stock) {
            var changeClass = stock.change >= 0 ? 'positive' : 'negative';
            var changeSign = stock.change >= 0 ? '+' : '';
            html += '<div class="stock-item">' +
                '<div class="stock-symbol">' + escapeHtml(stock.symbol) + '</div>' +
                '<div class="stock-price">$' + stock.price.toFixed(2) + '</div>' +
                '<div class="stock-change ' + changeClass + '">' + changeSign + stock.change_percent.toFixed(2) + '%</div>' +
            '</div>';
        });
        html += '</div>';
        container.innerHTML = html;
    }

    function renderCryptoWidget(container, data) {
        if (!data || !data.coins || data.coins.length === 0) {
            container.innerHTML = '<div class="widget-placeholder"><div class="widget-placeholder-text">Loading crypto prices...</div></div>';
            return;
        }

        var html = '<div class="crypto-widget">';
        data.coins.forEach(function(coin) {
            var changeClass = coin.change_24h >= 0 ? 'positive' : 'negative';
            var changeSign = coin.change_24h >= 0 ? '+' : '';
            html += '<div class="crypto-item">' +
                '<div class="crypto-name">' + escapeHtml(coin.name) + '</div>' +
                '<div class="crypto-price">$' + coin.price.toLocaleString(undefined, {minimumFractionDigits: 2, maximumFractionDigits: 2}) + '</div>' +
                '<div class="crypto-change ' + changeClass + '">' + changeSign + coin.change_24h.toFixed(2) + '%</div>' +
            '</div>';
        });
        html += '</div>';
        container.innerHTML = html;
    }

    function renderSportsWidget(container, data) {
        if (!data || !data.games || data.games.length === 0) {
            container.innerHTML = '<div class="widget-placeholder"><div class="widget-placeholder-text">No games today</div></div>';
            return;
        }

        var html = '<div class="sports-widget">';
        data.games.slice(0, 3).forEach(function(game) {
            html += '<div class="sports-game">' +
                '<div class="sports-teams"><span>' + escapeHtml(game.home_team) + '</span><span class="sports-vs">vs</span><span>' + escapeHtml(game.away_team) + '</span></div>' +
                '<div class="sports-score">' + game.home_score + ' - ' + game.away_score + '</div>' +
                '<div class="sports-status">' + escapeHtml(game.status) + '</div>' +
            '</div>';
        });
        html += '</div>';
        container.innerHTML = html;
    }

    function renderRSSWidget(container, data, settings) {
        var feeds = settings.feeds || [];

        if (feeds.length === 0) {
            container.innerHTML =
                '<div class="widget-placeholder">' +
                    '<div class="widget-placeholder-text">No RSS feeds configured</div>' +
                    '<button class="widget-settings-btn" data-widget-settings="rss">Add Feed</button>' +
                '</div>';
            return;
        }

        if (!data || !data.items || data.items.length === 0) {
            container.innerHTML = '<div class="widget-placeholder"><div class="widget-placeholder-text">Loading feeds...</div></div>';
            return;
        }

        var html = '<div class="rss-widget">';
        data.items.slice(0, 5).forEach(function(item) {
            html += '<a href="' + escapeHtml(item.url) + '" class="rss-item" target="_blank" rel="noopener">' +
                '<div class="rss-title">' + escapeHtml(item.title) + '</div>' +
                '<div class="rss-source">' + escapeHtml(item.source) + '</div>' +
            '</a>';
        });
        html += '</div>';
        container.innerHTML = html;
    }

    // Widget Manager object
    var WidgetManager = {
        intervals: {},
        defaultWidgets: ['clock', 'weather', 'quicklinks', 'calculator'],

        init: function(defaultWidgets) {
            if (defaultWidgets && defaultWidgets.length > 0) {
                this.defaultWidgets = defaultWidgets;
            }

            var grid = document.getElementById('widget-grid');
            if (!grid) return;

            var enabledWidgets = getEnabledWidgets() || this.defaultWidgets;
            this.renderWidgetGrid(enabledWidgets);
            this.initEventDelegation();
        },

        initEventDelegation: function() {
            var self = this;

            document.addEventListener('click', function(e) {
                var target = e.target;

                // Calculator buttons
                if (target.matches('[data-calc]')) {
                    var val = target.dataset.calc;
                    if (val === '=') {
                        self.calcEquals();
                    } else if (val === 'C') {
                        self.calcClear();
                    } else {
                        self.calcInput(val);
                    }
                    return;
                }

                // Widget settings button
                if (target.matches('[data-widget-settings]')) {
                    self.showSettings(target.dataset.widgetSettings);
                    return;
                }

                // Widget menu button
                if (target.matches('.widget-menu')) {
                    var widgetType = target.closest('[data-widget]').dataset.widget;
                    self.showMenu(widgetType);
                    return;
                }

                // Add quicklink
                if (target.matches('[data-widget-action="add-quicklink"]')) {
                    self.addQuickLink();
                    return;
                }
            });

            // Notes textarea
            document.addEventListener('input', function(e) {
                if (e.target.matches('[data-widget-notes]')) {
                    self.saveNotes(e.target.value);
                }

                // Converter input
                if (e.target.matches('[data-converter-from]')) {
                    self.convert();
                }
            });

            // Converter selects
            document.addEventListener('change', function(e) {
                if (e.target.matches('[data-converter-category]')) {
                    self.updateConverterUnits();
                }
                if (e.target.matches('[data-converter-from-unit]') || e.target.matches('[data-converter-to-unit]')) {
                    self.convert();
                }
            });
        },

        renderWidgetGrid: function(enabledWidgets) {
            var self = this;
            var grid = document.getElementById('widget-grid');
            if (!grid) return;

            Object.values(this.intervals).forEach(clearInterval);
            this.intervals = {};

            grid.innerHTML = '';

            enabledWidgets.forEach(function(widgetType) {
                if (!WIDGETS[widgetType]) return;

                var widget = WIDGETS[widgetType];
                var div = document.createElement('div');
                div.className = 'widget widget-' + widgetType;
                div.dataset.widget = widgetType;
                div.innerHTML =
                    '<div class="widget-header">' +
                        '<span class="widget-title">' + widget.name + '</span>' +
                        '<button class="widget-menu" title="Widget options">&#8942;</button>' +
                    '</div>' +
                    '<div class="widget-content" id="widget-content-' + widgetType + '">' +
                        '<div class="widget-loading">Loading...</div>' +
                    '</div>';
                grid.appendChild(div);

                self.initWidget(widgetType);
            });
        },

        initWidget: async function(widgetType) {
            var self = this;
            var widget = WIDGETS[widgetType];
            var container = document.getElementById('widget-content-' + widgetType);
            if (!container || !widget) return;

            var settings = getWidgetSettings(widgetType);

            if (widget.category === 'data') {
                var data = await fetchWidgetData(widgetType, settings);
                var interval = widget.render(container, data, settings);
                if (interval) {
                    this.intervals[widgetType] = interval;
                }

                if (widget.refreshInterval) {
                    this.intervals[widgetType + '_refresh'] = setInterval(async function() {
                        var newData = await fetchWidgetData(widgetType, settings);
                        widget.render(container, newData, settings);
                    }, widget.refreshInterval);
                }
            } else {
                var interval = widget.render(container, null, settings);
                if (interval) {
                    this.intervals[widgetType] = interval;
                }
            }
        },

        toggleWidget: function(widgetType) {
            var enabled = getEnabledWidgets() || this.defaultWidgets;
            var idx = enabled.indexOf(widgetType);
            if (idx >= 0) {
                enabled.splice(idx, 1);
            } else {
                enabled.push(widgetType);
            }
            saveEnabledWidgets(enabled);
            this.renderWidgetGrid(enabled);
        },

        removeWidget: function(widgetType) {
            var enabled = getEnabledWidgets() || this.defaultWidgets;
            var idx = enabled.indexOf(widgetType);
            if (idx >= 0) {
                enabled.splice(idx, 1);
                saveEnabledWidgets(enabled);
                this.renderWidgetGrid(enabled);
            }
        },

        showMenu: function(widgetType) {
            var existingMenu = document.querySelector('.widget-dropdown-menu');
            if (existingMenu) existingMenu.remove();

            var widget = document.querySelector('[data-widget="' + widgetType + '"]');
            if (!widget) return;

            var self = this;
            var menu = document.createElement('div');
            menu.className = 'widget-dropdown-menu';
            menu.innerHTML =
                '<button data-menu-action="refresh">Refresh</button>' +
                '<button data-menu-action="settings">Settings</button>' +
                '<button data-menu-action="remove" class="danger">Remove</button>';

            menu.addEventListener('click', function(e) {
                var action = e.target.dataset.menuAction;
                if (action === 'refresh') {
                    self.refreshWidget(widgetType);
                } else if (action === 'settings') {
                    self.showSettings(widgetType);
                } else if (action === 'remove') {
                    self.removeWidget(widgetType);
                }
                menu.remove();
            });

            widget.querySelector('.widget-header').appendChild(menu);

            setTimeout(function() {
                document.addEventListener('click', function closeMenu(e) {
                    if (!menu.contains(e.target)) {
                        menu.remove();
                        document.removeEventListener('click', closeMenu);
                    }
                });
            }, 0);
        },

        refreshWidget: function(widgetType) {
            this.initWidget(widgetType);
        },

        showSettings: function(widgetType) {
            var self = this;
            var settings = getWidgetSettings(widgetType);
            var content = '';

            switch (widgetType) {
                case 'weather':
                    content =
                        '<label>City:<input type="text" id="setting-city" value="' + escapeHtml(settings.city || '') + '" placeholder="e.g., London, UK"></label>' +
                        '<label>Units:<select id="setting-units">' +
                            '<option value="metric"' + (settings.units !== 'imperial' ? ' selected' : '') + '>Celsius</option>' +
                            '<option value="imperial"' + (settings.units === 'imperial' ? ' selected' : '') + '>Fahrenheit</option>' +
                        '</select></label>';
                    break;
                case 'clock':
                    content =
                        '<label>Format:<select id="setting-format">' +
                            '<option value="24h"' + (settings.format !== '12h' ? ' selected' : '') + '>24-hour</option>' +
                            '<option value="12h"' + (settings.format === '12h' ? ' selected' : '') + '>12-hour</option>' +
                        '</select></label>';
                    break;
                case 'stocks':
                    content = '<label>Stock Symbols (comma-separated):<input type="text" id="setting-symbols" value="' + escapeHtml((settings.symbols || ['AAPL', 'GOOGL', 'MSFT']).join(', ')) + '" placeholder="e.g., AAPL, GOOGL, MSFT"></label>';
                    break;
                case 'crypto':
                    content = '<label>Coins (comma-separated):<input type="text" id="setting-coins" value="' + escapeHtml((settings.coins || ['bitcoin', 'ethereum']).join(', ')) + '" placeholder="e.g., bitcoin, ethereum"></label>';
                    break;
                case 'rss':
                    content = '<label>RSS Feed URLs (one per line):<textarea id="setting-feeds" rows="4" placeholder="https://example.com/feed.xml">' + escapeHtml((settings.feeds || []).join('\n')) + '</textarea></label>';
                    break;
                default:
                    content = '<p>No settings available for this widget.</p>';
            }

            var modal = document.createElement('div');
            modal.className = 'widget-settings-modal';
            modal.innerHTML =
                '<div class="widget-settings-content">' +
                    '<h3>' + (WIDGETS[widgetType]?.name || widgetType) + ' Settings</h3>' +
                    '<form id="widget-settings-form">' +
                        content +
                        '<div class="widget-settings-actions">' +
                            '<button type="button" data-action="cancel">Cancel</button>' +
                            '<button type="submit">Save</button>' +
                        '</div>' +
                    '</form>' +
                '</div>';

            document.body.appendChild(modal);

            modal.querySelector('[data-action="cancel"]').addEventListener('click', function() {
                modal.remove();
            });

            modal.querySelector('form').addEventListener('submit', function(e) {
                e.preventDefault();
                self.saveSettings(widgetType);
                modal.remove();
            });

            modal.addEventListener('click', function(e) {
                if (e.target === modal) modal.remove();
            });
        },

        saveSettings: function(widgetType) {
            var settings = getWidgetSettings(widgetType);

            switch (widgetType) {
                case 'weather':
                    settings.city = document.getElementById('setting-city')?.value || '';
                    settings.units = document.getElementById('setting-units')?.value || 'metric';
                    break;
                case 'clock':
                    settings.format = document.getElementById('setting-format')?.value || '24h';
                    break;
                case 'stocks':
                    var symbolsStr = document.getElementById('setting-symbols')?.value || '';
                    settings.symbols = symbolsStr.split(',').map(function(s) { return s.trim().toUpperCase(); }).filter(function(s) { return s; });
                    break;
                case 'crypto':
                    var coinsStr = document.getElementById('setting-coins')?.value || '';
                    settings.coins = coinsStr.split(',').map(function(s) { return s.trim().toLowerCase(); }).filter(function(s) { return s; });
                    break;
                case 'rss':
                    var feedsStr = document.getElementById('setting-feeds')?.value || '';
                    settings.feeds = feedsStr.split('\n').map(function(s) { return s.trim(); }).filter(function(s) { return s; });
                    break;
            }

            saveWidgetSettings(widgetType, settings);
            this.initWidget(widgetType);
        },

        // Calculator
        calcExpression: '',
        calcInput: function(val) {
            this.calcExpression += val;
            var display = document.getElementById('calc-display');
            if (display) display.value = this.calcExpression;
        },
        calcClear: function() {
            this.calcExpression = '';
            var display = document.getElementById('calc-display');
            if (display) display.value = '';
        },
        calcEquals: function() {
            try {
                var result = Function('"use strict"; return (' + this.calcExpression.replace(/[^0-9+\-*/.()]/g, '') + ')')();
                var display = document.getElementById('calc-display');
                if (display) display.value = result;
                this.calcExpression = String(result);
            } catch (e) {
                var display = document.getElementById('calc-display');
                if (display) display.value = 'Error';
                this.calcExpression = '';
            }
        },

        // Quick Links
        addQuickLink: async function() {
            var name = await window.showPrompt('Link name:');
            if (!name) return;

            var url = await window.showPrompt('URL (include https://):');
            if (!url) return;

            try {
                new URL(url);
            } catch (e) {
                await window.showAlert('Invalid URL. Please include https://');
                return;
            }

            var settings = getWidgetSettings('quicklinks');
            settings.links = settings.links || [];
            settings.links.push({ name: name, url: url });
            saveWidgetSettings('quicklinks', settings);
            this.initWidget('quicklinks');
        },

        // Notes
        saveNotes: function(value) {
            var settings = getWidgetSettings('notes');
            settings.notes = value;
            saveWidgetSettings('notes', settings);
        },

        // Converter
        converterUnits: {
            length: { m: 1, km: 1000, cm: 0.01, mm: 0.001, mi: 1609.34, ft: 0.3048, in: 0.0254, yd: 0.9144 },
            weight: { kg: 1, g: 0.001, mg: 0.000001, lb: 0.453592, oz: 0.0283495, st: 6.35029 },
            temperature: { c: 'c', f: 'f', k: 'k' },
            volume: { l: 1, ml: 0.001, gal: 3.78541, qt: 0.946353, pt: 0.473176, cup: 0.236588, floz: 0.0295735 }
        },

        updateConverterUnits: function() {
            var category = document.getElementById('converter-category')?.value || 'length';
            var units = Object.keys(this.converterUnits[category] || {});

            ['from', 'to'].forEach(function(side) {
                var select = document.getElementById('converter-' + side + '-unit');
                if (select) {
                    select.innerHTML = units.map(function(u) { return '<option value="' + u + '">' + u.toUpperCase() + '</option>'; }).join('');
                }
            });

            this.convert();
        },

        convert: function() {
            var category = document.getElementById('converter-category')?.value || 'length';
            var fromValue = parseFloat(document.getElementById('converter-from')?.value) || 0;
            var fromUnit = document.getElementById('converter-from-unit')?.value || '';
            var toUnit = document.getElementById('converter-to-unit')?.value || '';
            var toInput = document.getElementById('converter-to');

            if (!toInput) return;

            var result;
            if (category === 'temperature') {
                result = this.convertTemperature(fromValue, fromUnit, toUnit);
            } else {
                var units = this.converterUnits[category];
                if (!units || !units[fromUnit] || !units[toUnit]) {
                    toInput.value = '';
                    return;
                }
                var baseValue = fromValue * units[fromUnit];
                result = baseValue / units[toUnit];
            }

            toInput.value = isNaN(result) ? '' : result.toFixed(4);
        },

        convertTemperature: function(value, from, to) {
            if (from === to) return value;

            var celsius;
            switch (from) {
                case 'c': celsius = value; break;
                case 'f': celsius = (value - 32) * 5/9; break;
                case 'k': celsius = value - 273.15; break;
                default: return NaN;
            }

            switch (to) {
                case 'c': return celsius;
                case 'f': return celsius * 9/5 + 32;
                case 'k': return celsius + 273.15;
                default: return NaN;
            }
        },

        getAllWidgets: function() {
            return Object.keys(WIDGETS).map(function(type) {
                return Object.assign({ type: type }, WIDGETS[type]);
            });
        },

        getEnabledWidgets: function() {
            return getEnabledWidgets() || this.defaultWidgets;
        }
    };

    // Initialize widgets on DOM ready
    document.addEventListener('DOMContentLoaded', function() {
        if (document.getElementById('widget-grid')) {
            var grid = document.getElementById('widget-grid');
            var defaultWidgets = grid.dataset.defaults ? JSON.parse(grid.dataset.defaults) : null;
            WidgetManager.init(defaultWidgets);
        }
    });

    window.WidgetManager = WidgetManager;
})();


// ============================================================================
// BANG AUTOCOMPLETE
// ============================================================================
(function() {
    'use strict';

    var BANGS_KEY = 'search_custom_bangs';
    var PREFS_KEY = 'search_preferences';

    function getCustomBangs() {
        try {
            return JSON.parse(localStorage.getItem(BANGS_KEY) || '[]');
        } catch (e) {
            return [];
        }
    }

    function getPreferences() {
        try {
            return JSON.parse(localStorage.getItem(PREFS_KEY) || '{}');
        } catch (e) {
            return {};
        }
    }

    function applyPreferences() {
        var prefs = getPreferences();

        if (prefs.theme && prefs.theme !== 'system') {
            document.documentElement.setAttribute('data-theme', prefs.theme);
        }

        if (prefs.new_tab) {
            document.querySelectorAll('.result a').forEach(function(link) {
                link.setAttribute('target', '_blank');
                link.setAttribute('rel', 'noopener noreferrer');
            });
        }
    }

    function initBangSuggestions() {
        var searchInput = document.querySelector('input[name="q"]');
        if (!searchInput) return;

        var suggestionBox = null;

        function createSuggestionBox() {
            if (suggestionBox) return suggestionBox;

            suggestionBox = document.createElement('div');
            suggestionBox.className = 'bang-suggestions';
            suggestionBox.style.cssText = 'position:absolute;background:var(--bg-secondary,#1e1e2e);border:1px solid var(--border-color,#313244);border-radius:4px;max-height:300px;overflow-y:auto;z-index:1000;display:none;width:100%;box-shadow:0 4px 6px rgba(0,0,0,0.3);';

            var parent = searchInput.parentElement;
            parent.style.position = 'relative';
            parent.appendChild(suggestionBox);

            return suggestionBox;
        }

        function escapeHtml(text) {
            var div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        function showSuggestions(bangs) {
            var box = createSuggestionBox();
            box.innerHTML = '';

            if (bangs.length === 0) {
                box.style.display = 'none';
                return;
            }

            bangs.slice(0, 10).forEach(function(bang) {
                var item = document.createElement('div');
                item.className = 'bang-suggestion-item';
                item.innerHTML = '<span class="bang-shortcut">!' + escapeHtml(bang.shortcut) + '</span><span>' + escapeHtml(bang.name) + '</span>';

                item.addEventListener('click', function() {
                    var currentValue = searchInput.value;
                    var bangMatch = currentValue.match(/!(\w*)$/);
                    if (bangMatch) {
                        searchInput.value = currentValue.slice(0, -bangMatch[0].length) + '!' + bang.shortcut + ' ';
                    }
                    searchInput.focus();
                    hideSuggestions();
                });

                box.appendChild(item);
            });

            box.style.display = 'block';
        }

        function hideSuggestions() {
            if (suggestionBox) {
                suggestionBox.style.display = 'none';
            }
        }

        function filterBangs(partial) {
            var builtinBangs = window.__BUILTIN_BANGS || [];
            var customBangs = getCustomBangs();
            var allBangs = customBangs.concat(builtinBangs);

            partial = partial.toLowerCase();
            return allBangs.filter(function(bang) {
                return bang.shortcut.toLowerCase().startsWith(partial) ||
                    bang.name.toLowerCase().includes(partial) ||
                    (bang.aliases && bang.aliases.some(function(a) { return a.toLowerCase().startsWith(partial); }));
            });
        }

        searchInput.addEventListener('input', function() {
            var value = this.value;
            var bangMatch = value.match(/!(\w*)$/);

            if (bangMatch && bangMatch[1].length > 0) {
                var matches = filterBangs(bangMatch[1]);
                showSuggestions(matches);
            } else {
                hideSuggestions();
            }
        });

        searchInput.addEventListener('keydown', function(e) {
            if (!suggestionBox || suggestionBox.style.display === 'none') return;

            var items = suggestionBox.querySelectorAll('.bang-suggestion-item');
            var selected = suggestionBox.querySelector('.bang-suggestion-item.selected');
            var index = Array.from(items).indexOf(selected);

            if (e.key === 'ArrowDown') {
                e.preventDefault();
                if (selected) selected.classList.remove('selected');
                index = (index + 1) % items.length;
                items[index].classList.add('selected');
                items[index].style.background = 'var(--bg-tertiary,#313244)';
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                if (selected) selected.classList.remove('selected');
                index = index <= 0 ? items.length - 1 : index - 1;
                items[index].classList.add('selected');
                items[index].style.background = 'var(--bg-tertiary,#313244)';
            } else if (e.key === 'Enter' && selected) {
                e.preventDefault();
                selected.click();
            } else if (e.key === 'Escape') {
                hideSuggestions();
            }
        });

        document.addEventListener('click', function(e) {
            if (!searchInput.contains(e.target) && (!suggestionBox || !suggestionBox.contains(e.target))) {
                hideSuggestions();
            }
        });
    }

    document.addEventListener('DOMContentLoaded', function() {
        applyPreferences();
        initBangSuggestions();
    });

    window.SearchBangs = {
        getCustomBangs: getCustomBangs,
        getPreferences: getPreferences
    };
})();
