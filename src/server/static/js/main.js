// Theme Management
(function() {
    'use strict';

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

    window.toggleTheme = function() {
        const current = document.documentElement.getAttribute('data-theme') || 'dark';
        const next = current === 'dark' ? 'light' : 'dark';
        setTheme(next);
    };

    // Initialize theme on load
    document.addEventListener('DOMContentLoaded', function() {
        setTheme(getPreferredTheme());
    });

    // Listen for system theme changes
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function(e) {
        if (!localStorage.getItem(THEME_KEY)) {
            setTheme(e.matches ? 'dark' : 'light');
        }
    });
})();

// Mobile Navigation
(function() {
    'use strict';

    window.toggleNav = function() {
        const header = document.querySelector('.site-header');
        const navLinks = document.querySelector('.nav-links');
        if (header && navLinks) {
            header.classList.toggle('nav-open');
            navLinks.classList.toggle('active');
        }
    };

    // Close nav on outside click
    document.addEventListener('click', function(e) {
        const header = document.querySelector('.site-header');
        const navToggle = document.querySelector('.nav-toggle');
        if (header && header.classList.contains('nav-open')) {
            if (!e.target.closest('.nav-links') && !e.target.closest('.nav-toggle')) {
                header.classList.remove('nav-open');
                document.querySelector('.nav-links').classList.remove('active');
            }
        }
    });

    // Close nav on escape key
    document.addEventListener('keydown', function(e) {
        if (e.key === 'Escape') {
            const header = document.querySelector('.site-header');
            if (header && header.classList.contains('nav-open')) {
                header.classList.remove('nav-open');
                document.querySelector('.nav-links').classList.remove('active');
            }
        }
    });
})();

// Flash Messages
(function() {
    'use strict';

    document.addEventListener('DOMContentLoaded', function() {
        const flashes = document.querySelectorAll('.flash');
        flashes.forEach(function(flash) {
            // Auto-dismiss after 5 seconds
            setTimeout(function() {
                flash.classList.add('flash-fade');
                setTimeout(function() {
                    flash.remove();
                }, 300);
            }, 5000);
        });
    });
})();

// Search Autocomplete placeholder
(function() {
    'use strict';

    const searchInputs = document.querySelectorAll('.search-input, .header-search-input');

    searchInputs.forEach(function(input) {
        // Debounced input handler for future autocomplete
        let debounceTimer;
        input.addEventListener('input', function() {
            clearTimeout(debounceTimer);
            debounceTimer = setTimeout(function() {
                // Autocomplete logic will go here
            }, 300);
        });

        // Clear button functionality
        input.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                this.value = '';
                this.focus();
            }
        });
    });
})();

// Keyboard shortcuts
(function() {
    'use strict';

    document.addEventListener('keydown', function(e) {
        // Focus search with /
        if (e.key === '/' && !isInputFocused()) {
            e.preventDefault();
            const searchInput = document.querySelector('.search-input, .header-search-input');
            if (searchInput) {
                searchInput.focus();
                searchInput.select();
            }
        }

        // Toggle theme with t
        if (e.key === 't' && !isInputFocused()) {
            window.toggleTheme();
        }
    });

    function isInputFocused() {
        const activeElement = document.activeElement;
        return activeElement && (
            activeElement.tagName === 'INPUT' ||
            activeElement.tagName === 'TEXTAREA' ||
            activeElement.isContentEditable
        );
    }
})();

// Image lazy loading fallback
(function() {
    'use strict';

    if ('loading' in HTMLImageElement.prototype) {
        // Native lazy loading supported
        document.querySelectorAll('img[data-src]').forEach(function(img) {
            img.src = img.dataset.src;
        });
    } else {
        // Fallback with IntersectionObserver
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
            // Fallback for older browsers
            lazyImages.forEach(function(img) {
                img.src = img.dataset.src;
            });
        }
    }
})();

// Copy to clipboard utility
window.copyToClipboard = function(text, button) {
    navigator.clipboard.writeText(text).then(function() {
        const originalText = button.textContent;
        button.textContent = 'Copied!';
        button.classList.add('copied');
        setTimeout(function() {
            button.textContent = originalText;
            button.classList.remove('copied');
        }, 2000);
    }).catch(function(err) {
        console.error('Failed to copy:', err);
    });
};

