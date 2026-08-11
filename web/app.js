// AlgoEngine Client Dashboard Application (Vanilla JS + TradingView Lightweight Charts)

let chart = null;
let candlestickSeries = null;
let emaFastSeries = null;
let emaSlowSeries = null;
let vwapSeries = null;

let currentSymbol = "NSE:ITC-EQ"; // Default symbol: ITC Limited
let currentPeriod = "15m";       // Default interval: 15 minutes
let ws = null;
let isAlgoRunning = true;
let chartMarkers = [];
let isAuthenticated = false;

// OHLC candle storage for indicators
let candleDataMap = new Map();

document.addEventListener("DOMContentLoaded", () => {
    initChart();
    checkURLParams();
    connectWebSocket();
    fetchHealthStatus();
    fetchMetrics();
    fetchPositions();
    fetchSettings();
    fetchDataSummary();

    // Poll health status every 10s & Data summary every 15s
    setInterval(fetchHealthStatus, 10000);
    setInterval(fetchDataSummary, 15000);
});

function checkURLParams() {
    const params = new URLSearchParams(window.location.search);
    if (params.get("login") === "success") {
        logMessage("[Fyers Auth] Fyers OAuth Login Completed Successfully! Access Token set.");
        window.history.replaceState({}, document.title, window.location.pathname);
    } else if (params.get("login") === "error") {
        const err = params.get("message") || "Unknown error";
        logMessage(`[Fyers Auth Error] Fyers OAuth Login Failed: ${err}`);
        alert(`Fyers OAuth Login Failed: ${err}`);
        window.history.replaceState({}, document.title, window.location.pathname);
    }
}

function initChart() {
    const container = document.getElementById("chart-container");
    container.innerHTML = ""; // Clear existing

    chart = LightweightCharts.createChart(container, {
        layout: {
            background: { type: 'solid', color: '#0b0e14' },
            textColor: '#9CA3AF',
            fontSize: 12,
        },
        grid: {
            vertLines: { color: '#1f2430' },
            horzLines: { color: '#1f2430' },
        },
        crosshair: {
            mode: LightweightCharts.CrosshairMode.Normal,
        },
        rightPriceScale: {
            borderColor: '#2a2e3d',
            autoScale: true,
        },
        timeScale: {
            borderColor: '#2a2e3d',
            timeVisible: true,
            secondsVisible: false,
        },
    });

    candlestickSeries = chart.addCandlestickSeries({
        upColor: '#089981',
        downColor: '#f23645',
        borderUpColor: '#089981',
        borderDownColor: '#f23645',
        wickUpColor: '#089981',
        wickDownColor: '#f23645',
    });

    emaFastSeries = chart.addLineSeries({
        color: '#089981',
        lineWidth: 1.5,
        title: 'EMA 9',
    });

    emaSlowSeries = chart.addLineSeries({
        color: '#f97316',
        lineWidth: 1.5,
        title: 'EMA 21',
    });

    vwapSeries = chart.addLineSeries({
        color: '#a855f7',
        lineWidth: 1.5,
        lineStyle: LightweightCharts.LineStyle.Dashed,
        title: 'VWAP',
    });

    // Resize chart on window resize
    window.addEventListener('resize', () => {
        if (chart && container) {
            chart.applyOptions({
                width: container.clientWidth,
                height: container.clientHeight
            });
        }
    });
}

function loadHistoricalCandles() {
    if (!isAuthenticated) return;

    fetch(`/api/candles?symbol=${encodeURIComponent(currentSymbol)}&period=${currentPeriod}`)
        .then(res => {
            if (!res.ok) throw new Error("Unauthorized or empty data");
            return res.json();
        })
        .then(data => {
            if (!Array.isArray(data) || data.length === 0) return;

            candleDataMap.set(currentSymbol, data);
            renderChartBars(data);
        })
        .catch(err => {
            logMessage(`[Chart] Historical candles waiting for Fyers API stream...`);
        });
}

