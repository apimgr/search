/**
 * Widget Manager
 * Handles widget rendering, data fetching, and user preferences
 */
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
            refreshInterval: 900000, // 15 minutes
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
            refreshInterval: 1800000, // 30 minutes
            render: renderNewsWidget
        },
        stocks: {
            type: 'stocks',
            name: 'Stocks',
            icon: 'chart-line',
            category: 'data',
            refreshInterval: 300000, // 5 minutes
            render: renderStocksWidget
        },
        crypto: {
            type: 'crypto',
            name: 'Crypto',
            icon: 'bitcoin',
            category: 'data',
            refreshInterval: 300000, // 5 minutes
            render: renderCryptoWidget
        },
        sports: {
            type: 'sports',
            name: 'Sports',
            icon: 'futbol',
            category: 'data',
            refreshInterval: 300000, // 5 minutes
            render: renderSportsWidget
        },
        rss: {
            type: 'rss',
            name: 'RSS Feeds',
            icon: 'rss',
            category: 'user',
            refreshInterval: 1800000, // 30 minutes
            render: renderRSSWidget
        }
    };

    // Get enabled widgets from localStorage
    function getEnabledWidgets() {
        try {
            const saved = localStorage.getItem(WIDGETS_KEY);
            if (saved) {
                return JSON.parse(saved);
            }
        } catch (e) {
            console.error('Failed to load widget preferences:', e);
        }
        // Return default widgets if nothing saved
        return null;
    }

    // Save enabled widgets to localStorage
    function saveEnabledWidgets(widgets) {
        localStorage.setItem(WIDGETS_KEY, JSON.stringify(widgets));
    }

    // Get widget settings from localStorage
    function getWidgetSettings(widgetType) {
        try {
            const key = WIDGET_SETTINGS_PREFIX + widgetType;
            return JSON.parse(localStorage.getItem(key) || '{}');
        } catch (e) {
            return {};
        }
    }

    // Save widget settings to localStorage
    function saveWidgetSettings(widgetType, settings) {
        const key = WIDGET_SETTINGS_PREFIX + widgetType;
        localStorage.setItem(key, JSON.stringify(settings));
    }

    // Fetch data widget from API
    async function fetchWidgetData(widgetType, params = {}) {
        const queryString = new URLSearchParams(params).toString();
        const url = `/api/v1/widgets/${widgetType}${queryString ? '?' + queryString : ''}`;

        try {
            const response = await fetch(url);
            const result = await response.json();
            if (result.success) {
                return result.data;
            }
            throw new Error(result.error?.message || 'Unknown error');
        } catch (e) {
            console.error(`Failed to fetch ${widgetType} widget:`, e);
            return null;
        }
    }

    // Utility: escape HTML
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }

    // ========== WIDGET RENDER FUNCTIONS ==========

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

            container.innerHTML = `
                <div class="clock-widget">
                    <div class="clock-time">${timeStr}</div>
                    <div class="clock-date">${dateStr}</div>
                </div>
            `;
        }

        update();
        return setInterval(update, 1000);
    }

    function renderWeatherWidget(container, data, settings) {
        if (!data || data.error) {
            const city = settings.city || 'your location';
            container.innerHTML = `
                <div class="widget-placeholder">
                    <div class="widget-placeholder-icon">&#9925;</div>
                    <div class="widget-placeholder-text">Set your city in widget settings</div>
                    <button class="widget-settings-btn" onclick="WidgetManager.showSettings('weather')">Configure</button>
                </div>
            `;
            return;
        }

        const iconMap = {
            'clear': '&#9728;',
            'sunny': '&#9728;',
            'cloudy': '&#9729;',
            'partly-cloudy': '&#9925;',
            'rain': '&#127783;',
            'snow': '&#127784;',
            'thunderstorm': '&#9928;',
            'fog': '&#127787;'
        };

        const icon = iconMap[data.condition] || '&#9925;';
        const temp = Math.round(data.temperature);
        const feelsLike = Math.round(data.feels_like || data.temperature);

        container.innerHTML = `
            <div class="weather-widget">
                <div class="weather-main">
                    <span class="weather-icon">${icon}</span>
                    <span class="weather-temp">${temp}&deg;</span>
                </div>
                <div class="weather-details">
                    <div class="weather-location">${escapeHtml(data.location)}</div>
                    <div class="weather-description">${escapeHtml(data.description)}</div>
                    <div class="weather-extra">
                        Feels like ${feelsLike}&deg; &middot; ${data.humidity}% humidity
                    </div>
                </div>
            </div>
        `;
    }

    function renderQuickLinksWidget(container, data, settings) {
        const links = settings.links || [];

        if (links.length === 0) {
            container.innerHTML = `
                <div class="widget-placeholder">
                    <div class="widget-placeholder-text">No links yet</div>
                    <button class="widget-add-btn" onclick="WidgetManager.addQuickLink()">+ Add Link</button>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="quicklinks-widget">
                ${links.map((link, i) => `
                    <a href="${escapeHtml(link.url)}" class="quicklink-item" title="${escapeHtml(link.name)}">
                        <img src="https://www.google.com/s2/favicons?domain=${encodeURIComponent(new URL(link.url).hostname)}&sz=32"
                             alt="" class="quicklink-favicon" onerror="this.style.display='none'">
                        <span class="quicklink-name">${escapeHtml(link.name)}</span>
                    </a>
                `).join('')}
                <button class="quicklink-add" onclick="WidgetManager.addQuickLink()" title="Add link">+</button>
            </div>
        `;
    }

    function renderCalculatorWidget(container) {
        container.innerHTML = `
            <div class="calculator-widget">
                <input type="text" class="calc-display" id="calc-display" readonly placeholder="0">
                <div class="calc-buttons">
                    <button onclick="WidgetManager.calcInput('7')">7</button>
                    <button onclick="WidgetManager.calcInput('8')">8</button>
                    <button onclick="WidgetManager.calcInput('9')">9</button>
                    <button onclick="WidgetManager.calcInput('/')" class="calc-op">/</button>
                    <button onclick="WidgetManager.calcInput('4')">4</button>
                    <button onclick="WidgetManager.calcInput('5')">5</button>
                    <button onclick="WidgetManager.calcInput('6')">6</button>
                    <button onclick="WidgetManager.calcInput('*')" class="calc-op">*</button>
                    <button onclick="WidgetManager.calcInput('1')">1</button>
                    <button onclick="WidgetManager.calcInput('2')">2</button>
                    <button onclick="WidgetManager.calcInput('3')">3</button>
                    <button onclick="WidgetManager.calcInput('-')" class="calc-op">-</button>
                    <button onclick="WidgetManager.calcInput('0')">0</button>
                    <button onclick="WidgetManager.calcInput('.')">.</button>
                    <button onclick="WidgetManager.calcEquals()" class="calc-eq">=</button>
                    <button onclick="WidgetManager.calcInput('+')" class="calc-op">+</button>
                    <button onclick="WidgetManager.calcClear()" class="calc-clear">C</button>
                </div>
            </div>
        `;
    }

    function renderNotesWidget(container, data, settings) {
        const notes = settings.notes || '';

        container.innerHTML = `
            <div class="notes-widget">
                <textarea class="notes-textarea"
                          placeholder="Type your notes here..."
                          oninput="WidgetManager.saveNotes(this.value)">${escapeHtml(notes)}</textarea>
            </div>
        `;
    }

    function renderCalendarWidget(container) {
        const now = new Date();
        const month = now.getMonth();
        const year = now.getFullYear();
        const today = now.getDate();

        const firstDay = new Date(year, month, 1).getDay();
        const daysInMonth = new Date(year, month + 1, 0).getDate();

        const monthNames = ['January', 'February', 'March', 'April', 'May', 'June',
                           'July', 'August', 'September', 'October', 'November', 'December'];

        let days = '';
        for (let i = 0; i < firstDay; i++) {
            days += '<span class="calendar-day empty"></span>';
        }
        for (let d = 1; d <= daysInMonth; d++) {
            const isToday = d === today ? ' today' : '';
            days += `<span class="calendar-day${isToday}">${d}</span>`;
        }

        container.innerHTML = `
            <div class="calendar-widget">
                <div class="calendar-header">${monthNames[month]} ${year}</div>
                <div class="calendar-weekdays">
                    <span>Su</span><span>Mo</span><span>Tu</span><span>We</span>
                    <span>Th</span><span>Fr</span><span>Sa</span>
                </div>
                <div class="calendar-days">${days}</div>
            </div>
        `;
    }

    function renderConverterWidget(container, data, settings) {
        const category = settings.defaultCategory || 'length';

        container.innerHTML = `
            <div class="converter-widget">
                <select id="converter-category" onchange="WidgetManager.updateConverterUnits()">
                    <option value="length" ${category === 'length' ? 'selected' : ''}>Length</option>
                    <option value="weight" ${category === 'weight' ? 'selected' : ''}>Weight</option>
                    <option value="temperature" ${category === 'temperature' ? 'selected' : ''}>Temperature</option>
                    <option value="volume" ${category === 'volume' ? 'selected' : ''}>Volume</option>
                </select>
                <div class="converter-row">
                    <input type="number" id="converter-from" oninput="WidgetManager.convert()" placeholder="0">
                    <select id="converter-from-unit" onchange="WidgetManager.convert()"></select>
                </div>
                <div class="converter-equals">=</div>
                <div class="converter-row">
                    <input type="number" id="converter-to" readonly placeholder="0">
                    <select id="converter-to-unit" onchange="WidgetManager.convert()"></select>
                </div>
            </div>
        `;

        WidgetManager.updateConverterUnits();
    }

    function renderNewsWidget(container, data, settings) {
        if (!data || !data.items || data.items.length === 0) {
            container.innerHTML = `
                <div class="widget-placeholder">
                    <div class="widget-placeholder-text">No news available</div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="news-widget">
                ${data.items.slice(0, 5).map(item => `
                    <a href="${escapeHtml(item.url)}" class="news-item" target="_blank" rel="noopener">
                        <div class="news-title">${escapeHtml(item.title)}</div>
                        <div class="news-source">${escapeHtml(item.source)}</div>
                    </a>
                `).join('')}
            </div>
        `;
    }

    function renderStocksWidget(container, data, settings) {
        if (!data || !data.symbols || data.symbols.length === 0) {
            container.innerHTML = `
                <div class="widget-placeholder">
                    <div class="widget-placeholder-text">Configure stock symbols</div>
                    <button class="widget-settings-btn" onclick="WidgetManager.showSettings('stocks')">Configure</button>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="stocks-widget">
                ${data.symbols.map(stock => {
                    const changeClass = stock.change >= 0 ? 'positive' : 'negative';
                    const changeSign = stock.change >= 0 ? '+' : '';
                    return `
                        <div class="stock-item">
                            <div class="stock-symbol">${escapeHtml(stock.symbol)}</div>
                            <div class="stock-price">$${stock.price.toFixed(2)}</div>
                            <div class="stock-change ${changeClass}">${changeSign}${stock.change_percent.toFixed(2)}%</div>
                        </div>
                    `;
                }).join('')}
            </div>
        `;
    }

    function renderCryptoWidget(container, data, settings) {
        if (!data || !data.coins || data.coins.length === 0) {
            container.innerHTML = `
                <div class="widget-placeholder">
                    <div class="widget-placeholder-text">Loading crypto prices...</div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="crypto-widget">
                ${data.coins.map(coin => {
                    const changeClass = coin.change_24h >= 0 ? 'positive' : 'negative';
                    const changeSign = coin.change_24h >= 0 ? '+' : '';
                    return `
                        <div class="crypto-item">
                            <div class="crypto-name">${escapeHtml(coin.name)}</div>
                            <div class="crypto-price">$${coin.price.toLocaleString(undefined, {minimumFractionDigits: 2, maximumFractionDigits: 2})}</div>
                            <div class="crypto-change ${changeClass}">${changeSign}${coin.change_24h.toFixed(2)}%</div>
                        </div>
                    `;
                }).join('')}
            </div>
        `;
    }

    function renderSportsWidget(container, data, settings) {
        if (!data || !data.games || data.games.length === 0) {
            container.innerHTML = `
                <div class="widget-placeholder">
                    <div class="widget-placeholder-text">No games today</div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="sports-widget">
                ${data.games.slice(0, 3).map(game => `
                    <div class="sports-game">
                        <div class="sports-teams">
                            <span>${escapeHtml(game.home_team)}</span>
                            <span class="sports-vs">vs</span>
                            <span>${escapeHtml(game.away_team)}</span>
                        </div>
                        <div class="sports-score">${game.home_score} - ${game.away_score}</div>
                        <div class="sports-status">${escapeHtml(game.status)}</div>
                    </div>
                `).join('')}
            </div>
        `;
    }

    function renderRSSWidget(container, data, settings) {
        const feeds = settings.feeds || [];

        if (feeds.length === 0) {
            container.innerHTML = `
                <div class="widget-placeholder">
                    <div class="widget-placeholder-text">No RSS feeds configured</div>
                    <button class="widget-settings-btn" onclick="WidgetManager.showSettings('rss')">Add Feed</button>
                </div>
            `;
            return;
        }

        if (!data || !data.items || data.items.length === 0) {
            container.innerHTML = `
                <div class="widget-placeholder">
                    <div class="widget-placeholder-text">Loading feeds...</div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="rss-widget">
                ${data.items.slice(0, 5).map(item => `
                    <a href="${escapeHtml(item.url)}" class="rss-item" target="_blank" rel="noopener">
                        <div class="rss-title">${escapeHtml(item.title)}</div>
                        <div class="rss-source">${escapeHtml(item.source)}</div>
                    </a>
                `).join('')}
            </div>
        `;
    }

    // ========== WIDGET MANAGER ==========

    const WidgetManager = {
        intervals: {},
        defaultWidgets: ['clock', 'weather', 'quicklinks', 'calculator'],

        init: function(defaultWidgets) {
            if (defaultWidgets && defaultWidgets.length > 0) {
                this.defaultWidgets = defaultWidgets;
            }

            const grid = document.getElementById('widget-grid');
            if (!grid) return;

            const enabledWidgets = getEnabledWidgets() || this.defaultWidgets;
            this.renderWidgetGrid(enabledWidgets);
        },

        renderWidgetGrid: function(enabledWidgets) {
            const grid = document.getElementById('widget-grid');
            if (!grid) return;

            // Clear existing intervals
            Object.values(this.intervals).forEach(clearInterval);
            this.intervals = {};

            grid.innerHTML = '';

            enabledWidgets.forEach(widgetType => {
                if (!WIDGETS[widgetType]) return;

                const widget = WIDGETS[widgetType];
                const div = document.createElement('div');
                div.className = `widget widget-${widgetType}`;
                div.dataset.widget = widgetType;
                div.innerHTML = `
                    <div class="widget-header">
                        <span class="widget-title">${widget.name}</span>
                        <button class="widget-menu" onclick="WidgetManager.showMenu('${widgetType}')" title="Widget options">&#8942;</button>
                    </div>
                    <div class="widget-content" id="widget-content-${widgetType}">
                        <div class="widget-loading">Loading...</div>
                    </div>
                `;
                grid.appendChild(div);

                // Initialize widget
                this.initWidget(widgetType);
            });
        },

        initWidget: async function(widgetType) {
            const widget = WIDGETS[widgetType];
            const container = document.getElementById(`widget-content-${widgetType}`);
            if (!container || !widget) return;

            const settings = getWidgetSettings(widgetType);

            if (widget.category === 'data') {
                // Fetch from API
                const data = await fetchWidgetData(widgetType, settings);
                const interval = widget.render(container, data, settings);
                if (interval) {
                    this.intervals[widgetType] = interval;
                }

                // Set up refresh
                if (widget.refreshInterval) {
                    this.intervals[widgetType + '_refresh'] = setInterval(async () => {
                        const newData = await fetchWidgetData(widgetType, settings);
                        widget.render(container, newData, settings);
                    }, widget.refreshInterval);
                }
            } else {
                // Render directly (tool/user widgets)
                const interval = widget.render(container, null, settings);
                if (interval) {
                    this.intervals[widgetType] = interval;
                }
            }
        },

        toggleWidget: function(widgetType) {
            const enabled = getEnabledWidgets() || this.defaultWidgets;
            const idx = enabled.indexOf(widgetType);
            if (idx >= 0) {
                enabled.splice(idx, 1);
            } else {
                enabled.push(widgetType);
            }
            saveEnabledWidgets(enabled);
            this.renderWidgetGrid(enabled);
        },

        removeWidget: function(widgetType) {
            const enabled = getEnabledWidgets() || this.defaultWidgets;
            const idx = enabled.indexOf(widgetType);
            if (idx >= 0) {
                enabled.splice(idx, 1);
                saveEnabledWidgets(enabled);
                this.renderWidgetGrid(enabled);
            }
        },

        showMenu: function(widgetType) {
            // Remove existing menu
            const existingMenu = document.querySelector('.widget-dropdown-menu');
            if (existingMenu) existingMenu.remove();

            const widget = document.querySelector(`[data-widget="${widgetType}"]`);
            if (!widget) return;

            const menu = document.createElement('div');
            menu.className = 'widget-dropdown-menu';
            menu.innerHTML = `
                <button onclick="WidgetManager.refreshWidget('${widgetType}')">Refresh</button>
                <button onclick="WidgetManager.showSettings('${widgetType}')">Settings</button>
                <button onclick="WidgetManager.removeWidget('${widgetType}')" class="danger">Remove</button>
            `;

            widget.querySelector('.widget-header').appendChild(menu);

            // Close on click outside
            setTimeout(() => {
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
            const menu = document.querySelector('.widget-dropdown-menu');
            if (menu) menu.remove();
        },

        showSettings: function(widgetType) {
            const settings = getWidgetSettings(widgetType);
            let content = '';

            switch (widgetType) {
                case 'weather':
                    content = `
                        <label>City:
                            <input type="text" id="setting-city" value="${escapeHtml(settings.city || '')}" placeholder="e.g., London, UK">
                        </label>
                        <label>Units:
                            <select id="setting-units">
                                <option value="metric" ${settings.units !== 'imperial' ? 'selected' : ''}>Celsius</option>
                                <option value="imperial" ${settings.units === 'imperial' ? 'selected' : ''}>Fahrenheit</option>
                            </select>
                        </label>
                    `;
                    break;
                case 'clock':
                    content = `
                        <label>Format:
                            <select id="setting-format">
                                <option value="24h" ${settings.format !== '12h' ? 'selected' : ''}>24-hour</option>
                                <option value="12h" ${settings.format === '12h' ? 'selected' : ''}>12-hour</option>
                            </select>
                        </label>
                    `;
                    break;
                case 'stocks':
                    content = `
                        <label>Stock Symbols (comma-separated):
                            <input type="text" id="setting-symbols" value="${escapeHtml((settings.symbols || ['AAPL', 'GOOGL', 'MSFT']).join(', '))}" placeholder="e.g., AAPL, GOOGL, MSFT">
                        </label>
                    `;
                    break;
                case 'crypto':
                    content = `
                        <label>Coins (comma-separated):
                            <input type="text" id="setting-coins" value="${escapeHtml((settings.coins || ['bitcoin', 'ethereum']).join(', '))}" placeholder="e.g., bitcoin, ethereum">
                        </label>
                    `;
                    break;
                case 'rss':
                    content = `
                        <label>RSS Feed URLs (one per line):
                            <textarea id="setting-feeds" rows="4" placeholder="https://example.com/feed.xml">${escapeHtml((settings.feeds || []).join('\n'))}</textarea>
                        </label>
                    `;
                    break;
                default:
                    content = '<p>No settings available for this widget.</p>';
            }

            // Create modal
            const modal = document.createElement('div');
            modal.className = 'widget-settings-modal';
            modal.innerHTML = `
                <div class="widget-settings-content">
                    <h3>${WIDGETS[widgetType]?.name || widgetType} Settings</h3>
                    <form id="widget-settings-form">
                        ${content}
                        <div class="widget-settings-actions">
                            <button type="button" onclick="this.closest('.widget-settings-modal').remove()">Cancel</button>
                            <button type="submit">Save</button>
                        </div>
                    </form>
                </div>
            `;

            document.body.appendChild(modal);

            // Handle form submission
            modal.querySelector('form').addEventListener('submit', (e) => {
                e.preventDefault();
                this.saveWidgetSettings(widgetType);
                modal.remove();
            });

            // Close on backdrop click
            modal.addEventListener('click', (e) => {
                if (e.target === modal) modal.remove();
            });

            // Close dropdown menu if open
            const menu = document.querySelector('.widget-dropdown-menu');
            if (menu) menu.remove();
        },

        saveWidgetSettings: function(widgetType) {
            const settings = getWidgetSettings(widgetType);

            switch (widgetType) {
                case 'weather':
                    settings.city = document.getElementById('setting-city')?.value || '';
                    settings.units = document.getElementById('setting-units')?.value || 'metric';
                    break;
                case 'clock':
                    settings.format = document.getElementById('setting-format')?.value || '24h';
                    break;
                case 'stocks':
                    const symbolsStr = document.getElementById('setting-symbols')?.value || '';
                    settings.symbols = symbolsStr.split(',').map(s => s.trim().toUpperCase()).filter(s => s);
                    break;
                case 'crypto':
                    const coinsStr = document.getElementById('setting-coins')?.value || '';
                    settings.coins = coinsStr.split(',').map(s => s.trim().toLowerCase()).filter(s => s);
                    break;
                case 'rss':
                    const feedsStr = document.getElementById('setting-feeds')?.value || '';
                    settings.feeds = feedsStr.split('\n').map(s => s.trim()).filter(s => s);
                    break;
            }

            saveWidgetSettings(widgetType, settings);
            this.initWidget(widgetType);
        },

        // Calculator methods
        calcExpression: '',
        calcInput: function(val) {
            this.calcExpression += val;
            const display = document.getElementById('calc-display');
            if (display) display.value = this.calcExpression;
        },
        calcClear: function() {
            this.calcExpression = '';
            const display = document.getElementById('calc-display');
            if (display) display.value = '';
        },
        calcEquals: function() {
            try {
                // Safe evaluation
                const result = Function('"use strict"; return (' + this.calcExpression.replace(/[^0-9+\-*/.()]/g, '') + ')')();
                const display = document.getElementById('calc-display');
                if (display) display.value = result;
                this.calcExpression = String(result);
            } catch (e) {
                const display = document.getElementById('calc-display');
                if (display) display.value = 'Error';
                this.calcExpression = '';
            }
        },

        // Quick Links methods
        addQuickLink: async function() {
            // Use custom prompts instead of JavaScript prompt()
            // Per TEMPLATE.md PART 16: NO JavaScript alerts
            const name = await showPrompt('Link name:');
            if (!name) return;

            const url = await showPrompt('URL (include https://):');
            if (!url) return;

            try {
                new URL(url);
            } catch {
                await showAlert('Invalid URL. Please include https://');
                return;
            }

            const settings = getWidgetSettings('quicklinks');
            settings.links = settings.links || [];
            settings.links.push({ name, url });
            saveWidgetSettings('quicklinks', settings);
            this.initWidget('quicklinks');
        },

        // Notes methods
        saveNotes: function(value) {
            const settings = getWidgetSettings('notes');
            settings.notes = value;
            saveWidgetSettings('notes', settings);
        },

        // Converter methods
        converterUnits: {
            length: { m: 1, km: 1000, cm: 0.01, mm: 0.001, mi: 1609.34, ft: 0.3048, in: 0.0254, yd: 0.9144 },
            weight: { kg: 1, g: 0.001, mg: 0.000001, lb: 0.453592, oz: 0.0283495, st: 6.35029 },
            temperature: { c: 'c', f: 'f', k: 'k' },
            volume: { l: 1, ml: 0.001, gal: 3.78541, qt: 0.946353, pt: 0.473176, cup: 0.236588, floz: 0.0295735 }
        },

        updateConverterUnits: function() {
            const category = document.getElementById('converter-category')?.value || 'length';
            const units = Object.keys(this.converterUnits[category] || {});

            ['from', 'to'].forEach(side => {
                const select = document.getElementById(`converter-${side}-unit`);
                if (select) {
                    select.innerHTML = units.map(u => `<option value="${u}">${u.toUpperCase()}</option>`).join('');
                }
            });

            this.convert();
        },

        convert: function() {
            const category = document.getElementById('converter-category')?.value || 'length';
            const fromValue = parseFloat(document.getElementById('converter-from')?.value) || 0;
            const fromUnit = document.getElementById('converter-from-unit')?.value || '';
            const toUnit = document.getElementById('converter-to-unit')?.value || '';
            const toInput = document.getElementById('converter-to');

            if (!toInput) return;

            let result;
            if (category === 'temperature') {
                result = this.convertTemperature(fromValue, fromUnit, toUnit);
            } else {
                const units = this.converterUnits[category];
                if (!units || !units[fromUnit] || !units[toUnit]) {
                    toInput.value = '';
                    return;
                }
                const baseValue = fromValue * units[fromUnit];
                result = baseValue / units[toUnit];
            }

            toInput.value = isNaN(result) ? '' : result.toFixed(4);
        },

        convertTemperature: function(value, from, to) {
            if (from === to) return value;

            // Convert to Celsius first
            let celsius;
            switch (from) {
                case 'c': celsius = value; break;
                case 'f': celsius = (value - 32) * 5/9; break;
                case 'k': celsius = value - 273.15; break;
                default: return NaN;
            }

            // Convert from Celsius to target
            switch (to) {
                case 'c': return celsius;
                case 'f': return celsius * 9/5 + 32;
                case 'k': return celsius + 273.15;
                default: return NaN;
            }
        },

        // Get list of all widgets
        getAllWidgets: function() {
            return Object.keys(WIDGETS).map(type => ({
                type,
                ...WIDGETS[type]
            }));
        },

        // Get enabled widgets
        getEnabledWidgets: function() {
            return getEnabledWidgets() || this.defaultWidgets;
        }
    };

    // Initialize on DOM ready
    document.addEventListener('DOMContentLoaded', function() {
        // Check if widget grid exists (homepage only)
        if (document.getElementById('widget-grid')) {
            // Get default widgets from data attribute if available
            const grid = document.getElementById('widget-grid');
            const defaultWidgets = grid.dataset.defaults ? JSON.parse(grid.dataset.defaults) : null;
            WidgetManager.init(defaultWidgets);
        }
    });

    // Export for global access
    window.WidgetManager = WidgetManager;
})();
