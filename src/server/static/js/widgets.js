/**
 * Widgets - Dynamic content widgets
 * Per AI.md: Progressive enhancement - content works without JS
 */
(function() {
    'use strict';

    // Widget refresh intervals (in milliseconds)
    const REFRESH_INTERVALS = {
        weather: 600000,    // 10 minutes
        news: 300000,       // 5 minutes
        stocks: 60000,      // 1 minute
        crypto: 30000,      // 30 seconds
        sports: 300000      // 5 minutes
    };

    // Widget configuration
    const widgets = new Map();

    // Register a widget
    function registerWidget(element) {
        const id = element.dataset.widgetId;
        const type = element.dataset.widgetType;
        const autoRefresh = element.dataset.widgetAutoRefresh !== 'false';

        if (!id || !type) return;

        widgets.set(id, {
            element,
            type,
            autoRefresh,
            lastUpdate: 0,
            interval: null
        });

        // Initial load
        loadWidget(id);

        // Setup auto-refresh if enabled
        if (autoRefresh && REFRESH_INTERVALS[type]) {
            const widget = widgets.get(id);
            widget.interval = setInterval(() => loadWidget(id), REFRESH_INTERVALS[type]);
        }
    }

    // Load widget content from API
    async function loadWidget(id) {
        const widget = widgets.get(id);
        if (!widget) return;

        const { element, type } = widget;

        try {
            element.classList.add('widget-loading');

            const response = await fetch(`/api/v1/widgets/${type}?id=${id}`);
            if (!response.ok) throw new Error('Widget load failed');

            const data = await response.json();
            if (!data.ok) throw new Error(data.message || 'Widget error');

            renderWidget(element, type, data.data);
            widget.lastUpdate = Date.now();
            element.classList.remove('widget-error');
        } catch (e) {
            console.error(`Widget ${id} error:`, e);
            element.classList.add('widget-error');
            // Don't clear existing content on refresh failure
        } finally {
            element.classList.remove('widget-loading');
        }
    }

    // Render widget content
    function renderWidget(element, type, data) {
        const content = element.querySelector('.widget-content');
        if (!content) return;

        switch (type) {
            case 'weather':
                renderWeatherWidget(content, data);
                break;
            case 'news':
                renderNewsWidget(content, data);
                break;
            case 'stocks':
                renderStocksWidget(content, data);
                break;
            case 'crypto':
                renderCryptoWidget(content, data);
                break;
            case 'sports':
                renderSportsWidget(content, data);
                break;
            default:
                content.innerHTML = '<p class="widget-message">Unknown widget type</p>';
        }
    }

    // Weather widget renderer
    function renderWeatherWidget(container, data) {
        if (!data || !data.temperature) {
            container.innerHTML = '<p class="widget-message">Weather data unavailable</p>';
            return;
        }
        container.innerHTML = `
            <div class="weather-current">
                <span class="weather-icon">${data.icon || '☁️'}</span>
                <span class="weather-temp">${data.temperature}°${data.unit || 'C'}</span>
                <span class="weather-condition">${data.condition || ''}</span>
            </div>
            <div class="weather-location">${data.location || ''}</div>
        `;
    }

    // News widget renderer
    function renderNewsWidget(container, data) {
        if (!data || !data.items || !data.items.length) {
            container.innerHTML = '<p class="widget-message">No news available</p>';
            return;
        }
        const items = data.items.slice(0, 5).map(item => `
            <li class="news-item">
                <a href="${escapeHtml(item.url)}" target="_blank" rel="noopener">
                    ${escapeHtml(item.title)}
                </a>
                <span class="news-source">${escapeHtml(item.source || '')}</span>
            </li>
        `).join('');
        container.innerHTML = `<ul class="news-list">${items}</ul>`;
    }

    // Stocks widget renderer
    function renderStocksWidget(container, data) {
        if (!data || !data.symbols || !data.symbols.length) {
            container.innerHTML = '<p class="widget-message">No stock data available</p>';
            return;
        }
        const rows = data.symbols.map(s => `
            <tr class="stock-row ${s.change >= 0 ? 'stock-up' : 'stock-down'}">
                <td class="stock-symbol">${escapeHtml(s.symbol)}</td>
                <td class="stock-price">${s.price.toFixed(2)}</td>
                <td class="stock-change">${s.change >= 0 ? '+' : ''}${s.change.toFixed(2)}%</td>
            </tr>
        `).join('');
        container.innerHTML = `<table class="stocks-table"><tbody>${rows}</tbody></table>`;
    }

    // Crypto widget renderer
    function renderCryptoWidget(container, data) {
        if (!data || !data.coins || !data.coins.length) {
            container.innerHTML = '<p class="widget-message">No crypto data available</p>';
            return;
        }
        const rows = data.coins.map(c => `
            <tr class="crypto-row ${c.change_24h >= 0 ? 'crypto-up' : 'crypto-down'}">
                <td class="crypto-symbol">${escapeHtml(c.symbol)}</td>
                <td class="crypto-price">$${formatNumber(c.price)}</td>
                <td class="crypto-change">${c.change_24h >= 0 ? '+' : ''}${c.change_24h.toFixed(2)}%</td>
            </tr>
        `).join('');
        container.innerHTML = `<table class="crypto-table"><tbody>${rows}</tbody></table>`;
    }

    // Sports widget renderer
    function renderSportsWidget(container, data) {
        if (!data || !data.scores || !data.scores.length) {
            container.innerHTML = '<p class="widget-message">No sports scores available</p>';
            return;
        }
        const games = data.scores.slice(0, 3).map(g => `
            <div class="sports-game">
                <div class="sports-teams">
                    <span class="team">${escapeHtml(g.home_team)}</span>
                    <span class="vs">vs</span>
                    <span class="team">${escapeHtml(g.away_team)}</span>
                </div>
                <div class="sports-score">${g.home_score} - ${g.away_score}</div>
                <div class="sports-status">${escapeHtml(g.status || '')}</div>
            </div>
        `).join('');
        container.innerHTML = games;
    }

    // Utility functions
    function escapeHtml(str) {
        if (!str) return '';
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    function formatNumber(num) {
        if (num >= 1000) return num.toLocaleString('en-US', { maximumFractionDigits: 2 });
        if (num >= 1) return num.toFixed(2);
        return num.toPrecision(4);
    }

    // Cleanup on page unload
    function cleanup() {
        widgets.forEach(widget => {
            if (widget.interval) {
                clearInterval(widget.interval);
            }
        });
        widgets.clear();
    }

    // Initialize
    document.addEventListener('DOMContentLoaded', function() {
        // Find and register all widgets
        const widgetElements = document.querySelectorAll('[data-widget-id]');
        widgetElements.forEach(registerWidget);
    });

    // Cleanup on unload
    window.addEventListener('beforeunload', cleanup);

    // Expose for manual refresh
    window.refreshWidget = loadWidget;
})();
