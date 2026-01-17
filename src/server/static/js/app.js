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
        // Per AI.md PART 16: Apply theme class to <html> element: theme-light, theme-dark
        document.documentElement.classList.remove('theme-dark', 'theme-light');
        document.documentElement.classList.add('theme-' + theme);
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
        // Per AI.md PART 16: Read theme from class, not attribute
        const current = document.documentElement.classList.contains('theme-light') ? 'light' : 'dark';
        const next = current === 'dark' ? 'light' : 'dark';
        setTheme(next);
    }

    // ========================================================================
    // MOBILE NAVIGATION
    // ========================================================================
    function toggleNav() {
        const header = document.querySelector('.site-header');
        const navLinks = document.querySelector('.nav-links');
        const navToggle = document.querySelector('.nav-toggle');
        if (header && navLinks) {
            header.classList.toggle('nav-open');
            navLinks.classList.toggle('active');
            // Update ARIA state for accessibility
            if (navToggle) {
                const isOpen = header.classList.contains('nav-open');
                navToggle.setAttribute('aria-expanded', isOpen ? 'true' : 'false');
            }
        }
    }

    function closeNav() {
        const header = document.querySelector('.site-header');
        const navLinks = document.querySelector('.nav-links');
        const navToggle = document.querySelector('.nav-toggle');
        if (header) {
            header.classList.remove('nav-open');
        }
        if (navLinks) {
            navLinks.classList.remove('active');
        }
        // Update ARIA state for accessibility
        if (navToggle) {
            navToggle.setAttribute('aria-expanded', 'false');
        }
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
    // KEYBOARD SHORTCUTS (Full Navigation per IDEA.md)
    // ========================================================================
    var currentResultIndex = -1;

    function initKeyboardShortcuts() {
        document.addEventListener('keydown', function(e) {
            // Skip if modifier keys (except shift for O)
            if (e.ctrlKey || e.altKey || e.metaKey) return;

            // Focus search with /
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
            var title = results[index].querySelector('h3, .result-title, a')?.textContent || 'Result ' + (index + 1);
            announce('Selected: ' + title);
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
        modal.innerHTML =
            '<header><h2 id="keyboard-help-title">Keyboard Shortcuts</h2></header>' +
            '<main class="keyboard-help-content">' +
                '<div class="shortcut-group">' +
                    '<h3>Navigation</h3>' +
                    '<div class="shortcut"><kbd>j</kbd> / <kbd>k</kbd> <span>Next / Previous result</span></div>' +
                    '<div class="shortcut"><kbd>h</kbd> / <kbd>l</kbd> <span>Previous / Next page</span></div>' +
                    '<div class="shortcut"><kbd>g</kbd><kbd>g</kbd> <span>Go to first result</span></div>' +
                    '<div class="shortcut"><kbd>G</kbd> <span>Go to last result</span></div>' +
                    '<div class="shortcut"><kbd>1</kbd>-<kbd>9</kbd> <span>Jump to result N</span></div>' +
                '</div>' +
                '<div class="shortcut-group">' +
                    '<h3>Actions</h3>' +
                    '<div class="shortcut"><kbd>/</kbd> <span>Focus search box</span></div>' +
                    '<div class="shortcut"><kbd>Enter</kbd> / <kbd>o</kbd> <span>Open selected result</span></div>' +
                    '<div class="shortcut"><kbd>O</kbd> <span>Open in new tab</span></div>' +
                    '<div class="shortcut"><kbd>t</kbd> <span>Toggle theme</span></div>' +
                    '<div class="shortcut"><kbd>Esc</kbd> <span>Clear / Close</span></div>' +
                '</div>' +
                '<div class="shortcut-group">' +
                    '<h3>Help</h3>' +
                    '<div class="shortcut"><kbd>?</kbd> <span>Show this help</span></div>' +
                '</div>' +
            '</main>' +
            '<footer>' +
                '<button type="button" class="btn btn-primary" data-action="close-help">Close</button>' +
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
        announce('Keyboard shortcuts help opened. Press Escape or ? to close.');
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
                        '<button type="button" class="btn btn-primary" data-action="ok">OK</button>' +
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
        initTabKeyboardNav();
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

    // Additional instant answer render functions per IDEA.md

    function renderCurrencyWidget(container, data, settings) {
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
                '<button class="currency-convert-btn" data-action="convert-currency">Convert</button>' +
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
        container.innerHTML =
            '<div class="translate-widget">' +
                '<div class="translate-row">' +
                    '<select id="translate-from" data-translate-from>' +
                        '<option value="auto">Auto-detect</option>' +
                        '<option value="en">English</option>' +
                        '<option value="es">Spanish</option>' +
                        '<option value="fr">French</option>' +
                        '<option value="de">German</option>' +
                        '<option value="it">Italian</option>' +
                        '<option value="pt">Portuguese</option>' +
                        '<option value="ru">Russian</option>' +
                        '<option value="ja">Japanese</option>' +
                        '<option value="ko">Korean</option>' +
                        '<option value="zh">Chinese</option>' +
                    '</select>' +
                    '<span class="translate-arrow">&#8594;</span>' +
                    '<select id="translate-to" data-translate-to>' +
                        '<option value="en">English</option>' +
                        '<option value="es">Spanish</option>' +
                        '<option value="fr">French</option>' +
                        '<option value="de">German</option>' +
                        '<option value="it">Italian</option>' +
                        '<option value="pt">Portuguese</option>' +
                        '<option value="ru">Russian</option>' +
                        '<option value="ja">Japanese</option>' +
                        '<option value="ko">Korean</option>' +
                        '<option value="zh">Chinese</option>' +
                    '</select>' +
                '</div>' +
                '<textarea id="translate-input" data-translate-input placeholder="Enter text to translate..." rows="3"></textarea>' +
                '<div id="translate-output" class="translate-output"></div>' +
            '</div>';
    }

    function renderWikipediaWidget(container, data, settings) {
        if (!data || data.error) {
            container.innerHTML =
                '<div class="wikipedia-widget">' +
                    '<input type="text" id="wiki-search" data-wiki-search placeholder="Search Wikipedia...">' +
                    '<div id="wiki-result" class="wiki-placeholder">Enter a topic to search</div>' +
                '</div>';
            return;
        }

        container.innerHTML =
            '<div class="wikipedia-widget">' +
                '<input type="text" id="wiki-search" data-wiki-search placeholder="Search Wikipedia...">' +
                '<div class="wiki-result">' +
                    (data.thumbnail ? '<img src="' + escapeHtml(data.thumbnail) + '" class="wiki-thumb" alt="">' : '') +
                    '<div class="wiki-content">' +
                        '<h4 class="wiki-title">' + escapeHtml(data.title) + '</h4>' +
                        '<p class="wiki-extract">' + escapeHtml(data.extract ? data.extract.substring(0, 200) + '...' : '') + '</p>' +
                        '<a href="' + escapeHtml(data.url) + '" target="_blank" rel="noopener" class="wiki-link">Read more</a>' +
                    '</div>' +
                '</div>' +
            '</div>';
    }

    function renderTrackingWidget(container, data, settings) {
        container.innerHTML =
            '<div class="tracking-widget">' +
                '<div class="tracking-input-row">' +
                    '<input type="text" id="tracking-number" data-tracking-number placeholder="Enter tracking number...">' +
                    '<button data-action="track-package">Track</button>' +
                '</div>' +
                '<div id="tracking-result" class="tracking-placeholder">Enter a tracking number to check status</div>' +
            '</div>';
    }

    function renderNutritionWidget(container, data, settings) {
        if (!data || data.error) {
            container.innerHTML =
                '<div class="nutrition-widget">' +
                    '<input type="text" id="nutrition-search" data-nutrition-search placeholder="Search food...">' +
                    '<div class="nutrition-placeholder">Search for a food item</div>' +
                '</div>';
            return;
        }

        var html = '<div class="nutrition-widget">' +
            '<input type="text" id="nutrition-search" data-nutrition-search placeholder="Search food...">' +
            '<div class="nutrition-result">' +
                '<h4 class="nutrition-name">' + escapeHtml(data.name) + '</h4>' +
                (data.serving_size ? '<div class="nutrition-serving">Per 100g</div>' : '') +
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
        container.innerHTML =
            '<div class="qrcode-widget">' +
                '<input type="text" id="qr-text" data-qr-text placeholder="Enter text or URL...">' +
                '<div id="qr-canvas" class="qr-canvas">' +
                    '<div class="qr-placeholder">Enter text to generate QR code</div>' +
                '</div>' +
                '<button data-action="generate-qr">Generate QR Code</button>' +
            '</div>';
    }

    function renderTimerWidget(container, data, settings) {
        container.innerHTML =
            '<div class="timer-widget">' +
                '<div class="timer-display" id="timer-display">00:00:00</div>' +
                '<div class="timer-buttons">' +
                    '<button data-timer-action="start">Start</button>' +
                    '<button data-timer-action="pause">Pause</button>' +
                    '<button data-timer-action="reset">Reset</button>' +
                '</div>' +
                '<div class="timer-presets">' +
                    '<button data-timer-preset="60">1 min</button>' +
                    '<button data-timer-preset="300">5 min</button>' +
                    '<button data-timer-preset="600">10 min</button>' +
                    '<button data-timer-preset="1500">25 min</button>' +
                '</div>' +
            '</div>';
    }

    function renderLoremWidget(container, data, settings) {
        var loremText = 'Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.';

        container.innerHTML =
            '<div class="lorem-widget">' +
                '<div class="lorem-options">' +
                    '<select id="lorem-type" data-lorem-type>' +
                        '<option value="paragraphs">Paragraphs</option>' +
                        '<option value="sentences">Sentences</option>' +
                        '<option value="words">Words</option>' +
                    '</select>' +
                    '<input type="number" id="lorem-count" data-lorem-count value="3" min="1" max="20">' +
                    '<button data-action="generate-lorem">Generate</button>' +
                '</div>' +
                '<div id="lorem-output" class="lorem-output">' + loremText + '</div>' +
                '<button class="lorem-copy" data-action="copy-lorem">Copy</button>' +
            '</div>';
    }

    function renderDictionaryWidget(container, data, settings) {
        if (!data || data.error) {
            container.innerHTML =
                '<div class="dictionary-widget">' +
                    '<input type="text" id="dict-word" data-dict-word placeholder="Enter a word...">' +
                    '<div class="dict-placeholder">Search for a word definition</div>' +
                '</div>';
            return;
        }

        var html = '<div class="dictionary-widget">' +
            '<input type="text" id="dict-word" data-dict-word placeholder="Enter a word...">' +
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
        // Get IP info - this is client-side detectable
        fetch('https://api.ipify.org?format=json')
            .then(function(r) { return r.json(); })
            .then(function(ipData) {
                container.innerHTML =
                    '<div class="ip-widget">' +
                        '<div class="ip-label">Your IP Address</div>' +
                        '<div class="ip-value">' + escapeHtml(ipData.ip) + '</div>' +
                        '<button class="ip-copy" data-action="copy-ip" data-ip="' + escapeHtml(ipData.ip) + '">Copy</button>' +
                    '</div>';
            })
            .catch(function() {
                container.innerHTML =
                    '<div class="ip-widget">' +
                        '<div class="ip-error">Unable to detect IP address</div>' +
                    '</div>';
            });
    }

    function renderColorPickerWidget(container, data, settings) {
        var currentColor = settings.color || '#1e90ff';

        container.innerHTML =
            '<div class="colorpicker-widget">' +
                '<input type="color" id="color-input" data-color-input value="' + currentColor + '">' +
                '<div class="color-values">' +
                    '<div class="color-row"><label>HEX</label><input type="text" id="color-hex" data-color-hex value="' + currentColor + '" readonly></div>' +
                    '<div class="color-row"><label>RGB</label><input type="text" id="color-rgb" data-color-rgb readonly></div>' +
                    '<div class="color-row"><label>HSL</label><input type="text" id="color-hsl" data-color-hsl readonly></div>' +
                '</div>' +
                '<button data-action="copy-color">Copy HEX</button>' +
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

        // Per AI.md PART 16: Apply theme class to <html> element: theme-light, theme-dark
        if (prefs.theme && prefs.theme !== 'system') {
            document.documentElement.classList.remove('theme-dark', 'theme-light');
            document.documentElement.classList.add('theme-' + prefs.theme);
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


// ============================================================================
// VIDEO PREVIEWS (per IDEA.md - hover-to-play video thumbnails)
// ============================================================================
(function() {
    'use strict';

    var hoverTimeout = null;
    var activePreview = null;
    var previewContainer = null;
    var isTouchDevice = 'ontouchstart' in window;

    // Initialize video preview functionality
    function initVideoPreview() {
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

        // Touch device: swipe scrubbing
        if (isTouchDevice) {
            document.addEventListener('touchstart', handleTouchStart, { passive: true });
            document.addEventListener('touchmove', handleTouchMove, { passive: false });
            document.addEventListener('touchend', handleTouchEnd, { passive: true });
        }
    }

    function handleMouseEnter(e) {
        var videoResult = e.target.closest('.video-result, .video-item, [data-video-id]');
        if (!videoResult) return;

        clearTimeout(hoverTimeout);

        // Delay before showing preview to prevent accidental triggers
        hoverTimeout = setTimeout(function() {
            showVideoPreview(videoResult);
        }, 500);
    }

    function handleMouseLeave(e) {
        var videoResult = e.target.closest('.video-result, .video-item, [data-video-id]');
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
        var videoId = videoResult.dataset.videoId;
        var previewUrl = videoResult.dataset.previewUrl;
        var videoSource = videoResult.dataset.videoSource || 'youtube';

        if (!videoId && !previewUrl) {
            // Try to extract from YouTube thumbnail
            var thumbnail = videoResult.querySelector('img[src*="ytimg"], img[src*="youtube"]');
            if (thumbnail) {
                var match = thumbnail.src.match(/vi\/([^\/]+)/);
                if (match) {
                    videoId = match[1];
                    videoSource = 'youtube';
                }
            }
        }

        if (!videoId && !previewUrl) return;

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

        // Show preview content
        if (previewUrl) {
            // Direct preview URL (animated GIF or video)
            if (previewUrl.endsWith('.gif') || previewUrl.includes('giphy')) {
                previewContainer.innerHTML = '<img src="' + previewUrl + '" alt="Video preview" class="video-preview-gif">';
            } else {
                previewContainer.innerHTML = '<video src="' + previewUrl + '" autoplay muted loop class="video-preview-video"></video>';
            }
        } else if (videoSource === 'youtube' && videoId) {
            // Use YouTube animated thumbnail
            // YouTube provides animated thumbnails at specific URLs
            var animatedThumb = 'https://i.ytimg.com/vi/' + videoId + '/hqdefault.jpg';

            // Create preview with storyboard simulation
            previewContainer.innerHTML =
                '<div class="video-preview-youtube">' +
                    '<img src="' + animatedThumb + '" alt="Video preview" class="video-preview-thumb">' +
                    '<div class="video-preview-progress"></div>' +
                    '<div class="video-preview-play">&#9658;</div>' +
                '</div>';

            // Simulate video scrubbing with different thumbnail timestamps
            simulateVideoScrub(videoId);
        } else {
            // Generic preview
            var thumbUrl = videoResult.querySelector('img')?.src || '';
            previewContainer.innerHTML =
                '<div class="video-preview-generic">' +
                    '<img src="' + thumbUrl + '" alt="Video preview">' +
                    '<div class="video-preview-play">&#9658;</div>' +
                '</div>';
        }

        previewContainer.classList.add('visible');
        previewContainer.setAttribute('aria-hidden', 'false');
    }

    function hideVideoPreview() {
        if (previewContainer) {
            previewContainer.classList.remove('visible');
            previewContainer.setAttribute('aria-hidden', 'true');
            previewContainer.innerHTML = '';
        }
        activePreview = null;
    }

    // Simulate video scrubbing using YouTube thumbnail storyboards
    function simulateVideoScrub(videoId) {
        var storyboardIndex = 0;
        var maxIndex = 3;
        var thumb = previewContainer.querySelector('.video-preview-thumb');
        var progress = previewContainer.querySelector('.video-preview-progress');

        if (!thumb) return;

        // YouTube storyboard thumbnails (different quality levels)
        var qualities = ['mqdefault', 'hqdefault', 'sddefault', 'maxresdefault'];

        var scrubInterval = setInterval(function() {
            if (!previewContainer.classList.contains('visible')) {
                clearInterval(scrubInterval);
                return;
            }

            storyboardIndex = (storyboardIndex + 1) % qualities.length;
            thumb.src = 'https://i.ytimg.com/vi/' + videoId + '/' + qualities[storyboardIndex] + '.jpg';

            // Update progress bar
            if (progress) {
                progress.style.width = ((storyboardIndex + 1) / qualities.length * 100) + '%';
            }
        }, 800);
    }

    // Touch handling for mobile swipe scrubbing
    var touchStartX = 0;
    var touchVideoResult = null;

    function handleTouchStart(e) {
        var videoResult = e.target.closest('.video-result, .video-item, [data-video-id]');
        if (!videoResult) return;

        touchStartX = e.touches[0].clientX;
        touchVideoResult = videoResult;
    }

    function handleTouchMove(e) {
        if (!touchVideoResult) return;

        var deltaX = e.touches[0].clientX - touchStartX;
        var progress = Math.min(100, Math.max(0, (deltaX / touchVideoResult.offsetWidth) * 100 + 50));

        // Show visual feedback for scrubbing
        var progressIndicator = touchVideoResult.querySelector('.touch-scrub-indicator');
        if (!progressIndicator) {
            progressIndicator = document.createElement('div');
            progressIndicator.className = 'touch-scrub-indicator';
            touchVideoResult.appendChild(progressIndicator);
        }
        progressIndicator.style.width = progress + '%';
    }

    function handleTouchEnd(e) {
        if (!touchVideoResult) return;

        var indicator = touchVideoResult.querySelector('.touch-scrub-indicator');
        if (indicator) {
            indicator.remove();
        }

        touchVideoResult = null;
        touchStartX = 0;
    }

    // Initialize on DOM ready
    document.addEventListener('DOMContentLoaded', initVideoPreview);

    // Expose for external use
    window.VideoPreview = {
        show: showVideoPreview,
        hide: hideVideoPreview
    };
})();


// ============================================================================
// ADVANCED SEARCH FORM (per IDEA.md)
// ============================================================================
(function() {
    'use strict';

    var advancedSearchModal = null;

    // Advanced search operators
    var OPERATORS = {
        exact: { label: 'Exact phrase', prefix: '"', suffix: '"', placeholder: 'exact words' },
        exclude: { label: 'Exclude', prefix: '-', suffix: '', placeholder: 'unwanted term' },
        site: { label: 'Site', prefix: 'site:', suffix: '', placeholder: 'example.com' },
        filetype: { label: 'File type', prefix: 'filetype:', suffix: '', placeholder: 'pdf' },
        intitle: { label: 'In title', prefix: 'intitle:', suffix: '', placeholder: 'title word' },
        inurl: { label: 'In URL', prefix: 'inurl:', suffix: '', placeholder: 'url part' },
        intext: { label: 'In text', prefix: 'intext:', suffix: '', placeholder: 'body text' },
        before: { label: 'Before date', prefix: 'before:', suffix: '', placeholder: '2024-01-01' },
        after: { label: 'After date', prefix: 'after:', suffix: '', placeholder: '2023-01-01' },
        or: { label: 'OR search', prefix: '', suffix: '', placeholder: 'term1 OR term2' }
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

        var html = '<header>' +
            '<h2 id="advanced-search-title">Advanced Search</h2>' +
            '<button type="button" class="close-btn" data-action="close" aria-label="Close">&times;</button>' +
        '</header>' +
        '<main class="advanced-search-content">' +
            '<form id="advanced-search-form">' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-main">All these words</label>' +
                    '<input type="text" id="adv-main" name="main" placeholder="search terms" autofocus>' +
                '</div>' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-exact">Exact phrase</label>' +
                    '<input type="text" id="adv-exact" name="exact" placeholder="&quot;exact phrase&quot;">' +
                '</div>' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-any">Any of these words</label>' +
                    '<input type="text" id="adv-any" name="any" placeholder="word1 OR word2">' +
                '</div>' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-exclude">None of these words</label>' +
                    '<input type="text" id="adv-exclude" name="exclude" placeholder="-unwanted">' +
                '</div>' +
                '<div class="advanced-search-row">' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-site">Site/domain</label>' +
                        '<input type="text" id="adv-site" name="site" placeholder="example.com">' +
                    '</div>' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-filetype">File type</label>' +
                        '<select id="adv-filetype" name="filetype">' +
                            '<option value="">Any</option>' +
                            '<option value="pdf">PDF</option>' +
                            '<option value="doc">Word (doc)</option>' +
                            '<option value="docx">Word (docx)</option>' +
                            '<option value="xls">Excel (xls)</option>' +
                            '<option value="xlsx">Excel (xlsx)</option>' +
                            '<option value="ppt">PowerPoint</option>' +
                            '<option value="txt">Text</option>' +
                            '<option value="csv">CSV</option>' +
                            '<option value="json">JSON</option>' +
                            '<option value="xml">XML</option>' +
                        '</select>' +
                    '</div>' +
                '</div>' +
                '<div class="advanced-search-row">' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-intitle">Words in title</label>' +
                        '<input type="text" id="adv-intitle" name="intitle" placeholder="title words">' +
                    '</div>' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-inurl">Words in URL</label>' +
                        '<input type="text" id="adv-inurl" name="inurl" placeholder="url-segment">' +
                    '</div>' +
                '</div>' +
                '<div class="advanced-search-row">' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-after">After date</label>' +
                        '<input type="date" id="adv-after" name="after">' +
                    '</div>' +
                    '<div class="advanced-search-group half">' +
                        '<label for="adv-before">Before date</label>' +
                        '<input type="date" id="adv-before" name="before">' +
                    '</div>' +
                '</div>' +
                '<div class="advanced-search-group">' +
                    '<label for="adv-region">Region</label>' +
                    '<select id="adv-region" name="region">' +
                        '<option value="">Any region</option>' +
                        '<option value="us">United States</option>' +
                        '<option value="uk">United Kingdom</option>' +
                        '<option value="de">Germany</option>' +
                        '<option value="fr">France</option>' +
                        '<option value="es">Spain</option>' +
                        '<option value="it">Italy</option>' +
                        '<option value="jp">Japan</option>' +
                        '<option value="cn">China</option>' +
                        '<option value="br">Brazil</option>' +
                        '<option value="au">Australia</option>' +
                        '<option value="ca">Canada</option>' +
                        '<option value="in">India</option>' +
                    '</select>' +
                '</div>' +
                '<div class="advanced-search-preview">' +
                    '<label>Query preview:</label>' +
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
            window.srAnnounce('Advanced search dialog opened');
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
            trigger.setAttribute('aria-label', 'Advanced search');
            trigger.setAttribute('title', 'Advanced search');
            trigger.addEventListener('click', showAdvancedSearch);

            var searchBtn = form.querySelector('button[type="submit"], .search-btn');
            if (searchBtn) {
                searchBtn.parentNode.insertBefore(trigger, searchBtn);
            } else {
                form.appendChild(trigger);
            }
        });
    }

    document.addEventListener('DOMContentLoaded', init);

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
        // Try API first
        fetch('/api/v1/search/related?q=' + encodeURIComponent(query) + '&limit=8')
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

        var html = '<h3 id="related-title" class="related-title">Related searches</h3>' +
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
            window.srAnnounce(suggestions.length + ' related searches available');
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

        var html = '<h3 class="paa-title">People also ask</h3>' +
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
                    (q.answer ? '<p>' + escapeHtml(q.answer) + '</p>' : '<p class="paa-loading">Loading...</p>') +
                    '<a href="/search?q=' + encodeURIComponent(q.question) + '" class="paa-link">Search for this</a>' +
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

    document.addEventListener('DOMContentLoaded', init);

    // Expose for external use
    window.RelatedSearches = {
        fetch: fetchRelatedSearches,
        render: renderRelatedSearches,
        renderPAA: renderPeopleAlsoAsk
    };
})();