// Image viewer for image results
(function() {
    'use strict';

    document.addEventListener('click', function(e) {
        const imageResult = e.target.closest('.image-result');
        if (imageResult && !e.target.closest('a')) {
            const fullUrl = imageResult.dataset.fullUrl;
            if (fullUrl) {
                window.open(fullUrl, '_blank', 'noopener,noreferrer');
            }
        }
    });
})();

// Infinite scroll placeholder for results
(function() {
    'use strict';

    const resultsContainer = document.querySelector('.search-results');
    if (!resultsContainer) return;

    const loadMoreTrigger = document.querySelector('.load-more-trigger');
    if (!loadMoreTrigger) return;

    if ('IntersectionObserver' in window) {
        const observer = new IntersectionObserver(function(entries) {
            entries.forEach(function(entry) {
                if (entry.isIntersecting) {
                    // Load more results logic will go here
                    const event = new CustomEvent('loadmore');
                    document.dispatchEvent(event);
                }
            });
        }, { rootMargin: '200px' });

        observer.observe(loadMoreTrigger);
    }
})();

// Toast Notification System (per AI.md PART 17)
// Position: top-right corner, stacked vertically
// Types: success (3s auto-dismiss), error (manual dismiss only), warning (5s), info (3s)
(function() {
    'use strict';

    // Create toast container if it doesn't exist
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

    // Show toast notification
    window.showToast = function(message, type) {
        type = type || 'info';
        const container = getToastContainer();

        const toast = document.createElement('div');
        toast.className = 'toast toast-' + type;
        toast.setAttribute('role', 'status');

        // Icon based on type
        const icons = {
            success: '✓',
            error: '✗',
            warning: '⚠',
            info: 'ℹ'
        };

        toast.innerHTML = '<span class="toast-icon">' + (icons[type] || icons.info) + '</span>' +
                         '<span class="toast-message">' + escapeHtml(message) + '</span>' +
                         '<button class="toast-close" aria-label="Dismiss">&times;</button>';

        // Close button handler
        toast.querySelector('.toast-close').addEventListener('click', function() {
            dismissToast(toast);
        });

        container.appendChild(toast);

        // Auto-dismiss based on type (error requires manual dismiss)
        const delays = {
            success: 3000,
            error: 0,      // No auto-dismiss
            warning: 5000,
            info: 3000
        };

        const delay = delays[type] || 3000;
        if (delay > 0) {
            setTimeout(function() {
                dismissToast(toast);
            }, delay);
        }

        // Trigger animation
        requestAnimationFrame(function() {
            toast.classList.add('toast-visible');
        });

        return toast;
    };

    function dismissToast(toast) {
        toast.classList.remove('toast-visible');
        toast.classList.add('toast-hiding');
        setTimeout(function() {
            if (toast.parentNode) {
                toast.parentNode.removeChild(toast);
            }
        }, 300);
    }

    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
})();

// Confirmation Dialog (per AI.md PART 17 - replaces confirm())
// Uses native <dialog> element with proper accessibility
(function() {
    'use strict';

    // Create confirmation dialog if it doesn't exist
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

    // Show confirmation dialog (returns Promise)
    window.showConfirm = function(message, options) {
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

            // Set danger styling if specified
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
    };
})();

// Service Worker registration
(function() {
    'use strict';

    if ('serviceWorker' in navigator) {
        window.addEventListener('load', function() {
            navigator.serviceWorker.register('/sw.js').catch(function(err) {
                // Service worker registration failed, likely doesn't exist
            });
        });
    }
})();

// Prevent double form submission
(function() {
    'use strict';

    document.addEventListener('DOMContentLoaded', function() {
        const forms = document.querySelectorAll('form');

        forms.forEach(function(form) {
            form.addEventListener('submit', function(e) {
                // Check if already submitting
                if (form.dataset.submitting === 'true') {
                    e.preventDefault();
                    return false;
                }

                // Mark as submitting
                form.dataset.submitting = 'true';

                // Disable all submit buttons in the form
                const buttons = form.querySelectorAll('button[type="submit"], input[type="submit"], .search-btn');
                buttons.forEach(function(btn) {
                    btn.disabled = true;
                    btn.classList.add('submitting');
                });

                // Re-enable after timeout (in case of error)
                setTimeout(function() {
                    form.dataset.submitting = 'false';
                    buttons.forEach(function(btn) {
                        btn.disabled = false;
                        btn.classList.remove('submitting');
                    });
                }, 5000);
            });
        });
    });
})();