function renderChartBars(bars) {
    if (!candlestickSeries || !bars) return;

    const formattedBars = bars.map(b => ({
        time: b.timestamp,
        open: b.open,
        high: b.high,
        low: b.low,
        close: b.close
    }));

    // Remove duplicates and sort by timestamp
    const uniqueBars = [];
    const seen = new Set();
    for (const b of formattedBars) {
        if (!seen.has(b.time)) {
            seen.add(b.time);
            uniqueBars.push(b);
        }
    }
    uniqueBars.sort((a, b) => a.time - b.time);

    candlestickSeries.setData(uniqueBars);

    // Calculate Indicators
    const ema9Data = calculateEMA(uniqueBars, 9);
    const ema21Data = calculateEMA(uniqueBars, 21);
    const vwapData = calculateVWAP(uniqueBars);

    emaFastSeries.setData(ema9Data);
    emaSlowSeries.setData(ema21Data);
    vwapSeries.setData(vwapData);

    chart.timeScale().fitContent();
}

function connectWebSocket() {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
        logMessage("[WS] Connected to Go Algo Engine WebSocket hub");
    };

    ws.onclose = () => {
        logMessage("[WS] Connection lost. Retrying in 3 seconds...");
        setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = (err) => {
        console.error("WS error:", err);
    };

    ws.onmessage = (event) => {
        try {
            const msg = JSON.parse(event.data);
            handleWsEvent(msg);
        } catch (e) {
            console.error("Failed to parse WS msg:", e);
        }
    };
}

function handleWsEvent(msg) {
    switch (msg.type) {
        case "candle_update":
            onCandleUpdate(msg.data);
            break;
        case "trade_execution":
            onTradeExecution(msg.data);
            break;
        case "metrics_update":
            updateMetricsUI(msg.data);
            break;
        case "system_log":
            logMessage(msg.data);
            break;
    }
}

function onCandleUpdate(candle) {
    if (!isAuthenticated || !candle || candle.symbol !== currentSymbol) {
        return;
    }

    const bar = {
        time: candle.timestamp,
        open: candle.open,
        high: candle.high,
        low: candle.low,
        close: candle.close
    };

    candlestickSeries.update(bar);

    // Update indicators dynamically
    let bars = candleDataMap.get(currentSymbol) || [];
    const idx = bars.findIndex(b => b.timestamp === candle.timestamp);
    if (idx >= 0) {
        bars[idx] = candle;
    } else {
        bars.push(candle);
        if (bars.length > 2000) bars.shift();
    }
    candleDataMap.set(currentSymbol, bars);

    // Recalculate indicator latest points
    const ema9 = calculateEMA(bars, 9);
    const ema21 = calculateEMA(bars, 21);
    const vwap = calculateVWAP(bars);

    if (ema9.length > 0) emaFastSeries.update(ema9[ema9.length - 1]);
    if (ema21.length > 0) emaSlowSeries.update(ema21[ema21.length - 1]);
    if (vwap.length > 0) vwapSeries.update(vwap[vwap.length - 1]);
}

function onTradeExecution(data) {
    logMessage(`[Trade Signal] ${data.side} ${data.qty} shares of ${data.symbol} @ ₹${data.price} (${data.reason})`);

    if (data.symbol === currentSymbol) {
        const marker = {
            time: data.timestamp,
            position: data.side === "BUY" ? "belowBar" : "aboveBar",
            color: data.side === "BUY" ? "#089981" : "#f23645",
            shape: data.side === "BUY" ? "arrowUp" : "arrowDown",
            text: `${data.side} @ ₹${data.price}`,
        };
        chartMarkers.push(marker);
        candlestickSeries.setMarkers(chartMarkers);
    }

    fetchPositions();
    fetchMetrics();
}

