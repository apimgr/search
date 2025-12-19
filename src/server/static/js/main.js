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
