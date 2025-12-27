/**
 * Bang autocomplete and management
 */
(function() {
    'use strict';

    const BANGS_KEY = 'search_custom_bangs';
    const PREFS_KEY = 'search_preferences';

    // Get custom bangs from localStorage
    function getCustomBangs() {
        try {
            return JSON.parse(localStorage.getItem(BANGS_KEY) || '[]');
        } catch (e) {
            return [];
        }
    }

    // Get user preferences
    function getPreferences() {
        try {
            return JSON.parse(localStorage.getItem(PREFS_KEY) || '{}');
        } catch (e) {
            return {};
        }
    }

    // Apply saved preferences
    function applyPreferences() {
        const prefs = getPreferences();

        // Apply theme
        if (prefs.theme && prefs.theme !== 'system') {
            document.documentElement.setAttribute('data-theme', prefs.theme);
        }

        // Apply new tab preference to search results
        if (prefs.new_tab) {
            document.querySelectorAll('.result a').forEach(link => {
                link.setAttribute('target', '_blank');
                link.setAttribute('rel', 'noopener noreferrer');
            });
        }
    }

    // Initialize bang suggestions on search input
    function initBangSuggestions() {
        const searchInput = document.querySelector('input[name="q"]');
        if (!searchInput) return;

        let suggestionBox = null;

        // Create suggestion box
        function createSuggestionBox() {
            if (suggestionBox) return suggestionBox;

            suggestionBox = document.createElement('div');
            suggestionBox.className = 'bang-suggestions';
            suggestionBox.style.cssText = `
                position: absolute;
                background: var(--bg-secondary, #1e1e2e);
                border: 1px solid var(--border-color, #313244);
                border-radius: 4px;
                max-height: 300px;
                overflow-y: auto;
                z-index: 1000;
                display: none;
                width: 100%;
                box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3);
            `;

            // Position relative to input
            const parent = searchInput.parentElement;
            parent.style.position = 'relative';
            parent.appendChild(suggestionBox);

            return suggestionBox;
        }

        // Show suggestions
        function showSuggestions(bangs, query) {
            const box = createSuggestionBox();
            box.innerHTML = '';

            if (bangs.length === 0) {
                box.style.display = 'none';
                return;
            }

            bangs.slice(0, 10).forEach((bang, index) => {
                const item = document.createElement('div');
                item.className = 'bang-suggestion-item';
                item.innerHTML = `
                    <span class="bang-shortcut">!${escapeHtml(bang.shortcut)}</span>
                    <span>${escapeHtml(bang.name)}</span>
                `;

                item.addEventListener('click', () => {
                    // Replace bang in input
                    const currentValue = searchInput.value;
                    const bangMatch = currentValue.match(/!(\w*)$/);
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

        // Hide suggestions
        function hideSuggestions() {
            if (suggestionBox) {
                suggestionBox.style.display = 'none';
            }
        }

        // Filter bangs by partial shortcut
        function filterBangs(partial) {
            const builtinBangs = window.__BUILTIN_BANGS || [];
            const customBangs = getCustomBangs();
            const allBangs = [...customBangs, ...builtinBangs];

            partial = partial.toLowerCase();
            return allBangs.filter(bang =>
                bang.shortcut.toLowerCase().startsWith(partial) ||
                bang.name.toLowerCase().includes(partial) ||
                (bang.aliases && bang.aliases.some(a => a.toLowerCase().startsWith(partial)))
            );
        }

        // Handle input
        searchInput.addEventListener('input', function() {
            const value = this.value;
            const bangMatch = value.match(/!(\w*)$/);

            if (bangMatch && bangMatch[1].length > 0) {
                const matches = filterBangs(bangMatch[1]);
                showSuggestions(matches, bangMatch[1]);
            } else {
                hideSuggestions();
            }
        });

        // Handle keyboard navigation
        searchInput.addEventListener('keydown', function(e) {
            if (!suggestionBox || suggestionBox.style.display === 'none') return;

            const items = suggestionBox.querySelectorAll('.bang-suggestion-item');
            const selected = suggestionBox.querySelector('.bang-suggestion-item.selected');
            let index = Array.from(items).indexOf(selected);

            if (e.key === 'ArrowDown') {
                e.preventDefault();
                if (selected) selected.classList.remove('selected');
                index = (index + 1) % items.length;
                items[index].classList.add('selected');
                items[index].style.background = 'var(--bg-tertiary, #313244)';
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                if (selected) selected.classList.remove('selected');
                index = index <= 0 ? items.length - 1 : index - 1;
                items[index].classList.add('selected');
                items[index].style.background = 'var(--bg-tertiary, #313244)';
            } else if (e.key === 'Enter' && selected) {
                e.preventDefault();
                selected.click();
            } else if (e.key === 'Escape') {
                hideSuggestions();
            }
        });

        // Hide on click outside
        document.addEventListener('click', function(e) {
            if (!searchInput.contains(e.target) && (!suggestionBox || !suggestionBox.contains(e.target))) {
                hideSuggestions();
            }
        });
    }

    // Escape HTML
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Initialize on DOM ready
    document.addEventListener('DOMContentLoaded', function() {
        applyPreferences();
        initBangSuggestions();
    });

    // Export for use by other scripts
    window.SearchBangs = {
        getCustomBangs: getCustomBangs,
        getPreferences: getPreferences
    };
})();