function updateMetricsUI(m) {
    if (!m) return;

    const pnlElem = document.getElementById("metric-pnl");
    const headerPnl = document.getElementById("header-pnl");

    const pnlFormatted = `₹${m.total_pnl.toFixed(2)}`;
    pnlElem.innerText = pnlFormatted;
    headerPnl.innerText = pnlFormatted;

    if (m.total_pnl > 0) {
        pnlElem.className = "text-2xl font-bold font-mono mt-1 text-accentGreen";
        headerPnl.className = "text-sm font-bold font-mono text-accentGreen";
    } else if (m.total_pnl < 0) {
        pnlElem.className = "text-2xl font-bold font-mono mt-1 text-accentRed";
        headerPnl.className = "text-sm font-bold font-mono text-accentRed";
    } else {
        pnlElem.className = "text-2xl font-bold font-mono mt-1 text-gray-200";
        headerPnl.className = "text-sm font-bold font-mono text-gray-200";
    }

    document.getElementById("metric-trades").innerText = m.total_trades;
    document.getElementById("metric-winloss").innerText = `${m.winning_trades} Win / ${m.losing_trades} Loss`;
    document.getElementById("metric-winrate").innerText = `${m.win_rate.toFixed(1)}%`;
    document.getElementById("metric-drawdown").innerText = `${m.max_drawdown.toFixed(2)}%`;
    document.getElementById("metric-margin").innerText = `₹${m.available_margin.toLocaleString('en-IN')}`;
}

function fetchPositions() {
    fetch("/api/positions")
        .then(res => res.json())
        .then(positions => {
            renderPositionsTable(positions);
        });
}

function renderPositionsTable(positions) {
    const tbody = document.getElementById("positions-table-body");
    const countBadge = document.getElementById("open-pos-count");

    if (!Array.isArray(positions) || positions.length === 0) {
        tbody.innerHTML = `<tr><td colspan="6" class="py-8 text-center text-gray-500 font-sans">No active positions running</td></tr>`;
        countBadge.innerText = "0 Open";
        return;
    }

    const openPos = positions.filter(p => p.status === "OPEN");
    countBadge.innerText = `${openPos.length} Open`;

    let html = "";
    positions.forEach(p => {
        const sideColor = p.side === "BUY" ? "text-accentGreen" : "text-accentRed";
        const pnl = p.status === "OPEN" ? p.unrealized_pnl : p.realized_pnl;
        const pnlColor = pnl >= 0 ? "text-accentGreen" : "text-accentRed";

        html += `
            <tr class="hover:bg-cardBorder/30">
                <td class="py-2.5 px-2 font-semibold text-white">${p.symbol}</td>
                <td class="py-2.5 px-2 ${sideColor} font-bold">${p.side}</td>
                <td class="py-2.5 px-2">${p.qty}</td>
                <td class="py-2.5 px-2">₹${p.entry_price.toFixed(2)}</td>
                <td class="py-2.5 px-2">₹${p.current_price.toFixed(2)}</td>
                <td class="py-2.5 px-2 text-right font-bold ${pnlColor}">₹${pnl.toFixed(2)}</td>
            </tr>
        `;
    });
    tbody.innerHTML = html;
}

function fetchHealthStatus() {
    fetch("/api/health")
        .then(res => res.json())
        .then(data => {
            if (data.uptime) {
                document.getElementById("uptime-text").innerText = data.uptime;
            }
            setAlgoToggleUI(data.algo_running);

            const prevAuth = isAuthenticated;
            isAuthenticated = data.authenticated;

            const banner = document.getElementById("unauth-banner");
            const logoutBtn = document.getElementById("logout-btn");

            if (isAuthenticated) {
                banner.classList.add("hidden");
                logoutBtn.classList.remove("hidden");
                updateApiStatus(true, "FYERS API LIVE");

                if (!prevAuth) {
                    loadHistoricalCandles();
                }
            } else {
                banner.classList.remove("hidden");
                logoutBtn.classList.add("hidden");
                updateApiStatus(false, "AUTH REQUIRED");
            }
        });
}

function fetchMetrics() {
    fetch("/api/metrics")
        .then(res => res.json())
        .then(updateMetricsUI);
}

function toggleAlgo() {
    fetch("/api/strategy/toggle", { method: "POST" })
        .then(res => res.json())
        .then(data => {
            setAlgoToggleUI(data.algo_running);
            logMessage(data.message);
        });
}

function logout() {
    if (!confirm("Are you sure you want to log out of Fyers API?")) return;

    fetch("/api/fyers/logout", { method: "POST" })
        .then(res => res.json())
        .then(data => {
            logMessage("[Fyers Auth] Logged out.");
            isAuthenticated = false;
            fetchHealthStatus();

            if (candlestickSeries) candlestickSeries.setData([]);
            if (emaFastSeries) emaFastSeries.setData([]);
            if (emaSlowSeries) emaSlowSeries.setData([]);
            if (vwapSeries) vwapSeries.setData([]);
            renderPositionsTable([]);
        });
}

