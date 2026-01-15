/**
 * Bangs - DuckDuckGo-style bang shortcuts
 * Per AI.md: Progressive enhancement - core functionality works without JS
 */
(function() {
    'use strict';

    // Bang definitions - loaded from /api/v1/bangs or use defaults
    const defaultBangs = {
        '!g': 'https://www.google.com/search?q={query}',
        '!gi': 'https://www.google.com/search?tbm=isch&q={query}',
        '!gm': 'https://www.google.com/maps/search/{query}',
        '!b': 'https://www.bing.com/search?q={query}',
        '!d': 'https://duckduckgo.com/?q={query}',
        '!ddg': 'https://duckduckgo.com/?q={query}',
        '!w': 'https://en.wikipedia.org/wiki/Special:Search?search={query}',
        '!yt': 'https://www.youtube.com/results?search_query={query}',
        '!gh': 'https://github.com/search?q={query}',
        '!so': 'https://stackoverflow.com/search?q={query}',
        '!r': 'https://www.reddit.com/search/?q={query}',
        '!a': 'https://www.amazon.com/s?k={query}',
        '!npm': 'https://www.npmjs.com/search?q={query}',
        '!mdn': 'https://developer.mozilla.org/en-US/search?q={query}'
    };

    let bangs = defaultBangs;

    // Load bangs from API
    async function loadBangs() {
        try {
            const response = await fetch('/api/v1/bangs');
            if (response.ok) {
                const data = await response.json();
                if (data.ok && data.data) {
                    bangs = {};
                    data.data.forEach(bang => {
                        bangs[bang.trigger] = bang.url;
                    });
                }
            }
        } catch (e) {
            // Use defaults on error
            console.debug('Using default bangs');
        }
    }

    // Check if query starts with a bang
    function parseBang(query) {
        query = query.trim();
        const match = query.match(/^(![\w]+)\s+(.+)$/);
        if (match) {
            const [, bang, searchQuery] = match;
            if (bangs[bang.toLowerCase()]) {
                return {
                    bang: bang.toLowerCase(),
                    query: searchQuery.trim(),
                    url: bangs[bang.toLowerCase()]
                };
            }
        }
        return null;
    }

    // Handle form submission with bang detection
    function handleSearchSubmit(event) {
        const form = event.target;
        const input = form.querySelector('input[name="q"]');
        if (!input) return;

        const query = input.value.trim();
        const bangData = parseBang(query);

        if (bangData) {
            event.preventDefault();
            const url = bangData.url.replace('{query}', encodeURIComponent(bangData.query));
            window.location.href = url;
        }
        // Otherwise, let the form submit normally to the server
    }

    // Show bang suggestion in search input
    function showBangSuggestion(input) {
        const query = input.value.trim();
        if (!query.startsWith('!')) return;

        const bangMatch = query.match(/^(![\w]*)$/);
        if (bangMatch) {
            const partial = bangMatch[1].toLowerCase();
            const matches = Object.keys(bangs).filter(b => b.startsWith(partial));
            // Could show autocomplete here - but keeping it simple for now
        }
    }

    // Initialize
    document.addEventListener('DOMContentLoaded', function() {
        // Load bangs
        loadBangs();

        // Attach to all search forms
        const searchForms = document.querySelectorAll('form.search-form, form[action="/search"], form[action*="search"]');
        searchForms.forEach(form => {
            form.addEventListener('submit', handleSearchSubmit);
        });

        // Optional: bang suggestion on input
        const searchInputs = document.querySelectorAll('input[name="q"]');
        searchInputs.forEach(input => {
            input.addEventListener('input', () => showBangSuggestion(input));
        });
    });
})();
