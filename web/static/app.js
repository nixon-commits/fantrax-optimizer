// Rosterbot Web GUI

(function() {
    'use strict';

    const datePicker = document.getElementById('date-picker');
    const refreshBtn = document.getElementById('refresh-btn');
    const loadingEl = document.getElementById('loading');
    const warningsEl = document.getElementById('warnings');

    let blendChart = null;
    let cachedData = {};
    let sortState = {};

    // Default date to today.
    datePicker.value = new Date().toISOString().slice(0, 10);

    // --- Tab switching ---
    document.querySelectorAll('.tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
            tab.classList.add('active');
            const target = document.getElementById(tab.dataset.tab);
            if (target) target.classList.add('active');
            loadTab(tab.dataset.tab);
        });
    });

    refreshBtn.addEventListener('click', () => {
        cachedData = {};
        loadTab(activeTab());
    });

    datePicker.addEventListener('change', () => {
        cachedData = {};
        loadTab(activeTab());
    });

    function activeTab() {
        const active = document.querySelector('.tab.active');
        return active ? active.dataset.tab : 'projections';
    }

    function showLoading() { loadingEl.classList.remove('hidden'); }
    function hideLoading() { loadingEl.classList.add('hidden'); }

    function showWarnings(warnings) {
        if (!warnings || warnings.length === 0) {
            warningsEl.classList.add('hidden');
            return;
        }
        warningsEl.innerHTML = warnings.map(w => '<div>' + escapeHtml(w) + '</div>').join('');
        warningsEl.classList.remove('hidden');
    }

    function escapeHtml(s) {
        const div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    function fmt(n, decimals) {
        if (n === undefined || n === null) return '-';
        return n.toFixed(decimals !== undefined ? decimals : 2);
    }

    function pctFmt(n) {
        if (n === undefined || n === null) return '-';
        return Math.round(n * 100) + '%';
    }

    // --- API calls ---
    async function fetchAPI(path) {
        const resp = await fetch(path);
        if (!resp.ok) {
            const body = await resp.text();
            throw new Error(body || resp.statusText);
        }
        return resp.json();
    }

    // --- Load tab data ---
    function loadTab(tab) {
        switch(tab) {
            case 'projections': loadProjections(); break;
            case 'blend-curves': loadBlendCurves(); break;
            case 'lineup-diff': loadLineupDiff(); break;
        }
    }

    // --- Projections tab ---
    async function loadProjections() {
        const date = datePicker.value;
        const cacheKey = 'proj-' + date;
        if (cachedData[cacheKey]) {
            renderProjections(cachedData[cacheKey]);
            return;
        }

        showLoading();
        try {
            const data = await fetchAPI('/api/projections?date=' + date);
            cachedData[cacheKey] = data;
            renderProjections(data);
        } catch(e) {
            hideLoading();
            showWarnings(['Failed to load projections: ' + e.message]);
        }
    }

    function renderProjections(data) {
        hideLoading();
        showWarnings(data.warnings);

        renderHitterTable(data.hitters || []);
        renderPitcherTable(data.pitchers || []);
    }

    function renderHitterTable(hitters) {
        const tbody = document.querySelector('#hitter-table tbody');
        tbody.innerHTML = '';

        for (const h of hitters) {
            const tr = document.createElement('tr');
            if (h.status !== 'Active') tr.classList.add('bench');

            const steamer = h.steamerPts;
            const recent = h.hasRecent ? fmt(h.recentFPG) : '-';
            const sw = h.hasRecent ? pctFmt(h.steamerWt) : '100%';
            const rw = h.hasRecent ? pctFmt(h.recentWt) : '0%';
            const blended = fmt(h.blendedPts);
            const park = h.parkFactor !== 1.0 ? fmt(h.parkFactor, 3) : '-';
            const matchup = h.matchupMult !== 1.0 ? fmt(h.matchupMult, 3) : '-';
            const game = h.hasGame ? (h.locked ? 'Locked' : 'Yes') : 'No';

            tr.innerHTML =
                '<td>' + escapeHtml(h.name) + '</td>' +
                '<td>' + escapeHtml(h.team) + '</td>' +
                '<td>' + escapeHtml(h.slot || '') + '</td>' +
                '<td class="num">' + fmt(steamer) + '</td>' +
                '<td class="num">' + recent + '</td>' +
                '<td class="num">' + sw + '</td>' +
                '<td class="num">' + rw + '</td>' +
                '<td class="num">' + blended + '</td>' +
                '<td class="num ' + parkClass(h.parkFactor) + '">' + park + '</td>' +
                '<td class="num ' + matchupClass(h.matchupMult) + '">' + matchup + '</td>' +
                '<td class="num"><strong>' + fmt(h.finalPts) + '</strong></td>' +
                '<td>' + game + '</td>';
            tbody.appendChild(tr);
        }

        setupSorting('hitter-table', hitters);
    }

    function renderPitcherTable(pitchers) {
        const tbody = document.querySelector('#pitcher-table tbody');
        tbody.innerHTML = '';

        for (const p of pitchers) {
            const tr = document.createElement('tr');
            if (p.status !== 'Active') tr.classList.add('bench');

            const recent = p.gamesPlayed > 0 ? fmt(p.recentFPG) : '-';
            const sw = p.gamesPlayed > 0 ? pctFmt(p.steamerWt) : '100%';
            const rw = p.gamesPlayed > 0 ? pctFmt(p.recentWt) : '0%';
            const blended = fmt(p.expectedPts);
            const role = p.positions + (p.isStarter ? ' (starting)' : '');
            const game = p.hasGame ? 'Yes' : 'No';

            tr.innerHTML =
                '<td>' + escapeHtml(p.name) + '</td>' +
                '<td>' + escapeHtml(p.team) + '</td>' +
                '<td>' + escapeHtml(p.slot || '') + '</td>' +
                '<td class="num">' + fmt(p.steamerPts) + '</td>' +
                '<td class="num">' + recent + '</td>' +
                '<td class="num">' + sw + '</td>' +
                '<td class="num">' + rw + '</td>' +
                '<td class="num"><strong>' + blended + '</strong></td>' +
                '<td>' + escapeHtml(role) + '</td>' +
                '<td>' + game + '</td>';
            tbody.appendChild(tr);
        }
    }

    function parkClass(v) {
        if (v > 1.01) return 'positive';
        if (v < 0.99) return 'negative';
        return 'neutral';
    }

    function matchupClass(v) {
        if (v > 1.01) return 'positive';
        if (v < 0.99) return 'negative';
        return 'neutral';
    }

    // --- Sorting ---
    function setupSorting(tableId, data) {
        const table = document.getElementById(tableId);
        table.querySelectorAll('thead th[data-sort]').forEach(th => {
            th.addEventListener('click', () => {
                const key = th.dataset.sort;
                const state = sortState[tableId] || {};
                const dir = (state.key === key && state.dir === 'desc') ? 'asc' : 'desc';
                sortState[tableId] = { key, dir };

                // Update header classes
                table.querySelectorAll('thead th').forEach(h => {
                    h.classList.remove('sorted-asc', 'sorted-desc');
                });
                th.classList.add(dir === 'asc' ? 'sorted-asc' : 'sorted-desc');

                sortTableRows(table, key, dir);
            });
        });
    }

    function sortTableRows(table, key, dir) {
        const tbody = table.querySelector('tbody');
        const rows = Array.from(tbody.rows);

        const colIndex = Array.from(table.querySelectorAll('thead th')).findIndex(th => th.dataset.sort === key);
        if (colIndex < 0) return;

        rows.sort((a, b) => {
            let va = a.cells[colIndex].textContent.trim();
            let vb = b.cells[colIndex].textContent.trim();

            // Try numeric
            const na = parseFloat(va.replace('%', ''));
            const nb = parseFloat(vb.replace('%', ''));
            if (!isNaN(na) && !isNaN(nb)) {
                return dir === 'asc' ? na - nb : nb - na;
            }

            // String sort
            return dir === 'asc' ? va.localeCompare(vb) : vb.localeCompare(va);
        });

        rows.forEach(r => tbody.appendChild(r));
    }

    // --- Blend Curves tab ---
    async function loadBlendCurves() {
        const date = datePicker.value;
        const cacheKey = 'blend-' + date;
        if (cachedData[cacheKey]) {
            renderBlendCurves(cachedData[cacheKey]);
            return;
        }

        showLoading();
        try {
            const data = await fetchAPI('/api/blend-curve?date=' + date);
            cachedData[cacheKey] = data;
            renderBlendCurves(data);
        } catch(e) {
            hideLoading();
            showWarnings(['Failed to load blend curves: ' + e.message]);
        }
    }

    function renderBlendCurves(data) {
        hideLoading();

        const ctx = document.getElementById('blend-chart').getContext('2d');

        if (blendChart) blendChart.destroy();

        const hitterCurve = data.hitterCurve || [];
        const spCurve = data.pitcherSPCurve || [];
        const rpCurve = data.pitcherRPCurve || [];
        const players = data.rosterPlayers || [];

        const datasets = [
            {
                label: 'Hitter Steamer Wt',
                data: hitterCurve.map(p => ({ x: p.gp, y: p.steamerWt * 100 })),
                borderColor: '#4fc3f7',
                backgroundColor: 'rgba(79, 195, 247, 0.1)',
                fill: true,
                tension: 0.3,
                pointRadius: 0,
                borderWidth: 2,
            },
            {
                label: 'SP Steamer Wt',
                data: spCurve.map(p => ({ x: p.gp, y: p.steamerWt * 100 })),
                borderColor: '#66bb6a',
                backgroundColor: 'rgba(102, 187, 106, 0.1)',
                fill: true,
                tension: 0.3,
                pointRadius: 0,
                borderWidth: 2,
            },
            {
                label: 'RP Steamer Wt',
                data: rpCurve.map(p => ({ x: p.gp, y: p.steamerWt * 100 })),
                borderColor: '#ffa726',
                backgroundColor: 'rgba(255, 167, 38, 0.1)',
                fill: true,
                tension: 0.3,
                pointRadius: 0,
                borderWidth: 2,
            },
        ];

        // Scatter overlay for roster players
        const colorMap = { hitter: '#4fc3f7', pitcherSP: '#66bb6a', pitcherRP: '#ffa726' };
        if (players.length > 0) {
            datasets.push({
                label: 'Roster Players',
                data: players.map(p => ({ x: p.gp, y: p.steamerWt * 100 })),
                type: 'scatter',
                backgroundColor: players.map(p => colorMap[p.type] || '#fff'),
                borderColor: '#fff',
                borderWidth: 1,
                pointRadius: 6,
                pointHoverRadius: 8,
            });
        }

        blendChart = new Chart(ctx, {
            type: 'line',
            data: { datasets },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: { mode: 'nearest', intersect: false },
                scales: {
                    x: {
                        type: 'linear',
                        title: { display: true, text: 'Games Played', color: '#8899aa' },
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#8899aa' },
                    },
                    y: {
                        title: { display: true, text: 'Steamer Weight %', color: '#8899aa' },
                        min: 20,
                        max: 105,
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#8899aa', callback: v => v + '%' },
                    },
                },
                plugins: {
                    legend: { labels: { color: '#e0e0e0' } },
                    tooltip: {
                        callbacks: {
                            label: function(ctx) {
                                if (ctx.dataset.label === 'Roster Players') {
                                    const p = players[ctx.dataIndex];
                                    return p.name + ' (' + p.gp + ' GP, ' + Math.round(p.steamerWt * 100) + '% Steamer)';
                                }
                                return ctx.dataset.label + ': ' + Math.round(ctx.parsed.y) + '%';
                            }
                        }
                    }
                }
            }
        });

        // Legend with player dots
        const legendEl = document.getElementById('blend-legend');
        legendEl.innerHTML = players.map(p => {
            const color = colorMap[p.type] || '#fff';
            return '<span class="player-dot"><span class="dot" style="background:' + color + '"></span>' +
                   escapeHtml(p.name) + ' (' + p.gp + ' GP)</span>';
        }).join('');
    }

    // --- Lineup Diff tab ---
    async function loadLineupDiff() {
        const date = datePicker.value;
        const cacheKey = 'diff-' + date;
        if (cachedData[cacheKey]) {
            renderLineupDiff(cachedData[cacheKey]);
            return;
        }

        showLoading();
        try {
            const data = await fetchAPI('/api/lineup-diff?date=' + date);
            cachedData[cacheKey] = data;
            renderLineupDiff(data);
        } catch(e) {
            hideLoading();
            showWarnings(['Failed to load lineup diff: ' + e.message]);
        }
    }

    function renderLineupDiff(data) {
        hideLoading();
        showWarnings(data.warnings);

        const summary = document.getElementById('diff-summary');
        const changes = data.changes || [];

        if (changes.length === 0) {
            summary.innerHTML = '<span style="color: var(--green)">No changes needed. Lineup is optimal.</span>';
        } else {
            const delta = data.totalDelta || 0;
            const cls = delta >= 0 ? 'positive' : 'negative';
            summary.innerHTML = changes.length + ' change(s) &mdash; <span class="' + cls + '">' + (delta >= 0 ? '+' : '') + fmt(delta) + ' pts</span>';
        }

        // Current lineup
        renderDiffColumn('current-lineup', data.current, data.changedPlayerNames || []);
        renderDiffColumn('optimized-lineup', data.optimized, data.changedPlayerNames || []);

        // Changes list
        const listEl = document.getElementById('changes-list');
        if (changes.length === 0) {
            listEl.innerHTML = '';
            return;
        }

        listEl.innerHTML = '<h3 style="margin-bottom: 0.5rem; color: var(--text-dim); font-size: 0.9rem;">Changes</h3>' +
            changes.map(c => {
                const isUp = c.direction === 'activate';
                const arrow = isUp ? '<span class="change-arrow up">&#9650;</span>' : '<span class="change-arrow down">&#9660;</span>';
                const delta = c.ptsDelta;
                const cls = delta >= 0 ? 'positive' : 'negative';
                return '<div class="change-item">' +
                    arrow +
                    '<span>' + escapeHtml(c.playerName) + ' &rarr; ' + escapeHtml(c.toSlot) + '</span>' +
                    '<span class="change-delta ' + cls + '">' + (delta >= 0 ? '+' : '') + fmt(delta) + '</span>' +
                    '</div>';
            }).join('');
    }

    function renderDiffColumn(elId, lineup, changedNames) {
        const el = document.getElementById(elId);
        if (!lineup) { el.innerHTML = ''; return; }

        const all = (lineup.hitters || []).concat(lineup.pitchers || []);
        el.innerHTML = all.map(p => {
            const changed = changedNames.indexOf(p.name) >= 0;
            return '<div class="diff-player' + (changed ? ' changed' : '') + '">' +
                '<span>' + escapeHtml(p.name) + ' <small style="color: var(--text-dim)">' + escapeHtml(p.slot || 'BN') + '</small></span>' +
                '<span>' + fmt(p.pts) + '</span>' +
                '</div>';
        }).join('');
    }

    // --- Initial load ---
    loadTab('projections');
})();