function setAlgoToggleUI(running) {
    isAlgoRunning = running;
    const btn = document.getElementById("algo-toggle-btn");
    const thumb = document.getElementById("algo-toggle-thumb");

    if (running) {
        btn.className = "relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out bg-accentGreen focus:outline-none";
        thumb.className = "pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out translate-x-5";
    } else {
        btn.className = "relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out bg-gray-600 focus:outline-none";
        thumb.className = "pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out translate-x-0";
    }
}

function updateApiStatus(connected, text) {
    const dot = document.getElementById("api-status-dot");
    const label = document.getElementById("api-status-text");

    if (connected) {
        dot.className = "w-2.5 h-2.5 rounded-full bg-accentGreen";
        label.innerText = text;
    } else {
        dot.className = "w-2.5 h-2.5 rounded-full bg-accentRed";
        label.innerText = text;
    }
}

function changeSymbol() {
    currentSymbol = document.getElementById("symbol-select").value;
    updateChartHeaderTitle(currentSymbol);
    chartMarkers = [];
    loadHistoricalCandles();
    logMessage(`[Chart] Switched symbol view to: ${currentSymbol}`);
}

function changePeriod(period) {
    currentPeriod = period;
    
    // Highlight period button
    ['15m', '1h', '1m', '5m'].forEach(p => {
        const btn = document.getElementById(`period-${p}-btn`);
        if (btn) {
            if (p === period) {
                btn.className = "px-2.5 py-1 rounded bg-cardBorder text-white font-bold";
            } else {
                btn.className = "px-2.5 py-1 rounded text-gray-400 hover:text-white";
            }
        }
    });

    loadHistoricalCandles();
    logMessage(`[Chart] Switched timeframe resolution to: ${period}`);
}

function logMessage(msg) {
    const term = document.getElementById("log-terminal");
    const div = document.createElement("div");
    const timeStr = new Date().toLocaleTimeString();
    div.innerHTML = `<span class="text-gray-500">[${timeStr}]</span> ${msg}`;
    term.appendChild(div);
    term.scrollTop = term.scrollHeight;
}

function clearLogs() {
    document.getElementById("log-terminal").innerHTML = "";
}

// Settings Modal Management
function openSettingsModal() {
    fetchSettings();
    document.getElementById("settings-modal").classList.remove("hidden");
}

function closeSettingsModal() {
    document.getElementById("settings-modal").classList.add("hidden");
}

function fetchSettings() {
    fetch("/api/settings")
        .then(res => res.json())
        .then(data => {
            if (!data) return;
            document.getElementById("setting-appid").value = data.fyers_app_id || "";
            document.getElementById("setting-secret").value = data.fyers_secret_key || "";
            document.getElementById("setting-redirect").value = data.fyers_redirect_uri || "http://localhost:8080/api/fyers/callback";
            document.getElementById("setting-pin").value = data.fyers_pin || "";
            document.getElementById("setting-authcode").value = data.fyers_auth_code || "";
            document.getElementById("setting-token").value = data.fyers_access_token || "";
        });
}

function saveSettings(e) {
    e.preventDefault();
    const payload = {
        fyers_app_id: document.getElementById("setting-appid").value.trim(),
        fyers_secret_key: document.getElementById("setting-secret").value.trim(),
        fyers_redirect_uri: document.getElementById("setting-redirect").value.trim(),
        fyers_pin: document.getElementById("setting-pin").value.trim(),
        fyers_access_token: document.getElementById("setting-token").value.trim(),
    };

    fetch("/api/settings", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
    })
    .then(res => res.json())
    .then(data => {
        logMessage(`[Settings] ${data.message}`);
        closeSettingsModal();
        fetchHealthStatus();
    });
}

function generateAuthURL() {
    fetch("/api/fyers/auth-url")
        .then(res => res.json())
        .then(data => {
            if (data.error) {
                alert(data.error);
                return;
            }
            window.location.href = data.auth_url;
        });
}

