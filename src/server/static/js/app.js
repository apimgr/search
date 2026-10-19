/**
 * Search Application - Consolidated JavaScript
 * Per AI.md PART 17: Single app.js with event delegation (no inline handlers)
 */
(function() {
    'use strict';

    // ========================================================================
    // THEME MANAGEMENT
    // ========================================================================
    const SEARCH_HISTORY_KEY = 'search_history';
    const SEARCH_HISTORY_LIMIT = 20;
    const CUSTOM_CSS_KEY = 'search_custom_css';
    const CUSTOM_CSS_LIMIT = 4096;
    const THEMES = ['dark', 'light', 'auto'];
    let clientTranslations = null;

    // Safe wrapper around Element.prototype.closest(). Event.target is not
    // always an Element (e.g. a text node under Safari's text-click target
    // resolution, or document/window on some drag/mouse edge cases), and
    // those node types have no closest() method, throwing a TypeError.
    // Delegated listeners across this file call this instead of
    // e.target.closest() directly.
    function closestFrom(target, selector) {
        if (!target) return null;
        const el = target.nodeType === 1 ? target : target.parentElement;
        return el ? el.closest(selector) : null;
    }

    function normalizeThemePreference(theme) {
        return THEMES.includes(theme) ? theme : 'dark';
    }

    function applyCustomCSS(css) {
        var style = document.getElementById('search-custom-css');
        if (!css || typeof css !== 'string' || css.length > CUSTOM_CSS_LIMIT) {
            if (style) style.remove();
            return;
        }
        if (!style) {
            style = document.createElement('style');
            style.id = 'search-custom-css';
            document.head.appendChild(style);
        }
        style.textContent = css;
    }

    function loadCustomCSS() {
        try {
            var css = localStorage.getItem(CUSTOM_CSS_KEY) || '';
            applyCustomCSS(css);
            return css.length <= CUSTOM_CSS_LIMIT ? css : '';
        } catch (e) {
            return '';
        }
    }

    function getClientLanguage() {
        var lang = (document.documentElement.lang || 'en').trim().toLowerCase();
        if (!lang) return 'en';
        return lang.split(/[-_]/, 1)[0] || 'en';
    }

    function getTranslationValue(key) {
        if (!clientTranslations) return '';
        return key.split('.').reduce(function(obj, part) {
            return obj && typeof obj === 'object' ? obj[part] : undefined;
        }, clientTranslations) || '';
    }

    function t(key, fallback) {
        return getTranslationValue(key) || fallback || key;
    }

    function loadClientTranslations() {
        return fetch('/locales/' + encodeURIComponent(getClientLanguage()) + '.json', {
            headers: {
                'Accept': 'application/json'
            }
        }).then(function(response) {
            if (!response.ok) {
                throw new Error('translation fetch failed');
            }
            return response.json();
        }).then(function(payload) {
            clientTranslations = payload;
            return payload;
        }).catch(function() {
            clientTranslations = null;
            return null;
        });
    }

    function readConsentCookie() {
        var match = document.cookie.match(/(?:^|;\s*)cookie_consent=([^;]+)/);
        if (!match) return null;
        try { return JSON.parse(decodeURIComponent(match[1])); }
        catch (e) { return null; }
    }

    function writeConsentCookie(consent) {
        document.cookie = 'cookie_consent=' + encodeURIComponent(JSON.stringify(consent)) +
            '; path=/; max-age=31536000; SameSite=Lax';
    }

    function clearCookie(name) {
        document.cookie = name + '=; path=/; max-age=0; SameSite=Lax';
    }

    function isPreferenceCookie(name) {
        return name === 'theme' || name === 'lang' || name === 'search_widgets' ||
            name.indexOf('search_pref_') === 0 || name.indexOf('search_widget_') === 0;
    }

    function canStorePreferenceCookies() {
        var consent = readConsentCookie();
        return !!(consent && consent.preferences === true);
    }

    function setCookie(name, value, maxAgeSeconds) {
        if (isPreferenceCookie(name) && !canStorePreferenceCookies()) return;
        document.cookie = name + '=' + encodeURIComponent(value) + '; path=/; max-age=' + maxAgeSeconds + '; SameSite=Lax' + (window.location.protocol === 'https:' ? '; Secure' : '');
    }

    // Ensure the theme cookie is set only after the visitor allows preferences.
    function syncThemeCookiePreference(theme) {
        var selectedTheme = normalizeThemePreference(theme || '');
        if (!selectedTheme) {
            selectedTheme = getPreferredTheme();
        }
        if (!selectedTheme) {
            selectedTheme = normalizeThemePreference(document.documentElement.getAttribute('data-theme-mode') || '');
        }
        if (selectedTheme) {
            setCookie('theme', selectedTheme, 31536000);
        }
    }

    function normalizeSearchPreferences(prefs) {
        prefs = prefs || {};
        return {
            theme: normalizeThemePreference(prefs.theme || 'dark'),
            lang: prefs.lang || document.documentElement.lang || 'en',
            default_category: prefs.default_category || 'general',
            safe_search: prefs.safe_search !== undefined ? String(prefs.safe_search) : '1',
            results_per_page: prefs.results_per_page ? String(prefs.results_per_page) : '100',
            new_tab: prefs.new_tab !== false,
            infinite_scroll: prefs.infinite_scroll !== false,
            keyboard_shortcuts: prefs.keyboard_shortcuts !== false,
            save_history: prefs.save_history !== false,
            units: prefs.units === 'metric' ? 'metric' : 'imperial',
            engines: typeof prefs.engines === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(prefs.engines) ? prefs.engines : '',
            engine_order: typeof prefs.engine_order === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(prefs.engine_order) ? prefs.engine_order : ''
        };
    }

    function getStoredSearchPreferences() {
        function read(key) {
            var match = document.cookie.match(new RegExp('(?:^|;\\s*)search_pref_' + key + '=([^;]*)'));
            return match ? decodeURIComponent(match[1]) : undefined;
        }
        return normalizeSearchPreferences({
            theme: getPreferredTheme(), lang: (function() { var match = document.cookie.match(/(?:^|;\\s*)lang=([^;]*)/); return match ? decodeURIComponent(match[1]) : document.documentElement.lang || 'en'; })(), default_category: read('category'), safe_search: read('safe_search'),
            results_per_page: read('results_per_page'), new_tab: read('new_tab') !== '0',
            infinite_scroll: read('infinite_scroll') !== '0', keyboard_shortcuts: read('keyboard_shortcuts') !== '0', save_history: read('save_history') !== '0',
            units: read('units'), engines: read('engines'), engine_order: read('engine_order')
        });
    }

    function writeSearchPreferences(value) {
        var prefs = normalizeSearchPreferences(value);
        setCookie('theme', prefs.theme, 31536000);
        setCookie('lang', prefs.lang, 31536000);
        setCookie('search_pref_category', prefs.default_category, 31536000);
        setCookie('search_pref_safe_search', prefs.safe_search, 31536000);
        setCookie('search_pref_results_per_page', prefs.results_per_page, 31536000);
        setCookie('search_pref_units', prefs.units, 31536000);
        setCookie('search_pref_new_tab', prefs.new_tab ? '1' : '0', 31536000);
        setCookie('search_pref_infinite_scroll', prefs.infinite_scroll ? '1' : '0', 31536000);
        setCookie('search_pref_keyboard_shortcuts', prefs.keyboard_shortcuts ? '1' : '0', 31536000);
        setCookie('search_pref_save_history', prefs.save_history ? '1' : '0', 31536000);
        if (prefs.engines) setCookie('search_pref_engines', prefs.engines, 31536000);
        if (prefs.engine_order) setCookie('search_pref_engine_order', prefs.engine_order, 31536000);
        return prefs;
    }

    function decodePreferencePayload(raw) {
        if (!raw) return '';
        try {
            return atob(raw.replace(/-/g, '+').replace(/_/g, '/'));
        } catch (e) {
            return raw;
        }
    }

    function parsePreferenceString(raw) {
        var prefs = normalizeSearchPreferences({});
        raw = decodePreferencePayload((raw || '').trim());
        if (!raw) return prefs;

        if (raw.charAt(0) === '{') {
            try {
                return normalizeSearchPreferences(JSON.parse(raw));
            } catch (e) {
                return prefs;
            }
        }

        raw.split(';').forEach(function(part) {
            if (!part) return;
            var bits = part.split('=');
            if (bits.length < 2) return;
            var key = bits[0].trim();
            var value = bits.slice(1).join('=').trim();

            switch (key) {
                case 't':
                    prefs.theme = normalizeThemePreference(value === 'd' ? 'dark' : value === 'l' ? 'light' : value === 'a' ? 'auto' : value);
                    break;
                case 'c':
                    prefs.default_category = value === 'web' ? 'general' : value;
                    break;
                case 's':
                    prefs.safe_search = value === 'o' ? '0' : value === 'm' ? '1' : value === 's' ? '2' : String(value || '1');
                    break;
                case 'r':
                    prefs.results_per_page = value || '100';
                    break;
                case 'n':
                    prefs.new_tab = value === '1';
                    break;
                case 'p':
                    prefs.infinite_scroll = value === 'i';
                    break;
                case 'k':
                    prefs.keyboard_shortcuts = value !== '0';
                    break;
            }
        });

        return normalizeSearchPreferences(prefs);
    }

    function getURLPreferenceString() {
        try {
            var urlParams = new URLSearchParams(window.location.search);
            return urlParams.get('prefs') || '';
        } catch (e) {
            return '';
        }
    }

    function getActiveSearchPreferences() {
        var stored = getStoredSearchPreferences();
        var urlPrefsRaw = getURLPreferenceString();
        if (!urlPrefsRaw) {
            return stored;
        }

        var urlPrefs = parsePreferenceString(urlPrefsRaw);
        var merged = normalizeSearchPreferences({
            theme: urlPrefs.theme || stored.theme,
            default_category: urlPrefs.default_category || stored.default_category,
            safe_search: urlPrefs.safe_search || stored.safe_search,
            results_per_page: urlPrefs.results_per_page || stored.results_per_page,
            new_tab: urlPrefs.new_tab,
            infinite_scroll: urlPrefs.infinite_scroll,
            keyboard_shortcuts: urlPrefs.keyboard_shortcuts,
            save_history: stored.save_history
        });
        return merged;
    }

    function upsertHiddenField(form, name, value) {
        if (!form) return;
        var input = form.querySelector('input[name="' + name + '"]');
        if (!input) {
            input = document.createElement('input');
            input.type = 'hidden';
            input.name = name;
            form.appendChild(input);
        }
        input.value = value;
    }

    function applySearchPreferencesToForms() {
        var prefs = getActiveSearchPreferences();
        var urlPrefs = getURLPreferenceString();

        document.querySelectorAll('form[action="/search"]').forEach(function(form) {
            upsertHiddenField(form, 'safe_search', prefs.safe_search);
            upsertHiddenField(form, 'per_page', prefs.results_per_page);
            if (urlPrefs) {
                upsertHiddenField(form, 'prefs', urlPrefs);
            }
        });
    }

    function getPreferredTheme() {
        var match = document.cookie.match(/(?:^|;\s*)theme=([^;]*)/);
        if (match) {
            var saved = decodeURIComponent(match[1]);
            if (THEMES.includes(saved)) {
                return saved;
            }
        }
        return 'dark';
    }

    function getPreferredUnits() {
        var match = document.cookie.match(/(?:^|;\s*)search_pref_units=([^;]*)/);
        if (match) {
            var saved = decodeURIComponent(match[1]);
            if (saved === 'metric' || saved === 'imperial') {
                return saved;
            }
        }
        return 'imperial';
    }

    function setUnitsCookie(units) {
        setCookie('search_pref_units', units === 'metric' ? 'metric' : 'imperial', 31536000);
    }

    // Resolve "auto" to actual dark/light based on system preference
    function resolveTheme(theme) {
        if (theme === 'auto') {
            return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
        }
        return theme;
    }

    function applyTheme(theme) {
        var resolved = resolveTheme(theme);
        // Preserve independent appearance classes while switching the theme.
        document.documentElement.classList.remove('theme-dark', 'theme-light', 'theme-auto');
        document.documentElement.classList.add('theme-' + resolved);
        document.documentElement.setAttribute('data-theme-mode', theme);
    }

    function setTheme(theme) {
        applyTheme(theme);
        setCookie('theme', theme, 31536000);
    }

    // No-op: icon display is now fully CSS-driven via [data-theme-mode] attribute
    function updateThemeIcon() {}

    function toggleTheme() {
        // Per AI.md PART 16: cycle dark → light → auto → dark
        const current = document.documentElement.getAttribute('data-theme-mode') || 'dark';
        const next = current === 'dark' ? 'light' : current === 'light' ? 'auto' : 'dark';
        setTheme(next);
    }

    // Listen for system preference changes when in auto mode
    var systemThemeQuery = window.matchMedia('(prefers-color-scheme: dark)');
    function onSystemThemeChange() {
        if (getPreferredTheme() === 'auto') {
            applyTheme('auto');
        }
    }
    if (systemThemeQuery.addEventListener) {
        systemThemeQuery.addEventListener('change', onSystemThemeChange);
    } else if (systemThemeQuery.addListener) {
        // Safari < 14 fallback
        systemThemeQuery.addListener(onSystemThemeChange);
    }

    // ========================================================================
    // MOBILE NAVIGATION - CSS-only per AI.md PART 16
    // ========================================================================
    // Navigation is handled purely via CSS checkbox hack (no JavaScript needed)
    // The checkbox #nav-toggle controls menu state via :checked pseudo-class
    // Kept as no-ops for backward compatibility if anything calls these
    function toggleNav() {
        // CSS-only: checkbox handles state via label click
    }

    function closeNav() {
        // CSS-only: uncheck the checkbox to close
        const checkbox = document.getElementById('nav-toggle');
        if (checkbox) checkbox.checked = false;
    }

    // ========================================================================
    // ACCESSIBILITY (A11Y) - Per AI.md PART 31
    // ========================================================================

    // Screen reader announcer - announces messages without moving focus
    var srAnnouncer = null;
    function initSRAnnouncr() {
        if (!srAnnouncer) {
            srAnnouncer = document.createElement('div');
            srAnnouncer.setAttribute('role', 'status');
            srAnnouncer.setAttribute('aria-live', 'polite');
            srAnnouncer.setAttribute('aria-atomic', 'true');
            srAnnouncer.className = 'sr-only';
            srAnnouncer.id = 'sr-announcer';
            document.body.appendChild(srAnnouncer);
        }
    }

    function announce(message, priority) {
        initSRAnnouncr();
        // Set assertive for urgent messages, polite for informational
        srAnnouncer.setAttribute('aria-live', priority === 'assertive' ? 'assertive' : 'polite');
        // Clear and re-set to trigger screen reader announcement
        srAnnouncer.textContent = '';
        setTimeout(function() {
            srAnnouncer.textContent = message;
        }, 100);
    }

    // Focus trap for modals - keeps focus inside modal when open
    function trapFocus(element) {
        var focusableElements = element.querySelectorAll(
            'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), ' +
            'textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
        );
        var firstFocusable = focusableElements[0];
        var lastFocusable = focusableElements[focusableElements.length - 1];

        function handleKeydown(e) {
            if (e.key !== 'Tab') return;

            if (e.shiftKey) {
                // Shift+Tab
                if (document.activeElement === firstFocusable) {
                    e.preventDefault();
                    lastFocusable.focus();
                }
            } else {
                // Tab
                if (document.activeElement === lastFocusable) {
                    e.preventDefault();
                    firstFocusable.focus();
                }
            }
        }

        element.addEventListener('keydown', handleKeydown);

        // Return cleanup function
        return function() {
            element.removeEventListener('keydown', handleKeydown);
        };
    }

    // Expose announce function globally for use by other scripts
    window.srAnnounce = announce;

    // Keyboard navigation for tabs (WCAG 2.1 Level AA)
    // Arrow keys navigate between tabs, Enter/Space activates
    function initTabKeyboardNav() {
        const tablists = document.querySelectorAll('[role="tablist"]');
        tablists.forEach(function(tablist) {
            const tabs = tablist.querySelectorAll('[role="tab"]');
            if (tabs.length === 0) return;

            tablist.addEventListener('keydown', function(e) {
                const currentTab = document.activeElement;
                if (!currentTab.matches('[role="tab"]')) return;

                let index = Array.from(tabs).indexOf(currentTab);
                let newIndex = index;

                switch (e.key) {
                    case 'ArrowLeft':
                    case 'ArrowUp':
                        e.preventDefault();
                        newIndex = index - 1;
                        if (newIndex < 0) newIndex = tabs.length - 1;
                        break;
                    case 'ArrowRight':
                    case 'ArrowDown':
                        e.preventDefault();
                        newIndex = index + 1;
                        if (newIndex >= tabs.length) newIndex = 0;
                        break;
                    case 'Home':
                        e.preventDefault();
                        newIndex = 0;
                        break;
                    case 'End':
                        e.preventDefault();
                        newIndex = tabs.length - 1;
                        break;
                    default:
                        return;
                }

                if (newIndex !== index) {
                    tabs[newIndex].focus();
                    // Activate tab on focus for category tabs
                    if (tabs[newIndex].classList.contains('category-tab')) {
                        tabs[newIndex].click();
                    }
                }
            });

            // Set tabindex properly (only active tab should be tabbable)
            tabs.forEach(function(tab, i) {
                tab.setAttribute('tabindex', tab.getAttribute('aria-selected') === 'true' ? '0' : '-1');
            });
        });
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
    function saveAndApplyConsent(consent) {
        writeConsentCookie(consent);
        if (!consent.preferences) {
            ['theme', 'lang', 'search_widgets', 'search_pref_category', 'search_pref_safe_search',
                'search_pref_results_per_page', 'search_pref_units', 'search_pref_new_tab',
                'search_pref_infinite_scroll', 'search_pref_keyboard_shortcuts', 'search_pref_save_history',
                'search_pref_video_previews', 'search_pref_font_size', 'search_pref_results_density',
                'search_pref_accent_color', 'search_pref_engines', 'search_pref_engine_order', 'search_widget_weather_units', 'preferencesEnabled'].forEach(clearCookie);
        }
        var banner = document.getElementById('cookie-consent');
        if (banner) banner.remove();
        applyConsent(consent);
    }

    function applyConsent(consent) {
        if (consent.analytics) {
            loadTracking();
        }
    }

    function loadTracking() {
        // Opt-in analytics only — placeholder for when server.tracking is configured
    }

    function initCCPA() {
        var banner = document.getElementById('cookie-consent');
        var dataSold = banner && banner.dataset.sold === 'true';
        if (dataSold) {
            var doNotSell = /(?:^|;\s*)ccpa_opt_out=true/.test(document.cookie);
            if (doNotSell) {
                applyCCPAOptOut();
            }
        }
        document.querySelectorAll('form[action="/server/ccpa"]').forEach(function(form) {
            form.addEventListener('submit', function(event) {
                if (form.elements.optout && form.elements.optout.value === 'true') {
                    event.preventDefault();
                    ccpaDoNotSell();
                }
            });
        });
    }

    function ccpaDoNotSell() {
        applyCCPAOptOut();
        var consent = {
            essential: true,
            preferences: false,
            analytics: false,
            timestamp: Date.now(),
            ccpaOptOut: true
        };
        saveAndApplyConsent(consent);
    }

    function applyCCPAOptOut() {
        document.cookie = 'ccpa_opt_out=true; path=/; max-age=31536000';
    }

    // ========================================================================
    // ANNOUNCEMENTS
    // ========================================================================

    // Read dismissed announcement ids from the cookie (comma-separated list)
    function getDismissedAnnouncements() {
        var match = document.cookie.match(/(?:^|;\s*)dismissed_announcements=([^;]*)/);
        return match ? decodeURIComponent(match[1]).split(',').filter(Boolean) : [];
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
                button.textContent = t('common.copied', 'Copied!');
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
                btn.textContent = t('common.copied', 'Copied!');
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
        return window.showConfirm(t('user.revoke_token_confirm_message', 'Are you sure you want to revoke this token? This action cannot be undone.'), {
            title: t('user.revoke_token_title', 'Revoke Token'),
            confirmText: t('user.revoke_token_action', 'Revoke'),
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

    // Built-in bangs list (generated from src/search/bang/defaults.go)
    var BANGS_KEY = 'search_custom_bangs';
    var BUILTIN_BANGS = [
        { shortcut: 'g', name: 'Google', category: 'general' },
        { shortcut: 'ddg', name: 'DuckDuckGo', category: 'general' },
        { shortcut: 'b', name: 'Bing', category: 'general' },
        { shortcut: 'sp', name: 'Startpage', category: 'general' },
        { shortcut: 'q', name: 'Qwant', category: 'general' },
        { shortcut: 'ec', name: 'Ecosia', category: 'general' },
        { shortcut: 'br', name: 'Brave Search', category: 'general' },
        { shortcut: 'ya', name: 'Yandex', category: 'general' },
        { shortcut: 'bd', name: 'Baidu', category: 'general' },
        { shortcut: 'swisscows', name: 'Swisscows', category: 'general' },
        { shortcut: 'mojeek', name: 'Mojeek', category: 'general' },
        { shortcut: 'metager', name: 'MetaGer', category: 'general' },
        { shortcut: 'gibiru', name: 'Gibiru', category: 'general' },
        { shortcut: 'yep', name: 'Yep', category: 'general' },
        { shortcut: 'kagi', name: 'Kagi', category: 'general' },
        { shortcut: 'you', name: 'You.com', category: 'general' },
        { shortcut: 'perplexity', name: 'Perplexity', category: 'general' },
        { shortcut: 'presearch', name: 'Presearch', category: 'general' },
        { shortcut: 'naver', name: 'Naver', category: 'general' },
        { shortcut: 'sogou', name: 'Sogou', category: 'general' },
        { shortcut: 'ask', name: 'Ask.com', category: 'general' },
        { shortcut: 'aol', name: 'AOL Search', category: 'general' },
        { shortcut: 'yahoo', name: 'Yahoo', category: 'general' },
        { shortcut: 'lycos', name: 'Lycos', category: 'general' },
        { shortcut: 'gi', name: 'Google Images', category: 'images' },
        { shortcut: 'bi', name: 'Bing Images', category: 'images' },
        { shortcut: 'yi', name: 'Yandex Images', category: 'images' },
        { shortcut: 'fl', name: 'Flickr', category: 'images' },
        { shortcut: 'us', name: 'Unsplash', category: 'images' },
        { shortcut: 'pexels', name: 'Pexels', category: 'images' },
        { shortcut: 'pixabay', name: 'Pixabay', category: 'images' },
        { shortcut: '500px', name: '500px', category: 'images' },
        { shortcut: 'pinterest', name: 'Pinterest', category: 'images' },
        { shortcut: 'imgur', name: 'Imgur', category: 'images' },
        { shortcut: 'deviantart', name: 'DeviantArt', category: 'images' },
        { shortcut: 'artstation', name: 'ArtStation', category: 'images' },
        { shortcut: 'behance', name: 'Behance', category: 'images' },
        { shortcut: 'dribbble', name: 'Dribbble', category: 'images' },
        { shortcut: 'giphy', name: 'Giphy', category: 'images' },
        { shortcut: 'tenor', name: 'Tenor GIFs', category: 'images' },
        { shortcut: 'tineye', name: 'TinEye', category: 'images' },
        { shortcut: 'icons8', name: 'Icons8', category: 'images' },
        { shortcut: 'flaticon', name: 'Flaticon', category: 'images' },
        { shortcut: 'iconfinder', name: 'Iconfinder', category: 'images' },
        { shortcut: 'freepik', name: 'Freepik', category: 'images' },
        { shortcut: 'stocksnap', name: 'StockSnap', category: 'images' },
        { shortcut: 'openclipart', name: 'OpenClipart', category: 'images' },
        { shortcut: 'wikimediacommons', name: 'Wikimedia Commons', category: 'images' },
        { shortcut: 'yt', name: 'YouTube', category: 'video' },
        { shortcut: 'v', name: 'Vimeo', category: 'video' },
        { shortcut: 'dtube', name: 'Dailymotion', category: 'video' },
        { shortcut: 'pt', name: 'PeerTube', category: 'video' },
        { shortcut: 'od', name: 'Odysee', category: 'video' },
        { shortcut: 'rumble', name: 'Rumble', category: 'video' },
        { shortcut: 'bitchute', name: 'BitChute', category: 'video' },
        { shortcut: 'twitch', name: 'Twitch', category: 'video' },
        { shortcut: 'tiktok', name: 'TikTok', category: 'video' },
        { shortcut: 'gv', name: 'Google Video', category: 'video' },
        { shortcut: 'bv', name: 'Bing Video', category: 'video' },
        { shortcut: 'veoh', name: 'Veoh', category: 'video' },
        { shortcut: 'metacafe', name: 'Metacafe', category: 'video' },
        { shortcut: 'nebula', name: 'Nebula', category: 'video' },
        { shortcut: 'floatplane', name: 'Floatplane', category: 'video' },
        { shortcut: 'gm', name: 'Google Maps', category: 'maps' },
        { shortcut: 'osm', name: 'OpenStreetMap', category: 'maps' },
        { shortcut: 'bm', name: 'Bing Maps', category: 'maps' },
        { shortcut: 'applemaps', name: 'Apple Maps', category: 'maps' },
        { shortcut: 'here', name: 'HERE WeGo', category: 'maps' },
        { shortcut: 'waze', name: 'Waze', category: 'maps' },
        { shortcut: 'mapquest', name: 'MapQuest', category: 'maps' },
        { shortcut: 'yelp', name: 'Yelp', category: 'maps' },
        { shortcut: 'tripadvisor', name: 'TripAdvisor', category: 'maps' },
        { shortcut: 'foursquare', name: 'Foursquare', category: 'maps' },
        { shortcut: 'gmapsat', name: 'Google Maps Satellite', category: 'maps' },
        { shortcut: 'gearth', name: 'Google Earth', category: 'maps' },
        { shortcut: 'gn', name: 'Google News', category: 'news' },
        { shortcut: 'bn', name: 'Bing News', category: 'news' },
        { shortcut: 'yn', name: 'Yahoo News', category: 'news' },
        { shortcut: 'reuters', name: 'Reuters', category: 'news' },
        { shortcut: 'ap', name: 'AP News', category: 'news' },
        { shortcut: 'bbc', name: 'BBC', category: 'news' },
        { shortcut: 'cnn', name: 'CNN', category: 'news' },
        { shortcut: 'nyt', name: 'New York Times', category: 'news' },
        { shortcut: 'wapo', name: 'Washington Post', category: 'news' },
        { shortcut: 'guardian', name: 'The Guardian', category: 'news' },
        { shortcut: 'aljazeera', name: 'Al Jazeera', category: 'news' },
        { shortcut: 'npr', name: 'NPR', category: 'news' },
        { shortcut: 'techcrunch', name: 'TechCrunch', category: 'news' },
        { shortcut: 'verge', name: 'The Verge', category: 'news' },
        { shortcut: 'wired', name: 'Wired', category: 'news' },
        { shortcut: 'arstechnica', name: 'Ars Technica', category: 'news' },
        { shortcut: 'engadget', name: 'Engadget', category: 'news' },
        { shortcut: 'cnet', name: 'CNET', category: 'news' },
        { shortcut: 'zdnet', name: 'ZDNet', category: 'news' },
        { shortcut: 'venturebeat', name: 'VentureBeat', category: 'news' },
        { shortcut: 'hackernoon', name: 'HackerNoon', category: 'news' },
        { shortcut: 'bloomberg', name: 'Bloomberg', category: 'news' },
        { shortcut: 'ft', name: 'Financial Times', category: 'news' },
        { shortcut: 'wsj', name: 'Wall Street Journal', category: 'news' },
        { shortcut: 'economist', name: 'The Economist', category: 'news' },
        { shortcut: 'forbes', name: 'Forbes', category: 'news' },
        { shortcut: 'businessinsider', name: 'Business Insider', category: 'news' },
        { shortcut: 'w', name: 'Wikipedia', category: 'knowledge' },
        { shortcut: 'wd', name: 'Wikidata', category: 'knowledge' },
        { shortcut: 'wt', name: 'Wiktionary', category: 'knowledge' },
        { shortcut: 'wb', name: 'Wikibooks', category: 'knowledge' },
        { shortcut: 'wq', name: 'Wikiquote', category: 'knowledge' },
        { shortcut: 'ws', name: 'Wikisource', category: 'knowledge' },
        { shortcut: 'wn', name: 'Wikinews', category: 'knowledge' },
        { shortcut: 'wv', name: 'Wikivoyage', category: 'knowledge' },
        { shortcut: 'wa', name: 'Wolfram Alpha', category: 'knowledge' },
        { shortcut: 'britannica', name: 'Britannica', category: 'knowledge' },
        { shortcut: 'quora', name: 'Quora', category: 'knowledge' },
        { shortcut: 'answers', name: 'Answers.com', category: 'knowledge' },
        { shortcut: 'howstuffworks', name: 'HowStuffWorks', category: 'knowledge' },
        { shortcut: 'scholarpedia', name: 'Scholarpedia', category: 'knowledge' },
        { shortcut: 'citizendium', name: 'Citizendium', category: 'knowledge' },
        { shortcut: 'infoplease', name: 'Infoplease', category: 'knowledge' },
        { shortcut: 'fandom', name: 'Fandom', category: 'knowledge' },
        { shortcut: 'tvtropes', name: 'TV Tropes', category: 'knowledge' },
        { shortcut: 'tw', name: 'Twitter/X', category: 'social' },
        { shortcut: 'mast', name: 'Mastodon', category: 'social' },
        { shortcut: 'r', name: 'Reddit', category: 'social' },
        { shortcut: 'hn', name: 'Hacker News', category: 'social' },
        { shortcut: 'lb', name: 'Lobsters', category: 'social' },
        { shortcut: 'fb', name: 'Facebook', category: 'social' },
        { shortcut: 'ig', name: 'Instagram', category: 'social' },
        { shortcut: 'linkedin', name: 'LinkedIn', category: 'social' },
        { shortcut: 'tumblr', name: 'Tumblr', category: 'social' },
        { shortcut: 'discord', name: 'Discord', category: 'social' },
        { shortcut: 'slack', name: 'Slack', category: 'social' },
        { shortcut: 'telegram', name: 'Telegram', category: 'social' },
        { shortcut: 'medium', name: 'Medium', category: 'social' },
        { shortcut: 'substack', name: 'Substack', category: 'social' },
        { shortcut: 'lemmy', name: 'Lemmy', category: 'social' },
        { shortcut: 'minds', name: 'Minds', category: 'social' },
        { shortcut: 'gab', name: 'Gab', category: 'social' },
        { shortcut: 'parler', name: 'Parler', category: 'social' },
        { shortcut: 'truthsocial', name: 'Truth Social', category: 'social' },
        { shortcut: 'bluesky', name: 'Bluesky', category: 'social' },
        { shortcut: 'threads', name: 'Threads', category: 'social' },
        { shortcut: 'snapchat', name: 'Snapchat', category: 'social' },
        { shortcut: 'whatsapp', name: 'WhatsApp', category: 'social' },
        { shortcut: 'signal', name: 'Signal', category: 'social' },
        { shortcut: 'matrix', name: 'Matrix', category: 'social' },
        { shortcut: 'gh', name: 'GitHub', category: 'code' },
        { shortcut: 'ghc', name: 'GitHub Code', category: 'code' },
        { shortcut: 'ghr', name: 'GitHub Repos', category: 'code' },
        { shortcut: 'ghu', name: 'GitHub Users', category: 'code' },
        { shortcut: 'ghi', name: 'GitHub Issues', category: 'code' },
        { shortcut: 'ghd', name: 'GitHub Discussions', category: 'code' },
        { shortcut: 'gist', name: 'GitHub Gist', category: 'code' },
        { shortcut: 'gl', name: 'GitLab', category: 'code' },
        { shortcut: 'bb', name: 'Bitbucket', category: 'code' },
        { shortcut: 'sr', name: 'Sourcegraph', category: 'code' },
        { shortcut: 'so', name: 'Stack Overflow', category: 'code' },
        { shortcut: 'se', name: 'Stack Exchange', category: 'code' },
        { shortcut: 'su', name: 'Super User', category: 'code' },
        { shortcut: 'sf', name: 'Server Fault', category: 'code' },
        { shortcut: 'au', name: 'Ask Ubuntu', category: 'code' },
        { shortcut: 'unix', name: 'Unix & Linux', category: 'code' },
        { shortcut: 'npm', name: 'npm', category: 'code' },
        { shortcut: 'yarn', name: 'Yarn', category: 'code' },
        { shortcut: 'pypi', name: 'PyPI', category: 'code' },
        { shortcut: 'crates', name: 'Crates.io', category: 'code' },
        { shortcut: 'gopkg', name: 'Go Packages', category: 'code' },
        { shortcut: 'hex', name: 'Hex.pm', category: 'code' },
        { shortcut: 'rubygems', name: 'RubyGems', category: 'code' },
        { shortcut: 'nuget', name: 'NuGet', category: 'code' },
        { shortcut: 'maven', name: 'Maven Central', category: 'code' },
        { shortcut: 'packagist', name: 'Packagist', category: 'code' },
        { shortcut: 'cpan', name: 'CPAN', category: 'code' },
        { shortcut: 'cran', name: 'CRAN', category: 'code' },
        { shortcut: 'cocoapods', name: 'CocoaPods', category: 'code' },
        { shortcut: 'pub', name: 'pub.dev', category: 'code' },
        { shortcut: 'haskell', name: 'Hackage', category: 'code' },
        { shortcut: 'docker', name: 'Docker Hub', category: 'code' },
        { shortcut: 'quay', name: 'Quay.io', category: 'code' },
        { shortcut: 'ghcr', name: 'GitHub Container Registry', category: 'code' },
        { shortcut: 'helm', name: 'Helm Hub', category: 'code' },
        { shortcut: 'terraform', name: 'Terraform Registry', category: 'code' },
        { shortcut: 'ansible', name: 'Ansible Galaxy', category: 'code' },
        { shortcut: 'mdn', name: 'MDN Web Docs', category: 'code' },
        { shortcut: 'can', name: 'Can I Use', category: 'code' },
        { shortcut: 'w3schools', name: 'W3Schools', category: 'code' },
        { shortcut: 'devdocs', name: 'DevDocs', category: 'code' },
        { shortcut: 'codepen', name: 'CodePen', category: 'code' },
        { shortcut: 'jsfiddle', name: 'JSFiddle', category: 'code' },
        { shortcut: 'codesandbox', name: 'CodeSandbox', category: 'code' },
        { shortcut: 'replit', name: 'Replit', category: 'code' },
        { shortcut: 'glitch', name: 'Glitch', category: 'code' },
        { shortcut: 'leetcode', name: 'LeetCode', category: 'code' },
        { shortcut: 'hackerrank', name: 'HackerRank', category: 'code' },
        { shortcut: 'codewars', name: 'Codewars', category: 'code' },
        { shortcut: 'exercism', name: 'Exercism', category: 'code' },
        { shortcut: 'rosetta', name: 'Rosetta Code', category: 'code' },
        { shortcut: 'regex101', name: 'Regex101', category: 'code' },
        { shortcut: 'explainshell', name: 'ExplainShell', category: 'code' },
        { shortcut: 'tldr', name: 'TLDR Pages', category: 'code' },
        { shortcut: 'devhints', name: 'Devhints', category: 'code' },
        { shortcut: 'cheatography', name: 'Cheatography', category: 'code' },
        { shortcut: 'amz', name: 'Amazon', category: 'shopping' },
        { shortcut: 'azuk', name: 'Amazon UK', category: 'shopping' },
        { shortcut: 'azde', name: 'Amazon Germany', category: 'shopping' },
        { shortcut: 'azjp', name: 'Amazon Japan', category: 'shopping' },
        { shortcut: 'eb', name: 'eBay', category: 'shopping' },
        { shortcut: 'aliexpress', name: 'AliExpress', category: 'shopping' },
        { shortcut: 'alibaba', name: 'Alibaba', category: 'shopping' },
        { shortcut: 'walmart', name: 'Walmart', category: 'shopping' },
        { shortcut: 'target', name: 'Target', category: 'shopping' },
        { shortcut: 'bestbuy', name: 'Best Buy', category: 'shopping' },
        { shortcut: 'newegg', name: 'Newegg', category: 'shopping' },
        { shortcut: 'etsy', name: 'Etsy', category: 'shopping' },
        { shortcut: 'wish', name: 'Wish', category: 'shopping' },
        { shortcut: 'shopify', name: 'Shopify', category: 'shopping' },
        { shortcut: 'camelcamelcamel', name: 'CamelCamelCamel', category: 'shopping' },
        { shortcut: 'keepa', name: 'Keepa', category: 'shopping' },
        { shortcut: 'pricespy', name: 'PriceSpy', category: 'shopping' },
        { shortcut: 'slickdeals', name: 'Slickdeals', category: 'shopping' },
        { shortcut: 'rakuten', name: 'Rakuten', category: 'shopping' },
        { shortcut: 'mercari', name: 'Mercari', category: 'shopping' },
        { shortcut: 'poshmark', name: 'Poshmark', category: 'shopping' },
        { shortcut: 'thriftbooks', name: 'ThriftBooks', category: 'shopping' },
        { shortcut: 'bookfinder', name: 'BookFinder', category: 'shopping' },
        { shortcut: 'archive', name: 'Internet Archive', category: 'files' },
        { shortcut: '1337x', name: '1337x', category: 'files' },
        { shortcut: 'rarbg', name: 'RARBG', category: 'files' },
        { shortcut: 'nyaa', name: 'Nyaa', category: 'files' },
        { shortcut: 'libgen', name: 'Library Genesis', category: 'files' },
        { shortcut: 'zlibrary', name: 'Z-Library', category: 'files' },
        { shortcut: 'annas', name: 'Anna\'s Archive', category: 'files' },
        { shortcut: 'sci-hub', name: 'Sci-Hub', category: 'files' },
        { shortcut: 'pdfdrive', name: 'PDF Drive', category: 'files' },
        { shortcut: 'openlib', name: 'Open Library', category: 'files' },
        { shortcut: 'gutenberg', name: 'Project Gutenberg', category: 'files' },
        { shortcut: 'standardebooks', name: 'Standard Ebooks', category: 'files' },
        { shortcut: 'audiobookbay', name: 'AudioBook Bay', category: 'files' },
        { shortcut: 'spot', name: 'Spotify', category: 'music' },
        { shortcut: 'applemusic', name: 'Apple Music', category: 'music' },
        { shortcut: 'ytm', name: 'YouTube Music', category: 'music' },
        { shortcut: 'sc', name: 'SoundCloud', category: 'music' },
        { shortcut: 'bc', name: 'Bandcamp', category: 'music' },
        { shortcut: 'lastfm', name: 'Last.fm', category: 'music' },
        { shortcut: 'discogs', name: 'Discogs', category: 'music' },
        { shortcut: 'allmusic', name: 'AllMusic', category: 'music' },
        { shortcut: 'lyrics', name: 'Genius Lyrics', category: 'music' },
        { shortcut: 'azlyrics', name: 'AZLyrics', category: 'music' },
        { shortcut: 'musixmatch', name: 'Musixmatch', category: 'music' },
        { shortcut: 'songmeanings', name: 'SongMeanings', category: 'music' },
        { shortcut: 'setlistfm', name: 'Setlist.fm', category: 'music' },
        { shortcut: 'songkick', name: 'Songkick', category: 'music' },
        { shortcut: 'rateyourmusic', name: 'Rate Your Music', category: 'music' },
        { shortcut: 'musicbrainz', name: 'MusicBrainz', category: 'music' },
        { shortcut: 'deezer', name: 'Deezer', category: 'music' },
        { shortcut: 'tidal', name: 'Tidal', category: 'music' },
        { shortcut: 'audiomack', name: 'Audiomack', category: 'music' },
        { shortcut: 'mixcloud', name: 'Mixcloud', category: 'music' },
        { shortcut: 'hearthis', name: 'hearthis.at', category: 'music' },
        { shortcut: 'scholar', name: 'Google Scholar', category: 'science' },
        { shortcut: 'arxiv', name: 'arXiv', category: 'science' },
        { shortcut: 'pubmed', name: 'PubMed', category: 'science' },
        { shortcut: 'semanticscholar', name: 'Semantic Scholar', category: 'science' },
        { shortcut: 'researchgate', name: 'ResearchGate', category: 'science' },
        { shortcut: 'academia', name: 'Academia.edu', category: 'science' },
        { shortcut: 'jstor', name: 'JSTOR', category: 'science' },
        { shortcut: 'ieee', name: 'IEEE Xplore', category: 'science' },
        { shortcut: 'acm', name: 'ACM Digital Library', category: 'science' },
        { shortcut: 'springerlink', name: 'SpringerLink', category: 'science' },
        { shortcut: 'nature', name: 'Nature', category: 'science' },
        { shortcut: 'science', name: 'Science', category: 'science' },
        { shortcut: 'plos', name: 'PLOS ONE', category: 'science' },
        { shortcut: 'biorxiv', name: 'bioRxiv', category: 'science' },
        { shortcut: 'medrxiv', name: 'medRxiv', category: 'science' },
        { shortcut: 'chemrxiv', name: 'ChemRxiv', category: 'science' },
        { shortcut: 'ssrn', name: 'SSRN', category: 'science' },
        { shortcut: 'philpapers', name: 'PhilPapers', category: 'science' },
        { shortcut: 'doaj', name: 'DOAJ', category: 'science' },
        { shortcut: 'core', name: 'CORE', category: 'science' },
        { shortcut: 'base', name: 'BASE', category: 'science' },
        { shortcut: 'dimensions', name: 'Dimensions', category: 'science' },
        { shortcut: 'lens', name: 'The Lens', category: 'science' },
        { shortcut: 'orcid', name: 'ORCID', category: 'science' },
        { shortcut: 'crossref', name: 'Crossref', category: 'science' },
        { shortcut: 'doi', name: 'DOI Resolver', category: 'science' },
        { shortcut: 'nasa', name: 'NASA', category: 'science' },
        { shortcut: 'esa', name: 'ESA', category: 'science' },
        { shortcut: 'ncbi', name: 'NCBI', category: 'science' },
        { shortcut: 'gt', name: 'Google Translate', category: 'translate' },
        { shortcut: 'deepl', name: 'DeepL', category: 'translate' },
        { shortcut: 'btrans', name: 'Bing Translator', category: 'translate' },
        { shortcut: 'reverso', name: 'Reverso', category: 'translate' },
        { shortcut: 'linguee', name: 'Linguee', category: 'translate' },
        { shortcut: 'dict', name: 'Dictionary.com', category: 'translate' },
        { shortcut: 'thesaurus', name: 'Thesaurus.com', category: 'translate' },
        { shortcut: 'mw', name: 'Merriam-Webster', category: 'translate' },
        { shortcut: 'oxford', name: 'Oxford Dictionary', category: 'translate' },
        { shortcut: 'cambridge', name: 'Cambridge Dictionary', category: 'translate' },
        { shortcut: 'collins', name: 'Collins Dictionary', category: 'translate' },
        { shortcut: 'longman', name: 'Longman Dictionary', category: 'translate' },
        { shortcut: 'macmillan', name: 'Macmillan Dictionary', category: 'translate' },
        { shortcut: 'ud', name: 'Urban Dictionary', category: 'translate' },
        { shortcut: 'etymonline', name: 'Etymonline', category: 'translate' },
        { shortcut: 'wordnik', name: 'Wordnik', category: 'translate' },
        { shortcut: 'forvo', name: 'Forvo', category: 'translate' },
        { shortcut: 'wordreference', name: 'WordReference', category: 'translate' },
        { shortcut: 'leo', name: 'LEO Dictionary', category: 'translate' },
        { shortcut: 'pons', name: 'PONS', category: 'translate' },
        { shortcut: 'jisho', name: 'Jisho', category: 'translate' },
        { shortcut: 'tangorin', name: 'Tangorin', category: 'translate' },
        { shortcut: 'navdict', name: 'Naver Dictionary', category: 'translate' },
        { shortcut: 'pleco', name: 'Pleco', category: 'translate' },
        { shortcut: 'mdbg', name: 'MDBG Chinese', category: 'translate' },
        { shortcut: 'wbm', name: 'Wayback Machine', category: 'privacy' },
        { shortcut: 'haveibeenpwned', name: 'Have I Been Pwned', category: 'privacy' },
        { shortcut: 'dehashed', name: 'DeHashed', category: 'privacy' },
        { shortcut: 'virustotal', name: 'VirusTotal', category: 'privacy' },
        { shortcut: 'shodan', name: 'Shodan', category: 'privacy' },
        { shortcut: 'censys', name: 'Censys', category: 'privacy' },
        { shortcut: 'crt', name: 'crt.sh', category: 'privacy' },
        { shortcut: 'dnsdumpster', name: 'DNSDumpster', category: 'privacy' },
        { shortcut: 'securitytrails', name: 'SecurityTrails', category: 'privacy' },
        { shortcut: 'whois', name: 'WHOIS', category: 'privacy' },
        { shortcut: 'ipinfo', name: 'IPinfo', category: 'privacy' },
        { shortcut: 'abuseipdb', name: 'AbuseIPDB', category: 'privacy' },
        { shortcut: 'urlscan', name: 'urlscan.io', category: 'privacy' },
        { shortcut: 'httpstatus', name: 'HTTP Status', category: 'privacy' },
        { shortcut: 'ssltest', name: 'SSL Labs', category: 'privacy' },
        { shortcut: 'securityheaders', name: 'Security Headers', category: 'privacy' },
        { shortcut: 'builtwith', name: 'BuiltWith', category: 'privacy' },
        { shortcut: 'wappalyzer', name: 'Wappalyzer', category: 'privacy' },
        { shortcut: 'downforeveryoneorjustme', name: 'Down For Everyone', category: 'privacy' },
        { shortcut: 'imdb', name: 'IMDb', category: 'movies' },
        { shortcut: 'rt', name: 'Rotten Tomatoes', category: 'movies' },
        { shortcut: 'metacritic', name: 'Metacritic', category: 'movies' },
        { shortcut: 'letterboxd', name: 'Letterboxd', category: 'movies' },
        { shortcut: 'tmdb', name: 'The Movie Database', category: 'movies' },
        { shortcut: 'trakt', name: 'Trakt', category: 'movies' },
        { shortcut: 'justwatch', name: 'JustWatch', category: 'movies' },
        { shortcut: 'reelgood', name: 'Reelgood', category: 'movies' },
        { shortcut: 'tvdb', name: 'TheTVDB', category: 'movies' },
        { shortcut: 'tvmaze', name: 'TVmaze', category: 'movies' },
        { shortcut: 'episodecalendar', name: 'Episode Calendar', category: 'movies' },
        { shortcut: 'netflix', name: 'Netflix', category: 'movies' },
        { shortcut: 'primevideo', name: 'Prime Video', category: 'movies' },
        { shortcut: 'hulu', name: 'Hulu', category: 'movies' },
        { shortcut: 'disneyplus', name: 'Disney+', category: 'movies' },
        { shortcut: 'hbomax', name: 'Max', category: 'movies' },
        { shortcut: 'peacock', name: 'Peacock', category: 'movies' },
        { shortcut: 'paramount', name: 'Paramount+', category: 'movies' },
        { shortcut: 'steam', name: 'Steam', category: 'games' },
        { shortcut: 'gog', name: 'GOG', category: 'games' },
        { shortcut: 'epicgames', name: 'Epic Games', category: 'games' },
        { shortcut: 'itch', name: 'itch.io', category: 'games' },
        { shortcut: 'humble', name: 'Humble Bundle', category: 'games' },
        { shortcut: 'isthereanydeal', name: 'IsThereAnyDeal', category: 'games' },
        { shortcut: 'gg', name: 'GG.deals', category: 'games' },
        { shortcut: 'pcgamingwiki', name: 'PCGamingWiki', category: 'games' },
        { shortcut: 'howlongtobeat', name: 'HowLongToBeat', category: 'games' },
        { shortcut: 'igdb', name: 'IGDB', category: 'games' },
        { shortcut: 'rawg', name: 'RAWG', category: 'games' },
        { shortcut: 'mobygames', name: 'MobyGames', category: 'games' },
        { shortcut: 'giantbomb', name: 'Giant Bomb', category: 'games' },
        { shortcut: 'gamefaqs', name: 'GameFAQs', category: 'games' },
        { shortcut: 'speedrun', name: 'Speedrun.com', category: 'games' },
        { shortcut: 'strategywiki', name: 'StrategyWiki', category: 'games' },
        { shortcut: 'steamdb', name: 'SteamDB', category: 'games' },
        { shortcut: 'protondb', name: 'ProtonDB', category: 'games' },
        { shortcut: 'nintendolife', name: 'Nintendo Life', category: 'games' },
        { shortcut: 'playstation', name: 'PlayStation Store', category: 'games' },
        { shortcut: 'xbox', name: 'Xbox Store', category: 'games' },
        { shortcut: 'goodreads', name: 'Goodreads', category: 'books' },
        { shortcut: 'storygraph', name: 'StoryGraph', category: 'books' },
        { shortcut: 'bookdepository', name: 'Book Depository', category: 'books' },
        { shortcut: 'worldcat', name: 'WorldCat', category: 'books' },
        { shortcut: 'googlebooks', name: 'Google Books', category: 'books' },
        { shortcut: 'overdrive', name: 'OverDrive', category: 'books' },
        { shortcut: 'libby', name: 'Libby', category: 'books' },
        { shortcut: 'audible', name: 'Audible', category: 'books' },
        { shortcut: 'kindle', name: 'Kindle Store', category: 'books' },
        { shortcut: 'kobo', name: 'Kobo', category: 'books' },
        { shortcut: 'barnesandnoble', name: 'Barnes & Noble', category: 'books' },
        { shortcut: 'bookshop', name: 'Bookshop.org', category: 'books' },
        { shortcut: 'isfdb', name: 'ISFDB', category: 'books' },
        { shortcut: 'fantasticfiction', name: 'Fantastic Fiction', category: 'books' },
        { shortcut: 'indeed', name: 'Indeed', category: 'jobs' },
        { shortcut: 'glassdoor', name: 'Glassdoor', category: 'jobs' },
        { shortcut: 'monster', name: 'Monster', category: 'jobs' },
        { shortcut: 'ziprecruiter', name: 'ZipRecruiter', category: 'jobs' },
        { shortcut: 'careerbuilder', name: 'CareerBuilder', category: 'jobs' },
        { shortcut: 'simplyhired', name: 'SimplyHired', category: 'jobs' },
        { shortcut: 'dice', name: 'Dice', category: 'jobs' },
        { shortcut: 'wellfound', name: 'Wellfound', category: 'jobs' },
        { shortcut: 'wwr', name: 'We Work Remotely', category: 'jobs' },
        { shortcut: 'remoteok', name: 'Remote OK', category: 'jobs' },
        { shortcut: 'flexjobs', name: 'FlexJobs', category: 'jobs' },
        { shortcut: 'ycombinator', name: 'Y Combinator Jobs', category: 'jobs' },
        { shortcut: 'levels', name: 'Levels.fyi', category: 'jobs' },
        { shortcut: 'crunchbase', name: 'Crunchbase', category: 'jobs' },
        { shortcut: 'allrecipes', name: 'Allrecipes', category: 'food' },
        { shortcut: 'foodnetwork', name: 'Food Network', category: 'food' },
        { shortcut: 'epicurious', name: 'Epicurious', category: 'food' },
        { shortcut: 'seriouseats', name: 'Serious Eats', category: 'food' },
        { shortcut: 'bonappetit', name: 'Bon Appetit', category: 'food' },
        { shortcut: 'tasty', name: 'Tasty', category: 'food' },
        { shortcut: 'yummly', name: 'Yummly', category: 'food' },
        { shortcut: 'cookpad', name: 'Cookpad', category: 'food' },
        { shortcut: 'foodcom', name: 'Food.com', category: 'food' },
        { shortcut: 'delish', name: 'Delish', category: 'food' },
        { shortcut: 'simplyrecipes', name: 'Simply Recipes', category: 'food' },
        { shortcut: 'budgetbytes', name: 'Budget Bytes', category: 'food' },
        { shortcut: 'doordash', name: 'DoorDash', category: 'food' },
        { shortcut: 'ubereats', name: 'Uber Eats', category: 'food' },
        { shortcut: 'grubhub', name: 'Grubhub', category: 'food' },
        { shortcut: 'espn', name: 'ESPN', category: 'sports' },
        { shortcut: 'cbssports', name: 'CBS Sports', category: 'sports' },
        { shortcut: 'foxsports', name: 'Fox Sports', category: 'sports' },
        { shortcut: 'bleacherreport', name: 'Bleacher Report', category: 'sports' },
        { shortcut: 'theathletic', name: 'The Athletic', category: 'sports' },
        { shortcut: 'nba', name: 'NBA', category: 'sports' },
        { shortcut: 'nfl', name: 'NFL', category: 'sports' },
        { shortcut: 'mlb', name: 'MLB', category: 'sports' },
        { shortcut: 'nhl', name: 'NHL', category: 'sports' },
        { shortcut: 'mls', name: 'MLS', category: 'sports' },
        { shortcut: 'transfermarkt', name: 'Transfermarkt', category: 'sports' },
        { shortcut: 'flashscore', name: 'Flashscore', category: 'sports' },
        { shortcut: 'sofascore', name: 'SofaScore', category: 'sports' },
        { shortcut: 'booking', name: 'Booking.com', category: 'travel' },
        { shortcut: 'airbnb', name: 'Airbnb', category: 'travel' },
        { shortcut: 'expedia', name: 'Expedia', category: 'travel' },
        { shortcut: 'kayak', name: 'Kayak', category: 'travel' },
        { shortcut: 'skyscanner', name: 'Skyscanner', category: 'travel' },
        { shortcut: 'googleflights', name: 'Google Flights', category: 'travel' },
        { shortcut: 'flightaware', name: 'FlightAware', category: 'travel' },
        { shortcut: 'rome2rio', name: 'Rome2rio', category: 'travel' },
        { shortcut: 'seat61', name: 'Seat61', category: 'travel' },
        { shortcut: 'hostelworld', name: 'Hostelworld', category: 'travel' },
        { shortcut: 'vrbo', name: 'VRBO', category: 'travel' },
        { shortcut: 'hotels', name: 'Hotels.com', category: 'travel' },
        { shortcut: 'agoda', name: 'Agoda', category: 'travel' },
        { shortcut: 'lonelyplanet', name: 'Lonely Planet', category: 'travel' },
        { shortcut: 'yfin', name: 'Yahoo Finance', category: 'finance' },
        { shortcut: 'gfinance', name: 'Google Finance', category: 'finance' },
        { shortcut: 'tradingview', name: 'TradingView', category: 'finance' },
        { shortcut: 'marketwatch', name: 'MarketWatch', category: 'finance' },
        { shortcut: 'seekingalpha', name: 'Seeking Alpha', category: 'finance' },
        { shortcut: 'investopedia', name: 'Investopedia', category: 'finance' },
        { shortcut: 'coinmarketcap', name: 'CoinMarketCap', category: 'finance' },
        { shortcut: 'coingecko', name: 'CoinGecko', category: 'finance' },
        { shortcut: 'etherscan', name: 'Etherscan', category: 'finance' },
        { shortcut: 'bscscan', name: 'BscScan', category: 'finance' },
        { shortcut: 'blockchain', name: 'Blockchain.com', category: 'finance' },
        { shortcut: 'defi', name: 'DeFi Llama', category: 'finance' },
        { shortcut: 'finviz', name: 'Finviz', category: 'finance' },
        { shortcut: 'stocktwits', name: 'Stocktwits', category: 'finance' },
        { shortcut: 'webmd', name: 'WebMD', category: 'health' },
        { shortcut: 'mayoclinic', name: 'Mayo Clinic', category: 'health' },
        { shortcut: 'healthline', name: 'Healthline', category: 'health' },
        { shortcut: 'medlineplus', name: 'MedlinePlus', category: 'health' },
        { shortcut: 'drugs', name: 'Drugs.com', category: 'health' },
        { shortcut: 'rxlist', name: 'RxList', category: 'health' },
        { shortcut: 'nih', name: 'NIH', category: 'health' },
        { shortcut: 'cdc', name: 'CDC', category: 'health' },
        { shortcut: 'who', name: 'WHO', category: 'health' },
        { shortcut: 'findlaw', name: 'FindLaw', category: 'legal' },
        { shortcut: 'justia', name: 'Justia', category: 'legal' },
        { shortcut: 'law', name: 'Law.com', category: 'legal' },
        { shortcut: 'courtlistener', name: 'CourtListener', category: 'legal' },
        { shortcut: 'caselaw', name: 'Caselaw Access', category: 'legal' },
        { shortcut: 'oyez', name: 'Oyez', category: 'legal' },
        { shortcut: 'cornell', name: 'Cornell Law', category: 'legal' },
        { shortcut: 'patents', name: 'Google Patents', category: 'legal' },
        { shortcut: 'uspto', name: 'USPTO', category: 'legal' },
        { shortcut: 'trademark', name: 'USPTO Trademark', category: 'legal' },
        { shortcut: 'coursera', name: 'Coursera', category: 'education' },
        { shortcut: 'udemy', name: 'Udemy', category: 'education' },
        { shortcut: 'edx', name: 'edX', category: 'education' },
        { shortcut: 'khanacademy', name: 'Khan Academy', category: 'education' },
        { shortcut: 'udacity', name: 'Udacity', category: 'education' },
        { shortcut: 'skillshare', name: 'Skillshare', category: 'education' },
        { shortcut: 'lilearn', name: 'LinkedIn Learning', category: 'education' },
        { shortcut: 'pluralsight', name: 'Pluralsight', category: 'education' },
        { shortcut: 'freecodecamp', name: 'freeCodeCamp', category: 'education' },
        { shortcut: 'codecademy', name: 'Codecademy', category: 'education' },
        { shortcut: 'brilliant', name: 'Brilliant', category: 'education' },
        { shortcut: 'mit', name: 'MIT OpenCourseWare', category: 'education' },
        { shortcut: 'ted', name: 'TED', category: 'education' },
        { shortcut: 'masterclass', name: 'MasterClass', category: 'education' },
        { shortcut: 'usagov', name: 'USA.gov', category: 'government' },
        { shortcut: 'govuk', name: 'GOV.UK', category: 'government' },
        { shortcut: 'data', name: 'Data.gov', category: 'government' },
        { shortcut: 'europeana', name: 'Europeana', category: 'government' },
        { shortcut: 'dpla', name: 'Digital Public Library', category: 'government' },
        { shortcut: 'loc', name: 'Library of Congress', category: 'government' },
        { shortcut: 'smithsonian', name: 'Smithsonian', category: 'government' },
        { shortcut: 'worldbank', name: 'World Bank', category: 'government' },
        { shortcut: 'un', name: 'United Nations', category: 'government' },
        { shortcut: 'census', name: 'US Census', category: 'government' },
        { shortcut: 'mal', name: 'MyAnimeList', category: 'anime' },
        { shortcut: 'anilist', name: 'AniList', category: 'anime' },
        { shortcut: 'anidb', name: 'AniDB', category: 'anime' },
        { shortcut: 'kitsu', name: 'Kitsu', category: 'anime' },
        { shortcut: 'crunchyroll', name: 'Crunchyroll', category: 'anime' },
        { shortcut: 'funimation', name: 'Funimation', category: 'anime' },
        { shortcut: 'mangadex', name: 'MangaDex', category: 'anime' },
        { shortcut: 'mangaupdates', name: 'Manga Updates', category: 'anime' },
        { shortcut: 'producthunt', name: 'Product Hunt', category: 'misc' },
        { shortcut: 'alternativeto', name: 'AlternativeTo', category: 'misc' },
        { shortcut: 'similarweb', name: 'SimilarWeb', category: 'misc' },
        { shortcut: 'pastebin', name: 'Pastebin', category: 'misc' },
        { shortcut: 'wolframcom', name: 'Wolfram', category: 'misc' },
        { shortcut: 'symbolab', name: 'Symbolab', category: 'misc' },
        { shortcut: 'desmos', name: 'Desmos', category: 'misc' },
        { shortcut: 'geogebra', name: 'GeoGebra', category: 'misc' },
        { shortcut: 'timeanddate', name: 'Time and Date', category: 'misc' },
        { shortcut: 'xe', name: 'XE Currency', category: 'misc' },
        { shortcut: 'oeis', name: 'OEIS', category: 'misc' },
    ];

    function initSearchAutocomplete() {
        var searchInputs = document.querySelectorAll('.search-input, .header-search-input, .nav-search-input');

        searchInputs.forEach(function(input) {
            setupAutocomplete(input);
        });
    }

    function setupAutocomplete(input) {
        var debounceTimer = null;
        var selectedIndex = -1;
        var suggestions = [];
        var dropdown = null;
        var abortController = null;

        // Create dropdown element
        function createDropdown() {
            if (dropdown) return dropdown;

            dropdown = document.createElement('div');
            dropdown.className = 'autocomplete-dropdown';
            dropdown.setAttribute('role', 'listbox');
            dropdown.setAttribute('aria-label', t('accessibility.search_suggestions', 'Search suggestions'));
            dropdown.style.display = 'none';

            // Position dropdown relative to input's parent container
            var container = input.closest('.search-box') || input.parentElement;
            if (container) {
                container.style.position = 'relative';
                container.appendChild(dropdown);
            } else {
                input.parentElement.style.position = 'relative';
                input.parentElement.appendChild(dropdown);
            }

            return dropdown;
        }

        // Show dropdown with suggestions
        function showDropdown(items) {
            if (!dropdown) createDropdown();
            suggestions = items;
            selectedIndex = -1;

            if (items.length === 0) {
                hideDropdown();
                return;
            }

            dropdown.innerHTML = items.map(function(item, index) {
                if (item.type === 'bang') {
                    return '<div class="autocomplete-item autocomplete-bang" data-index="' + index + '" role="option" aria-selected="false">' +
                        '<span class="autocomplete-bang-shortcut">!' + escapeHtml(item.shortcut) + '</span>' +
                        '<span class="autocomplete-bang-name">' + escapeHtml(item.name) + '</span>' +
                        '<span class="autocomplete-bang-category">' + escapeHtml(item.category) + '</span>' +
                    '</div>';
                } else {
                    return '<div class="autocomplete-item" data-index="' + index + '" role="option" aria-selected="false">' +
                        '<svg class="autocomplete-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"></circle><path d="m21 21-4.35-4.35"></path></svg>' +
                        '<span class="autocomplete-text">' + escapeHtml(item.text) + '</span>' +
                    '</div>';
                }
            }).join('');

            dropdown.style.display = 'block';

            // Add click handlers
            dropdown.querySelectorAll('.autocomplete-item').forEach(function(item) {
                item.addEventListener('mousedown', function(e) {
                    e.preventDefault();
                    selectSuggestion(parseInt(this.dataset.index, 10));
                });
                item.addEventListener('mouseover', function() {
                    updateSelection(parseInt(this.dataset.index, 10));
                });
            });

            // Announce to screen readers
            announce(items.length + ' suggestions available');
        }

        // Hide dropdown
        function hideDropdown() {
            if (dropdown) {
                dropdown.style.display = 'none';
                dropdown.innerHTML = '';
            }
            suggestions = [];
            selectedIndex = -1;
        }

        // Update visual selection
        function updateSelection(index) {
            if (!dropdown) return;

            var items = dropdown.querySelectorAll('.autocomplete-item');
            items.forEach(function(item, i) {
                if (i === index) {
                    item.classList.add('selected');
                    item.setAttribute('aria-selected', 'true');
                } else {
                    item.classList.remove('selected');
                    item.setAttribute('aria-selected', 'false');
                }
            });
            selectedIndex = index;
        }

        // Select a suggestion
        function selectSuggestion(index) {
            if (index < 0 || index >= suggestions.length) return;

            var item = suggestions[index];
            if (item.type === 'bang') {
                // For bangs, keep the bang syntax and add a space
                input.value = '!' + item.shortcut + ' ';
            } else {
                input.value = item.text;
            }

            hideDropdown();
            input.focus();

            // If not a bang, submit the form
            if (item.type !== 'bang') {
                var form = input.closest('form');
                if (form) {
                    form.submit();
                }
            }
        }

        // Fetch suggestions from API
        function fetchSuggestions(query) {
            // Cancel previous request
            if (abortController) {
                abortController.abort();
            }

            // Check for bang query
            if (query.startsWith('!')) {
                var bangQuery = query.slice(1).toLowerCase();
                var bangSuggestions = filterBangs(bangQuery);
                showDropdown(bangSuggestions);
                return;
            }

            // Skip very short queries
            if (query.length < 2) {
                hideDropdown();
                return;
            }

            // Create new abort controller
            abortController = new AbortController();

            // Fetch from autocomplete API - per AI.md PART 14: use origin prefix
            fetch(window.location.origin + '/autocomplete?q=' + encodeURIComponent(query), {
                signal: abortController.signal
            })
            .then(function(response) {
                if (!response.ok) throw new Error('Network error');
                return response.json();
            })
            .then(function(data) {
                // Handle API response format: {"ok": true, "data": [...]}
                var results = [];
                if (data && data.ok && Array.isArray(data.data)) {
                    results = data.data.map(function(text) {
                        return { type: 'suggestion', text: text };
                    });
                } else if (Array.isArray(data)) {
                    // Fallback for simple array response
                    results = data.map(function(text) {
                        return { type: 'suggestion', text: text };
                    });
                }
                showDropdown(results);
            })
            .catch(function(err) {
                if (err.name !== 'AbortError') {
                    hideDropdown();
                }
            });
        }

        // Filter bangs based on query
        function filterBangs(query) {
            var normalizedQuery = (query || '').toLowerCase();
            var builtins = BUILTIN_BANGS.filter(function(b) {
                return !normalizedQuery || b.shortcut.toLowerCase().startsWith(normalizedQuery) ||
                       b.name.toLowerCase().includes(normalizedQuery);
            });
            var custom = [];
            try {
                custom = JSON.parse(localStorage.getItem(BANGS_KEY) || '[]');
            } catch (e) {
                custom = [];
            }
            if (!Array.isArray(custom)) custom = [];
            var customMatches = custom.filter(function(b) {
                if (!b || b.enabled === false || typeof b.shortcut !== 'string' || typeof b.name !== 'string') return false;
                var shortcut = b.shortcut.toLowerCase();
                if (!/^[a-z0-9_-]+$/.test(shortcut) || BUILTIN_BANGS.some(function(builtin) { return builtin.shortcut === shortcut; })) return false;
                return !normalizedQuery || shortcut.startsWith(normalizedQuery) || b.name.toLowerCase().includes(normalizedQuery);
            });
            return customMatches.concat(builtins).slice(0, 10).map(function(b) {
                return { type: 'bang', shortcut: b.shortcut, name: b.name, category: b.category };
            });
        }

        function customBangTarget(query) {
            var prefix = query.match(/^!([a-z0-9_-]+)\s+(.+)$/i);
            var suffix = query.match(/^(.+?)\s+!([a-z0-9_-]+)$/i);
            var shortcut = prefix ? prefix[1] : suffix ? suffix[2] : '';
            var searchTerms = prefix ? prefix[2] : suffix ? suffix[1] : '';
            if (!shortcut || !searchTerms.trim()) return '';
            shortcut = shortcut.toLowerCase();
            if (BUILTIN_BANGS.some(function(b) { return b.shortcut === shortcut; })) return '';

            var bangs;
            try {
                bangs = JSON.parse(localStorage.getItem(BANGS_KEY) || '[]');
            } catch (e) {
                return '';
            }
            if (!Array.isArray(bangs)) return '';
            var bang = bangs.find(function(item) {
                return item && item.enabled !== false && typeof item.shortcut === 'string' && item.shortcut.toLowerCase() === shortcut;
            });
            if (!bang || typeof bang.url !== 'string' || !bang.url.includes('{query}')) return '';

            try {
                var target = new URL(bang.url.split('{query}').join(encodeURIComponent(searchTerms.trim())));
                if ((target.protocol !== 'http:' && target.protocol !== 'https:') || !target.hostname || target.username || target.password) return '';
                return target.href;
            } catch (e) {
                return '';
            }
        }

        var searchForm = input.closest('form');
        if (searchForm) {
            searchForm.addEventListener('submit', function(e) {
                var target = customBangTarget(input.value.trim());
                if (!target) return;
                e.preventDefault();
                window.location.assign(target);
            });
        }

        // Input event handler
        input.addEventListener('input', function() {
            clearTimeout(debounceTimer);
            var query = this.value.trim();

            if (!query) {
                hideDropdown();
                return;
            }

            debounceTimer = setTimeout(function() {
                fetchSuggestions(query);
            }, 300);
        });

        // Keyboard navigation
        input.addEventListener('keydown', function(e) {
            // Handle autocomplete navigation
            if (dropdown && dropdown.style.display !== 'none') {
                if (e.key === 'ArrowDown') {
                    e.preventDefault();
                    var newIndexDown = selectedIndex < suggestions.length - 1 ? selectedIndex + 1 : 0;
                    updateSelection(newIndexDown);
                    return;
                }

                if (e.key === 'ArrowUp') {
                    e.preventDefault();
                    var newIndexUp = selectedIndex > 0 ? selectedIndex - 1 : suggestions.length - 1;
                    updateSelection(newIndexUp);
                    return;
                }

                if (e.key === 'Enter' && selectedIndex >= 0) {
                    e.preventDefault();
                    selectSuggestion(selectedIndex);
                    return;
                }

                if (e.key === 'Escape') {
                    e.preventDefault();
                    hideDropdown();
                    return;
                }

                if (e.key === 'Tab') {
                    hideDropdown();
                    return;
                }
            }

            // Original escape handler for clearing input when dropdown is not visible
            if (e.key === 'Escape' && (!dropdown || dropdown.style.display === 'none')) {
                this.value = '';
                this.focus();
            }
        });

        // Hide dropdown on blur (with delay for click handling)
        input.addEventListener('blur', function() {
            setTimeout(function() {
                hideDropdown();
            }, 200);
        });

        // Show dropdown on focus if there's a value
        input.addEventListener('focus', function() {
            var query = this.value.trim();
            if (query && query.length >= 2) {
                fetchSuggestions(query);
            }
        });
    }

    // ========================================================================
    // KEYBOARD SHORTCUTS (Full Navigation per IDEA.md)
    // ========================================================================
    var currentResultIndex = -1;

    function initKeyboardShortcuts() {
        document.addEventListener('keydown', function(e) {
            if (!getActiveSearchPreferences().keyboard_shortcuts) return;
            // Skip if modifier keys (except shift for O)
            if (e.ctrlKey || e.altKey || e.metaKey) return;

            // Focus search with / and open preferences with s.
            if (e.key === 's' && !isInputFocused()) {
                e.preventDefault();
                window.location.assign('/server/preferences');
                return;
            }

            if (e.key === '/' && !isInputFocused()) {
                e.preventDefault();
                var searchInput = document.querySelector('.search-input, .header-search-input, input[name="q"]');
                if (searchInput) {
                    searchInput.focus();
                    searchInput.select();
                }
                return;
            }

            // Toggle theme with t
            if (e.key === 't' && !isInputFocused()) {
                toggleTheme();
                return;
            }

            // Escape to close nav/unfocus
            if (e.key === 'Escape') {
                closeNav();
                if (isInputFocused()) {
                    var activeElement = document.activeElement;
                    if (activeElement.matches('.search-input, .header-search-input, input[name="q"]')) {
                        activeElement.value = '';
                    }
                    document.activeElement.blur();
                }
                return;
            }

            // Show keyboard help with ?
            if (e.key === '?' && !isInputFocused()) {
                e.preventDefault();
                showKeyboardHelp();
                return;
            }

            // Result navigation with j/k
            if (e.key === 'j' && !isInputFocused()) {
                e.preventDefault();
                navigateResults(1);
                return;
            }

            if (e.key === 'k' && !isInputFocused()) {
                e.preventDefault();
                navigateResults(-1);
                return;
            }

            // Open result with Enter, o, or O (new tab)
            if ((e.key === 'Enter' || e.key === 'o' || e.key === 'O') && !isInputFocused()) {
                var results = getSearchResults();
                if (currentResultIndex >= 0 && currentResultIndex < results.length) {
                    e.preventDefault();
                    var link = results[currentResultIndex].querySelector('a');
                    if (link) {
                        if (e.key === 'O' || e.shiftKey) {
                            window.open(link.href, '_blank', 'noopener,noreferrer');
                        } else {
                            link.click();
                        }
                    }
                }
                return;
            }

            // Pagination with h/l or left/right arrows
            if ((e.key === 'h' || e.key === 'ArrowLeft') && !isInputFocused()) {
                var prevLink = document.querySelector('.pagination-prev, [rel="prev"], .page-prev');
                if (prevLink && !prevLink.classList.contains('disabled')) {
                    e.preventDefault();
                    prevLink.click();
                }
                return;
            }

            if ((e.key === 'l' || e.key === 'ArrowRight') && !isInputFocused()) {
                var nextLink = document.querySelector('.pagination-next, [rel="next"], .page-next');
                if (nextLink && !nextLink.classList.contains('disabled')) {
                    e.preventDefault();
                    nextLink.click();
                }
                return;
            }

            // Quick jump with 1-9
            if (/^[1-9]$/.test(e.key) && !isInputFocused()) {
                var index = parseInt(e.key, 10) - 1;
                var results = getSearchResults();
                if (index < results.length) {
                    e.preventDefault();
                    selectResult(index);
                    var link = results[index].querySelector('a');
                    if (link) {
                        if (e.shiftKey) {
                            window.open(link.href, '_blank', 'noopener,noreferrer');
                        } else {
                            link.click();
                        }
                    }
                }
                return;
            }

            // Go to first result with g then g
            if (e.key === 'g' && !isInputFocused()) {
                // Set up double-g detection
                if (window._lastKeyG && Date.now() - window._lastKeyG < 500) {
                    e.preventDefault();
                    navigateToResult(0);
                    window._lastKeyG = null;
                } else {
                    window._lastKeyG = Date.now();
                }
                return;
            }

            // Go to last result with G (shift+g)
            if (e.key === 'G' && !isInputFocused()) {
                e.preventDefault();
                var results = getSearchResults();
                if (results.length > 0) {
                    navigateToResult(results.length - 1);
                }
                return;
            }
        });
    }

    function isInputFocused() {
        var activeElement = document.activeElement;
        return activeElement && (
            activeElement.tagName === 'INPUT' ||
            activeElement.tagName === 'TEXTAREA' ||
            activeElement.isContentEditable
        );
    }

    function getSearchResults() {
        return document.querySelectorAll('.search-result, .result-item, .result');
    }

    function navigateResults(direction) {
        var results = getSearchResults();
        if (results.length === 0) return;

        var newIndex = currentResultIndex + direction;
        if (newIndex < 0) newIndex = 0;
        if (newIndex >= results.length) newIndex = results.length - 1;

        selectResult(newIndex);
    }

    function navigateToResult(index) {
        var results = getSearchResults();
        if (index >= 0 && index < results.length) {
            selectResult(index);
        }
    }

    function selectResult(index) {
        var results = getSearchResults();

        // Remove highlight from previous
        if (currentResultIndex >= 0 && currentResultIndex < results.length) {
            results[currentResultIndex].classList.remove('keyboard-selected');
        }

        // Highlight new
        currentResultIndex = index;
        if (index >= 0 && index < results.length) {
            results[index].classList.add('keyboard-selected');
            results[index].scrollIntoView({ behavior: 'smooth', block: 'center' });
            // Announce for screen readers
            var title = results[index].querySelector('h3, .result-title, a')?.textContent || (t('accessibility.result_fallback', 'Result') + ' ' + (index + 1));
            announce(t('accessibility.selected_prefix', 'Selected') + ': ' + title);
        }
    }

    function showKeyboardHelp() {
        var existingHelp = document.getElementById('keyboard-help-modal');
        if (existingHelp) {
            existingHelp.remove();
            return;
        }

        var modal = document.createElement('dialog');
        modal.id = 'keyboard-help-modal';
        modal.setAttribute('role', 'dialog');
        modal.setAttribute('aria-labelledby', 'keyboard-help-title');
        var keyboardShortcutsTitle = escapeHtml(t('accessibility.keyboard_shortcuts_title', 'Keyboard Shortcuts'));
        var keyboardNavigationHeading = escapeHtml(t('accessibility.keyboard_navigation_heading', 'Navigation'));
        var keyboardNavigationNextPrevious = escapeHtml(t('accessibility.keyboard_navigation_next_previous_result', 'Next / Previous result'));
        var keyboardNavigationPrevNextPage = escapeHtml(t('accessibility.keyboard_navigation_previous_next_page', 'Previous / Next page'));
        var keyboardNavigationFirstResult = escapeHtml(t('accessibility.keyboard_navigation_first_result', 'Go to first result'));
        var keyboardNavigationLastResult = escapeHtml(t('accessibility.keyboard_navigation_last_result', 'Go to last result'));
        var keyboardNavigationJumpResult = escapeHtml(t('accessibility.keyboard_navigation_jump_result', 'Jump to result N'));
        var keyboardActionsHeading = escapeHtml(t('accessibility.keyboard_actions_heading', 'Actions'));
        var keyboardActionFocusSearch = escapeHtml(t('accessibility.keyboard_action_focus_search', 'Focus search box'));
        var keyboardActionOpenSettings = escapeHtml(t('nav.settings', 'Settings'));
        var keyboardActionOpenSelected = escapeHtml(t('accessibility.keyboard_action_open_selected', 'Open selected result'));
        var keyboardActionOpenNewTab = escapeHtml(t('accessibility.keyboard_action_open_new_tab', 'Open in new tab'));
        var keyboardActionToggleTheme = escapeHtml(t('accessibility.keyboard_action_toggle_theme', 'Toggle theme'));
        var keyboardActionClearClose = escapeHtml(t('accessibility.keyboard_action_clear_close', 'Clear / Close'));
        var keyboardHelpHeading = escapeHtml(t('nav.help', 'Help'));
        var keyboardHelpShow = escapeHtml(t('accessibility.keyboard_help_show', 'Show this help'));
        var keyboardClose = escapeHtml(t('common.close', 'Close'));
        modal.innerHTML =
            '<header><h2 id="keyboard-help-title">' + keyboardShortcutsTitle + '</h2></header>' +
            '<main class="keyboard-help-content">' +
                '<div class="shortcut-group">' +
                    '<h3>' + keyboardNavigationHeading + '</h3>' +
                    '<div class="shortcut"><kbd>j</kbd> / <kbd>k</kbd> <span>' + keyboardNavigationNextPrevious + '</span></div>' +
                    '<div class="shortcut"><kbd>h</kbd> / <kbd>l</kbd> <span>' + keyboardNavigationPrevNextPage + '</span></div>' +
                    '<div class="shortcut"><kbd>g</kbd><kbd>g</kbd> <span>' + keyboardNavigationFirstResult + '</span></div>' +
                    '<div class="shortcut"><kbd>G</kbd> <span>' + keyboardNavigationLastResult + '</span></div>' +
                    '<div class="shortcut"><kbd>1</kbd>-<kbd>9</kbd> <span>' + keyboardNavigationJumpResult + '</span></div>' +
                '</div>' +
                '<div class="shortcut-group">' +
                    '<h3>' + keyboardActionsHeading + '</h3>' +
                    '<div class="shortcut"><kbd>/</kbd> <span>' + keyboardActionFocusSearch + '</span></div>' +
                    '<div class="shortcut"><kbd>s</kbd> <span>' + keyboardActionOpenSettings + '</span></div>' +
                    '<div class="shortcut"><kbd>Enter</kbd> / <kbd>o</kbd> <span>' + keyboardActionOpenSelected + '</span></div>' +
                    '<div class="shortcut"><kbd>O</kbd> <span>' + keyboardActionOpenNewTab + '</span></div>' +
                    '<div class="shortcut"><kbd>t</kbd> <span>' + keyboardActionToggleTheme + '</span></div>' +
                    '<div class="shortcut"><kbd>Esc</kbd> <span>' + keyboardActionClearClose + '</span></div>' +
                '</div>' +
                '<div class="shortcut-group">' +
                    '<h3>' + keyboardHelpHeading + '</h3>' +
                    '<div class="shortcut"><kbd>?</kbd> <span>' + keyboardHelpShow + '</span></div>' +
                '</div>' +
            '</main>' +
            '<footer>' +
                '<button type="button" class="btn btn-primary" data-action="close-help">' + keyboardClose + '</button>' +
            '</footer>';

        document.body.appendChild(modal);

        modal.querySelector('[data-action="close-help"]').addEventListener('click', function() {
            modal.close();
            modal.remove();
        });

        modal.addEventListener('close', function() {
            modal.remove();
        });

        modal.addEventListener('keydown', function(e) {
            if (e.key === 'Escape' || e.key === '?') {
                e.preventDefault();
                modal.close();
                modal.remove();
            }
        });

        modal.showModal();
        announce(t('accessibility.keyboard_help_opened', 'Keyboard shortcuts help opened. Press Escape or ? to close.'));
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
            const previewLink = closestFrom(e.target, '[data-image-preview]');
            if (!previewLink) return;
            const imageResult = closestFrom(previewLink, '.image-result');
            const dialog = document.getElementById('image-preview-dialog');
            if (!imageResult || !dialog || typeof dialog.showModal !== 'function') return;

            const previewURL = imageResult.dataset.previewUrl || '';
            if (!previewURL) return;
            e.preventDefault();

            const title = imageResult.dataset.imageTitle || t('search.image_preview', 'Preview image');
            const image = dialog.querySelector('[data-image-preview-image]');
            const titleElement = dialog.querySelector('[data-image-preview-title]');
            const dimensions = dialog.querySelector('[data-image-preview-dimensions]');
            const source = dialog.querySelector('[data-image-preview-source]');
            const download = dialog.querySelector('[data-image-preview-download]');
            titleElement.textContent = title;
            image.alt = title;
            dimensions.textContent = '';

            const width = Number(imageResult.dataset.imageWidth);
            const height = Number(imageResult.dataset.imageHeight);
            const hasDimensions = width > 0 && height > 0;
            if (hasDimensions) {
                dimensions.textContent = t('search.image_dimensions', 'Image dimensions') + ': ' + width + ' × ' + height + ' px';
            }
            image.onload = function() {
                if (!hasDimensions && image.naturalWidth > 0 && image.naturalHeight > 0) {
                    dimensions.textContent = t('search.image_dimensions', 'Image dimensions') + ': ' + image.naturalWidth + ' × ' + image.naturalHeight + ' px';
                }
            };
            image.src = previewURL;

            source.href = imageResult.dataset.fullUrl || previewLink.href;
            download.href = previewURL;
            const filename = title.replace(/[\\/:*?"<>|]+/g, '-').trim() || 'image';
            const format = (imageResult.dataset.imageFormat || '').replace(/[^a-z0-9]/gi, '');
            download.download = format ? filename + '.' + format : filename;
            dialog.showModal();
        });
    }

    // ========================================================================
    // INFINITE SCROLL
    // ========================================================================
    function initInfiniteScroll() {
        // Search results page infinite scroll
        // Per AI.md PART 16: NO inline JS - all in external files
        var container = document.querySelector('.search-results-page');
        if (!container) return;
        if (!getActiveSearchPreferences().infinite_scroll) return;

        var resultsContainer = document.getElementById('results-container');
        var scrollTrigger = document.getElementById('scroll-trigger');
        var loadingIndicator = document.getElementById('loading-indicator');
        var endIndicator = document.getElementById('end-indicator');
        var pagination = document.querySelector('.pagination');

        if (!resultsContainer || !scrollTrigger) return;
        if (pagination) pagination.classList.add('hidden');

        var query = container.dataset.query;
        var category = container.dataset.category;
        var currentPage = parseInt(container.dataset.page) || 1;
        var perPage = parseInt(container.dataset.perPage) || 20;
        var safeSearch = container.dataset.safeSearch || '1';
        var language = container.dataset.lang || '';
        var region = container.dataset.region || '';
        var timeRange = container.dataset.timeRange || '';
        var timeFrom = container.dataset.timeFrom || '';
        var timeTo = container.dataset.timeTo || '';
        var prefsParam = container.dataset.prefs || '';
        var isLoading = false;
        var hasMore = true;

        // Format duration from seconds to MM:SS or HH:MM:SS
        function formatDuration(seconds) {
            if (!seconds || seconds <= 0) return '';
            var h = Math.floor(seconds / 3600);
            var m = Math.floor((seconds % 3600) / 60);
            var s = seconds % 60;
            if (h > 0) {
                return h + ':' + m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0');
            }
            return m + ':' + s.toString().padStart(2, '0');
        }

        // Format view count (e.g., 1.2M, 500K)
        function formatViewCount(count) {
            if (!count || count <= 0) return '';
            if (count >= 1000000000) return (count / 1000000000).toFixed(1).replace(/\.0$/, '') + 'B';
            if (count >= 1000000) return (count / 1000000).toFixed(1).replace(/\.0$/, '') + 'M';
            if (count >= 1000) return (count / 1000).toFixed(1).replace(/\.0$/, '') + 'K';
            return count.toString();
        }

        // Escape HTML for safe insertion
        function escapeHtmlLocal(text) {
            if (!text) return '';
            var div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Create result card HTML based on category
        function createResultCard(result) {
            var firstLetter = result.title ? result.title.charAt(0).toUpperCase() : '?';

            if (category === 'images') {
                return '<div class="image-result" data-full-url="' + escapeHtmlLocal(result.url) + '">' +
                    '<a href="' + escapeHtmlLocal(result.url) + '" target="_blank" rel="noopener noreferrer">' +
                    (result.thumbnail
                        ? '<img src="' + escapeHtmlLocal(result.thumbnail) + '" alt="' + escapeHtmlLocal(result.title) + '" loading="lazy">'
                        : '<div class="image-placeholder"><span>\uD83D\uDDBC\uFE0F</span></div>'
                    ) +
                    '</a>' +
                    '<div class="image-result-info">' +
                    '<a href="' + escapeHtmlLocal(result.url) + '" class="image-title" target="_blank" rel="noopener noreferrer">' + escapeHtmlLocal(result.title) + '</a>' +
                    '<div class="image-source">' + escapeHtmlLocal(result.engine) + '</div>' +
                    '</div></div>';
            }

            if (category === 'videos') {
                var durationStr = result.duration ? formatDuration(result.duration) : '';
                var viewCountStr = result.view_count ? formatViewCount(result.view_count) + ' views' : '';
                return '<div class="video-result" data-video-embed="' + escapeHtmlLocal(result.embed_url || '') + '" data-video-preview-url="' + escapeHtmlLocal(result.preview_url || '') + '" data-video-duration="' + escapeHtmlLocal(result.duration || 0) + '">' +
                    '<a href="' + escapeHtmlLocal(result.url) + '" class="video-thumbnail-link" target="_blank" rel="noopener noreferrer">' +
                    '<div class="video-thumbnail-container">' +
                    (result.thumbnail
                        ? '<img src="' + escapeHtmlLocal(result.thumbnail) + '" alt="' + escapeHtmlLocal(result.title) + '" loading="lazy" class="video-thumbnail">'
                        : '<div class="video-placeholder"><span>\uD83C\uDFA5</span></div>'
                    ) +
                    '<div class="video-play-overlay"><svg class="play-icon" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg></div>' +
                    (durationStr ? '<span class="video-duration">' + escapeHtmlLocal(durationStr) + '</span>' : '') +
                    '</div></a>' +
                    '<div class="video-info">' +
                    '<h3 class="video-title"><a href="' + escapeHtmlLocal(result.url) + '" target="_blank" rel="noopener noreferrer">' + escapeHtmlLocal(result.title) + '</a></h3>' +
                    '<div class="video-meta">' +
                    '<span class="video-engine">' + escapeHtmlLocal(result.engine) + '</span>' +
                    (viewCountStr ? '<span class="video-views">' + escapeHtmlLocal(viewCountStr) + '</span>' : '') +
                    (result.author ? '<span class="video-author">' + escapeHtmlLocal(result.author) + '</span>' : '') +
                    '</div>' +
                    (result.description ? '<p class="video-description">' + escapeHtmlLocal(result.description) + '</p>' : '') +
                    '</div></div>';
            }

            // Standard results
            return '<article class="result-item">' +
                '<div class="result-favicon">' +
                '<img src="/api/v1/favicon?url=' + encodeURIComponent(result.url) + '" alt="" loading="lazy" data-favicon-fallback>' +
                '<span class="favicon-placeholder hidden">' + escapeHtmlLocal(firstLetter) + '</span>' +
                '</div>' +
                '<div class="result-body">' +
                '<h3 class="result-title"><a href="' + escapeHtmlLocal(result.url) + '" target="_blank" rel="noopener noreferrer">' + escapeHtmlLocal(result.title) + '</a></h3>' +
                '<div class="result-url"><span class="result-url-text">' + escapeHtmlLocal(result.url) + '</span></div>' +
                '<p class="result-description">' + escapeHtmlLocal(result.description || '') + '</p>' +
                '<div class="result-meta">' +
                '<span class="result-engine">' + escapeHtmlLocal(result.engine) + '</span>' +
                (result.date ? '<span class="result-date">' + escapeHtmlLocal(result.date) + '</span>' : '') +
                '</div></div></article>';
        }

        function loadMoreResults() {
            if (isLoading || !hasMore) return;

            isLoading = true;
            if (loadingIndicator) loadingIndicator.classList.remove('hidden');

            var nextPage = currentPage + 1;
            var apiURL = '/api/v1/search?q=' + encodeURIComponent(query) +
                '&category=' + encodeURIComponent(category) +
                '&page=' + nextPage +
                '&limit=' + perPage +
                '&safe_search=' + encodeURIComponent(safeSearch);
            if (language) apiURL += '&lang=' + encodeURIComponent(language);
            if (region) apiURL += '&region=' + encodeURIComponent(region);
            if (timeRange) apiURL += '&time_range=' + encodeURIComponent(timeRange);
            if (timeFrom) apiURL += '&time_from=' + encodeURIComponent(timeFrom);
            if (timeTo) apiURL += '&time_to=' + encodeURIComponent(timeTo);
            if (prefsParam) {
                apiURL += '&prefs=' + encodeURIComponent(prefsParam);
            }
            fetch(apiURL)
                .then(function(response) { return response.json(); })
                .then(function(data) {
                    if (data.ok && data.data && data.data.results && data.data.results.length > 0) {
                        data.data.results.forEach(function(result) {
                            resultsContainer.insertAdjacentHTML('beforeend', createResultCard(result));
                        });
                        currentPage = data.data.pagination.page;
                        hasMore = currentPage < data.data.pagination.pages;
                    } else {
                        hasMore = false;
                    }

                    if (!hasMore) {
                        if (endIndicator) endIndicator.classList.remove('hidden');
                        scrollTrigger.classList.add('hidden');
                    }
                })
                .catch(function(error) {
                    console.error('Failed to load more results:', error);
                })
                .finally(function() {
                    isLoading = false;
                    if (loadingIndicator) loadingIndicator.classList.add('hidden');
                });
        }

        // Intersection Observer for infinite scroll
        if ('IntersectionObserver' in window) {
            var observer = new IntersectionObserver(function(entries) {
                entries.forEach(function(entry) {
                    if (entry.isIntersecting && hasMore) {
                        loadMoreResults();
                    }
                });
            }, { rootMargin: '400px' });

            observer.observe(scrollTrigger);
        }
    }

    // ========================================================================
    // TOAST NOTIFICATIONS (per AI.md PART 16 Toast Notifications)
    // Max 5 visible at once (older toasts queue); auto-dismiss defaults:
    // success/info 3s, warning 5s, error 0 (manual dismiss only); pause on
    // hover; Escape dismisses the topmost toast; click anywhere to dismiss.
    // ========================================================================
    var MAX_VISIBLE_TOASTS = 5;
    var toastQueue = [];

    function getToastContainer() {
        let container = document.getElementById('toast-container');
        if (!container) {
            container = document.createElement('div');
            container.id = 'toast-container';
            container.setAttribute('aria-label', t('accessibility.notifications', 'Notifications'));
            document.body.appendChild(container);
        }
        return container;
    }

    function visibleToastCount(container) {
        return container.querySelectorAll('.toast:not(.toast-hiding)').length;
    }

    function drainToastQueue(container) {
        while (toastQueue.length > 0 && visibleToastCount(container) < MAX_VISIBLE_TOASTS) {
            renderToast(toastQueue.shift());
        }
    }

    function startToastTimer(toast) {
        if (toast._toastDuration <= 0) return;
        toast._toastRemaining = toast._toastRemaining == null ? toast._toastDuration : toast._toastRemaining;
        toast._toastStartedAt = Date.now();
        toast._toastTimer = setTimeout(function() {
            dismissToast(toast);
        }, toast._toastRemaining);
    }

    function pauseToastTimer(toast) {
        if (toast._toastDuration <= 0 || !toast._toastTimer) return;
        clearTimeout(toast._toastTimer);
        toast._toastTimer = null;
        toast._toastRemaining = Math.max(0, toast._toastRemaining - (Date.now() - toast._toastStartedAt));
        var progress = toast.querySelector('.toast-progress');
        if (progress) progress.style.animationPlayState = 'paused';
    }

    function resumeToastTimer(toast) {
        if (toast._toastDuration <= 0 || toast._toastTimer) return;
        startToastTimer(toast);
        var progress = toast.querySelector('.toast-progress');
        if (progress) progress.style.animationPlayState = 'running';
    }

    function renderToast(pending) {
        const container = getToastContainer();

        const toast = document.createElement('div');
        toast.className = 'toast toast-' + pending.type;
        toast.setAttribute('role', pending.type === 'error' ? 'alert' : 'status');
        toast.setAttribute('aria-live', pending.type === 'error' ? 'assertive' : 'polite');
        toast._toastDuration = pending.duration;
        toast._toastRemaining = pending.duration;

        const icons = {
            success: '\u2713',
            error: '\u2717',
            warning: '\u26A0',
            info: '\u2139'
        };

        toast.innerHTML = '<span class="toast-icon">' + (icons[pending.type] || icons.info) + '</span>' +
                         '<span class="toast-message">' + escapeHtml(pending.message) + '</span>' +
                         '<button class="toast-close" data-action="close-toast" aria-label="' + escapeHtml(t('common.close', 'Close')) + '">&times;</button>' +
                         (pending.duration > 0 ? '<div class="toast-progress" style="animation-duration:' + pending.duration + 'ms"></div>' : '');

        container.insertBefore(toast, container.firstChild);

        toast.addEventListener('click', function(e) {
            if (e.target.closest('.toast-close')) return;
            dismissToast(toast);
        });
        toast.addEventListener('mouseenter', function() { pauseToastTimer(toast); });
        toast.addEventListener('mouseleave', function() { resumeToastTimer(toast); });

        startToastTimer(toast);

        requestAnimationFrame(function() {
            toast.classList.add('toast-visible');
        });

        pending.resolveEl(toast);
    }

    // Show a toast. Returns the toast element (usable with dismissToast()).
    // duration: ms before auto-dismiss; 0 = manual dismiss only. Defaults per
    // type when omitted: success/info 3000, warning 5000, error 0.
    function showToast(message, type, duration) {
        type = type || 'info';
        const defaults = { success: 3000, error: 0, warning: 5000, info: 3000 };
        if (duration === undefined) duration = (type in defaults) ? defaults[type] : 3000;

        var toastEl;
        var pending = {
            message: message,
            type: type,
            duration: duration,
            resolveEl: function(el) { toastEl = el; }
        };

        const container = getToastContainer();
        if (visibleToastCount(container) < MAX_VISIBLE_TOASTS) {
            renderToast(pending);
        } else {
            toastQueue.push(pending);
        }

        return toastEl;
    }

    function dismissToast(toast) {
        if (!toast || toast.classList.contains('toast-hiding')) return;
        if (toast._toastTimer) clearTimeout(toast._toastTimer);
        toast.classList.remove('toast-visible');
        toast.classList.add('toast-hiding');
        setTimeout(function() {
            if (toast.parentNode) {
                toast.parentNode.removeChild(toast);
            }
            drainToastQueue(getToastContainer());
        }, 300);
    }

    function dismissAllToasts() {
        toastQueue = [];
        document.querySelectorAll('#toast-container .toast').forEach(function(toast) {
            dismissToast(toast);
        });
    }

    // Escape dismisses the topmost (most recently shown) toast
    document.addEventListener('keydown', function(e) {
        if (e.key !== 'Escape') return;
        const container = document.getElementById('toast-container');
        if (!container) return;
        const topmost = container.querySelector('.toast:not(.toast-hiding)');
        if (topmost) dismissToast(topmost);
    });

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
                    '<h2 id="confirm-dialog-title">' + escapeHtml(t('common.confirm', 'Confirm')) + '</h2>' +
                '</header>' +
                '<main id="confirm-dialog-message"></main>' +
                '<footer>' +
                    '<button type="button" class="btn btn-secondary" data-action="cancel">' + escapeHtml(t('common.cancel', 'Cancel')) + '</button>' +
                    '<button type="button" class="btn btn-danger" data-action="confirm">' + escapeHtml(t('common.confirm', 'Confirm')) + '</button>' +
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

            titleEl.textContent = options.title || t('common.confirm_action', 'Confirm Action');
            messageEl.textContent = message;
            confirmBtn.textContent = options.confirmText || t('common.confirm', 'Confirm');
            cancelBtn.textContent = options.cancelText || t('common.cancel', 'Cancel');

            if (options.danger) {
                confirmBtn.className = 'btn btn-danger';
            } else {
                confirmBtn.className = 'btn btn-primary';
            }

            // Store trigger element for focus return per AI.md PART 31
            var triggerElement = document.activeElement;
            var removeFocusTrap = null;

            function cleanup(result) {
                if (removeFocusTrap) removeFocusTrap();
                dialog.close();
                confirmBtn.removeEventListener('click', handleConfirm);
                cancelBtn.removeEventListener('click', handleCancel);
                dialog.removeEventListener('close', handleClose);
                // Return focus to trigger element per AI.md PART 31
                if (triggerElement && triggerElement.focus) {
                    triggerElement.focus();
                }
                resolve(result);
            }

            function handleConfirm() { cleanup(true); }
            function handleCancel() { cleanup(false); }
            function handleClose() { cleanup(false); }

            confirmBtn.addEventListener('click', handleConfirm);
            cancelBtn.addEventListener('click', handleCancel);
            dialog.addEventListener('close', handleClose);

            dialog.showModal();
            removeFocusTrap = trapFocus(dialog);
            confirmBtn.focus();
            // Announce dialog for screen readers
            announce(titleEl.textContent + ': ' + message);
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
                dialog.setAttribute('aria-labelledby', 'prompt-dialog-label');
                dialog.innerHTML =
                    '<form method="dialog">' +
                        '<label id="prompt-dialog-label"></label>' +
                        '<input type="text" id="prompt-dialog-input" class="form-control" aria-describedby="prompt-dialog-label">' +
                        '<footer>' +
                            '<button type="button" class="btn btn-secondary" data-action="cancel">' + escapeHtml(t('common.cancel', 'Cancel')) + '</button>' +
                            '<button type="submit" class="btn btn-primary">' + escapeHtml(t('common.ok', 'OK')) + '</button>' +
                        '</footer>' +
                    '</form>';
                document.body.appendChild(dialog);
            }

            const label = dialog.querySelector('#prompt-dialog-label');
            const input = dialog.querySelector('#prompt-dialog-input');
            const cancelBtn = dialog.querySelector('[data-action="cancel"]');

            label.textContent = message;
            input.value = defaultValue || '';

            // Store trigger element for focus return per AI.md PART 31
            var triggerElement = document.activeElement;
            var removeFocusTrap = null;

            function cleanup(result) {
                if (removeFocusTrap) removeFocusTrap();
                dialog.close();
                // Return focus to trigger element per AI.md PART 31
                if (triggerElement && triggerElement.focus) {
                    triggerElement.focus();
                }
                resolve(result);
            }

            cancelBtn.onclick = function() { cleanup(null); };
            dialog.querySelector('form').onsubmit = function(e) {
                e.preventDefault();
                cleanup(input.value);
            };
            dialog.onclose = function() { cleanup(null); };

            dialog.showModal();
            removeFocusTrap = trapFocus(dialog);
            input.focus();
            input.select();
            // Announce dialog for screen readers
            announce(message);
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
                dialog.setAttribute('aria-describedby', 'alert-dialog-message');
                dialog.innerHTML =
                    '<main id="alert-dialog-message"></main>' +
                    '<footer>' +
                        '<button type="button" class="btn btn-primary" data-action="ok">' + escapeHtml(t('common.ok', 'OK')) + '</button>' +
                    '</footer>';
                document.body.appendChild(dialog);
            }

            const messageEl = dialog.querySelector('#alert-dialog-message');
            const okBtn = dialog.querySelector('[data-action="ok"]');

            messageEl.textContent = message;

            // Store trigger element for focus return per AI.md PART 31
            var triggerElement = document.activeElement;
            var removeFocusTrap = null;

            function cleanup() {
                if (removeFocusTrap) removeFocusTrap();
                dialog.close();
                // Return focus to trigger element per AI.md PART 31
                if (triggerElement && triggerElement.focus) {
                    triggerElement.focus();
                }
                resolve();
            }

            okBtn.onclick = cleanup;
            dialog.onclose = cleanup;

            dialog.showModal();
            removeFocusTrap = trapFocus(dialog);
            okBtn.focus();
            // Announce alert for screen readers (assertive for alerts)
            announce(message, 'assertive');
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
    // FORM VALIDATION (per AI.md PART 16 Form Validation)
    // Optional enhancement: mirror the native validationMessage into the
    // styled .field-error element - CSS :user-invalid styling and native
    // submit-blocking already work without this.
    // ========================================================================
    function initFormValidation() {
        document.querySelectorAll('.form-group input, .form-group select, .form-group textarea').forEach(function(el) {
            el.addEventListener('invalid', function() {
                if (!el.id) return;
                const error = document.getElementById(el.id + '-error');
                if (!error) return;
                error.hidden = false;
                error.textContent = el.validationMessage;
                el.setAttribute('aria-invalid', 'true');
            });
            el.addEventListener('input', function() {
                if (el.validity.valid) {
                    el.setAttribute('aria-invalid', 'false');
                    if (el.id) {
                        const error = document.getElementById(el.id + '-error');
                        if (error) error.hidden = true;
                    }
                }
            });
        });
    }

    // ========================================================================
    // SERVICE WORKER
    // ========================================================================
    function initServiceWorker() {
        if (!('serviceWorker' in navigator)) {
            return;
        }
        window.addEventListener('load', function() {
            navigator.serviceWorker.register('/sw.js', { scope: '/' }).then(function(registration) {
                // Detect a new worker installing while an old one still controls the page
                registration.addEventListener('updatefound', function() {
                    var newWorker = registration.installing;
                    if (!newWorker) {
                        return;
                    }
                    newWorker.addEventListener('statechange', function() {
                        if (newWorker.state === 'installed' && navigator.serviceWorker.controller) {
                            showUpdateNotification();
                        }
                    });
                });

                // Check for updates every hour while the app is active
                setInterval(function() {
                    registration.update();
                }, 60 * 60 * 1000);
            }).catch(function() {
                // Service worker registration failed - the site still works without it
            });
        });
    }

    // Show the PWA update banner. Per AI.md PART 16 the overlay stacking order is
    // cookie consent -> announcements -> PWA update; the update banner sits above
    // the cookie banner via its z-index (see .update-banner in public.css).
    function showUpdateNotification() {
        if (document.querySelector('.update-banner')) {
            return;
        }
        var banner = document.createElement('div');
        banner.className = 'update-banner';
        banner.setAttribute('role', 'status');
        banner.setAttribute('aria-live', 'polite');

        var label = document.createElement('span');
        label.className = 'update-banner-text';
        label.textContent = t('pwa.update_available', 'A new version is available');

        var updateBtn = document.createElement('button');
        updateBtn.type = 'button';
        updateBtn.className = 'btn update-banner-update';
        updateBtn.textContent = t('pwa.update_now', 'Update Now');
        updateBtn.addEventListener('click', updateApp);

        var laterBtn = document.createElement('button');
        laterBtn.type = 'button';
        laterBtn.className = 'btn update-banner-later';
        laterBtn.textContent = t('pwa.update_later', 'Later');
        laterBtn.addEventListener('click', function() {
            banner.remove();
        });

        banner.append(label, updateBtn, laterBtn);
        document.body.appendChild(banner);
    }

    // Apply a waiting service-worker update and reload once it takes control
    function updateApp() {
        navigator.serviceWorker.ready.then(function(registration) {
            if (registration.waiting) {
                registration.waiting.postMessage({ type: 'SKIP_WAITING' });
            }
        });
        navigator.serviceWorker.addEventListener('controllerchange', function() {
            window.location.reload();
        });
    }

    // ========================================================================
    // PWA INSTALL PROMPT
    // Per AI.md PART 16: App Install Prompt
    // ========================================================================
    var deferredInstallPrompt = null;

    function initInstallPrompt() {
        // Already running as an installed PWA: no prompt UI is needed
        if (isInstalledPWA()) {
            hideInstallButton();
            return;
        }

        if (isIOSDevice()) {
            showInstallButton();
            document.querySelectorAll('[data-pwa-install]').forEach(function(btn) {
                btn.addEventListener('click', showIOSInstallInstructions);
            });
        } else {
            document.querySelectorAll('[data-pwa-install]').forEach(function(btn) {
                btn.addEventListener('click', installApp);
            });
        }

        window.addEventListener('beforeinstallprompt', function(event) {
            // Prevent the automatic mini-infobar; show custom UI instead
            event.preventDefault();
            deferredInstallPrompt = event;
            showInstallButton();
        });

        window.addEventListener('appinstalled', function() {
            deferredInstallPrompt = null;
            hideInstallButton();
        });

    }

    function isIOSDevice() {
        return /iPad|iPhone|iPod/.test(navigator.userAgent)
            || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
    }

    function showIOSInstallInstructions() {
        var dialog = document.getElementById('pwa-install-dialog');
        if (dialog && !dialog.open && typeof dialog.showModal === 'function') {
            dialog.showModal();
        }
    }

    function isInstalledPWA() {
        return window.matchMedia('(display-mode: standalone)').matches
            || window.navigator.standalone === true;
    }

    function showInstallButton() {
        document.querySelectorAll('[data-pwa-install]').forEach(function(btn) {
            btn.hidden = false;
        });
    }

    function hideInstallButton() {
        document.querySelectorAll('[data-pwa-install]').forEach(function(btn) {
            btn.hidden = true;
        });
    }

    function installApp() {
        if (!deferredInstallPrompt) {
            return;
        }
        deferredInstallPrompt.prompt();
        deferredInstallPrompt.userChoice.then(function() {
            deferredInstallPrompt = null;
            hideInstallButton();
        });
    }

    // ========================================================================
    // OFFLINE INDICATOR
    // Per AI.md PART 16: Offline detection
    // ========================================================================
    function getOfflineIndicator() {
        var indicator = document.getElementById('offline-indicator');
        if (!indicator) {
            indicator = document.createElement('div');
            indicator.id = 'offline-indicator';
            indicator.setAttribute('aria-live', 'polite');
            document.body.appendChild(indicator);
        }
        return indicator;
    }

    function showOfflineIndicator() {
        var indicator = getOfflineIndicator();
        indicator.textContent = t('pwa.offline', 'You are offline. Some features may be unavailable.');
        indicator.classList.add('visible');
    }

    function hideOfflineIndicator() {
        var indicator = document.getElementById('offline-indicator');
        if (indicator) {
            indicator.classList.remove('visible');
        }
    }

    function initOfflineIndicator() {
        window.addEventListener('offline', showOfflineIndicator);
        window.addEventListener('online', hideOfflineIndicator);
        if (!navigator.onLine) {
            showOfflineIndicator();
        }
    }

    // ========================================================================
    // EVENT DELEGATION (per AI.md PART 17 - no inline handlers)
    // ========================================================================
    function initEventDelegation() {
        document.addEventListener('click', function(e) {
            const target = e.target;
            const stopwatchButton = target.closest('.stopwatch-start, .stopwatch-stop, .stopwatch-reset, .stopwatch-lap');
            if (stopwatchButton) {
                handleStopwatchAction(stopwatchButton);
                return;
            }

            const timerButton = target.closest('.timer-start, .timer-pause, .timer-reset');
            if (timerButton) {
                handleTimerAction(timerButton);
                return;
            }

            if (target.matches('[data-action="regenerate-password"]')) {
                regeneratePassword(target.closest('[data-password-widget]'));
                return;
            }

            if (target.matches('[data-action="uuid-regenerate"]')) {
                regenerateUUID(target.closest('.uuid-result'));
                return;
            }

            if (target.matches('[data-action="roll-again"]')) {
                rollAgain(target.closest('.random-result'));
                return;
            }

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

            // Nav toggle and overlay - CSS-only per AI.md PART 16
            // The checkbox hack handles state via label clicks, no JS needed

            // Copy button (legacy .btn-copy)
            if (target.matches('.btn-copy') || target.closest('.btn-copy')) {
                const btn = target.closest('.btn-copy') || target;
                const elementId = btn.dataset.copyTarget;
                if (elementId) {
                    copyToClipboard(elementId, btn);
                }
                return;
            }

            // Copy button with data-copy attribute - per AI.md PART 16
            if (target.matches('.copy-btn') || target.closest('.copy-btn')) {
                const btn = target.closest('.copy-btn') || target;
                const text = btn.dataset.copy || (btn.previousElementSibling ? btn.previousElementSibling.textContent : null);
                if (text) {
                    navigator.clipboard.writeText(text).then(function() {
                        const icon = btn.querySelector('.copy-icon');
                        if (icon) {
                            const originalIcon = icon.textContent;
                            icon.textContent = '✓';
                            btn.classList.add('copied');
                            setTimeout(function() {
                                icon.textContent = originalIcon;
                                btn.classList.remove('copied');
                            }, 2000);
                        }
                    });
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

            // Password visibility toggle - per AI.md PART 16 (no inline JS)
            if (target.matches('.password-toggle') || target.closest('.password-toggle')) {
                const btn = target.closest('.password-toggle') || target;
                const wrapper = btn.closest('.password-input-wrapper');
                if (wrapper) {
                    const input = wrapper.querySelector('input');
                    if (input) {
                        const isPassword = input.type === 'password';
                        input.type = isPassword ? 'text' : 'password';
                        const icon = btn.querySelector('.icon-eye');
                        if (icon) {
                            icon.style.opacity = isPassword ? '0.5' : '1';
                        }
                    }
                }
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
            const header = document.querySelector('.header');
            if (header && header.classList.contains('nav-open')) {
                if (!target.closest('.nav-links') && !target.closest('.nav-toggle')) {
                    closeNav();
                }
            }
        });

        // Form submit handlers for token revoke
        document.addEventListener('submit', function(e) {
            const form = e.target;

            if (form.matches('.directory-scan-form')) {
                e.preventDefault();
                const query = (form.dataset.query || '').trim();
                window.location.assign(form.action + '?q=' + encodeURIComponent(query));
                return;
            }

            if (form.matches('.directory-filter-form')) {
                e.preventDefault();
                const query = ['directory:' + (form.dataset.query || '').trim()];
                const values = [
                    ['type', form.elements.directory_type.value],
                    ['min-size', form.elements.directory_min_size.value],
                    ['after', form.elements.directory_after.value],
                    ['before', form.elements.directory_before.value],
                    ['site', form.elements.directory_site.value]
                ];
                values.forEach(function(entry) {
                    const value = entry[1].trim();
                    if (value) query.push(entry[0] + ':' + value);
                });
                const scanIndex = form.elements.directory_scan.value.trim();
                if (scanIndex) query.push('scan:' + scanIndex);
                window.location.assign(form.action + '?q=' + encodeURIComponent(query.join(' ')));
                return;
            }

            if (form.matches('[data-confirm-revoke]')) {
                e.preventDefault();
                handleRevokeToken(form);
            }
        });

        document.addEventListener('input', function(e) {
            if (e.target.matches('.color-preview')) {
                updateColorResult(e.target);
            }
        });

        // Toggle switch (role="switch") - keep aria-checked in sync with the
        // native checked state per AI.md PART 16 Toggle Switches; the browser
        // does not update an explicit aria-checked attribute automatically.
        document.addEventListener('change', function(e) {
            if (e.target.matches('.toggle input[role="switch"]')) {
                e.target.setAttribute('aria-checked', e.target.checked ? 'true' : 'false');
            }
        });
    }

    // ========================================================================
    // AUTH FORMS - Per AI.md PART 16 (no inline JS)
    // ========================================================================
    function initAuthForms() {
        // Password strength indicator for registration
        const passwordInput = document.getElementById('password');
        const strengthIndicator = document.getElementById('password-strength');

        if (passwordInput && strengthIndicator) {
            passwordInput.addEventListener('input', function() {
                const password = this.value;
                var strength = 0;
                var color = 'var(--color-error)';

                if (password.length >= 8) strength += 25;
                if (password.length >= 12) strength += 15;
                if (/[a-z]/.test(password)) strength += 15;
                if (/[A-Z]/.test(password)) strength += 15;
                if (/[0-9]/.test(password)) strength += 15;
                if (/[^a-zA-Z0-9]/.test(password)) strength += 15;

                if (strength >= 80) color = 'var(--color-success)';
                else if (strength >= 50) color = 'var(--color-warning)';

                strengthIndicator.style.setProperty('--strength', strength + '%');
                strengthIndicator.style.setProperty('--strength-color', color);
            });
        }

        // Password confirmation validation
        var confirmInput = document.getElementById('confirm_password');
        var confirmHelp = document.getElementById('confirm-help');

        if (confirmInput && passwordInput && confirmHelp) {
            function validateConfirm() {
                if (confirmInput.value && confirmInput.value !== passwordInput.value) {
                    confirmHelp.style.display = 'block';
                    confirmInput.setCustomValidity('Passwords do not match');
                } else {
                    confirmHelp.style.display = 'none';
                    confirmInput.setCustomValidity('');
                }
            }

            confirmInput.addEventListener('input', validateConfirm);
            passwordInput.addEventListener('input', validateConfirm);
        }

        // Username normalization (lowercase, only allowed chars)
        var usernameInput = document.getElementById('username');
        if (usernameInput && document.getElementById('register-form')) {
            usernameInput.addEventListener('input', function() {
                this.value = this.value.toLowerCase().replace(/[^a-z0-9_-]/g, '');
            });
        }
    }

    // ========================================================================
    // HOMEPAGE - Category tabs and Advanced Search
    // Per AI.md PART 16: NO inline JS - all in external files
    // ========================================================================
    function initHomepage() {
        // Category tab switching
        var categoryTabs = document.querySelectorAll('.category-tab');
        var categoryInput = document.getElementById('categoryInput');
        var advancedCategoryInput = document.querySelector('.advanced-search-content input[name="category"]');
        var prefs = getActiveSearchPreferences();

        if (categoryTabs.length > 0 && categoryInput) {
            if (prefs.default_category) {
                categoryInput.value = prefs.default_category;
                if (advancedCategoryInput) advancedCategoryInput.value = prefs.default_category;
                categoryTabs.forEach(function(tab) {
                    var active = tab.dataset.category === prefs.default_category;
                    tab.classList.toggle('active', active);
                    tab.setAttribute('aria-selected', active ? 'true' : 'false');
                });
            }

            categoryTabs.forEach(function(tab) {
                tab.addEventListener('click', function() {
                    categoryTabs.forEach(function(t) {
                        t.classList.remove('active');
                        t.setAttribute('aria-selected', 'false');
                    });
                    this.classList.add('active');
                    this.setAttribute('aria-selected', 'true');
                    categoryInput.value = this.dataset.category;
                    if (advancedCategoryInput) advancedCategoryInput.value = this.dataset.category;
                });
            });
        }

        // Advanced search modal controls
        var advancedModal = document.getElementById('advancedSearchModal');
        var advancedSearchBtn = document.getElementById('advancedSearchBtn');
        var advancedSearchClose = document.getElementById('advancedSearchClose');
        var advancedSearchCancel = document.getElementById('advancedSearchCancel');
        var advancedSearchSubmit = document.getElementById('advancedSearchSubmit');

        if (advancedSearchBtn && advancedModal) {
            advancedSearchBtn.addEventListener('click', function() {
                advancedModal.showModal();
            });
        }

        if (advancedSearchClose && advancedModal) {
            advancedSearchClose.addEventListener('click', function() {
                advancedModal.close();
            });
        }

        if (advancedSearchCancel && advancedModal) {
            advancedSearchCancel.addEventListener('click', function() {
                advancedModal.close();
            });
        }

        // Advanced search query building
        function buildAdvancedQuery() {
            var parts = [];
            var allWords = document.getElementById('as-allwords');
            var exactPhrase = document.getElementById('as-exactphrase');
            var anyWords = document.getElementById('as-anywords');
            var noneWords = document.getElementById('as-nonewords');
            var site = document.getElementById('as-site');
            var filetype = document.getElementById('as-filetype');

            if (allWords && allWords.value.trim()) parts.push(allWords.value.trim());
            if (exactPhrase && exactPhrase.value.trim()) parts.push('"' + exactPhrase.value.trim() + '"');
            if (anyWords && anyWords.value.trim()) parts.push('(' + anyWords.value.trim().split(/\s+/).join(' OR ') + ')');
            if (noneWords && noneWords.value.trim()) parts.push(noneWords.value.trim().split(/\s+/).map(function(w) { return '-' + w; }).join(' '));
            if (site && site.value.trim()) parts.push('site:' + site.value.trim());
            if (filetype && filetype.value) parts.push('filetype:' + filetype.value);

            return parts.join(' ');
        }

        function updateAdvancedPreview() {
            var preview = document.getElementById('as-preview');
            if (preview) {
                var query = buildAdvancedQuery();
                preview.textContent = query || 'Enter search terms above';
            }
        }

        function submitAdvancedSearch() {
            var query = buildAdvancedQuery();
            var searchQuery = document.getElementById('searchQuery');
            var searchForm = document.getElementById('searchForm');

            if (query && searchQuery && searchForm && advancedModal) {
                searchQuery.value = query;
                var timeRange = document.getElementById('as-time');
                var language = document.getElementById('as-lang');
                var region = document.getElementById('as-region');
                [
                    ['time_range', timeRange && timeRange.value],
                    ['lang', language && language.value],
                    ['region', region && region.value]
                ].forEach(function(filter) {
                    var existing = searchForm.querySelector('input[name="' + filter[0] + '"]');
                    if (existing) existing.remove();
                    if (filter[1]) {
                        var input = document.createElement('input');
                        input.type = 'hidden';
                        input.name = filter[0];
                        input.value = filter[1];
                        searchForm.appendChild(input);
                    }
                });
                advancedModal.close();
                searchForm.submit();
            }
        }

        if (advancedSearchSubmit) {
            advancedSearchSubmit.addEventListener('click', submitAdvancedSearch);
        }

        // Update preview on input changes
        var advancedInputs = document.querySelectorAll('.advanced-search-content input, .advanced-search-content select');
        advancedInputs.forEach(function(el) {
            el.addEventListener('input', updateAdvancedPreview);
            el.addEventListener('change', updateAdvancedPreview);
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

    // Search history is browser-local state only; queries are never sent to a
    // history endpoint or included in preference sync URLs.
    function readSearchHistory() {
        try {
            var entries = JSON.parse(localStorage.getItem(SEARCH_HISTORY_KEY) || '[]');
            if (!Array.isArray(entries)) return [];
            return entries.filter(function(entry) {
                return entry && typeof entry.query === 'string' && entry.query.trim() &&
                    typeof entry.category === 'string';
            }).slice(0, SEARCH_HISTORY_LIMIT);
        } catch (error) {
            return [];
        }
    }

    function renderSearchHistory() {
        var section = document.getElementById('search-history');
        var list = document.getElementById('search-history-list');
        if (!section || !list) return;

        var enabled = getStoredSearchPreferences().save_history;
        var entries = enabled ? readSearchHistory() : [];
        list.replaceChildren();
        section.hidden = entries.length === 0;
        entries.forEach(function(entry) {
            var item = document.createElement('li');
            var link = document.createElement('a');
            var target = new URL('/search', window.location.origin);
            target.searchParams.set('q', entry.query);
            target.searchParams.set('category', entry.category);
            link.href = target.pathname + target.search;
            link.textContent = entry.query;
            item.appendChild(link);
            list.appendChild(item);
        });
    }

    function recordSearchHistory() {
        var page = document.querySelector('.search-results-page');
        if (!page || !getStoredSearchPreferences().save_history) return;
        var query = (page.dataset.query || '').trim();
        if (!query) return;

        var category = page.dataset.category || 'general';
        var entries = readSearchHistory().filter(function(entry) {
            return entry.query !== query || entry.category !== category;
        });
        entries.unshift({ query: query, category: category });
        try {
            localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(entries.slice(0, SEARCH_HISTORY_LIMIT)));
        } catch (error) {
            return;
        }
    }

    function initSearchHistory() {
        document.querySelectorAll('#clear-search-history').forEach(function(button) {
            button.addEventListener('click', function() {
                try {
                    localStorage.removeItem(SEARCH_HISTORY_KEY);
                } catch (error) {
                    // Storage can be unavailable in private browsing modes.
                }
                renderSearchHistory();
                var status = document.getElementById('save-status');
                if (status) status.textContent = t('preferences.clear_history', 'Clear history');
            });
        });
        recordSearchHistory();
        renderSearchHistory();
    }

    // ========================================================================
    // PREFERENCES PAGE - Per AI.md PART 16: NO inline JS
    // ========================================================================
    function initPreferencesPage() {
        var prefsPage = document.querySelector('.preferences-page');
        if (!prefsPage) return;

        var preferencesTabs = prefsPage.querySelector('.preferences-tabs');
        if (preferencesTabs) {
            var tabButtons = Array.from(preferencesTabs.querySelectorAll('[role="tab"]'));
            var tabGroups = {
                general: ['lang', 'default-category', 'safe-search', 'units', 'weather-units'],
                interface: ['theme', 'font-size', 'results-density', 'accent-color', 'results-per-page', 'new-tab', 'infinite-scroll', 'keyboard-shortcuts', 'video-previews'],
                privacy: ['save-history', 'clear-search-history']
            };
            Object.keys(tabGroups).forEach(function(tabName) {
                tabGroups[tabName].forEach(function(controlID) {
                    var control = document.getElementById(controlID);
                    var group = control && control.closest('.form-group');
                    if (group) group.dataset.preferenceTab = tabName;
                });
            });
            var setPreferencesTab = function(tabName, focusTab) {
                tabButtons.forEach(function(button) {
                    var selected = button.dataset.preferencesTab === tabName;
                    button.setAttribute('aria-selected', selected ? 'true' : 'false');
                    button.tabIndex = selected ? 0 : -1;
                    if (focusTab && selected) button.focus();
                });
                prefsPage.querySelectorAll('[data-preference-tab]').forEach(function(panel) {
                    panel.hidden = panel.dataset.preferenceTab !== tabName;
                    if (!panel.hasAttribute('role')) panel.setAttribute('role', 'tabpanel');
                    panel.setAttribute('aria-labelledby', 'preferences-tab-' + panel.dataset.preferenceTab);
                });
                var settingsForm = document.getElementById('general-settings');
                var settingsSection = document.getElementById('preferences-settings-section');
                var generalHeading = document.getElementById('preferences-general-heading');
                if (settingsSection) settingsSection.hidden = tabName !== 'general' && tabName !== 'interface' && tabName !== 'privacy';
                if (settingsSection) {
                    settingsSection.setAttribute('role', 'tabpanel');
                    settingsSection.setAttribute('aria-labelledby', 'preferences-tab-' + tabName);
                }
                if (generalHeading) generalHeading.hidden = tabName !== 'general';
                if (settingsForm) {
                    settingsForm.querySelectorAll('.form-group[data-preference-tab]').forEach(function(group) {
                        group.hidden = group.dataset.preferenceTab !== tabName;
                    });
                }
            };
            preferencesTabs.hidden = false;
            tabButtons.forEach(function(button, index) {
                button.addEventListener('click', function() { setPreferencesTab(button.dataset.preferencesTab, false); });
                button.addEventListener('keydown', function(event) {
                    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight' && event.key !== 'Home' && event.key !== 'End') return;
                    event.preventDefault();
                    var nextIndex = event.key === 'Home' ? 0 : event.key === 'End' ? tabButtons.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabButtons.length) % tabButtons.length;
                    setPreferencesTab(tabButtons[nextIndex].dataset.preferencesTab, true);
                });
            });
            setPreferencesTab('general', false);
        }

        // Storage keys (widgets are server-side cookie — no WIDGETS_KEY here)

        // Load preferences from cookies; only the search-history records stay in localStorage.
        function loadPreferences() {
            try {
                var prefs = getStoredSearchPreferences();
                var themeSelect = document.getElementById('theme');
                var langSelect = document.getElementById('lang');
                var unitsSelect = document.getElementById('units');
                var defaultCategorySelect = document.getElementById('default-category');
                var safeSearchSelect = document.getElementById('safe-search');
                var resultsPerPageSelect = document.getElementById('results-per-page');
                var newTabCheckbox = document.getElementById('new-tab');
                var infiniteScrollCheckbox = document.getElementById('infinite-scroll');
                var keyboardShortcutsCheckbox = document.getElementById('keyboard-shortcuts');
                var saveHistoryCheckbox = document.getElementById('save-history');
                var videoPreviewsCheckbox = document.getElementById('video-previews');
                var customCSSInput = document.getElementById('custom-css');

                if (themeSelect) themeSelect.value = getPreferredTheme();
                if (langSelect) langSelect.value = prefs.lang;
                if (unitsSelect) unitsSelect.value = getPreferredUnits();
                if (prefs.default_category && defaultCategorySelect) defaultCategorySelect.value = prefs.default_category;
                if (prefs.safe_search !== undefined && safeSearchSelect) safeSearchSelect.value = prefs.safe_search;
                if (prefs.results_per_page && resultsPerPageSelect) resultsPerPageSelect.value = prefs.results_per_page;
                if (newTabCheckbox) newTabCheckbox.checked = prefs.new_tab !== false;
                if (infiniteScrollCheckbox) infiniteScrollCheckbox.checked = prefs.infinite_scroll !== false;
                if (keyboardShortcutsCheckbox) keyboardShortcutsCheckbox.checked = prefs.keyboard_shortcuts !== false;
                if (saveHistoryCheckbox) saveHistoryCheckbox.checked = prefs.save_history !== false;
                if (videoPreviewsCheckbox) {
                    var videoPreviewCookie = document.cookie.split('; ').find(function(item) { return item.indexOf('search_pref_video_previews=') === 0; });
                    videoPreviewsCheckbox.checked = !videoPreviewCookie || decodeURIComponent(videoPreviewCookie.split('=').slice(1).join('=')) !== '0';
                }
                if (customCSSInput) customCSSInput.value = loadCustomCSS();
            } catch (e) {
                console.error('Failed to load preferences:', e);
            }
        }

        // Save preferences to cookies and keep history records client-side.
        function savePreferences() {
            var themeSelect = document.getElementById('theme');
            var fontSizeSelect = document.getElementById('font-size');
            var resultsDensitySelect = document.getElementById('results-density');
            var accentColorInput = document.getElementById('accent-color');
            var langSelect = document.getElementById('lang');
            var unitsSelect = document.getElementById('units');
            var weatherUnitsSelect = document.getElementById('weather-units');
            var defaultCategorySelect = document.getElementById('default-category');
            var safeSearchSelect = document.getElementById('safe-search');
            var resultsPerPageSelect = document.getElementById('results-per-page');
            var newTabCheckbox = document.getElementById('new-tab');
            var infiniteScrollCheckbox = document.getElementById('infinite-scroll');
            var keyboardShortcutsCheckbox = document.getElementById('keyboard-shortcuts');
            var saveHistoryCheckbox = document.getElementById('save-history');
            var videoPreviewsCheckbox = document.getElementById('video-previews');

            var prefs = {
                theme: themeSelect ? normalizeThemePreference(themeSelect.value) : 'dark',
                font_size: fontSizeSelect && ['small', 'medium', 'large'].indexOf(fontSizeSelect.value) !== -1 ? fontSizeSelect.value : 'medium',
                results_density: resultsDensitySelect && ['compact', 'comfortable', 'spacious'].indexOf(resultsDensitySelect.value) !== -1 ? resultsDensitySelect.value : 'comfortable',
                accent_color: accentColorInput && /^#[0-9a-f]{6}$/i.test(accentColorInput.value) ? accentColorInput.value : '#8250df',
                lang: langSelect ? langSelect.value : getClientLanguage(),
                units: unitsSelect && unitsSelect.value === 'metric' ? 'metric' : 'imperial',
                weather_units: weatherUnitsSelect && ['auto', 'metric', 'imperial'].indexOf(weatherUnitsSelect.value) !== -1 ? weatherUnitsSelect.value : 'auto',
                default_category: defaultCategorySelect ? defaultCategorySelect.value : 'general',
                safe_search: safeSearchSelect ? safeSearchSelect.value : '1',
                results_per_page: resultsPerPageSelect ? resultsPerPageSelect.value : '100',
                new_tab: newTabCheckbox ? newTabCheckbox.checked : true,
                infinite_scroll: infiniteScrollCheckbox ? infiniteScrollCheckbox.checked : true,
                keyboard_shortcuts: keyboardShortcutsCheckbox ? keyboardShortcutsCheckbox.checked : true,
                save_history: saveHistoryCheckbox ? saveHistoryCheckbox.checked : true,
                video_previews: videoPreviewsCheckbox ? videoPreviewsCheckbox.checked : true
            };

            if (!prefs.save_history) localStorage.removeItem(SEARCH_HISTORY_KEY);
            setCookie('theme', prefs.theme, 31536000);
            setCookie('lang', prefs.lang, 31536000);
            setCookie('search_pref_font_size', prefs.font_size, 31536000);
            setCookie('search_pref_results_density', prefs.results_density, 31536000);
            setCookie('search_pref_accent_color', prefs.accent_color, 31536000);
            setUnitsCookie(prefs.units);
            setCookie('search_widget_weather_units', prefs.weather_units, 31536000);
            setCookie('search_pref_category', prefs.default_category, 31536000);
            setCookie('search_pref_safe_search', prefs.safe_search, 31536000);
            setCookie('search_pref_results_per_page', prefs.results_per_page, 31536000);
            setCookie('search_pref_new_tab', prefs.new_tab ? '1' : '0', 31536000);
            setCookie('search_pref_infinite_scroll', prefs.infinite_scroll ? '1' : '0', 31536000);
            setCookie('search_pref_keyboard_shortcuts', prefs.keyboard_shortcuts ? '1' : '0', 31536000);
            setCookie('search_pref_save_history', prefs.save_history ? '1' : '0', 31536000);
            setCookie('search_pref_video_previews', prefs.video_previews ? '1' : '0', 31536000);

            applyTheme(prefs.theme);

            // Save through the canonical preferences endpoint so cookies and
            // the no-JavaScript form path share server-side validation. Widget toggles are
            // form="general-settings" checkboxes rendered elsewhere on the
            // page, so they are collected and sent in the same request -
            // there is a single Save action, not a separate one per section.
            syncGeneralPreferences(prefs);

            applySearchPreferencesToForms();
            showPrefStatus(t('preferences.saved', 'Preferences saved'));

            // Redirect back to previous page after a short delay
            var referrer = document.referrer;
            setTimeout(function() {
                if (referrer && referrer.indexOf(window.location.hostname) !== -1 && referrer !== window.location.href) {
                    window.location.href = referrer;
                } else {
                    window.location.href = '/';
                }
            }, 500);
        }

        // Persist the general preferences (including the homepage widget
        // toggles, which are form="general-settings" checkboxes rendered in
        // a different section of the page) to the server-side cookies via
        // AJAX, so .Prefs on the next server-rendered page reflects the
        // JS-enabled save path too, not just the no-JS form POST fallback.
        function syncGeneralPreferences(prefs) {
            var csrfInput = document.querySelector('#general-settings input[name="csrf_token"]');
            var params = new URLSearchParams();
            if (csrfInput) {
                params.append('csrf_token', csrfInput.value);
            }
            params.append('theme', prefs.theme);
            params.append('lang', prefs.lang);
            params.append('search_pref_font_size', prefs.font_size);
            params.append('search_pref_results_density', prefs.results_density);
            params.append('search_pref_accent_color', prefs.accent_color);
            params.append('search_pref_units', prefs.units);
            params.append('search_widget_weather_units', prefs.weather_units);
            params.append('search_pref_category', prefs.default_category);
            params.append('search_pref_safe_search', prefs.safe_search);
            params.append('search_pref_results_per_page', prefs.results_per_page);
            params.append('search_pref_new_tab', prefs.new_tab ? '1' : '0');
            params.append('search_pref_infinite_scroll', prefs.infinite_scroll ? '1' : '0');
            params.append('search_pref_keyboard_shortcuts', prefs.keyboard_shortcuts ? '1' : '0');
            params.append('search_pref_save_history', prefs.save_history ? '1' : '0');
            params.append('search_pref_video_previews', prefs.video_previews ? '1' : '0');
            params.append('search_pref_video_previews_present', '1');
            params.append('search_pref_engines_present', '1');
            document.querySelectorAll('input[name="search_pref_engine"]:checked:not(:disabled)').forEach(function(input) {
                params.append('search_pref_engine', input.value);
            });
            var engineOrder = document.getElementById('search-pref-engine-order');
            params.append('search_pref_engine_order_present', '1');
            params.append('search_pref_engine_order', engineOrder ? engineOrder.value : '');

            var toggles = document.getElementById('widget-toggles');
            if (toggles) {
                toggles.querySelectorAll('input[name="widget"]:checked').forEach(function(input) {
                    params.append('widget', input.value);
                });
            }

            fetch('/server/preferences', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: params.toString(),
                redirect: 'follow'
            }).catch(function(e) {
                console.error('General preference save failed:', e);
            });
        }

        // loadWidgetPreferences is a no-op — the server renders the correct
        // checked state on each preference page load via the server-side cookie.
        function loadWidgetPreferences() {}

        // Load custom bangs
        function loadCustomBangs() {
            try {
                var bangs = JSON.parse(localStorage.getItem(BANGS_KEY) || '[]');
                renderCustomBangs(bangs);
            } catch (e) {
                console.error('Failed to load custom bangs:', e);
            }
        }

        // Render custom bangs list
        function renderCustomBangs(bangs) {
            var list = document.getElementById('custom-bangs-list');
            if (!list) return;
            list.innerHTML = '';

            if (bangs.length === 0) {
                list.innerHTML = '<p class="help-text">' + escapeHtml(t('preferences.custom_bangs_empty', 'No custom bangs defined.')) + '</p>';
                return;
            }

            bangs.forEach(function(bang, index) {
                var item = document.createElement('div');
                item.className = 'custom-bang-item';
                item.innerHTML = '<div class="bang-info">' +
                    '<span class="bang-shortcut">!' + escapeHtmlLocal(bang.shortcut) + '</span>' +
                    '<span class="bang-name">' + escapeHtmlLocal(bang.name) + '</span>' +
                '</div>' +
                '<button class="delete-btn" data-bang-index="' + index + '">' + escapeHtml(t('common.delete', 'Delete')) + '</button>';
                list.appendChild(item);
            });
        }

        function escapeHtmlLocal(text) {
            var div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Filter bangs by category
        function filterBangs(category) {
            var items = document.querySelectorAll('.bang-item');
            items.forEach(function(item) {
                if (category === 'all' || item.dataset.category === category) {
                    item.classList.remove('hidden');
                } else {
                    item.classList.add('hidden');
                }
            });
        }

        // Show status message
        function showPrefStatus(message, isError) {
            var status = document.getElementById('save-status');
            if (!status) return;
            status.textContent = message;
            status.style.color = isError ? 'var(--color-error)' : 'var(--color-success)';
            setTimeout(function() {
                status.textContent = '';
            }, 3000);
        }

        // Event handlers via delegation
        prefsPage.addEventListener('click', function(e) {
            // Save preferences button - the button is a real submit control
            // (form="general-settings") so no-JS clients get a plain POST;
            // when JS is available, prevent that default submit and instead
            // save via cookies/localStorage + AJAX for a faster, no-reload UX.
            if (e.target.id === 'save-preferences') {
                e.preventDefault();
                savePreferences();
                return;
            }

            if (e.target.id === 'save-custom-css') {
                var cssInput = document.getElementById('custom-css');
                var cssStatus = document.getElementById('custom-css-status');
                var css = cssInput ? cssInput.value : '';
                if (css.length > CUSTOM_CSS_LIMIT) {
                    if (cssStatus) cssStatus.textContent = t('preferences.custom_css_too_long', 'Custom CSS is too long.');
                    return;
                }
                try {
                    if (css.trim()) localStorage.setItem(CUSTOM_CSS_KEY, css);
                    else localStorage.removeItem(CUSTOM_CSS_KEY);
                    applyCustomCSS(css);
                    if (cssStatus) cssStatus.textContent = t('preferences.custom_css_saved', 'Custom CSS saved for this browser.');
                } catch (error) {
                    if (cssStatus) cssStatus.textContent = t('preferences.custom_css_storage_error', 'Custom CSS could not be saved in this browser.');
                }
                return;
            }

            if (e.target.id === 'clear-custom-css') {
                localStorage.removeItem(CUSTOM_CSS_KEY);
                applyCustomCSS('');
                var cssInput = document.getElementById('custom-css');
                var cssStatus = document.getElementById('custom-css-status');
                if (cssInput) cssInput.value = '';
                if (cssStatus) cssStatus.textContent = t('preferences.custom_css_cleared', 'Custom CSS cleared.');
                return;
            }

            // Category filter buttons
            if (e.target.matches('.filter-btn')) {
                document.querySelectorAll('.filter-btn').forEach(function(b) { b.classList.remove('active'); });
                e.target.classList.add('active');
                filterBangs(e.target.dataset.category);
                return;
            }

            // Delete custom bang button
            if (e.target.matches('.delete-btn[data-bang-index]')) {
                var idx = parseInt(e.target.dataset.bangIndex);
                var bangs = JSON.parse(localStorage.getItem(BANGS_KEY) || '[]');
                bangs.splice(idx, 1);
                localStorage.setItem(BANGS_KEY, JSON.stringify(bangs));
                renderCustomBangs(bangs);
                showPrefStatus(t('preferences.custom_bang_deleted', 'Bang deleted'));
                return;
            }

            // Copy OpenSearch URL
            if (e.target.id === 'copy-opensearch') {
                var customName = document.getElementById('custom-engine-name');
                var url = window.location.origin + '/opensearch.xml';
                if (customName && customName.value) {
                    url += '?name=' + encodeURIComponent(customName.value);
                }
                navigator.clipboard.writeText(url).then(function() {
                    showPrefStatus(t('preferences.opensearch_url_copied', 'OpenSearch URL copied!'));
                }).catch(function() {
                    var osUrl = document.getElementById('opensearch-url');
                    if (osUrl) osUrl.textContent = url;
                    showPrefStatus(t('preferences.opensearch_copy_manual', 'URL updated above - copy manually'));
                });
                return;
            }

            // Export preferences (widgets are server-side cookie — not included in export)
            if (e.target.id === 'export-prefs') {
                var data = {
                    preferences: getStoredSearchPreferences(),
                    custom_bangs: JSON.parse(localStorage.getItem(BANGS_KEY) || '[]'),
                    custom_css: loadCustomCSS(),
                    search_history: readSearchHistory(),
                    exported_at: new Date().toISOString()
                };
                var blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
                var url = URL.createObjectURL(blob);
                var a = document.createElement('a');
                a.href = url;
                a.download = 'search-preferences.json';
                a.click();
                URL.revokeObjectURL(url);
                showPrefStatus(t('preferences.exported_success', 'Preferences exported!'));
                return;
            }

            // Import preferences button
            if (e.target.id === 'import-prefs') {
                var importFile = document.getElementById('import-file');
                if (importFile) importFile.click();
                return;
            }

            // Copy preferences (cross-device theme/lang sync per AI.md
            // "Cross-device preference sync") - fetches the stateless
            // server-side export endpoint, copies the full import URL, and
            // points the QR code link at the same URL.
            if (e.target.id === 'copy-sync-prefs') {
                fetch('/server/preferences/export', { headers: { 'Accept': 'application/json' } })
                    .then(function(res) { return res.json(); })
                    .then(function(json) {
                        var data = json && json.data ? json.data : {};
                        if (!data.url) {
                            showPrefStatus(t('preferences.sync_export_failed', 'Could not copy preferences'), true);
                            return;
                        }
                        var urlField = document.getElementById('sync-url');
                        var codeField = document.getElementById('sync-code');
                        var qrLink = document.getElementById('qr-sync-prefs');
                        if (urlField) urlField.value = data.url;
                        if (codeField) codeField.value = data.code || '';
                        if (qrLink) qrLink.href = '/direct/qr/' + encodeURIComponent(data.url);
                        // Populating the fields above is the source of truth for success.
                        // Some browsers (notably Safari) drop the user-activation state
                        // the async Clipboard API needs by the time this fetch resolves,
                        // so a clipboard failure must never be reported as the whole
                        // action failing when the fields were already filled in.
                        if (navigator.clipboard && navigator.clipboard.writeText) {
                            navigator.clipboard.writeText(data.url).then(function() {
                                showPrefStatus(t('preferences.sync_copied', 'Preferences copied!'));
                            }).catch(function() {
                                if (urlField) urlField.select();
                                showPrefStatus(t('preferences.sync_copy_manual', 'Sync link ready - copy it manually below'));
                            });
                        } else {
                            if (urlField) urlField.select();
                            showPrefStatus(t('preferences.sync_copy_manual', 'Sync link ready - copy it manually below'));
                        }
                    })
                    .catch(function() {
                        showPrefStatus(t('preferences.sync_export_failed', 'Could not copy preferences'), true);
                    });
                return;
            }

            // Apply a pasted sync URL or short code by navigating to the
            // stateless import route, which validates, sets cookies, and
            // redirects back here.
            if (e.target.id === 'apply-sync-prefs') {
                var pasteField = document.getElementById('sync-paste');
                var pasted = pasteField ? pasteField.value.trim() : '';
                if (!pasted) {
                    showPrefStatus(t('preferences.sync_invalid', 'Enter a preferences code or URL first'), true);
                    return;
                }
                window.location.href = '/server/preferences/import?code=' + encodeURIComponent(pasted) + '&redirect=' + encodeURIComponent(window.location.pathname);
                return;
            }

            // Reset preferences
            if (e.target.id === 'reset-prefs') {
                showConfirm(t('preferences.reset_confirm_message', 'Are you sure you want to reset all preferences?'), {
                    title: t('preferences.reset_confirm_title', 'Reset Preferences'),
                    confirmText: t('common.reset', 'Reset'),
                    danger: true
                }).then(function(confirmed) {
                    if (!confirmed) return;
                    ['theme', 'lang', 'search_pref_category', 'search_pref_safe_search', 'search_pref_results_per_page', 'search_pref_units', 'search_widget_weather_units', 'search_pref_new_tab', 'search_pref_infinite_scroll', 'search_pref_keyboard_shortcuts', 'search_pref_save_history', 'search_pref_video_previews', 'search_pref_font_size', 'search_pref_results_density', 'search_pref_accent_color', 'search_pref_engines', 'search_pref_engine_order'].forEach(function(name) { clearCookie(name); });
                    localStorage.removeItem(BANGS_KEY);
                    localStorage.removeItem(SEARCH_HISTORY_KEY);
                    localStorage.removeItem(CUSTOM_CSS_KEY);
                    applyCustomCSS('');
                    // Clear per-widget settings
                    for (var i = localStorage.length - 1; i >= 0; i--) {
                        var key = localStorage.key(i);
                        if (key && key.startsWith('search_widget_')) {
                            localStorage.removeItem(key);
                        }
                    }
                    location.reload();
                });
                return;
            }
        });

        // Widget toggle state is persisted by syncGeneralPreferences(), called
        // from savePreferences() when the user clicks "Save & Return" — see
        // above. The toggles are form="general-settings" checkboxes, not
        // their own <form>, so there is nothing to intercept here.

        // Add custom bang form
        var addBangForm = document.getElementById('add-custom-bang');
        if (addBangForm) {
            addBangForm.addEventListener('submit', function(e) {
                e.preventDefault();
                var shortcutInput = document.getElementById('bang-shortcut');
                var nameInput = document.getElementById('bang-name');
                var urlInput = document.getElementById('bang-url');
                var categoryInput = document.getElementById('bang-category');

                var shortcut = shortcutInput.value.toLowerCase().replace(/[^a-z0-9_-]/g, '');
                var name = nameInput.value;
                var url = urlInput.value;
                var category = categoryInput.value || 'custom';

                if (!shortcut || !name || !url) {
                    showPrefStatus(t('preferences.required_fields_error', 'Please fill in all required fields'), true);
                    return;
                }

                if (!url.includes('{query}')) {
                    showPrefStatus(t('preferences.required_fields_error', 'Please fill in all required fields'), true);
                    urlInput.focus();
                    return;
                }
                try {
                    var templateURL = new URL(url.split('{query}').join('search-query'));
                    if ((templateURL.protocol !== 'http:' && templateURL.protocol !== 'https:') || !templateURL.hostname || templateURL.username || templateURL.password) {
                        throw new Error('invalid custom bang URL');
                    }
                } catch (e) {
                    showPrefStatus(t('preferences.required_fields_error', 'Please fill in all required fields'), true);
                    urlInput.focus();
                    return;
                }

                var bangs = JSON.parse(localStorage.getItem(BANGS_KEY) || '[]');
                if (bangs.some(function(b) { return b.shortcut === shortcut; }) || BUILTIN_BANGS.some(function(b) { return b.shortcut === shortcut; })) {
                    showPrefStatus(t('preferences.bang_shortcut_exists', 'A bang with this shortcut already exists'), true);
                    return;
                }

                bangs.push({ shortcut: shortcut, name: name, url: url, category: category, enabled: true });
                localStorage.setItem(BANGS_KEY, JSON.stringify(bangs));
                renderCustomBangs(bangs);
                addBangForm.reset();
                categoryInput.value = 'custom';
                showPrefStatus(t('preferences.custom_bang_added', 'Custom bang added!'));
            });
        }

        // Import file handler
        var importFile = document.getElementById('import-file');
        if (importFile) {
            importFile.addEventListener('change', function(e) {
                var file = e.target.files[0];
                if (!file) return;

                var reader = new FileReader();
                reader.onload = function(ev) {
                    try {
                        var data = JSON.parse(ev.target.result);
                        if (data.preferences && typeof data.preferences === 'object' && !Array.isArray(data.preferences)) writeSearchPreferences(data.preferences);
                        if (Array.isArray(data.custom_bangs)) {
                            localStorage.setItem(BANGS_KEY, JSON.stringify(data.custom_bangs));
                        }
                        if (typeof data.custom_css === 'string') {
                            if (data.custom_css.length > CUSTOM_CSS_LIMIT) throw new Error('custom CSS exceeds the supported limit');
                            if (data.custom_css) localStorage.setItem(CUSTOM_CSS_KEY, data.custom_css);
                            else localStorage.removeItem(CUSTOM_CSS_KEY);
                            applyCustomCSS(data.custom_css);
                        }
                        var importedPreferences = data.preferences || getStoredSearchPreferences();
                        if (importedPreferences.save_history === false) {
                            localStorage.removeItem(SEARCH_HISTORY_KEY);
                        } else if (Array.isArray(data.search_history)) {
                            var importedHistory = data.search_history.filter(function(entry) {
                                return entry && typeof entry.query === 'string' && entry.query.trim() &&
                                    typeof entry.category === 'string';
                            }).slice(0, SEARCH_HISTORY_LIMIT);
                            localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(importedHistory));
                        }
                        // widgets are server-side (enabled_widgets cookie) — not imported from file
                        loadPreferences();
                        loadCustomBangs();
                        loadWidgetPreferences();
                        showPrefStatus(t('preferences.imported_success', 'Preferences imported!'));
                    } catch (err) {
                        showPrefStatus(t('preferences.import_failed', 'Failed to import preferences'), true);
                    }
                };
                reader.readAsText(file);
            });
        }

        // OpenSearch URL update on custom name change
        var customEngineInput = document.getElementById('custom-engine-name');
        if (customEngineInput) {
            customEngineInput.addEventListener('input', function() {
                var osUrl = document.getElementById('opensearch-url');
                if (!osUrl) return;
                var url = window.location.origin + '/opensearch.xml';
                if (this.value) {
                    url += '?name=' + encodeURIComponent(this.value);
                }
                osUrl.textContent = url;
            });
        }

        var engineList = document.getElementById('engine-preference-list');
        var draggedEngine = null;
        if (engineList) {
            engineList.addEventListener('dragstart', function(event) {
                var item = closestFrom(event.target, '.engine-preference-item[draggable="true"]');
                if (!item) return;
                draggedEngine = item;
                item.classList.add('is-dragging');
                if (event.dataTransfer) {
                    event.dataTransfer.effectAllowed = 'move';
                    event.dataTransfer.setData('text/plain', item.dataset.engineId || '');
                }
            });
            engineList.addEventListener('dragover', function(event) {
                var target = closestFrom(event.target, '.engine-preference-item');
                if (!draggedEngine || !target || target === draggedEngine) return;
                event.preventDefault();
                var bounds = target.getBoundingClientRect();
                var insertAfter = event.clientY > bounds.top + bounds.height / 2;
                engineList.insertBefore(draggedEngine, insertAfter ? target.nextSibling : target);
            });
            engineList.addEventListener('drop', function(event) {
                if (!draggedEngine) return;
                event.preventDefault();
                var orderInput = document.getElementById('search-pref-engine-order');
                if (orderInput) {
                    orderInput.value = Array.from(engineList.querySelectorAll('.engine-preference-item')).map(function(item) {
                        return item.dataset.engineId;
                    }).join(',');
                }
            });
            engineList.addEventListener('dragend', function() {
                if (draggedEngine) draggedEngine.classList.remove('is-dragging');
                draggedEngine = null;
            });
        }

        // Initialize
        loadPreferences();
        loadCustomBangs();
        loadWidgetPreferences();
    }

    // ========================================================================
    // USER PAGES (Profile, Security, Tokens) - Per AI.md PART 16: NO inline JS
    // ========================================================================
    function initUserPages() {
        // Profile page - bio character count
        var bioInput = document.getElementById('bio');
        var bioCount = document.getElementById('bio-count');
        if (bioInput && bioCount) {
            bioInput.addEventListener('input', function() {
                bioCount.textContent = this.value.length;
            });
        }

        // Profile page - avatar preview
        var avatarInput = document.querySelector('input[name="avatar"]');
        var avatarPreview = document.querySelector('.current-avatar');
        if (avatarInput && avatarPreview) {
            avatarInput.addEventListener('change', function(e) {
                var file = e.target.files[0];
                if (file) {
                    var reader = new FileReader();
                    reader.onload = function(ev) {
                        var img = avatarPreview.querySelector('.avatar-img');
                        if (img) {
                            img.src = ev.target.result;
                        } else {
                            avatarPreview.innerHTML = '<img src="' + ev.target.result + '" alt="Avatar" class="avatar-img">';
                        }
                    };
                    reader.readAsDataURL(file);
                }
            });
        }

        // Security page - password strength indicator
        var newPassword = document.getElementById('new_password');
        var strengthIndicator = document.getElementById('password-strength');
        if (newPassword && strengthIndicator) {
            newPassword.addEventListener('input', function() {
                var password = this.value;
                var score = 0;
                var color = 'var(--color-error)';

                if (password.length >= 8) score += 25;
                if (password.length >= 12) score += 15;
                if (/[a-z]/.test(password)) score += 15;
                if (/[A-Z]/.test(password)) score += 15;
                if (/[0-9]/.test(password)) score += 15;
                if (/[^a-zA-Z0-9]/.test(password)) score += 15;

                if (score >= 80) color = 'var(--color-success)';
                else if (score >= 50) color = 'var(--color-warning)';

                strengthIndicator.style.setProperty('--strength', score + '%');
                strengthIndicator.style.setProperty('--strength-color', color);
            });
        }

        // Security page - confirm password match
        var confirmPassword = document.getElementById('confirm_password');
        if (newPassword && confirmPassword) {
            confirmPassword.addEventListener('input', function() {
                if (this.value !== newPassword.value) {
                    this.setCustomValidity('Passwords do not match');
                } else {
                    this.setCustomValidity('');
                }
            });
        }

        // Tokens page - modal handling
        var createTokenBtn = document.getElementById('create-token-btn');
        var tokenModal = document.getElementById('create-token-modal');
        if (createTokenBtn && tokenModal) {
            createTokenBtn.addEventListener('click', function() {
                tokenModal.classList.add('active');
            });

            // Close modal on Escape
            document.addEventListener('keydown', function(e) {
                if (e.key === 'Escape' && tokenModal.classList.contains('active')) {
                    tokenModal.classList.remove('active');
                }
            });
        }

        // Token page event delegation
        document.addEventListener('click', function(e) {
            // Close token modal
            if (e.target.matches('[data-action="close-token-modal"]')) {
                if (tokenModal) tokenModal.classList.remove('active');
                return;
            }

            // Copy token
            if (e.target.matches('[data-action="copy-token"]')) {
                var tokenEl = document.getElementById('new-token');
                if (tokenEl) {
                    navigator.clipboard.writeText(tokenEl.textContent).then(function() {
                        var btn = e.target;
                        var original = btn.textContent;
                        btn.textContent = t('common.copied', 'Copied!');
                        setTimeout(function() {
                            btn.textContent = original;
                        }, 2000);
                    });
                }
                return;
            }
        });

        // Token revoke confirmation
        document.addEventListener('submit', function(e) {
            if (e.target.matches('[data-confirm-revoke]')) {
                e.preventDefault();
                var form = e.target;
                showConfirm(t('user.revoke_api_token_confirm_message', 'Are you sure you want to revoke this token?'), {
                    title: t('user.revoke_api_token_title', 'Revoke API Token'),
                    confirmText: t('user.revoke_token_action', 'Revoke'),
                    danger: true
                }).then(function(confirmed) {
                    if (confirmed) {
                        form.submit();
                    }
                });
            }
        });
    }

    // ========================================================================
    // ANNOUNCEMENTS - Per AI.md PART 16: NO inline JS
    // Intercept the dismiss form to skip the reload; the cookie stays
    // server-readable so dismissed announcements are never rendered again.
    // ========================================================================
    function initAnnouncements() {
        // Remove already-dismissed banners immediately
        var dismissed = getDismissedAnnouncements();
        dismissed.forEach(function(id) {
            var el = document.querySelector('[data-announcement-id="' + id + '"]');
            if (el) el.remove();
        });

        // Intercept dismiss form submits — persist id in cookie and remove banner
        document.querySelectorAll('.site-banner .site-banner-dismiss').forEach(function(form) {
            form.addEventListener('submit', function(event) {
                event.preventDefault();
                var banner = form.closest('.site-banner');
                if (!banner) return;
                var announcementId = banner.dataset.announcementId;
                if (!announcementId) return;
                var ids = getDismissedAnnouncements();
                if (!ids.includes(announcementId)) {
                    ids.push(announcementId);
                }
                document.cookie = 'dismissed_announcements=' + encodeURIComponent(ids.join(',')) +
                    '; path=/; max-age=31536000; SameSite=Lax';
                banner.remove();
            });
        });
    }

    // ========================================================================
    // COOKIE CONSENT - Per AI.md PART 16: NO inline JS
    // Enhancement only — the banner forms POST to /server/consent and work without JS.
    // This intercepts the submits to skip the reload.
    // ========================================================================
    function initCookieConsent() {
        var consentBanner = document.getElementById('cookie-consent');
        if (!consentBanner) return;

        var consent = readConsentCookie();
        if (consent === null) {
            consentBanner.classList.add('visible');
        } else {
            consentBanner.remove();
            return;
        }

        // Intercept the banner forms to skip the page reload
        consentBanner.querySelectorAll('form').forEach(function(form) {
            form.addEventListener('submit', function(event) {
                event.preventDefault();
                var choiceEl = form.elements.choice;
                var accepted = choiceEl && choiceEl.value === 'accept';
                var newConsent = {
                    essential: true,
                    preferences: accepted,
                    analytics: accepted,
                    timestamp: Date.now()
                };
                saveAndApplyConsent(newConsent);
            });
        });

        // Intercept the granular preferences button
        consentBanner.querySelectorAll('[data-action="cookie-preferences"]').forEach(function(el) {
            el.addEventListener('click', function() {
                var modal = document.getElementById('cookie-preferences-modal');
                if (modal) modal.showModal();
            });
        });
    }

    // ========================================================================
    // FAVICON ERROR HANDLING - Per AI.md PART 16: NO inline JS
    // Replaces inline onerror handler with event delegation
    // ========================================================================
    function initFaviconErrorHandling() {
        // Helper to show placeholder and hide image
        function showPlaceholder(img) {
            img.classList.add('hidden');
            var placeholder = img.nextElementSibling;
            if (placeholder && placeholder.classList.contains('favicon-placeholder')) {
                placeholder.classList.remove('hidden');
            }
        }

        document.addEventListener('error', function(e) {
            // The page's own 'error' event (e.g. resource load failures during
            // capture phase) can target non-Element nodes such as document,
            // which have no matches() method.
            if (!(e.target instanceof Element)) return;
            if (e.target.matches('.result-favicon img[data-favicon-fallback]')) {
                showPlaceholder(e.target);
            }
            // Quicklink favicons: hide on error (no sibling placeholder)
            if (e.target.matches('img[data-quicklink-favicon]')) {
                e.target.style.display = 'none';
            }
        }, true);

        document.addEventListener('load', function(e) {
            // The page's own 'load' event targets document, which has no
            // matches() method — only real Element nodes support it.
            if (!(e.target instanceof Element)) return;
            if (e.target.matches('.result-favicon img[data-favicon-fallback]')) {
                if (e.target.naturalWidth <= 1 && e.target.naturalHeight <= 1) {
                    showPlaceholder(e.target);
                }
            }
        }, true);

        document.querySelectorAll('.result-favicon img[data-favicon-fallback]').forEach(function(img) {
            if (img.complete && img.naturalWidth <= 1 && img.naturalHeight <= 1) {
                showPlaceholder(img);
            }
        });
    }

    function regenerateUUID(widget) {
        if (!widget || !window.crypto || !window.crypto.getRandomValues) return;
        var bytes = new Uint8Array(16);
        window.crypto.getRandomValues(bytes);
        bytes[6] = (bytes[6] & 0x0f) | 0x40;
        bytes[8] = (bytes[8] & 0x3f) | 0x80;
        var hex = Array.from(bytes, function(value) { return value.toString(16).padStart(2, '0'); }).join('');
        var uuid = hex.slice(0, 8) + '-' + hex.slice(8, 12) + '-' + hex.slice(12, 16) + '-' + hex.slice(16, 20) + '-' + hex.slice(20);
        var values = { standard: uuid, uppercase: uuid.toUpperCase(), compact: hex };
        widget.querySelectorAll('[data-uuid-format]').forEach(function(code) {
            var value = values[code.dataset.uuidFormat];
            code.textContent = value;
            var copy = code.nextElementSibling;
            if (copy && copy.matches('.copy-btn')) copy.dataset.copy = value;
        });
    }

    function secureRandomIndex(max) {
        if (!window.crypto || !window.crypto.getRandomValues || max < 1 || max > 4294967296) {
            throw new Error('Secure random number generation is unavailable.');
        }
        var limit = Math.floor(4294967296 / max) * max;
        var value = new Uint32Array(1);
        do {
            window.crypto.getRandomValues(value);
        } while (value[0] >= limit);
        return value[0] % max;
    }

    function regeneratePassword(widget) {
        if (!widget) return;
        var lengthInput = widget.querySelector('.password-length');
        var length = Math.max(8, Math.min(64, parseInt(lengthInput.value, 10) || 16));
        lengthInput.value = String(length);
        var sets = {
            lowercase: 'abcdefghijklmnopqrstuvwxyz',
            uppercase: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ',
            numbers: '0123456789',
            symbols: '!@#$%^&*()-_=+[]{}|;:,.<>?'
        };
        var chosen = Array.from(widget.querySelectorAll('[data-password-set]:checked'))
            .map(function(input) { return sets[input.dataset.passwordSet]; })
            .filter(Boolean);
        if (!chosen.length) {
            widget.querySelector('[data-password-set="lowercase"]').checked = true;
            chosen = [sets.lowercase];
        }
        var alphabet = chosen.join('');
        var chars = chosen.map(function(set) { return set[secureRandomIndex(set.length)]; });
        while (chars.length < length) chars.push(alphabet[secureRandomIndex(alphabet.length)]);
        for (var i = chars.length - 1; i > 0; i--) {
            var j = secureRandomIndex(i + 1);
            var temp = chars[i];
            chars[i] = chars[j];
            chars[j] = temp;
        }
        var password = chars.join('');
        var entropy = Math.floor(length * Math.log2(alphabet.length));
        var strength = entropy < 40 ? 'Weak' : entropy < 70 ? 'Fair' : entropy < 100 ? 'Strong' : 'Very strong';
        widget.querySelector('.password-value').textContent = password;
        var copy = widget.querySelector('.copy-btn');
        copy.dataset.copy = password;
        widget.querySelector('.password-strength-meter').value = Math.min(entropy, 100);
        widget.querySelector('.password-strength-label').textContent = strength;
    }

    function rollAgain(widget) {
        if (!widget) return;
        var value = widget.querySelector('.random-value');
        var result;
        if (widget.dataset.randomKind === 'coin') {
            result = secureRandomIndex(2) === 0 ? 'Heads' : 'Tails';
        } else {
            var min = parseInt(widget.dataset.min, 10);
            var max = parseInt(widget.dataset.max, 10);
            if (!Number.isSafeInteger(min) || !Number.isSafeInteger(max) || max < min || max - min >= 4294967296) return;
            var count = Math.max(1, Math.min(100, parseInt(widget.dataset.count || '1', 10)));
            var rolls = [];
            for (var i = 0; i < count; i++) rolls.push(min + secureRandomIndex(max - min + 1));
            result = count > 1 ? rolls.join(', ') + ' (total ' + rolls.reduce(function(sum, roll) { return sum + roll; }, 0) + ')' : String(rolls[0]);
        }
        value.textContent = result;
        widget.classList.remove('is-rolling');
        void widget.offsetWidth;
        widget.classList.add('is-rolling');
        window.setTimeout(function() { widget.classList.remove('is-rolling'); }, 350);
    }

    function updateColorResult(input) {
        var widget = input.closest('.color-result');
        if (!widget || !/^#[0-9a-f]{6}$/i.test(input.value)) return;
        var hex = input.value.toUpperCase();
        var r = parseInt(hex.slice(1, 3), 16);
        var g = parseInt(hex.slice(3, 5), 16);
        var b = parseInt(hex.slice(5, 7), 16);
        var rn = r / 255, gn = g / 255, bn = b / 255;
        var max = Math.max(rn, gn, bn), min = Math.min(rn, gn, bn), delta = max - min;
        var light = (max + min) / 2, hue = 0, saturation = 0;
        if (delta) {
            saturation = light > 0.5 ? delta / (2 - max - min) : delta / (max + min);
            if (max === rn) hue = (gn - bn) / delta + (gn < bn ? 6 : 0);
            else if (max === gn) hue = (bn - rn) / delta + 2;
            else hue = (rn - gn) / delta + 4;
            hue *= 60;
        }
        var k = 1 - max;
        var cmyk = k === 1 ? [0, 0, 0, 100] : [(1 - rn - k) / (1 - k) * 100, (1 - gn - k) / (1 - k) * 100, (1 - bn - k) / (1 - k) * 100, k * 100].map(Math.round);
        var complementary = colorFromHSL((hue + 180) % 360, saturation, light);
        var names = {};
        try { names = JSON.parse(widget.dataset.colorNames || '{}'); } catch (error) { names = {}; }
        var values = {
            hex: hex.slice(1),
            rgb: 'rgb(' + r + ', ' + g + ', ' + b + ')',
            hsl: 'hsl(' + Math.round(hue) + ', ' + Math.round(saturation * 100) + '%, ' + Math.round(light * 100) + '%)',
            cmyk: 'cmyk(' + cmyk.join('%, ') + '%)',
            name: names[hex.slice(1).toLowerCase()] || '',
            complementary: complementary
        };
        Object.keys(values).forEach(function(key) {
            var value = key === 'complementary' ? '#' + values[key] : key === 'hex' ? '#' + values[key] : values[key];
            var display = widget.querySelector('[data-color-value="' + key + '"]');
            if (display) display.textContent = value;
            var copy = widget.querySelector('[data-color-copy="' + key + '"]');
            if (copy) copy.dataset.copy = value;
        });
    }

    function colorFromHSL(hue, saturation, lightness) {
        var c = (1 - Math.abs(2 * lightness - 1)) * saturation;
        var x = c * (1 - Math.abs((hue / 60) % 2 - 1));
        var m = lightness - c / 2;
        var rgb = hue < 60 ? [c, x, 0] : hue < 120 ? [x, c, 0] : hue < 180 ? [0, c, x] : hue < 240 ? [0, x, c] : hue < 300 ? [x, 0, c] : [c, 0, x];
        return rgb.map(function(value) { return Math.round((value + m) * 255).toString(16).padStart(2, '0'); }).join('').toUpperCase();
    }

    function formatWidgetTime(totalSeconds, includeHours) {
        var seconds = Math.max(0, Math.floor(totalSeconds));
        var hours = Math.floor(seconds / 3600);
        var minutes = Math.floor((seconds % 3600) / 60);
        var remaining = seconds % 60;
        var pad = function(value) { return String(value).padStart(2, '0'); };
        return (includeHours || hours > 0 ? pad(hours) + ':' : '') + pad(minutes) + ':' + pad(remaining);
    }

    function handleStopwatchAction(button) {
        var widget = button.closest('.stopwatch-widget');
        if (!widget) return;
        var state = widget._stopwatchState || (widget._stopwatchState = { elapsed: 0, startedAt: 0, interval: 0, laps: [], storageKey: stopwatchStorageKey(widget) });
        var display = widget.querySelector('.stopwatch-display');
        var render = function() {
            var elapsed = state.elapsed + (state.startedAt ? Date.now() - state.startedAt : 0);
            display.textContent = formatWidgetTime(elapsed / 1000, true);
        };
        if (button.matches('.stopwatch-start') && !state.startedAt) {
            state.startedAt = Date.now();
            state.interval = window.setInterval(render, 100);
            saveStopwatchState(state);
            widget.querySelector('.stopwatch-start').disabled = true;
            widget.querySelector('.stopwatch-stop').disabled = false;
            widget.querySelector('.stopwatch-reset').disabled = false;
        } else if (button.matches('.stopwatch-stop') && state.startedAt) {
            state.elapsed += Date.now() - state.startedAt;
            state.startedAt = 0;
            window.clearInterval(state.interval);
            render();
            saveStopwatchState(state);
            widget.querySelector('.stopwatch-start').disabled = false;
            widget.querySelector('.stopwatch-stop').disabled = true;
        } else if (button.matches('.stopwatch-reset')) {
            window.clearInterval(state.interval);
            state.elapsed = 0;
            state.startedAt = 0;
            render();
            widget.querySelector('.stopwatch-start').disabled = false;
            widget.querySelector('.stopwatch-stop').disabled = true;
            widget.querySelector('.stopwatch-reset').disabled = true;
            widget.querySelector('.stopwatch-laps').replaceChildren();
            state.laps = [];
            try { sessionStorage.removeItem(state.storageKey); } catch (error) { /* Storage is optional. */ }
        } else if (button.matches('.stopwatch-lap')) {
            var lapText = display.textContent;
            state.laps.push(lapText);
            var lap = document.createElement('div');
            lap.textContent = 'Lap ' + state.laps.length + ': ' + lapText;
            widget.querySelector('.stopwatch-laps').appendChild(lap);
            saveStopwatchState(state);
        }
    }

    function stopwatchStorageKey(widget) {
        var stopwatches = Array.from(document.querySelectorAll('.instant-answer-content .stopwatch-widget'));
        return 'search.instant-stopwatch:' + location.pathname + location.search + ':' + stopwatches.indexOf(widget);
    }

    function saveStopwatchState(state) {
        try {
            sessionStorage.setItem(state.storageKey, JSON.stringify({ elapsed: state.elapsed, startedAt: state.startedAt, laps: state.laps }));
        } catch (error) {
            // The stopwatch remains usable when browser storage is unavailable.
        }
    }

    function timerStorageKey(widget) {
        var timers = Array.from(document.querySelectorAll('.instant-answer-content .timer-widget'));
        return 'search.instant-timer:' + location.pathname + location.search + ':' + timers.indexOf(widget);
    }

    function initInstantWidgetPersistence() {
        document.querySelectorAll('.instant-answer-content .stopwatch-widget').forEach(function(widget) {
            var state = { elapsed: 0, startedAt: 0, interval: 0, laps: [], storageKey: stopwatchStorageKey(widget) };
            try {
                var saved = JSON.parse(sessionStorage.getItem(state.storageKey) || 'null');
                if (saved && Number.isFinite(saved.elapsed) && saved.elapsed >= 0) {
                    state.elapsed = saved.elapsed;
                    state.startedAt = Number.isFinite(saved.startedAt) ? saved.startedAt : 0;
                    state.laps = Array.isArray(saved.laps) ? saved.laps.filter(function(lap) { return typeof lap === 'string'; }).slice(0, 1000) : [];
                }
            } catch (error) {
                try { sessionStorage.removeItem(state.storageKey); } catch (storageError) { /* Storage is optional. */ }
            }
            widget._stopwatchState = state;
            var display = widget.querySelector('.stopwatch-display');
            state.laps.forEach(function(text, index) {
                var lap = document.createElement('div');
                lap.textContent = 'Lap ' + (index + 1) + ': ' + text;
                widget.querySelector('.stopwatch-laps').appendChild(lap);
            });
            var render = function() {
                var elapsed = state.elapsed + (state.startedAt ? Date.now() - state.startedAt : 0);
                display.textContent = formatWidgetTime(elapsed / 1000, true);
            };
            render();
            if (state.startedAt) {
                state.interval = window.setInterval(render, 100);
                widget.querySelector('.stopwatch-start').disabled = true;
                widget.querySelector('.stopwatch-stop').disabled = false;
                widget.querySelector('.stopwatch-reset').disabled = false;
            }
        });
        document.querySelectorAll('.instant-answer-content .timer-widget').forEach(function(widget) {
            var key = timerStorageKey(widget);
            var total = Math.max(0, parseInt(widget.dataset.duration || '0', 10));
            var state = { total: total, remaining: total, interval: 0, deadline: 0, storageKey: key };
            try {
                var saved = JSON.parse(sessionStorage.getItem(key) || 'null');
                if (saved && Number.isFinite(saved.total) && saved.total >= 0) {
                    state.total = saved.total;
                    state.remaining = Math.max(0, Number(saved.remaining) || 0);
                    if (Number.isFinite(saved.deadline) && saved.deadline > Date.now()) {
                        state.deadline = saved.deadline;
                        state.interval = window.setInterval(function() { renderTimer(widget, state); }, 200);
                        var start = widget.querySelector('.timer-start');
                        var pause = widget.querySelector('.timer-pause');
                        if (start) start.disabled = true;
                        if (pause) pause.disabled = false;
                    } else if (Number.isFinite(saved.deadline) && saved.deadline > 0) {
                        state.remaining = 0;
                        sessionStorage.removeItem(key);
                    }
                }
            } catch (error) {
                try { sessionStorage.removeItem(key); } catch (storageError) { /* Storage is optional. */ }
            }
            widget._timerState = state;
            if (!state.interval) {
                var display = widget.querySelector('.timer-display');
                if (display) display.textContent = formatWidgetTime(state.remaining, state.remaining >= 3600);
            }
        });
    }

    function saveTimerState(widget, state, running) {
        try {
            sessionStorage.setItem(state.storageKey || timerStorageKey(widget), JSON.stringify({
                total: state.total,
                remaining: state.remaining,
                deadline: running ? state.deadline : 0
            }));
        } catch (error) {
            // Timers continue to run if browser storage is unavailable.
        }
    }

    function renderTimer(widget, state) {
        var display = widget.querySelector('.timer-display');
        state.remaining = Math.max(0, Math.ceil((state.deadline - Date.now()) / 1000));
        display.textContent = formatWidgetTime(state.remaining, state.remaining >= 3600);
        if (state.remaining === 0) {
            window.clearInterval(state.interval);
            state.interval = 0;
            try { sessionStorage.removeItem(state.storageKey); } catch (error) { /* Storage is optional. */ }
            widget.dispatchEvent(new CustomEvent('timercomplete', { bubbles: true }));
            playTimerNotification();
            var start = widget.querySelector('.timer-start');
            var pause = widget.querySelector('.timer-pause');
            if (start) start.disabled = false;
            if (pause) pause.disabled = true;
        }
    }

    function playTimerNotification() {
        try {
            var AudioContextType = window.AudioContext || window.webkitAudioContext;
            if (!AudioContextType) return;
            var audio = new AudioContextType();
            var oscillator = audio.createOscillator();
            var gain = audio.createGain();
            oscillator.frequency.value = 880;
            gain.gain.setValueAtTime(0.12, audio.currentTime);
            gain.gain.exponentialRampToValueAtTime(0.001, audio.currentTime + 0.8);
            oscillator.connect(gain);
            gain.connect(audio.destination);
            oscillator.start();
            oscillator.stop(audio.currentTime + 0.8);
            oscillator.onended = function() { audio.close(); };
        } catch (error) {
            // Completion remains visible when audio is unavailable or blocked.
        }
    }

    function handleTimerAction(button) {
        var widget = button.closest('.timer-widget');
        if (!widget) return;
        var display = widget.querySelector('.timer-display');
        var state = widget._timerState || (widget._timerState = {
            total: Math.max(0, parseInt(widget.dataset.duration || '0', 10)),
            remaining: Math.max(0, parseInt(widget.dataset.duration || '0', 10)),
            interval: 0,
            deadline: 0,
            storageKey: timerStorageKey(widget)
        });
        var start = widget.querySelector('.timer-start');
        var pause = widget.querySelector('.timer-pause');
        if (button.matches('.timer-start') && !state.interval && state.remaining > 0) {
            state.deadline = Date.now() + state.remaining * 1000;
            state.interval = window.setInterval(function() { renderTimer(widget, state); }, 200);
            saveTimerState(widget, state, true);
            if (start) start.disabled = true;
            if (pause) pause.disabled = false;
        } else if (button.matches('.timer-pause') && state.interval) {
            renderTimer(widget, state);
            if (state.remaining === 0) return;
            window.clearInterval(state.interval);
            state.interval = 0;
            saveTimerState(widget, state, false);
            if (start) start.disabled = false;
            if (pause) pause.disabled = true;
        } else if (button.matches('.timer-reset')) {
            window.clearInterval(state.interval);
            state.interval = 0;
            state.remaining = state.total;
            display.textContent = formatWidgetTime(state.remaining, state.remaining >= 3600);
            try { sessionStorage.removeItem(state.storageKey); } catch (error) { /* Storage is optional. */ }
            if (start) start.disabled = false;
            if (pause) pause.disabled = true;
        }
    }

    // ========================================================================
    // INITIALIZATION
    // ========================================================================
    function init() {
        loadClientTranslations();
        applyTheme(getActiveSearchPreferences().theme || getPreferredTheme());
        loadCustomCSS();
        applySearchPreferencesToForms();
        initLanguageSelector();
        initFlashMessages();
        initSearchAutocomplete();
        initKeyboardShortcuts();
        initLazyLoading();
        initImageViewer();
        initInfiniteScroll();
        initFormProtection();
        initFormValidation();
        initServiceWorker();
        initInstallPrompt();
        initOfflineIndicator();
        initEventDelegation();
        initInstantWidgetPersistence();
        initTabKeyboardNav();
        initAuthForms();
        initSearchHistory();
        initHomepage();
        initHealthzCountdown();
        initPreferencesPage();
        initUserPages();
        initAnnouncements();
        initCookieConsent();
        initCCPA();
        initFaviconErrorHandling();
    }

    function initLanguageSelector() {
        document.querySelectorAll('[data-language-selector]').forEach(function(select) {
            select.addEventListener('change', function() {
                var nextLang = (select.value || '').trim();
                if (!nextLang) {
                    return;
                }

                var url = new URL(window.location.href);
                url.searchParams.set('lang', nextLang);
                window.location.assign(url.toString());
            });
        });
    }

    // ========================================================================
    // HEALTHZ PAGE - Auto-refresh countdown
    // Per AI.md PART 16: NO inline JS
    // ========================================================================
    function initHealthzCountdown() {
        var countdown = document.getElementById('countdown');
        if (!countdown) return;

        var seconds = 30;
        var timer = setInterval(function() {
            seconds--;
            countdown.textContent = seconds;
            if (seconds <= 0) {
                clearInterval(timer);
                window.location.reload();
            }
        }, 1000);
    }

    // System theme change listener is registered in THEME MANAGEMENT section above

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
    window.dismissToast = dismissToast;
    window.dismissAllToasts = dismissAllToasts;
    // Exposed for WidgetManager (a separate top-level IIFE below) which
    // needs the global units preference when a widget has no per-widget
    // override saved yet.
    window.getPreferredUnits = getPreferredUnits;
    window.closestFrom = closestFrom;
    window.showConfirm = showConfirm;
    window.showPrompt = showPrompt;
    window.showAlert = showAlert;
    window.toggleTheme = toggleTheme;
    window.toggleNav = toggleNav;
    window.closeNav = closeNav;
    window.copyToClipboard = copyToClipboard;
    window.getActiveSearchPreferences = getActiveSearchPreferences;
    window.applyTheme = applyTheme;
    // Export the PWA install trigger so install controls anywhere can invoke it
    window.installApp = installApp;
    // Export i18n lookup so the widget IIFE and other IIFEs can call t() globally
    window.t = t;

})();

(function () {
    'use strict';
    document.addEventListener('input', function (event) {
        var field = event.target;
        if (!field.matches('[data-man-search]')) return;
        var query = field.value.trim().toLocaleLowerCase();
        var content = field.closest('.man-content');
        content.querySelectorAll('.man-section').forEach(function (section) {
            var matches = !query || section.textContent.toLocaleLowerCase().includes(query);
            section.hidden = !matches;
            if (matches && section.tagName === 'DETAILS' && query) section.open = true;
        });
    });
    document.addEventListener('click', function (event) {
        var button = event.target.closest('[data-man-copy]');
        if (!button) return;
        var block = button.previousElementSibling;
        var code = block && block.matches('pre') ? block.querySelector('code') : null;
        var text = code ? code.textContent : (block && block.matches('pre') ? block.textContent : '');
        var legacyCopy = function () {
            var buffer = document.createElement('textarea');
            buffer.className = 'man-copy-buffer';
            buffer.value = text;
            buffer.setAttribute('aria-hidden', 'true');
            buffer.tabIndex = -1;
            document.body.appendChild(buffer);
            buffer.select();
            var copied = false;
            try { copied = document.execCommand('copy'); } catch (error) {}
            buffer.remove();
            return copied;
        };
        var copyPromise;
        if (navigator.clipboard && navigator.clipboard.writeText) {
            try {
                copyPromise = navigator.clipboard.writeText(text).then(function () { return true; }).catch(legacyCopy);
            } catch (error) {
                copyPromise = Promise.resolve(legacyCopy());
            }
        } else {
            copyPromise = Promise.resolve(legacyCopy());
        }
        copyPromise.then(function (copied) {
            if (!copied) return;
            button.textContent = 'Copied';
            window.setTimeout(function () { button.textContent = 'Copy'; }, 1500);
        });
    });
})();


// ============================================================================
// WIDGET MANAGER (separate IIFE to maintain encapsulation)
// ============================================================================
(function() {
    'use strict';

    const WIDGETS_KEY = 'search_widgets';
    const WIDGET_SETTINGS_PREFIX = 'search_widget_';

    // Widget types with a real configuration form in showSettings() below.
    // Every other widget type falls through to that switch's default case
    // ("No settings available for this widget."), so its menu should not
    // offer a Settings entry that only opens a dead-end modal.
    const WIDGETS_WITH_SETTINGS = ['weather', 'clock', 'stocks', 'crypto', 'rss'];

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
        },
        // Additional instant answers per IDEA.md
        currency: {
            type: 'currency',
            name: 'Currency',
            icon: 'dollar-sign',
            category: 'data',
            refreshInterval: 1800000,
            render: renderCurrencyWidget
        },
        timezone: {
            type: 'timezone',
            name: 'Timezone',
            icon: 'globe',
            category: 'tool',
            refreshInterval: 60000,
            render: renderTimezoneWidget
        },
        translate: {
            type: 'translate',
            name: 'Translate',
            icon: 'language',
            category: 'data',
            render: renderTranslateWidget
        },
        wikipedia: {
            type: 'wikipedia',
            name: 'Wikipedia',
            icon: 'book',
            category: 'data',
            refreshInterval: 3600000,
            render: renderWikipediaWidget
        },
        tracking: {
            type: 'tracking',
            name: 'Package Tracking',
            icon: 'truck',
            category: 'data',
            render: renderTrackingWidget
        },
        nutrition: {
            type: 'nutrition',
            name: 'Nutrition',
            icon: 'apple-alt',
            category: 'data',
            render: renderNutritionWidget
        },
        qrcode: {
            type: 'qrcode',
            name: 'QR Code',
            icon: 'qrcode',
            category: 'tool',
            render: renderQRCodeWidget
        },
        timer: {
            type: 'timer',
            name: 'Timer',
            icon: 'stopwatch',
            category: 'tool',
            render: renderTimerWidget
        },
        lorem: {
            type: 'lorem',
            name: 'Lorem Ipsum',
            icon: 'align-left',
            category: 'tool',
            render: renderLoremWidget
        },
        dictionary: {
            type: 'dictionary',
            name: 'Dictionary',
            icon: 'spell-check',
            category: 'data',
            render: renderDictionaryWidget
        },
        ipaddress: {
            type: 'ipaddress',
            name: 'IP Address',
            icon: 'network-wired',
            category: 'tool',
            render: renderIPAddressWidget
        },
        colorpicker: {
            type: 'colorpicker',
            name: 'Color Picker',
            icon: 'palette',
            category: 'tool',
            render: renderColorPickerWidget
        }
    };

    // Read enabled widget types from the server-rendered DOM.
    // Returns an array of widget type strings in DOM order.
    function getEnabledWidgets() {
        var grid = document.getElementById('widget-grid');
        if (!grid) return [];
        var types = [];
        grid.querySelectorAll('[data-widget]').forEach(function(el) {
            if (el.dataset.widget) types.push(el.dataset.widget);
        });
        return types;
    }

    // Persist the widget list to the server-side cookie via AJAX POST.
    // Uses fetch so drag-and-drop reorder does not navigate away from the page.
    function saveEnabledWidgets(widgets) {
        var params = new URLSearchParams();
        var csrf = document.querySelector('meta[name="csrf-token"]');
        if (csrf) {
            params.append('csrf_token', csrf.getAttribute('content'));
        }
        widgets.forEach(function(wt) {
            params.append('widget', wt);
        });
        return fetch('/server/preferences/widgets', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: params.toString(),
            redirect: 'follow'
        }).then(function(response) {
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return true;
        }).catch(function(e) {
            console.error('Widget preference save failed:', e);
            return false;
        });
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
            if (result.ok) {
                // Widget data structure: { type, data, error, updated_at }
                // If there's an error in the widget data, return it for the renderer to handle
                if (result.data.error) {
                    return { error: result.data.error };
                }
                // Return the actual widget data (e.g., weather data, stocks data, etc.)
                return result.data.data;
            }
            throw new Error(result.message || 'Unknown error');
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
        var weatherEmpty = escapeHtml(t('widgets_ui.weather_empty', 'Set your city in widget settings'));
        var configureLabel = escapeHtml(t('widgets_ui.configure', 'Configure'));
        var weatherFeelsLike = escapeHtml(t('widgets_ui.weather_feels_like', 'Feels like'));
        var weatherHumidity = escapeHtml(t('widgets_ui.weather_humidity', 'humidity'));
        if (!data || data.error) {
            container.innerHTML =
                '<div class="widget-placeholder">' +
                    '<div class="widget-placeholder-icon">&#9925;</div>' +
                    '<div class="widget-placeholder-text">' + weatherEmpty + '</div>' +
                    '<button class="widget-settings-btn" data-widget-settings="weather">' + configureLabel + '</button>' +
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
        var unitLetter = data.units === 'metric' ? 'C' : 'F';

        container.innerHTML =
            '<div class="weather-widget">' +
                '<div class="weather-main">' +
                    '<span class="weather-icon">' + icon + '</span>' +
                    '<span class="weather-temp">' + temp + '&deg;' + unitLetter + '</span>' +
                '</div>' +
                '<div class="weather-details">' +
                    '<div class="weather-location">' + escapeHtml(data.location) + '</div>' +
                    '<div class="weather-description">' + escapeHtml(data.description) + '</div>' +
                    '<div class="weather-extra">' + weatherFeelsLike + ' ' + feelsLike + '&deg;' + unitLetter + ' &middot; ' + data.humidity + '% ' + weatherHumidity + '</div>' +
                '</div>' +
            '</div>';
    }

    function renderQuickLinksWidget(container, data, settings) {
        var links = settings.links || [];
        var quicklinksEmpty = escapeHtml(t('widgets_ui.quicklinks_empty', 'No links yet'));
        var quicklinksAdd = escapeHtml(t('widgets_ui.quicklinks_add', 'Add Link'));

        if (links.length === 0) {
            container.innerHTML =
                '<div class="widget-placeholder">' +
                    '<div class="widget-placeholder-icon" aria-hidden="true">&#128279;</div>' +
                    '<div class="widget-placeholder-text">' + quicklinksEmpty + '</div>' +
                    '<button class="widget-add-btn" data-widget-action="add-quicklink">+ ' + quicklinksAdd + '</button>' +
                '</div>';
            return;
        }

        var html = '<div class="quicklinks-widget">';
        links.forEach(function(link) {
            var hostname = '';
            try { hostname = new URL(link.url).hostname; } catch(e) {}
            var displayName = link.name || hostname || link.url;
            html += '<a href="' + escapeHtml(link.url) + '" class="quicklink-item" title="' + escapeHtml(displayName) + '">' +
                '<img src="https://www.google.com/s2/favicons?domain=' + encodeURIComponent(hostname) + '&sz=32" alt="" class="quicklink-favicon" data-quicklink-favicon>' +
                '<span class="quicklink-name">' + escapeHtml(displayName) + '</span>' +
            '</a>';
        });
        html += '<button class="quicklink-add" data-widget-action="add-quicklink" title="' + quicklinksAdd + '">+</button></div>';
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
        var notesPlaceholder = escapeHtml(t('widgets_ui.notes_placeholder', 'Type your notes here...'));
        container.innerHTML =
            '<div class="notes-widget">' +
                '<textarea class="notes-textarea" data-widget-notes placeholder="' + notesPlaceholder + '">' + escapeHtml(notes) + '</textarea>' +
            '</div>';
    }

    function renderCalendarWidget(container) {
        var now = new Date();
        var month = now.getMonth();
        var year = now.getFullYear();
        var today = now.getDate();

        var firstDay = new Date(year, month, 1).getDay();
        var daysInMonth = new Date(year, month + 1, 0).getDate();

        // Use Intl.DateTimeFormat so month and day names follow the user's locale
        var lang = document.documentElement.lang || navigator.language || 'en';
        var monthName = new Intl.DateTimeFormat(lang, { month: 'long', year: 'numeric' }).format(now);
        var dayFmt = new Intl.DateTimeFormat(lang, { weekday: 'short' });
        // Build Sun-Sat day abbreviations using a reference week starting on 2000-01-02 (Sunday)
        var dayHeaders = '';
        for (var di = 0; di < 7; di++) {
            var refDay = new Date(2000, 0, 2 + di);
            dayHeaders += '<span>' + escapeHtml(dayFmt.format(refDay).slice(0, 2)) + '</span>';
        }

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
                '<div class="calendar-header">' + escapeHtml(monthName) + '</div>' +
                '<div class="calendar-weekdays">' + dayHeaders + '</div>' +
                '<div class="calendar-days">' + days + '</div>' +
            '</div>';
    }

    function renderConverterWidget(container, data, settings) {
        var category = settings.defaultCategory || 'length';
        var converterLength = escapeHtml(t('widgets_ui.converter_length', 'Length'));
        var converterWeight = escapeHtml(t('widgets_ui.converter_weight', 'Weight'));
        var converterTemperature = escapeHtml(t('widgets_ui.converter_temperature', 'Temperature'));
        var converterVolume = escapeHtml(t('widgets_ui.converter_volume', 'Volume'));
        container.innerHTML =
            '<div class="converter-widget">' +
                '<select id="converter-category" data-converter-category>' +
                    '<option value="length"' + (category === 'length' ? ' selected' : '') + '>' + converterLength + '</option>' +
                    '<option value="weight"' + (category === 'weight' ? ' selected' : '') + '>' + converterWeight + '</option>' +
                    '<option value="temperature"' + (category === 'temperature' ? ' selected' : '') + '>' + converterTemperature + '</option>' +
                    '<option value="volume"' + (category === 'volume' ? ' selected' : '') + '>' + converterVolume + '</option>' +
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
        var newsEmpty = escapeHtml(t('widgets_ui.news_empty', 'No news available'));
        if (!data || !data.items || data.items.length === 0) {
            container.innerHTML = '<div class="widget-placeholder"><div class="widget-placeholder-text">' + newsEmpty + '</div></div>';
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
        var stocksEmpty = escapeHtml(t('widgets_ui.stocks_empty', 'Configure stock symbols'));
        var configureLabel = escapeHtml(t('widgets_ui.configure', 'Configure'));
        if (!data || !data.symbols || data.symbols.length === 0) {
            container.innerHTML =
                '<div class="widget-placeholder">' +
                    '<div class="widget-placeholder-text">' + stocksEmpty + '</div>' +
                    '<button class="widget-settings-btn" data-widget-settings="stocks">' + configureLabel + '</button>' +
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
        var cryptoLoading = escapeHtml(t('widgets_ui.crypto_loading', 'Loading crypto prices...'));
        if (!data || !data.coins || data.coins.length === 0) {
            container.innerHTML = '<div class="widget-placeholder"><div class="widget-placeholder-text">' + cryptoLoading + '</div></div>';
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
        var sportsEmpty = escapeHtml(t('widgets_ui.sports_empty', 'No games today'));
        var sportsVersus = escapeHtml(t('widgets_ui.sports_versus', 'vs'));
        if (!data || !data.games || data.games.length === 0) {
            container.innerHTML = '<div class="widget-placeholder"><div class="widget-placeholder-text">' + sportsEmpty + '</div></div>';
            return;
        }

        var html = '<div class="sports-widget">';
        data.games.slice(0, 3).forEach(function(game) {
            html += '<div class="sports-game">' +
                '<div class="sports-teams"><span>' + escapeHtml(game.home_team) + '</span><span class="sports-vs">' + sportsVersus + '</span><span>' + escapeHtml(game.away_team) + '</span></div>' +
                '<div class="sports-score">' + game.home_score + ' - ' + game.away_score + '</div>' +
                '<div class="sports-status">' + escapeHtml(game.status) + '</div>' +
            '</div>';
        });
        html += '</div>';
        container.innerHTML = html;
    }

    function renderRSSWidget(container, data, settings) {
        var feeds = settings.feeds || [];
        var rssEmpty = escapeHtml(t('widgets_ui.rss_empty', 'No RSS feeds configured'));
        var rssAddFeed = escapeHtml(t('widgets_ui.rss_add_feed', 'Add Feed'));
        var rssLoading = escapeHtml(t('widgets_ui.rss_loading', 'Loading feeds...'));

        if (feeds.length === 0) {
            container.innerHTML =
                '<div class="widget-placeholder">' +
                    '<div class="widget-placeholder-text">' + rssEmpty + '</div>' +
                    '<button class="widget-settings-btn" data-widget-settings="rss">' + rssAddFeed + '</button>' +
                '</div>';
            return;
        }

        if (!data || !data.items || data.items.length === 0) {
            container.innerHTML = '<div class="widget-placeholder"><div class="widget-placeholder-text">' + rssLoading + '</div></div>';
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

    // Additional instant answer render functions per IDEA.md

    function renderCurrencyWidget(container, data, settings) {
        var currencyConvert = escapeHtml(t('widgets_ui.currency_convert', 'Convert'));
        container.innerHTML =
            '<div class="currency-widget">' +
                '<div class="currency-row">' +
                    '<input type="number" id="currency-amount" data-currency-amount value="1" min="0" step="any">' +
                    '<select id="currency-from" data-currency-from>' +
                        '<option value="USD">USD</option>' +
                        '<option value="EUR">EUR</option>' +
                        '<option value="GBP">GBP</option>' +
                        '<option value="JPY">JPY</option>' +
                        '<option value="CNY">CNY</option>' +
                        '<option value="AUD">AUD</option>' +
                        '<option value="CAD">CAD</option>' +
                        '<option value="CHF">CHF</option>' +
                        '<option value="INR">INR</option>' +
                    '</select>' +
                '</div>' +
                '<div class="currency-equals">=</div>' +
                '<div class="currency-row">' +
                    '<input type="text" id="currency-result" readonly placeholder="...">' +
                    '<select id="currency-to" data-currency-to>' +
                        '<option value="EUR" selected>EUR</option>' +
                        '<option value="USD">USD</option>' +
                        '<option value="GBP">GBP</option>' +
                        '<option value="JPY">JPY</option>' +
                        '<option value="CNY">CNY</option>' +
                        '<option value="AUD">AUD</option>' +
                        '<option value="CAD">CAD</option>' +
                        '<option value="CHF">CHF</option>' +
                        '<option value="INR">INR</option>' +
                    '</select>' +
                '</div>' +
                '<button class="currency-convert-btn" data-action="convert-currency">' + currencyConvert + '</button>' +
            '</div>';
    }

    function renderTimezoneWidget(container, data, settings) {
        var timezones = settings.timezones || ['America/New_York', 'Europe/London', 'Asia/Tokyo'];

        function update() {
            var html = '<div class="timezone-widget">';
            timezones.forEach(function(tz) {
                var now = new Date();
                var timeStr = now.toLocaleTimeString('en-US', { timeZone: tz, hour: '2-digit', minute: '2-digit', hour12: true });
                var cityName = tz.split('/')[1].replace(/_/g, ' ');
                html += '<div class="timezone-item">' +
                    '<span class="timezone-city">' + cityName + '</span>' +
                    '<span class="timezone-time">' + timeStr + '</span>' +
                '</div>';
            });
            html += '</div>';
            container.innerHTML = html;
        }

        update();
        return setInterval(update, 60000);
    }

    function renderTranslateWidget(container, data, settings) {
        var translateAutoDetect = escapeHtml(t('widgets_ui.translate_auto_detect', 'Auto-detect'));
        var translateLanguageEnglish = escapeHtml(t('widgets_ui.translate_language_english', 'English'));
        var translateLanguageSpanish = escapeHtml(t('widgets_ui.translate_language_spanish', 'Spanish'));
        var translateLanguageFrench = escapeHtml(t('widgets_ui.translate_language_french', 'French'));
        var translateLanguageGerman = escapeHtml(t('widgets_ui.translate_language_german', 'German'));
        var translateLanguageItalian = escapeHtml(t('widgets_ui.translate_language_italian', 'Italian'));
        var translateLanguagePortuguese = escapeHtml(t('widgets_ui.translate_language_portuguese', 'Portuguese'));
        var translateLanguageRussian = escapeHtml(t('widgets_ui.translate_language_russian', 'Russian'));
        var translateLanguageJapanese = escapeHtml(t('widgets_ui.translate_language_japanese', 'Japanese'));
        var translateLanguageKorean = escapeHtml(t('widgets_ui.translate_language_korean', 'Korean'));
        var translateLanguageChinese = escapeHtml(t('widgets_ui.translate_language_chinese', 'Chinese'));
        var translateInputPlaceholder = escapeHtml(t('widgets_ui.translate_input_placeholder', 'Enter text to translate...'));
        container.innerHTML =
            '<div class="translate-widget">' +
                '<div class="translate-row">' +
                    '<select id="translate-from" data-translate-from>' +
                        '<option value="auto">' + translateAutoDetect + '</option>' +
                        '<option value="en">' + translateLanguageEnglish + '</option>' +
                        '<option value="es">' + translateLanguageSpanish + '</option>' +
                        '<option value="fr">' + translateLanguageFrench + '</option>' +
                        '<option value="de">' + translateLanguageGerman + '</option>' +
                        '<option value="it">' + translateLanguageItalian + '</option>' +
                        '<option value="pt">' + translateLanguagePortuguese + '</option>' +
                        '<option value="ru">' + translateLanguageRussian + '</option>' +
                        '<option value="ja">' + translateLanguageJapanese + '</option>' +
                        '<option value="ko">' + translateLanguageKorean + '</option>' +
                        '<option value="zh">' + translateLanguageChinese + '</option>' +
                    '</select>' +
                    '<span class="translate-arrow">&#8594;</span>' +
                    '<select id="translate-to" data-translate-to>' +
                        '<option value="en">' + translateLanguageEnglish + '</option>' +
                        '<option value="es">' + translateLanguageSpanish + '</option>' +
                        '<option value="fr">' + translateLanguageFrench + '</option>' +
                        '<option value="de">' + translateLanguageGerman + '</option>' +
                        '<option value="it">' + translateLanguageItalian + '</option>' +
                        '<option value="pt">' + translateLanguagePortuguese + '</option>' +
                        '<option value="ru">' + translateLanguageRussian + '</option>' +
                        '<option value="ja">' + translateLanguageJapanese + '</option>' +
                        '<option value="ko">' + translateLanguageKorean + '</option>' +
                        '<option value="zh">' + translateLanguageChinese + '</option>' +
                    '</select>' +
                '</div>' +
                '<textarea id="translate-input" data-translate-input placeholder="' + translateInputPlaceholder + '" rows="3"></textarea>' +
                '<div id="translate-output" class="translate-output"></div>' +
            '</div>';
    }

    function renderWikipediaWidget(container, data, settings) {
        var wikipediaSearchPlaceholder = escapeHtml(t('widgets_ui.wikipedia_search_placeholder', 'Search Wikipedia...'));
        var wikipediaEnterTopic = escapeHtml(t('widgets_ui.wikipedia_enter_topic', 'Enter a topic to search'));
        var wikipediaReadMore = escapeHtml(t('widgets_ui.wikipedia_read_more', 'Read more'));
        if (!data || data.error) {
            container.innerHTML =
                '<div class="wikipedia-widget">' +
                    '<input type="text" id="wiki-search" data-wiki-search placeholder="' + wikipediaSearchPlaceholder + '">' +
                    '<div id="wiki-result" class="wiki-placeholder">' + wikipediaEnterTopic + '</div>' +
                '</div>';
            return;
        }

        container.innerHTML =
            '<div class="wikipedia-widget">' +
                '<input type="text" id="wiki-search" data-wiki-search placeholder="' + wikipediaSearchPlaceholder + '">' +
                '<div class="wiki-result">' +
                    (data.thumbnail ? '<img src="' + escapeHtml(data.thumbnail) + '" class="wiki-thumb" alt="">' : '') +
                    '<div class="wiki-content">' +
                        '<h4 class="wiki-title">' + escapeHtml(data.title) + '</h4>' +
                        '<p class="wiki-extract">' + escapeHtml(data.extract ? data.extract.substring(0, 200) + '...' : '') + '</p>' +
                        '<a href="' + escapeHtml(data.url) + '" target="_blank" rel="noopener" class="wiki-link">' + wikipediaReadMore + '</a>' +
                    '</div>' +
                '</div>' +
            '</div>';
    }

    function renderTrackingWidget(container, data, settings) {
        var trackingPlaceholder = escapeHtml(t('widgets_ui.tracking_placeholder', 'Enter tracking number...'));
        var trackingButton = escapeHtml(t('widgets_ui.tracking_button', 'Track'));
        var trackingStatusPlaceholder = escapeHtml(t('widgets_ui.tracking_status_placeholder', 'Enter a tracking number to check status'));
        container.innerHTML =
            '<div class="tracking-widget">' +
                '<div class="tracking-input-row">' +
                    '<input type="text" id="tracking-number" data-tracking-number placeholder="' + trackingPlaceholder + '">' +
                    '<button data-action="track-package">' + trackingButton + '</button>' +
                '</div>' +
                '<div id="tracking-result" class="tracking-placeholder">' + trackingStatusPlaceholder + '</div>' +
            '</div>';
    }

    function renderNutritionWidget(container, data, settings) {
        var nutritionSearchPlaceholder = escapeHtml(t('widgets_ui.nutrition_search_placeholder', 'Search food...'));
        var nutritionEmptyPlaceholder = escapeHtml(t('widgets_ui.nutrition_empty_placeholder', 'Search for a food item'));
        var nutritionServingSize = escapeHtml(t('widgets_ui.nutrition_serving_size', 'Per 100g'));
        if (!data || data.error) {
            container.innerHTML =
                '<div class="nutrition-widget">' +
                    '<input type="text" id="nutrition-search" data-nutrition-search placeholder="' + nutritionSearchPlaceholder + '">' +
                    '<div class="nutrition-placeholder">' + nutritionEmptyPlaceholder + '</div>' +
                '</div>';
            return;
        }

        var html = '<div class="nutrition-widget">' +
            '<input type="text" id="nutrition-search" data-nutrition-search placeholder="' + nutritionSearchPlaceholder + '">' +
            '<div class="nutrition-result">' +
                '<h4 class="nutrition-name">' + escapeHtml(data.name) + '</h4>' +
                (data.serving_size ? '<div class="nutrition-serving">' + nutritionServingSize + '</div>' : '') +
                '<div class="nutrition-facts">';

        (data.nutrients || []).slice(0, 8).forEach(function(n) {
            html += '<div class="nutrition-row">' +
                '<span>' + escapeHtml(n.name) + '</span>' +
                '<span>' + n.amount.toFixed(1) + ' ' + n.unit + '</span>' +
            '</div>';
        });

        html += '</div></div></div>';
        container.innerHTML = html;
    }

    function renderQRCodeWidget(container, data, settings) {
        var qrTextPlaceholder = escapeHtml(t('widgets_ui.qr_text_placeholder', 'Enter text or URL...'));
        var qrEmptyPlaceholder = escapeHtml(t('widgets_ui.qr_empty_placeholder', 'Enter text to generate QR code'));
        var qrGenerateButton = escapeHtml(t('widgets_ui.qr_generate_button', 'Generate QR Code'));
        container.innerHTML =
            '<div class="qrcode-widget">' +
                '<input type="text" id="qr-text" data-qr-text placeholder="' + qrTextPlaceholder + '">' +
                '<div id="qr-canvas" class="qr-canvas">' +
                    '<div class="qr-placeholder">' + qrEmptyPlaceholder + '</div>' +
                '</div>' +
                '<button data-action="generate-qr">' + qrGenerateButton + '</button>' +
            '</div>';
    }

    function renderTimerWidget(container, data, settings) {
        var timerStart = escapeHtml(t('widgets_ui.timer_start', 'Start'));
        var timerPause = escapeHtml(t('widgets_ui.timer_pause', 'Pause'));
        var timerReset = escapeHtml(t('widgets_ui.timer_reset', 'Reset'));
        var timerPresetOneMinute = escapeHtml(t('widgets_ui.timer_preset_1_minute', '1 min'));
        var timerPresetFiveMinutes = escapeHtml(t('widgets_ui.timer_preset_5_minutes', '5 min'));
        var timerPresetTenMinutes = escapeHtml(t('widgets_ui.timer_preset_10_minutes', '10 min'));
        var timerPresetTwentyFiveMinutes = escapeHtml(t('widgets_ui.timer_preset_25_minutes', '25 min'));
        container.innerHTML =
            '<div class="timer-widget">' +
                '<div class="timer-display" id="timer-display">00:00:00</div>' +
                '<div class="timer-buttons">' +
                    '<button data-timer-action="start">' + timerStart + '</button>' +
                    '<button data-timer-action="pause">' + timerPause + '</button>' +
                    '<button data-timer-action="reset">' + timerReset + '</button>' +
                '</div>' +
                '<div class="timer-presets">' +
                    '<button data-timer-preset="60">' + timerPresetOneMinute + '</button>' +
                    '<button data-timer-preset="300">' + timerPresetFiveMinutes + '</button>' +
                    '<button data-timer-preset="600">' + timerPresetTenMinutes + '</button>' +
                    '<button data-timer-preset="1500">' + timerPresetTwentyFiveMinutes + '</button>' +
                '</div>' +
            '</div>';
    }

    function renderLoremWidget(container, data, settings) {
        var loremText = 'Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.';
        var loremParagraphs = escapeHtml(t('widgets_ui.lorem_paragraphs', 'Paragraphs'));
        var loremSentences = escapeHtml(t('widgets_ui.lorem_sentences', 'Sentences'));
        var loremWords = escapeHtml(t('widgets_ui.lorem_words', 'Words'));
        var loremGenerate = escapeHtml(t('widgets_ui.lorem_generate', 'Generate'));
        var loremCopy = escapeHtml(t('widgets_ui.lorem_copy', 'Copy'));

        container.innerHTML =
            '<div class="lorem-widget">' +
                '<div class="lorem-options">' +
                    '<select id="lorem-type" data-lorem-type>' +
                        '<option value="paragraphs">' + loremParagraphs + '</option>' +
                        '<option value="sentences">' + loremSentences + '</option>' +
                        '<option value="words">' + loremWords + '</option>' +
                    '</select>' +
                    '<input type="number" id="lorem-count" data-lorem-count value="3" min="1" max="20">' +
                    '<button data-action="generate-lorem">' + loremGenerate + '</button>' +
                '</div>' +
                '<div id="lorem-output" class="lorem-output">' + loremText + '</div>' +
                '<button class="lorem-copy" data-action="copy-lorem">' + loremCopy + '</button>' +
            '</div>';
    }

    function renderDictionaryWidget(container, data, settings) {
        var dictionaryWordPlaceholder = escapeHtml(t('widgets_ui.dictionary_word_placeholder', 'Enter a word...'));
        var dictionaryEmptyPlaceholder = escapeHtml(t('widgets_ui.dictionary_empty_placeholder', 'Search for a word definition'));
        if (!data || data.error) {
            container.innerHTML =
                '<div class="dictionary-widget">' +
                    '<input type="text" id="dict-word" data-dict-word placeholder="' + dictionaryWordPlaceholder + '">' +
                    '<div class="dict-placeholder">' + dictionaryEmptyPlaceholder + '</div>' +
                '</div>';
            return;
        }

        var html = '<div class="dictionary-widget">' +
            '<input type="text" id="dict-word" data-dict-word placeholder="' + dictionaryWordPlaceholder + '">' +
            '<div class="dict-result">' +
                '<h4 class="dict-word">' + escapeHtml(data.word) + '</h4>' +
                (data.phonetic ? '<span class="dict-phonetic">' + escapeHtml(data.phonetic) + '</span>' : '');

        (data.meanings || []).slice(0, 2).forEach(function(m) {
            html += '<div class="dict-meaning">' +
                '<span class="dict-pos">' + escapeHtml(m.part_of_speech) + '</span>';
            (m.definitions || []).slice(0, 2).forEach(function(d) {
                html += '<p class="dict-def">' + escapeHtml(d.definition) + '</p>';
                if (d.example) {
                    html += '<p class="dict-example">"' + escapeHtml(d.example) + '"</p>';
                }
            });
            html += '</div>';
        });

        html += '</div></div>';
        container.innerHTML = html;
    }

    function renderIPAddressWidget(container, data, settings) {
        var ipLabel = escapeHtml(t('widgets_ui.ip_label', 'Your IP Address'));
        var ipCopy = escapeHtml(t('widgets_ui.ip_copy', 'Copy'));
        var ipError = escapeHtml(t('widgets_ui.ip_error', 'Unable to detect IP address'));
        // Get IP info - this is client-side detectable
        fetch('https://api.ipify.org?format=json')
            .then(function(r) { return r.json(); })
            .then(function(ipData) {
                container.innerHTML =
                    '<div class="ip-widget">' +
                        '<div class="ip-label">' + ipLabel + '</div>' +
                        '<div class="ip-value">' + escapeHtml(ipData.ip) + '</div>' +
                        '<button class="ip-copy" data-action="copy-ip" data-ip="' + escapeHtml(ipData.ip) + '">' + ipCopy + '</button>' +
                    '</div>';
            })
            .catch(function() {
                container.innerHTML =
                    '<div class="ip-widget">' +
                        '<div class="ip-error">' + ipError + '</div>' +
                    '</div>';
            });
    }

    function renderColorPickerWidget(container, data, settings) {
        var currentColor = settings.color || '#1e90ff';
        var colorHEX = escapeHtml(t('widgets_ui.color_hex', 'HEX'));
        var colorRGB = escapeHtml(t('widgets_ui.color_rgb', 'RGB'));
        var colorHSL = escapeHtml(t('widgets_ui.color_hsl', 'HSL'));
        var colorCopyHEX = escapeHtml(t('widgets_ui.color_copy_hex', 'Copy HEX'));

        container.innerHTML =
            '<div class="colorpicker-widget">' +
                '<input type="color" id="color-input" data-color-input value="' + currentColor + '">' +
                '<div class="color-values">' +
                    '<div class="color-row"><label>' + colorHEX + '</label><input type="text" id="color-hex" data-color-hex value="' + currentColor + '" readonly></div>' +
                    '<div class="color-row"><label>' + colorRGB + '</label><input type="text" id="color-rgb" data-color-rgb readonly></div>' +
                    '<div class="color-row"><label>' + colorHSL + '</label><input type="text" id="color-hsl" data-color-hsl readonly></div>' +
                '</div>' +
                '<button data-action="copy-color">' + colorCopyHEX + '</button>' +
            '</div>';

        // Update color values
        updateColorValues(currentColor);
    }

    function updateColorValues(hex) {
        var hexInput = document.getElementById('color-hex');
        var rgbInput = document.getElementById('color-rgb');
        var hslInput = document.getElementById('color-hsl');

        if (hexInput) hexInput.value = hex;
        if (rgbInput) {
            var r = parseInt(hex.substr(1, 2), 16);
            var g = parseInt(hex.substr(3, 2), 16);
            var b = parseInt(hex.substr(5, 2), 16);
            rgbInput.value = 'rgb(' + r + ', ' + g + ', ' + b + ')';
        }
        if (hslInput) {
            var rgb = hexToRgb(hex);
            var hsl = rgbToHsl(rgb.r, rgb.g, rgb.b);
            hslInput.value = 'hsl(' + Math.round(hsl.h) + ', ' + Math.round(hsl.s) + '%, ' + Math.round(hsl.l) + '%)';
        }
    }

    function hexToRgb(hex) {
        var result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
        return result ? {
            r: parseInt(result[1], 16),
            g: parseInt(result[2], 16),
            b: parseInt(result[3], 16)
        } : { r: 0, g: 0, b: 0 };
    }

    function rgbToHsl(r, g, b) {
        r /= 255; g /= 255; b /= 255;
        var max = Math.max(r, g, b), min = Math.min(r, g, b);
        var h, s, l = (max + min) / 2;

        if (max === min) {
            h = s = 0;
        } else {
            var d = max - min;
            s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
            switch (max) {
                case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break;
                case g: h = ((b - r) / d + 2) / 6; break;
                case b: h = ((r - g) / d + 4) / 6; break;
            }
        }

        return { h: h * 360, s: s * 100, l: l * 100 };
    }

    // Widget Manager object
    var WidgetManager = {
        intervals: {},
        defaultWidgets: ['clock', 'weather', 'quicklinks', 'calculator'],

        init: function() {
            var grid = document.getElementById('widget-grid');
            if (!grid) return;

            // Server renders widget shells; activate each one in DOM order.
            var self = this;
            grid.querySelectorAll('[data-widget]').forEach(function(el) {
                var widgetType = el.dataset.widget;
                if (widgetType && WIDGETS[widgetType]) {
                    self.initWidget(widgetType);
                }
            });
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

                // Widget collapse/expand button
                if (target.matches('.widget-collapse')) {
                    var widgetEl = target.closest('.widget');
                    if (widgetEl) {
                        var expanded = widgetEl.classList.toggle('widget-collapsed') === false;
                        target.setAttribute('aria-expanded', String(expanded));
                    }
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

            // Widget drag and drop
            self.initDragAndDrop();
        },

        // Drag and drop state
        dragState: {
            dragging: null,
            placeholder: null
        },

        initDragAndDrop: function() {
            var self = this;
            var grid = document.getElementById('widget-grid');
            if (!grid) return;

            // Dragstart - when user starts dragging a widget
            grid.addEventListener('dragstart', function(e) {
                var widget = window.closestFrom(e.target, '.widget');
                if (!widget) return;

                self.dragState.dragging = widget;
                widget.classList.add('widget-dragging');

                // Set drag data
                e.dataTransfer.effectAllowed = 'move';
                e.dataTransfer.setData('text/plain', widget.dataset.widget);

                // Create placeholder
                self.dragState.placeholder = document.createElement('div');
                self.dragState.placeholder.className = 'widget-placeholder-drop';
                self.dragState.placeholder.setAttribute('data-drop-label', t('widgets_ui.drop_here', 'Drop here'));

                // Delay adding placeholder to avoid visual glitch
                setTimeout(function() {
                    if (self.dragState.dragging) {
                        widget.classList.add('widget-dragging-active');
                    }
                }, 0);
            });

            // Dragover - while dragging over the grid
            grid.addEventListener('dragover', function(e) {
                e.preventDefault();
                e.dataTransfer.dropEffect = 'move';

                var dragging = self.dragState.dragging;
                if (!dragging) return;

                var afterElement = self.getDragAfterElement(grid, e.clientY);
                var placeholder = self.dragState.placeholder;

                if (afterElement == null) {
                    if (placeholder.parentNode !== grid) {
                        grid.appendChild(placeholder);
                    }
                } else if (afterElement !== dragging && afterElement !== placeholder) {
                    grid.insertBefore(placeholder, afterElement);
                }
            });

            // Dragenter - visual feedback when entering a drop zone
            grid.addEventListener('dragenter', function(e) {
                e.preventDefault();
            });

            // Dragleave - when leaving the grid
            grid.addEventListener('dragleave', function(e) {
                // Only handle if leaving the grid entirely
                if (!grid.contains(e.relatedTarget)) {
                    if (self.dragState.placeholder && self.dragState.placeholder.parentNode) {
                        self.dragState.placeholder.remove();
                    }
                }
            });

            // Drop - when widget is dropped
            grid.addEventListener('drop', function(e) {
                e.preventDefault();
                var dragging = self.dragState.dragging;
                var placeholder = self.dragState.placeholder;

                if (!dragging) return;

                // Insert the widget at the placeholder position
                if (placeholder && placeholder.parentNode === grid) {
                    grid.insertBefore(dragging, placeholder);
                    placeholder.remove();
                }

                // Get new order and save
                self.saveWidgetOrder();
            });

            // Dragend - cleanup when drag ends (whether dropped or cancelled)
            grid.addEventListener('dragend', function(e) {
                var dragging = self.dragState.dragging;
                if (dragging) {
                    dragging.classList.remove('widget-dragging', 'widget-dragging-active');
                }

                if (self.dragState.placeholder && self.dragState.placeholder.parentNode) {
                    self.dragState.placeholder.remove();
                }

                self.dragState.dragging = null;
                self.dragState.placeholder = null;
            });
        },

        // Helper to find the element after which we should insert
        getDragAfterElement: function(container, y) {
            var draggableElements = Array.from(container.querySelectorAll('.widget:not(.widget-dragging-active)'));

            return draggableElements.reduce(function(closest, child) {
                var box = child.getBoundingClientRect();
                var offset = y - box.top - box.height / 2;

                if (offset < 0 && offset > closest.offset) {
                    return { offset: offset, element: child };
                } else {
                    return closest;
                }
            }, { offset: Number.NEGATIVE_INFINITY }).element;
        },

        // Save the current widget order based on DOM position.
        // POSTs new order to server so it persists across page loads.
        saveWidgetOrder: function() {
            var grid = document.getElementById('widget-grid');
            if (!grid) return;

            var order = [];
            grid.querySelectorAll('[data-widget]').forEach(function(el) {
                if (el.dataset.widget) order.push(el.dataset.widget);
            });

            if (order.length > 0) {
                saveEnabledWidgets(order);
            }
        },

        renderWidgetGrid: function(enabledWidgets) {
            var self = this;
            var grid = document.getElementById('widget-grid');
            if (!grid) return;
            var widgetOptionsTitle = escapeHtml(t('widgets_ui.widget_options', 'Widget options'));
            var widgetLoadingText = escapeHtml(t('common.loading', 'Loading...'));

            Object.values(this.intervals).forEach(clearInterval);
            this.intervals = {};

            grid.innerHTML = '';

            enabledWidgets.forEach(function(widgetType) {
                if (!WIDGETS[widgetType]) return;

                var widget = WIDGETS[widgetType];
                var div = document.createElement('div');
                div.className = 'widget widget-' + widgetType;
                div.dataset.widget = widgetType;
                div.draggable = true;
                div.innerHTML =
                    '<div class="widget-header">' +
                        '<span class="widget-title">' + widget.name + '</span>' +
                        '<button class="widget-menu" title="' + widgetOptionsTitle + '">&#8942;</button>' +
                    '</div>' +
                    '<div class="widget-content" id="widget-content-' + widgetType + '">' +
                        '<div class="widget-loading">' + widgetLoadingText + '</div>' +
                    '</div>';
                grid.appendChild(div);

                self.initWidget(widgetType);
            });
        },

        initWidget: async function(widgetType) {
            var self = this;
            var widget = WIDGETS[widgetType];
            // Support both server-rendered (widget-body-*) and legacy (widget-content-*) IDs.
            var container = document.getElementById('widget-body-' + widgetType) ||
                            document.getElementById('widget-content-' + widgetType);
            if (!container || !widget) return;

            var settings = getWidgetSettings(widgetType);

            // Units are a single global preference (set on the preferences
            // page), never a per-widget override - avoids two redundant
            // controls for the same setting. getPreferredUnits is declared
            // in a different top-level IIFE and is only reachable here via
            // its window export.
            if (widgetType === 'weather') {
                settings.units = window.getPreferredUnits();
            }

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
            var enabled = getEnabledWidgets();
            var idx = enabled.indexOf(widgetType);
            if (idx >= 0) {
                enabled.splice(idx, 1);
            } else {
                enabled.push(widgetType);
            }
            // POST updated list to server; page reloads with new cookie-driven state.
            saveEnabledWidgets(enabled);
        },

        removeWidget: function(widgetType) {
            var enabled = getEnabledWidgets();
            var idx = enabled.indexOf(widgetType);
            if (idx < 0) return;

            enabled.splice(idx, 1);
            var widget = document.querySelector('[data-widget="' + widgetType + '"]');
            var parent = widget && widget.parentNode;
            var next = widget && widget.nextSibling;
            if (widget) widget.remove();
            saveEnabledWidgets(enabled).then(function(saved) {
                if (!saved) {
                    if (widget && parent) parent.insertBefore(widget, next && next.parentNode === parent ? next : null);
                    return;
                }
                clearInterval(this.intervals[widgetType]);
                clearInterval(this.intervals[widgetType + '_refresh']);
                delete this.intervals[widgetType];
                delete this.intervals[widgetType + '_refresh'];
            }.bind(this));
        },

        getWidgetDisplayName: function(widgetType) {
            switch (widgetType) {
                case 'weather':
                    return t('widgets.weather', WIDGETS[widgetType]?.name || widgetType);
                case 'clock':
                    return t('widgets.clock', WIDGETS[widgetType]?.name || widgetType);
                case 'stocks':
                    return t('widgets_ui.widget_name_stocks', WIDGETS[widgetType]?.name || widgetType);
                case 'crypto':
                    return t('widgets_ui.widget_name_crypto', WIDGETS[widgetType]?.name || widgetType);
                case 'rss':
                    return t('widgets_ui.widget_name_rss', WIDGETS[widgetType]?.name || widgetType);
                default:
                    return WIDGETS[widgetType]?.name || widgetType;
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
            var refreshLabel = escapeHtml(t('widgets_ui.menu_refresh', 'Refresh'));
            var settingsLabel = escapeHtml(t('nav.settings', 'Settings'));
            var removeLabel = escapeHtml(t('widgets_ui.menu_remove', 'Remove'));
            var settingsButton = WIDGETS_WITH_SETTINGS.indexOf(widgetType) >= 0
                ? '<button data-menu-action="settings">' + settingsLabel + '</button>'
                : '';
            menu.innerHTML =
                '<button data-menu-action="refresh">' + refreshLabel + '</button>' +
                settingsButton +
                '<button data-menu-action="remove" class="danger">' + removeLabel + '</button>';

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
            var settingsTitle = escapeHtml(this.getWidgetDisplayName(widgetType) + ' ' + t('nav.settings', 'Settings'));
            var cancelLabel = escapeHtml(t('common.cancel', 'Cancel'));
            var saveLabel = escapeHtml(t('common.save', 'Save'));

            switch (widgetType) {
                case 'weather':
                    var useGeolocation = settings.useGeolocation || false;
                    var useLocationLabel = escapeHtml(t('widgets_ui.settings_weather_use_current_location', 'Use current location (requires HTTPS)'));
                    var usingLocationText = escapeHtml(t('widgets_ui.settings_weather_using_location', 'Using your location'));
                    var cityHintText = escapeHtml(t('widgets_ui.settings_weather_city_hint', 'Enter a city (e.g., "Albany, NY" or "Paris, France")'));
                    var cityLabel = escapeHtml(t('widgets_ui.settings_weather_city_label', 'City'));
                    var cityPlaceholder = escapeHtml(t('widgets_ui.settings_weather_city_placeholder', 'Albany, NY or Paris, France'));
                    content =
                        '<div class="setting-group">' +
                            '<label class="setting-checkbox"><input type="checkbox" id="setting-use-location"' + (useGeolocation ? ' checked' : '') + '> ' + useLocationLabel + '</label>' +
                            '<p class="setting-help" id="geolocation-status">' + (useGeolocation && settings.lat ? usingLocationText : cityHintText) + '</p>' +
                        '</div>' +
                        '<label>' + cityLabel + ':<input type="text" id="setting-city" value="' + escapeHtml(settings.city || '') + '" placeholder="' + cityPlaceholder + '"' + (useGeolocation ? ' disabled' : '') + '></label>' +
                        '<input type="hidden" id="setting-lat" value="' + (settings.lat || '') + '">' +
                        '<input type="hidden" id="setting-lon" value="' + (settings.lon || '') + '">';
                    break;
                case 'clock':
                    var formatLabel = escapeHtml(t('widgets_ui.settings_clock_format_label', 'Format'));
                    var format24h = escapeHtml(t('widgets_ui.settings_clock_format_24h', '24-hour'));
                    var format12h = escapeHtml(t('widgets_ui.settings_clock_format_12h', '12-hour'));
                    content =
                        '<label>' + formatLabel + ':<select id="setting-format">' +
                            '<option value="24h"' + (settings.format !== '12h' ? ' selected' : '') + '>' + format24h + '</option>' +
                            '<option value="12h"' + (settings.format === '12h' ? ' selected' : '') + '>' + format12h + '</option>' +
                        '</select></label>';
                    break;
                case 'stocks':
                    var stocksLabel = escapeHtml(t('widgets_ui.settings_stocks_symbols_label', 'Stock Symbols (comma-separated)'));
                    var stocksPlaceholder = escapeHtml(t('widgets_ui.settings_stocks_symbols_placeholder', 'e.g., AAPL, GOOGL, MSFT'));
                    content = '<label>' + stocksLabel + ':<input type="text" id="setting-symbols" value="' + escapeHtml((settings.symbols || ['AAPL', 'GOOGL', 'MSFT']).join(', ')) + '" placeholder="' + stocksPlaceholder + '"></label>';
                    break;
                case 'crypto':
                    var cryptoLabel = escapeHtml(t('widgets_ui.settings_crypto_coins_label', 'Coins (comma-separated)'));
                    var cryptoPlaceholder = escapeHtml(t('widgets_ui.settings_crypto_coins_placeholder', 'e.g., bitcoin, ethereum'));
                    content = '<label>' + cryptoLabel + ':<input type="text" id="setting-coins" value="' + escapeHtml((settings.coins || ['bitcoin', 'ethereum']).join(', ')) + '" placeholder="' + cryptoPlaceholder + '"></label>';
                    break;
                case 'rss':
                    var rssLabel = escapeHtml(t('widgets_ui.settings_rss_feeds_label', 'RSS Feed URLs (one per line)'));
                    var rssPlaceholder = escapeHtml(t('widgets_ui.settings_rss_feeds_placeholder', 'https://example.com/feed.xml'));
                    content = '<label>' + rssLabel + ':<textarea id="setting-feeds" rows="4" placeholder="' + rssPlaceholder + '">' + escapeHtml((settings.feeds || []).join('\n')) + '</textarea></label>';
                    break;
                default:
                    content = '<p>' + escapeHtml(t('widgets_ui.settings_unavailable', 'No settings available for this widget.')) + '</p>';
            }

            var modal = document.createElement('div');
            modal.className = 'widget-settings-modal';
            modal.innerHTML =
                '<div class="widget-settings-content">' +
                    '<h3>' + settingsTitle + '</h3>' +
                    '<form id="widget-settings-form">' +
                        content +
                        '<div class="widget-settings-actions">' +
                            '<button type="button" data-action="cancel">' + cancelLabel + '</button>' +
                            '<button type="submit">' + saveLabel + '</button>' +
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

            // Weather-specific geolocation handling
            if (widgetType === 'weather') {
                var useLocationCheckbox = modal.querySelector('#setting-use-location');
                var cityInput = modal.querySelector('#setting-city');
                var latInput = modal.querySelector('#setting-lat');
                var lonInput = modal.querySelector('#setting-lon');
                var statusEl = modal.querySelector('#geolocation-status');

                if (useLocationCheckbox) {
                    useLocationCheckbox.addEventListener('change', function() {
                        if (this.checked) {
                            // Check if we're in a secure context (HTTPS or localhost)
                            var isSecure = window.isSecureContext || window.location.protocol === 'https:' || window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';

                            if (!isSecure) {
                                useLocationCheckbox.checked = false;
                                statusEl.textContent = t('widgets_ui.settings_weather_location_requires_https', 'Location requires HTTPS. Please enter a city (e.g., "London" or "Paris, France").');
                                statusEl.style.color = 'var(--color-warning, #ffb86c)';
                                return;
                            }

                            cityInput.disabled = true;
                            statusEl.textContent = t('widgets_ui.settings_weather_getting_location', 'Getting your location...');
                            statusEl.style.color = '';

                            if ('geolocation' in navigator) {
                                navigator.geolocation.getCurrentPosition(
                                    function(position) {
                                        latInput.value = position.coords.latitude;
                                        lonInput.value = position.coords.longitude;
                                        statusEl.textContent = t('widgets_ui.settings_weather_location_acquired', 'Location acquired!') + ' (' + position.coords.latitude.toFixed(2) + ', ' + position.coords.longitude.toFixed(2) + ')';
                                        statusEl.style.color = 'var(--color-success, #50fa7b)';
                                    },
                                    function(error) {
                                        useLocationCheckbox.checked = false;
                                        cityInput.disabled = false;
                                        latInput.value = '';
                                        lonInput.value = '';
                                        statusEl.style.color = 'var(--color-warning, #ffb86c)';
                                        switch(error.code) {
                                            case error.PERMISSION_DENIED:
                                                statusEl.textContent = t('widgets_ui.settings_weather_location_permission_denied', 'Location permission denied. Enter a city (e.g., "Albany, NY").');
                                                break;
                                            case error.POSITION_UNAVAILABLE:
                                                statusEl.textContent = t('widgets_ui.settings_weather_location_unavailable', 'Location unavailable. Enter a city (e.g., "London, UK").');
                                                break;
                                            case error.TIMEOUT:
                                                statusEl.textContent = t('widgets_ui.settings_weather_location_timeout', 'Location timed out. Enter a city (e.g., "Paris, France").');
                                                break;
                                            default:
                                                statusEl.textContent = t('widgets_ui.settings_weather_location_failed', 'Could not get location. Enter a city (e.g., "Tokyo, Japan").');
                                        }
                                    },
                                    { enableHighAccuracy: false, timeout: 10000, maximumAge: 300000 }
                                );
                            } else {
                                useLocationCheckbox.checked = false;
                                cityInput.disabled = false;
                                statusEl.textContent = t('widgets_ui.settings_weather_location_unsupported', 'Geolocation not supported. Enter a city (e.g., "New York, NY").');
                                statusEl.style.color = 'var(--color-warning, #ffb86c)';
                            }
                        } else {
                            cityInput.disabled = false;
                            latInput.value = '';
                            lonInput.value = '';
                            statusEl.textContent = t('widgets_ui.settings_weather_city_hint', 'Enter a city (e.g., "Albany, NY" or "Paris, France")');
                            statusEl.style.color = '';
                        }
                    });
                }
            }
        },

        saveSettings: function(widgetType) {
            var settings = getWidgetSettings(widgetType);

            switch (widgetType) {
                case 'weather':
                    settings.useGeolocation = document.getElementById('setting-use-location')?.checked || false;
                    settings.city = document.getElementById('setting-city')?.value || '';
                    settings.lat = document.getElementById('setting-lat')?.value || '';
                    settings.lon = document.getElementById('setting-lon')?.value || '';
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
            var display = document.getElementById('calc-display');
            if (!display) return;

            // Don't evaluate empty expression
            var expr = this.calcExpression;
            if (!expr || expr.trim() === '') {
                return;
            }

            try {
                var result = this.safeEval(expr);
                // Handle invalid results
                if (result === undefined || result === null || isNaN(result)) {
                    display.value = 'Error';
                    this.calcExpression = '';
                } else if (!isFinite(result)) {
                    display.value = result > 0 ? 'Infinity' : '-Infinity';
                    this.calcExpression = '';
                } else {
                    // Round to avoid floating point issues
                    result = Math.round(result * 1000000000) / 1000000000;
                    display.value = result;
                    this.calcExpression = String(result);
                }
            } catch (e) {
                display.value = 'Error';
                this.calcExpression = '';
            }
        },

        // Safe expression evaluator (no eval/Function)
        safeEval: function(expr) {
            // Tokenize the expression
            var tokens = [];
            var numBuffer = '';

            for (var i = 0; i < expr.length; i++) {
                var c = expr[i];
                if ((c >= '0' && c <= '9') || c === '.') {
                    numBuffer += c;
                } else if (c === '+' || c === '-' || c === '*' || c === '/') {
                    if (numBuffer) {
                        tokens.push(parseFloat(numBuffer));
                        numBuffer = '';
                    } else if (c === '-' && (tokens.length === 0 || typeof tokens[tokens.length - 1] === 'string')) {
                        // Negative number
                        numBuffer = '-';
                        continue;
                    }
                    tokens.push(c);
                }
            }
            if (numBuffer) {
                tokens.push(parseFloat(numBuffer));
            }

            // Handle multiplication and division first
            var i = 0;
            while (i < tokens.length) {
                if (tokens[i] === '*' || tokens[i] === '/') {
                    var left = tokens[i - 1];
                    var right = tokens[i + 1];
                    var result = tokens[i] === '*' ? left * right : left / right;
                    tokens.splice(i - 1, 3, result);
                } else {
                    i++;
                }
            }

            // Handle addition and subtraction
            var result = tokens[0];
            for (var i = 1; i < tokens.length; i += 2) {
                var op = tokens[i];
                var num = tokens[i + 1];
                if (op === '+') result += num;
                else if (op === '-') result -= num;
            }

            return result;
        },

        // Quick Links
        addQuickLink: async function() {
            var url = await window.showPrompt(t('widgets_ui.quicklinks_prompt_url', 'URL (include https://):'));
            if (!url) return;

            try {
                new URL(url);
            } catch (e) {
                await window.showAlert(t('widgets_ui.quicklinks_invalid_url', 'Invalid URL. Please include https://'));
                return;
            }

            var name = await window.showPrompt(t('widgets_ui.quicklinks_prompt_name', 'Link name (optional):'));

            var settings = getWidgetSettings('quicklinks');
            settings.links = settings.links || [];
            settings.links.push({ name: (name || '').trim(), url: url });
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
            // Return widget types in current DOM order.
            return getEnabledWidgets();
        }
    };

    // Initialize widgets on DOM ready — widget shells are server-rendered.
    // Script loads at bottom of <body>, so DOMContentLoaded may have already fired;
    // fall through to immediate init when readyState is not 'loading'.
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function() {
            if (document.getElementById('widget-grid')) {
                WidgetManager.init();
            }
        });
    } else {
        if (document.getElementById('widget-grid')) {
            WidgetManager.init();
        }
    }

    window.WidgetManager = WidgetManager;
})();


// ============================================================================
// CHMOD CALCULATOR (progressive enhancement for the native permission form)
// ============================================================================
(function() {
    'use strict';

    function updateChmodCalculator(root) {
        var principals = [
            {key: 'u', bits: [4, 2, 1]},
            {key: 'g', bits: [4, 2, 1]},
            {key: 'o', bits: [4, 2, 1]}
        ];
        var symbolic = '';
        var digits = '';
        principals.forEach(function(principal) {
            var digit = 0;
            principal.bits.forEach(function(bit) {
                var input = root.querySelector('.chmod-toggle[data-principal="' + principal.key + '"][data-permission="' + (bit === 4 ? 'read' : bit === 2 ? 'write' : 'execute') + '"]');
                if (input && input.checked) digit += bit;
            });
            digits += String(digit);
            var digitOutput = root.querySelector('[data-chmod-digit="' + principal.key + '"]');
            if (digitOutput) digitOutput.textContent = String(digit);
            symbolic += (digit & 4 ? 'r' : '-') + (digit & 2 ? 'w' : '-') + (digit & 1 ? 'x' : '-');
        });
        var octal = root.querySelector('[data-chmod-octal]');
        var symbolicOutput = root.querySelector('[data-chmod-symbolic]');
        var command = root.querySelector('[data-chmod-command]');
        var warning = root.querySelector('[data-chmod-warning]');
        if (octal) octal.textContent = digits;
        if (symbolicOutput) symbolicOutput.textContent = symbolic;
        if (command) command.textContent = 'chmod ' + digits + ' filename';
        if (warning) warning.hidden = digits !== '777';
    }

    document.addEventListener('change', function(event) {
        var target = event.target && (event.target.nodeType === 1 ? event.target : event.target.parentElement);
        var input = target && target.closest('.chmod-toggle');
        if (!input) return;
        var root = input.closest('[data-chmod-calculator]');
        if (root) updateChmodCalculator(root);
    });
})();

// ============================================================================
// ARCHIVE TIMELINE (progressive enhancement for the native snapshot form)
// ============================================================================
(function() {
    'use strict';

    function updateTimeline(form, index) {
        var select = form.querySelector('select[name="snapshot_id"]');
        var range = form.querySelector('input[type="range"]');
        var output = form.querySelector('output[for="archive-snapshot-range"]');
        if (!select || !range || !output || !select.options[index]) return;
        select.selectedIndex = index;
        var label = select.options[index].textContent;
        output.textContent = label;
        range.setAttribute('aria-valuetext', label);
    }

    document.addEventListener('input', function(event) {
        if (!event.target) return;
        var target = event.target.nodeType === 1 ? event.target : event.target.parentElement;
        var range = target && target.closest('.archive-timeline input[type="range"]');
        if (range) updateTimeline(range.form, Number(range.value));
    });

    document.addEventListener('change', function(event) {
        if (!event.target) return;
        var target = event.target.nodeType === 1 ? event.target : event.target.parentElement;
        var select = target && target.closest('.archive-timeline select[name="snapshot_id"]');
        if (select) updateTimeline(select.form, select.selectedIndex);
    });
})();

// ============================================================================
// VIDEO PREVIEWS (per IDEA.md - hover-to-play video thumbnails)
// ============================================================================
(function() {
    'use strict';

    var hoverTimeout = null;
    var activePreview = null;
    var previewContainer = null;
    var isTouchDevice = window.matchMedia('(max-width: 767px)').matches || 'ontouchstart' in window;
    var lastTapResult = null;
    var lastTapAt = 0;
    var scrubStartX = 0;
    var scrubResult = null;
    var scrubMoved = false;
    var suppressClickUntil = 0;

    // Initialize video preview functionality
    function initVideoPreview() {
        var previewPreference = document.cookie.split('; ').find(function(item) { return item.indexOf('search_pref_video_previews=') === 0; });
        if (previewPreference && decodeURIComponent(previewPreference.split('=').slice(1).join('=')) === '0') return;

        // Create preview container
        if (!previewContainer) {
            previewContainer = document.createElement('div');
            previewContainer.id = 'video-preview-container';
            previewContainer.className = 'video-preview-container';
            previewContainer.setAttribute('aria-hidden', 'true');
            document.body.appendChild(previewContainer);
        }

        // Event delegation for video results
        document.addEventListener('mouseenter', handleMouseEnter, true);
        document.addEventListener('mouseleave', handleMouseLeave, true);
        document.addEventListener('securitypolicyviolation', function(event) {
            var blockedURI = event.blockedURI || '';
            if (activePreview && event.effectiveDirective === 'frame-src' && blockedURI.indexOf('youtube-nocookie.com') !== -1) hideVideoPreview();
        });

        // Touch device: swipe scrubbing
        if (isTouchDevice) {
            document.addEventListener('touchstart', handleTouchStart, { passive: true });
            document.addEventListener('touchmove', handleTouchMove, { passive: false });
            document.addEventListener('touchend', handleTouchEnd, { passive: true });
        }
    }

    function handleMouseEnter(e) {
        var videoResult = window.closestFrom(e.target, '.video-result, .video-item, [data-video-id]');
        if (!videoResult) return;

        clearTimeout(hoverTimeout);

        // Delay before showing preview to prevent accidental triggers
        hoverTimeout = setTimeout(function() {
            showVideoPreview(videoResult);
        }, 500);
    }

    function handleMouseLeave(e) {
        var videoResult = window.closestFrom(e.target, '.video-result, .video-item, [data-video-id]');
        if (!videoResult) return;

        clearTimeout(hoverTimeout);

        // Only hide if we're actually leaving the result
        var relatedTarget = e.relatedTarget;
        if (relatedTarget && (videoResult.contains(relatedTarget) || previewContainer.contains(relatedTarget))) {
            return;
        }

        hideVideoPreview();
    }

    function showVideoPreview(videoResult) {
        var parsedPreview = getVideoPreviewURL(videoResult);
        if (!parsedPreview) return false;
        var previewURL = parsedPreview.href;
        activePreview = videoResult;

        // Position the preview
        var rect = videoResult.getBoundingClientRect();
        previewContainer.style.top = rect.top + 'px';
        previewContainer.style.left = (rect.right + 10) + 'px';
        previewContainer.style.width = '320px';
        previewContainer.style.height = '180px';

        // Check if preview would go off screen
        if (rect.right + 340 > window.innerWidth) {
            previewContainer.style.left = (rect.left - 330) + 'px';
        }
        if (rect.top + 180 > window.innerHeight) {
            previewContainer.style.top = (window.innerHeight - 190) + 'px';
        }

        var video = document.createElement('video');
        video.src = previewURL;
        video.muted = true;
        video.autoplay = true;
        video.loop = true;
        video.playsInline = true;
        video.preload = 'none';
        video.title = videoResult.querySelector('.video-title') ? videoResult.querySelector('.video-title').textContent.trim() : 'Video preview';
        video.className = 'video-preview-video';
        var progress = document.createElement('div');
        progress.className = 'video-preview-progress';
        progress.setAttribute('role', 'progressbar');
        progress.setAttribute('aria-label', 'Video preview position');
        progress.setAttribute('aria-valuemin', '0');
        progress.setAttribute('aria-valuemax', String(Math.max(1, Math.min(10, Number(videoResult.dataset.videoDuration) || 8))));
        progress.setAttribute('aria-valuenow', '0');
        var progressFill = document.createElement('span');
        progress.appendChild(progressFill);
        previewContainer.replaceChildren(video, progress);
        video.addEventListener('timeupdate', function() {
            if (activePreview === videoResult) updatePreviewPosition(videoResult, video.currentTime, false);
        });

        previewContainer.classList.add('visible');
        previewContainer.setAttribute('aria-hidden', 'false');
        video.play().catch(function() {});
        return true;
    }

    function getVideoPreviewURL(videoResult) {
        if (!videoResult || !videoResult.dataset.videoPreviewUrl) return null;
        var parsedPreview;
        try {
            parsedPreview = new URL(videoResult.dataset.videoPreviewUrl, window.location.href);
        } catch (e) {
            return null;
        }
        if (parsedPreview.origin !== window.location.origin || parsedPreview.username || parsedPreview.password || !/\.(?:mp4|webm)$/i.test(parsedPreview.pathname)) return null;
        return parsedPreview;
    }

    function hideVideoPreview() {
        if (previewContainer) {
            previewContainer.classList.remove('visible');
            previewContainer.classList.remove('inline');
            previewContainer.setAttribute('aria-hidden', 'true');
            previewContainer.innerHTML = '';
            if (previewContainer.parentElement !== document.body) document.body.appendChild(previewContainer);
            delete previewContainer.dataset.embedOrigin;
        }
        activePreview = null;
    }

    // Touch handling for mobile swipe scrubbing
    function handleTouchStart(e) {
        var videoResult = window.closestFrom(e.target, '.video-result, .video-item, [data-video-id]');
        if (!getVideoPreviewURL(videoResult) || !e.touches.length) return;
        scrubStartX = e.touches[0].clientX;
        scrubResult = videoResult;
        scrubMoved = false;
    }

    function handleTouchMove(e) {
        if (!scrubResult || !e.touches.length) return;
        var deltaX = e.touches[0].clientX - scrubStartX;
        if (Math.abs(deltaX) < 8) return;
        e.preventDefault();
        scrubMoved = true;
        var duration = Math.max(1, Math.min(10, Number(scrubResult.dataset.videoDuration) || 8));
        var bounds = scrubResult.querySelector('.video-thumbnail-container').getBoundingClientRect();
        var progress = Math.min(1, Math.max(0, (e.touches[0].clientX - bounds.left) / bounds.width));
        updatePreviewPosition(scrubResult, progress * duration, true);
    }

    function handleTouchEnd(e) {
        if (scrubResult && scrubMoved) suppressClickUntil = Date.now() + 500;
        scrubResult = null;
        scrubStartX = 0;
        scrubMoved = false;
    }

    function updatePreviewPosition(videoResult, seconds, seek) {
        if (!activePreview || activePreview !== videoResult || !previewContainer) return;
        var video = previewContainer.querySelector('video');
        var progress = previewContainer.querySelector('.video-preview-progress');
        if (!video || !progress) return;
        var duration = Math.max(1, Math.min(10, Number(video.duration) || Number(videoResult.dataset.videoDuration) || 8));
        var current = Math.min(duration, Math.max(0, seconds));
        var percent = current / duration * 100;
        progress.firstElementChild.style.width = percent + '%';
        progress.setAttribute('aria-valuenow', String(Math.round(current)));
        if (seek) video.currentTime = current;
    }

    document.addEventListener('click', function(event) {
        var target = event.target && event.target.nodeType === 1 ? event.target : event.target.parentElement;
        var link = target && target.closest('.video-thumbnail-link');
        var result = link && link.closest('.video-result');
        if (!result || !isTouchDevice || !getVideoPreviewURL(result)) return;
        if (Date.now() < suppressClickUntil) {
            event.preventDefault();
            return;
        }
        var now = Date.now();
        if (lastTapResult === result && now - lastTapAt < 400) {
            hideVideoPreview();
            return;
        }
        event.preventDefault();
        lastTapResult = result;
        lastTapAt = now;
        if (activePreview === result) return;
        hideVideoPreview();
        showVideoPreview(result);
        previewContainer.classList.add('inline');
        var thumbnail = result.querySelector('.video-thumbnail-container');
        if (thumbnail) thumbnail.appendChild(previewContainer);
    });

    // Initialize on DOM ready — script loads at bottom of <body> so DOMContentLoaded
    // may have already fired; fall through to immediate init when readyState is not 'loading'.
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initVideoPreview);
    } else {
        initVideoPreview();
    }

    // Expose for external use
    window.VideoPreview = {
        show: showVideoPreview,
        hide: hideVideoPreview
    };
})();


// ============================================================================
// MOBILE PREFERENCES SHEET (native route remains available without JavaScript)
// ============================================================================
(function() {
    'use strict';

    document.addEventListener('click', function(event) {
        var target = event.target && event.target.nodeType === 1 ? event.target : event.target.parentElement;
        var trigger = target && target.closest('[data-mobile-preferences-trigger]');
        if (!trigger || !window.matchMedia('(max-width: 767px)').matches) return;

        var dialog = document.getElementById('mobile-preferences-dialog');
        if (!dialog || typeof dialog.showModal !== 'function') return;

        event.preventDefault();
        if (!dialog.open) dialog.showModal();
        var frame = dialog.querySelector('[data-mobile-preferences-frame]');
        if (frame && !frame.hasAttribute('src')) frame.src = trigger.href;
    });
})();


// ============================================================================
// ADVANCED SEARCH FORM (per IDEA.md)
// ============================================================================
(function() {
    'use strict';

    var advancedSearchModal = null;

    // Advanced search operators
    var OPERATORS = {
        exact: { label: t('search.advanced_modal.exact_phrase', 'Exact phrase'), prefix: '"', suffix: '"', placeholder: t('search.advanced_modal.operator_exact_placeholder', 'exact words') },
        exclude: { label: t('search.advanced_modal.operator_exclude', 'Exclude'), prefix: '-', suffix: '', placeholder: t('search.advanced_modal.operator_exclude_placeholder', 'unwanted term') },
        site: { label: t('search.advanced_modal.operator_site', 'Site'), prefix: 'site:', suffix: '', placeholder: 'example.com' },
        filetype: { label: t('search.advanced_modal.file_type', 'File type'), prefix: 'filetype:', suffix: '', placeholder: 'pdf' },
        intitle: { label: t('search.advanced_modal.operator_in_title', 'In title'), prefix: 'intitle:', suffix: '', placeholder: t('search.advanced_modal.operator_in_title_placeholder', 'title word') },
        inurl: { label: t('search.advanced_modal.operator_in_url', 'In URL'), prefix: 'inurl:', suffix: '', placeholder: t('search.advanced_modal.operator_in_url_placeholder', 'url part') },
        intext: { label: t('search.advanced_modal.operator_in_text', 'In text'), prefix: 'intext:', suffix: '', placeholder: t('search.advanced_modal.operator_in_text_placeholder', 'body text') },
        before: { label: t('search.advanced_modal.before_date', 'Before date'), prefix: 'before:', suffix: '', placeholder: '2024-01-01' },
        after: { label: t('search.advanced_modal.after_date', 'After date'), prefix: 'after:', suffix: '', placeholder: '2023-01-01' },
        or: { label: t('search.advanced_modal.operator_or_search', 'OR search'), prefix: '', suffix: '', placeholder: t('search.advanced_modal.operator_or_search_placeholder', 'term1 OR term2') }
    };

    function createAdvancedSearchForm() {
        if (advancedSearchModal) {
            advancedSearchModal.remove();
        }

        advancedSearchModal = document.createElement('dialog');
        advancedSearchModal.id = 'advanced-search-modal';
        advancedSearchModal.className = 'advanced-search-modal';
        advancedSearchModal.setAttribute('role', 'dialog');
        advancedSearchModal.setAttribute('aria-labelledby', 'advanced-search-title');

        var advancedTitle = escapeHtml(t('search.advanced', 'Advanced Search'));
        var advancedAllWords = escapeHtml(t('search.advanced_modal.all_words', 'All these words'));
        var advancedAllWordsPlaceholder = escapeHtml(t('search.advanced_modal.all_words_placeholder', 'search terms'));
        var advancedExactPhrase = escapeHtml(t('search.advanced_modal.exact_phrase', 'Exact phrase'));
        var advancedExactPhrasePlaceholder = escapeHtml(t('search.advanced_modal.exact_phrase_placeholder', '"exact phrase"'));
        var advancedAnyWords = escapeHtml(t('search.advanced_modal.any_words', 'Any of these words'));
        var advancedAnyWordsPlaceholder = escapeHtml(t('search.advanced_modal.any_words_placeholder', 'word1 OR word2'));
        var advancedExcludeWords = escapeHtml(t('search.advanced_modal.exclude_words', 'None of these words'));
        var advancedExcludeWordsPlaceholder = escapeHtml(t('search.advanced_modal.exclude_words_placeholder', '-unwanted'));
        var advancedSite = escapeHtml(t('search.advanced_modal.site', 'Site/domain'));
        var advancedFileType = escapeHtml(t('search.advanced_modal.file_type', 'File type'));
        var advancedAnyFileType = escapeHtml(t('search.advanced_modal.any_file_type', 'Any'));
        var advancedWordDoc = escapeHtml(t('search.advanced_modal.word_doc', 'Word (doc)'));
        var advancedWordDocx = escapeHtml(t('search.advanced_modal.word_docx', 'Word (docx)'));
        var advancedExcelXls = escapeHtml(t('search.advanced_modal.excel_xls', 'Excel (xls)'));
        var advancedExcelXlsx = escapeHtml(t('search.advanced_modal.excel_xlsx', 'Excel (xlsx)'));
        var advancedPowerPoint = escapeHtml(t('search.advanced_modal.powerpoint', 'PowerPoint'));
        var advancedText = escapeHtml(t('search.advanced_modal.text', 'Text'));
        var advancedTitleWords = escapeHtml(t('search.advanced_modal.title_words', 'Words in title'));
        var advancedTitleWordsPlaceholder = escapeHtml(t('search.advanced_modal.title_words_placeholder', 'title words'));
        var advancedURLWords = escapeHtml(t('search.advanced_modal.url_words', 'Words in URL'));
        var advancedURLWordsPlaceholder = escapeHtml(t('search.advanced_modal.url_words_placeholder', 'url-segment'));
        var advancedAfterDate = escapeHtml(t('search.advanced_modal.after_date', 'After date'));
        var advancedBeforeDate = escapeHtml(t('search.advanced_modal.before_date', 'Before date'));
        var advancedRegion = escapeHtml(t('search.region', 'Region'));
        var advancedAnyRegion = escapeHtml(t('search.advanced_modal.any_region', 'Any region'));
        var advancedUnitedStates = escapeHtml(t('search.advanced_modal.region_us', 'United States'));
        var advancedUnitedKingdom = escapeHtml(t('search.advanced_modal.region_uk', 'United Kingdom'));
        var advancedGermany = escapeHtml(t('search.advanced_modal.region_de', 'Germany'));
        var advancedFrance = escapeHtml(t('search.advanced_modal.region_fr', 'France'));
        var advancedSpain = escapeHtml(t('search.advanced_modal.region_es', 'Spain'));
        var advancedItaly = escapeHtml(t('search.advanced_modal.region_it', 'Italy'));
        var advancedJapan = escapeHtml(t('search.advanced_modal.region_jp', 'Japan'));
        var advancedChina = escapeHtml(t('search.advanced_modal.region_cn', 'China'));
        var advancedBrazil = escapeHtml(t('search.advanced_modal.region_br', 'Brazil'));
        var advancedAustralia = escapeHtml(t('search.advanced_modal.region_au', 'Australia'));
        var advancedCanada = escapeHtml(t('search.advanced_modal.region_ca', 'Canada'));
        var advancedIndia = escapeHtml(t('search.advanced_modal.region_in', 'India'));
        var advancedPreview = escapeHtml(t('search.advanced_modal.query_preview', 'Query preview:'));
        var html = '<header>' +
            '<h2 id="advanced-search-title">' + advancedTitle + '</h2>' +
            '<button type="button" class="close-btn" data-action="close" aria-label="' + escapeHtml(t('common.close', 'Close')) + '">&times;</button>' +
        '</header>' +
        '<main class="advanced-search-content">' +
            '<form id="advanced-search-form">' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-main">' + advancedAllWords + '</label>' +
                    '<input type="text" id="adv-main" name="main" placeholder="' + advancedAllWordsPlaceholder + '" autofocus>' +
                '</div>' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-exact">' + advancedExactPhrase + '</label>' +
                    '<input type="text" id="adv-exact" name="exact" placeholder="' + advancedExactPhrasePlaceholder + '">' +
                '</div>' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-any">' + advancedAnyWords + '</label>' +
                    '<input type="text" id="adv-any" name="any" placeholder="' + advancedAnyWordsPlaceholder + '">' +
                '</div>' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-exclude">' + advancedExcludeWords + '</label>' +
                    '<input type="text" id="adv-exclude" name="exclude" placeholder="' + advancedExcludeWordsPlaceholder + '">' +
                '</div>' +
                '<div class="advanced-search-row">' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-site">' + advancedSite + '</label>' +
                        '<input type="text" id="adv-site" name="site" placeholder="example.com">' +
                    '</div>' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-filetype">' + advancedFileType + '</label>' +
                        '<select id="adv-filetype" name="filetype">' +
                            '<option value="">' + advancedAnyFileType + '</option>' +
                            '<option value="pdf">PDF</option>' +
                            '<option value="doc">' + advancedWordDoc + '</option>' +
                            '<option value="docx">' + advancedWordDocx + '</option>' +
                            '<option value="xls">' + advancedExcelXls + '</option>' +
                            '<option value="xlsx">' + advancedExcelXlsx + '</option>' +
                            '<option value="ppt">' + advancedPowerPoint + '</option>' +
                            '<option value="txt">' + advancedText + '</option>' +
                            '<option value="csv">CSV</option>' +
                            '<option value="json">JSON</option>' +
                            '<option value="xml">XML</option>' +
                        '</select>' +
                    '</div>' +
                '</div>' +
                '<div class="advanced-search-row">' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-intitle">' + advancedTitleWords + '</label>' +
                        '<input type="text" id="adv-intitle" name="intitle" placeholder="' + advancedTitleWordsPlaceholder + '">' +
                    '</div>' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-inurl">' + advancedURLWords + '</label>' +
                        '<input type="text" id="adv-inurl" name="inurl" placeholder="' + advancedURLWordsPlaceholder + '">' +
                    '</div>' +
                '</div>' +
                '<div class="advanced-search-row">' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-after">' + advancedAfterDate + '</label>' +
                        '<input type="date" id="adv-after" name="after">' +
                    '</div>' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-before">' + advancedBeforeDate + '</label>' +
                        '<input type="date" id="adv-before" name="before">' +
                    '</div>' +
                '</div>' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-region">' + advancedRegion + '</label>' +
                    '<select id="adv-region" name="region">' +
                        '<option value="">' + advancedAnyRegion + '</option>' +
                        '<option value="us">' + advancedUnitedStates + '</option>' +
                        '<option value="uk">' + advancedUnitedKingdom + '</option>' +
                        '<option value="de">' + advancedGermany + '</option>' +
                        '<option value="fr">' + advancedFrance + '</option>' +
                        '<option value="es">' + advancedSpain + '</option>' +
                        '<option value="it">' + advancedItaly + '</option>' +
                        '<option value="jp">' + advancedJapan + '</option>' +
                        '<option value="cn">' + advancedChina + '</option>' +
                        '<option value="br">' + advancedBrazil + '</option>' +
                        '<option value="au">' + advancedAustralia + '</option>' +
                        '<option value="ca">' + advancedCanada + '</option>' +
                        '<option value="in">' + advancedIndia + '</option>' +
                    '</select>' +
                '</div>' +
                '<div class="advanced-search-preview">' +
                    '<label>' + advancedPreview + '</label>' +
                    '<code id="adv-preview"></code>' +
                '</div>' +
            '</form>' +
        '</main>' +
        '<footer>' +
            '<button type="button" class="btn btn-secondary" data-action="clear">Clear</button>' +
            '<button type="button" class="btn btn-primary" data-action="search">Search</button>' +
        '</footer>';

        advancedSearchModal.innerHTML = html;
        document.body.appendChild(advancedSearchModal);

        // Event listeners
        advancedSearchModal.querySelector('[data-action="close"]').addEventListener('click', function() {
            advancedSearchModal.close();
        });

        advancedSearchModal.querySelector('[data-action="clear"]').addEventListener('click', function() {
            advancedSearchModal.querySelector('form').reset();
            updatePreview();
        });

        advancedSearchModal.querySelector('[data-action="search"]').addEventListener('click', function() {
            performAdvancedSearch();
        });

        advancedSearchModal.querySelector('form').addEventListener('submit', function(e) {
            e.preventDefault();
            performAdvancedSearch();
        });

        // Update preview on input changes
        advancedSearchModal.querySelectorAll('input, select').forEach(function(input) {
            input.addEventListener('input', updatePreview);
            input.addEventListener('change', updatePreview);
        });

        // Keyboard handling
        advancedSearchModal.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                advancedSearchModal.close();
            }
        });

        return advancedSearchModal;
    }

    function updatePreview() {
        var preview = document.getElementById('adv-preview');
        if (!preview) return;

        var query = buildQuery();
        preview.textContent = query || '(enter search terms)';
    }

    function buildQuery() {
        var parts = [];

        // Main query
        var main = document.getElementById('adv-main')?.value.trim();
        if (main) parts.push(main);

        // Exact phrase
        var exact = document.getElementById('adv-exact')?.value.trim();
        if (exact) parts.push('"' + exact + '"');

        // Any of (OR)
        var any = document.getElementById('adv-any')?.value.trim();
        if (any) {
            var anyTerms = any.split(/\s+/).filter(function(t) { return t; });
            if (anyTerms.length > 1) {
                parts.push('(' + anyTerms.join(' OR ') + ')');
            } else if (anyTerms.length === 1) {
                parts.push(anyTerms[0]);
            }
        }

        // Exclude
        var exclude = document.getElementById('adv-exclude')?.value.trim();
        if (exclude) {
            exclude.split(/\s+/).forEach(function(term) {
                if (term && !term.startsWith('-')) {
                    parts.push('-' + term);
                } else if (term) {
                    parts.push(term);
                }
            });
        }

        // Site
        var site = document.getElementById('adv-site')?.value.trim();
        if (site) parts.push('site:' + site);

        // Filetype
        var filetype = document.getElementById('adv-filetype')?.value;
        if (filetype) parts.push('filetype:' + filetype);

        // In title
        var intitle = document.getElementById('adv-intitle')?.value.trim();
        if (intitle) parts.push('intitle:' + intitle);

        // In URL
        var inurl = document.getElementById('adv-inurl')?.value.trim();
        if (inurl) parts.push('inurl:' + inurl);

        // Date range
        var after = document.getElementById('adv-after')?.value;
        if (after) parts.push('after:' + after);

        var before = document.getElementById('adv-before')?.value;
        if (before) parts.push('before:' + before);

        return parts.join(' ');
    }

    function performAdvancedSearch() {
        var query = buildQuery();
        if (!query) return;

        var region = document.getElementById('adv-region')?.value;
        var searchUrl = '/search?q=' + encodeURIComponent(query);
        if (region) {
            searchUrl += '&region=' + encodeURIComponent(region);
        }

        advancedSearchModal.close();
        window.location.href = searchUrl;
    }

    function showAdvancedSearch() {
        var modal = createAdvancedSearchForm();

        // Pre-populate with current search query if on search page
        var urlParams = new URLSearchParams(window.location.search);
        var currentQuery = urlParams.get('q');
        if (currentQuery) {
            var mainInput = modal.querySelector('#adv-main');
            if (mainInput) mainInput.value = currentQuery;
        }

        updatePreview();
        modal.showModal();

        // Announce for screen readers
        if (window.srAnnounce) {
            window.srAnnounce(t('search.advanced_modal.dialog_opened', 'Advanced search dialog opened'));
        }
    }

    // Initialize
    function init() {
        // Add advanced search button to search forms
        document.querySelectorAll('.search-form, form[action*="search"]').forEach(function(form) {
            if (form.querySelector('.advanced-search-trigger')) return;

            var trigger = document.createElement('button');
            trigger.type = 'button';
            trigger.className = 'advanced-search-trigger';
            trigger.innerHTML = '&#8942;'; // Vertical ellipsis
            trigger.setAttribute('aria-label', t('search.advanced', 'Advanced Search'));
            trigger.setAttribute('title', t('search.advanced', 'Advanced Search'));
            trigger.addEventListener('click', showAdvancedSearch);

            var searchBtn = form.querySelector('button[type="submit"], .search-btn');
            if (searchBtn) {
                searchBtn.parentNode.insertBefore(trigger, searchBtn);
            } else {
                form.appendChild(trigger);
            }
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    // Expose for external use
    window.AdvancedSearch = {
        show: showAdvancedSearch,
        buildQuery: buildQuery
    };
})();


// ============================================================================
// RELATED SEARCHES (per IDEA.md - query refinement suggestions)
// ============================================================================
(function() {
    'use strict';

    var relatedContainer = null;
    var currentQuery = null;

    function init() {
        // Only on search results page
        if (!window.location.pathname.includes('/search')) return;

        var urlParams = new URLSearchParams(window.location.search);
        currentQuery = urlParams.get('q');

        if (!currentQuery) return;

        // Create related searches container
        relatedContainer = document.createElement('div');
        relatedContainer.id = 'related-searches';
        relatedContainer.className = 'related-searches';
        relatedContainer.setAttribute('aria-labelledby', 'related-title');

        // Find the best place to insert (after results, before pagination)
        var resultsContainer = document.querySelector('.search-results, .results-container, #results');
        var pagination = document.querySelector('.pagination, .pager, nav[aria-label*="pagination"]');

        if (pagination && pagination.parentNode) {
            pagination.parentNode.insertBefore(relatedContainer, pagination);
        } else if (resultsContainer) {
            resultsContainer.appendChild(relatedContainer);
        } else {
            // Add to main content area
            var main = document.querySelector('main, .main-content, #content');
            if (main) {
                main.appendChild(relatedContainer);
            }
        }

        // Fetch related searches
        fetchRelatedSearches(currentQuery);
    }

    function fetchRelatedSearches(query) {
        // Try API first - per AI.md PART 14: use origin prefix
        fetch(window.location.origin + '/api/v1/search/related?q=' + encodeURIComponent(query) + '&limit=8')
            .then(function(response) {
                if (!response.ok) throw new Error('API not available');
                return response.json();
            })
            .then(function(data) {
                if (data.success && data.data && data.data.length > 0) {
                    renderRelatedSearches(data.data);
                } else {
                    // Fall back to client-side generation
                    var suggestions = generateClientSideSuggestions(query);
                    if (suggestions.length > 0) {
                        renderRelatedSearches(suggestions);
                    }
                }
            })
            .catch(function() {
                // Fall back to client-side generation
                var suggestions = generateClientSideSuggestions(query);
                if (suggestions.length > 0) {
                    renderRelatedSearches(suggestions);
                }
            });
    }

    function generateClientSideSuggestions(query) {
        var suggestions = [];
        var words = query.split(/\s+/).filter(function(w) { return w; });

        if (words.length === 0) return suggestions;

        // Question variations
        var questionPrefixes = ['what is', 'how to', 'why', 'best', 'top'];
        questionPrefixes.forEach(function(prefix) {
            if (!query.toLowerCase().startsWith(prefix)) {
                suggestions.push(prefix + ' ' + query);
            }
        });

        // Add common suffixes
        var suffixes = ['examples', 'tutorial', 'guide', 'vs', 'alternatives', 'review', '2024', '2025'];
        suffixes.forEach(function(suffix) {
            if (!query.toLowerCase().includes(suffix)) {
                suggestions.push(query + ' ' + suffix);
            }
        });

        // If multiple words, try variations
        if (words.length > 1) {
            suggestions.push(words.slice(1).join(' '));
            suggestions.push(words[0] + ' alternatives');
        }

        // Deduplicate and limit
        var seen = {};
        var unique = [];
        suggestions.forEach(function(s) {
            var lower = s.toLowerCase();
            if (!seen[lower] && lower !== query.toLowerCase()) {
                seen[lower] = true;
                unique.push(s);
            }
        });

        return unique.slice(0, 8);
    }

    function renderRelatedSearches(suggestions) {
        if (!relatedContainer || suggestions.length === 0) return;

        var html = '<h3 id="related-title" class="related-title">' + escapeHtml(t('search.related_title', 'Related searches')) + '</h3>' +
            '<div class="related-list">';

        suggestions.forEach(function(suggestion) {
            var searchUrl = '/search?q=' + encodeURIComponent(suggestion);
            html += '<a href="' + searchUrl + '" class="related-item">' +
                '<svg class="related-icon" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">' +
                    '<circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/>' +
                '</svg>' +
                '<span>' + escapeHtml(suggestion) + '</span>' +
            '</a>';
        });

        html += '</div>';
        relatedContainer.innerHTML = html;
        relatedContainer.style.display = 'block';

        // Announce for screen readers
        if (window.srAnnounce) {
            window.srAnnounce(t('search.related_available', suggestions.length + ' related searches available'));
        }
    }

    function escapeHtml(text) {
        var div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }

    // Also provide "People also ask" style suggestions
    function renderPeopleAlsoAsk(questions) {
        if (!questions || questions.length === 0) return;

        var container = document.createElement('div');
        container.id = 'people-also-ask';
        container.className = 'people-also-ask';

        var html = '<h3 class="paa-title">' + escapeHtml(t('search.people_also_ask', 'People also ask')) + '</h3>' +
            '<div class="paa-list">';

        questions.forEach(function(q, index) {
            html += '<details class="paa-item">' +
                '<summary class="paa-question">' +
                    '<span>' + escapeHtml(q.question) + '</span>' +
                    '<svg class="paa-arrow" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">' +
                        '<path d="M6 9l6 6 6-6"/>' +
                    '</svg>' +
                '</summary>' +
                '<div class="paa-answer">' +
                    (q.answer ? '<p>' + escapeHtml(q.answer) + '</p>' : '<p class="paa-loading">' + escapeHtml(t('common.loading', 'Loading...')) + '</p>') +
                    '<a href="/search?q=' + encodeURIComponent(q.question) + '" class="paa-link">' + escapeHtml(t('search.search_for_this', 'Search for this')) + '</a>' +
                '</div>' +
            '</details>';
        });

        html += '</div>';
        container.innerHTML = html;

        // Insert after first few results
        var results = document.querySelectorAll('.search-result, .result-item');
        if (results.length > 3) {
            results[3].parentNode.insertBefore(container, results[3].nextSibling);
        } else if (relatedContainer) {
            relatedContainer.parentNode.insertBefore(container, relatedContainer);
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    // Expose for external use
    window.RelatedSearches = {
        fetch: fetchRelatedSearches,
        render: renderRelatedSearches,
        renderPAA: renderPeopleAlsoAsk
    };
})();