function validateAuthCode() {
    const code = document.getElementById("setting-authcode").value.trim();
    if (!code) {
        alert("Please paste the Fyers Authorization Code first");
        return;
    }

    fetch("/api/fyers/validate-code", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ auth_code: code })
    })
    .then(res => res.json())
    .then(data => {
        if (data.error) {
            alert(`Validation Error: ${data.error}`);
            return;
        }
        document.getElementById("setting-token").value = data.access_token;
        logMessage("[Fyers Auth] Access Token validated successfully!");
        alert("Fyers Login Verified! Access Token updated.");
        fetchHealthStatus();
    });
}

// Cash Market Data Manager & Left Watchlist Sidebar Client Logic
function fetchDataSummary() {
    fetch("/api/datamanager/summary")
        .then(res => res.json())
        .then(data => {
            if (!data) return;

            document.getElementById("data-stock-csv").value = data.stock_list_csv || "";
            document.getElementById("data-interval").value = data.interval || "15m";

            const badge = document.getElementById("data-status-badge");
            if (data.is_syncing) {
                badge.className = "bg-yellow-950 text-yellow-300 border border-yellow-800/60 text-[10px] px-2 py-0.5 rounded-full font-mono animate-pulse";
                badge.innerText = "Downloading In Progress...";
            } else {
                badge.className = "bg-blue-950 text-blue-300 border border-blue-800/60 text-[10px] px-2 py-0.5 rounded-full font-mono";
                badge.innerText = `Idle (Last Sync: ${data.last_sync_time})`;
            }

            document.getElementById("data-summary-meta").innerText = `${data.total_symbols} Stocks • ${data.total_bars.toLocaleString('en-IN')} Total Bars`;
            renderDataSummaryTable(data.symbol_details);
            renderWatchlistSidebar(data.symbol_details);
        });
}

function renderWatchlistSidebar(details) {
    const container = document.getElementById("watchlist-container");
    const countBadge = document.getElementById("watchlist-count");

    if (!Array.isArray(details) || details.length === 0) {
        container.innerHTML = `<div class="text-center text-gray-500 text-xs py-8">No stocks in database</div>`;
        countBadge.innerText = "0";
        return;
    }

    countBadge.innerText = details.length;
    let html = "";

    details.forEach(item => {
        const isSelected = item.symbol === currentSymbol;
        const activeClass = isSelected
            ? "bg-accentGreen/20 border-accentGreen text-white font-bold"
            : "bg-cardBg/60 border-cardBorder text-gray-300 hover:bg-cardBorder/40 hover:text-white";

        const cleanName = item.symbol.replace("NSE:", "").replace("-EQ", "");

        html += `
            <div onclick="selectWatchlistStock('${item.symbol}')" class="p-2.5 rounded-lg border ${activeClass} cursor-pointer transition flex items-center justify-between group">
                <div>
                    <div class="text-xs font-semibold group-hover:text-emerald-400 flex items-center space-x-1.5">
                        <span>${cleanName}</span>
                    </div>
                    <div class="text-[10px] text-gray-400 font-mono mt-0.5">
                        ${item.total_bars.toLocaleString('en-IN')} bars (${item.interval})
                    </div>
                </div>

                <div class="flex flex-col items-end">
                    ${item.is_fresh 
                        ? `<span class="w-2 h-2 rounded-full bg-accentGreen inline-block" title="Data Fresh"></span>` 
                        : `<span class="w-2 h-2 rounded-full bg-yellow-500 inline-block" title="Pending Sync"></span>`}
                </div>
            </div>
        `;
    });

    container.innerHTML = html;
}

function selectWatchlistStock(symbol) {
    currentSymbol = symbol;
    updateChartHeaderTitle(symbol);
    
    // Update top dropdown if matching
    const select = document.getElementById("symbol-select");
    if (select) {
        for (let i = 0; i < select.options.length; i++) {
            if (select.options[i].value === symbol) {
                select.selectedIndex = i;
                break;
            }
        }
    }

    chartMarkers = [];
    loadHistoricalCandles();
    logMessage(`[Watchlist] Selected ${symbol} -> Loading local candles & live socket feed.`);

    // Re-render sidebar active state
    fetchDataSummary();
}

function updateChartHeaderTitle(symbol) {
    const titleElem = document.getElementById("current-chart-title");
    if (titleElem) {
        const clean = symbol.replace("NSE:", "").replace("-EQ", "");
        titleElem.innerText = `NSE: ${clean}`;
    }
}

function renderDataSummaryTable(details) {
    const tbody = document.getElementById("data-summary-body");
    if (!Array.isArray(details) || details.length === 0) {
        tbody.innerHTML = `<tr><td colspan="5" class="py-6 text-center text-gray-500 font-sans">No stock data loaded yet</td></tr>`;
        return;
    }

    let html = "";
    details.forEach(item => {
        const statusBadge = item.is_fresh 
            ? `<span class="text-accentGreen font-bold text-[10px] bg-emerald-950 px-2 py-0.5 rounded border border-emerald-800">FRESH</span>`
            : `<span class="text-yellow-400 font-bold text-[10px] bg-yellow-950 px-2 py-0.5 rounded border border-yellow-800">PENDING SYNC</span>`;

        html += `
            <tr class="hover:bg-cardBorder/30">
                <td class="py-2 px-3 font-semibold text-white">${item.symbol}</td>
                <td class="py-2 px-3 text-gray-300">${item.interval}</td>
                <td class="py-2 px-3 font-mono">${item.total_bars.toLocaleString('en-IN')}</td>
                <td class="py-2 px-3 text-gray-300 font-mono text-[11px]">${item.newest_time}</td>
                <td class="py-2 px-3 text-right">${statusBadge}</td>
            </tr>
        `;
    });
    tbody.innerHTML = html;
}

function saveDataSettings() {
    const csv = document.getElementById("data-stock-csv").value.trim();
    const interval = document.getElementById("data-interval").value;

    if (!csv) {
        alert("Please enter at least one stock symbol (e.g. ITC, RELIANCE, SBIN)");
        return;
    }

    fetch("/api/datamanager/settings", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ stock_list_csv: csv, interval: interval })
    })
    .then(res => res.json())
    .then(data => {
        logMessage(`[Data Manager] ${data.message}`);
        fetchDataSummary();
    });
}

function triggerDataSync() {
    fetch("/api/datamanager/sync", { method: "POST" })
        .then(res => res.json())
        .then(data => {
            logMessage(`[Data Manager] ${data.message}`);
            fetchDataSummary();
        });
}

function triggerDataRedownload() {
    if (!confirm("Are you sure you want to re-download the full 90 days of historical data for all stocks?")) return;

    fetch("/api/datamanager/redownload", { method: "POST" })
        .then(res => res.json())
        .then(data => {
            logMessage(`[Data Manager] ${data.message}`);
            fetchDataSummary();
        });
}

function triggerDataClear() {
    if (!confirm("Are you sure you want to DELETE all stored historical candle data?")) return;

    fetch("/api/datamanager/clear", { method: "POST" })
        .then(res => res.json())
        .then(data => {
            logMessage(`[Data Manager] ${data.message}`);
            fetchDataSummary();
        });
}

// Indicator Calculation Helpers
function calculateEMA(bars, period) {
    if (!bars || bars.length < period) return [];
    const results = [];
    const multiplier = 2 / (period + 1);

    let sum = 0;
    for (let i = 0; i < period; i++) {
        sum += bars[i].close;
    }
    let ema = sum / period;
    results.push({ time: bars[period - 1].timestamp, value: ema });

    for (let i = period; i < bars.length; i++) {
        ema = (bars[i].close - ema) * multiplier + ema;
        results.push({ time: bars[i].timestamp, value: ema });
    }
    return results;
}

function calculateVWAP(bars) {
    if (!bars || bars.length === 0) return [];
    const results = [];
    let cumTPV = 0;
    let cumVol = 0;

    for (let i = 0; i < bars.length; i++) {
        const tp = (bars[i].high + bars[i].low + bars[i].close) / 3;
        const vol = bars[i].volume || 1;
        cumTPV += tp * vol;
        cumVol += vol;
        results.push({ time: bars[i].timestamp, value: cumTPV / cumVol });
    }
    return results;
}
