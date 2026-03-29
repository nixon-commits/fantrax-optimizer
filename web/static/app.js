// Rosterbot Web GUI

(function() {
    'use strict';

    const datePicker = document.getElementById('date-picker');
    const projPicker = document.getElementById('proj-picker');
    const refreshBtn = document.getElementById('refresh-btn');
    const loadingEl = document.getElementById('loading');
    const warningsEl = document.getElementById('warnings');

    let blendChart = null;
    let cachedData = {};
    let sortState = {};

    // Default date to today.
    datePicker.value = new Date().toISOString().slice(0, 10);

    // --- Helpers ---
    function activeTab() {
        const active = document.querySelector('.tab.active');
        return active ? active.dataset.tab : 'projections';
    }

    function selectedProj() { return projPicker.value; }
    function selectedDate() { return datePicker.value; }

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

    // Build query string with date and projections params.
    function apiParams() {
        return 'date=' + selectedDate() + '&projections=' + selectedProj();
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

    projPicker.addEventListener('change', () => {
        // Keep compare cache (it fetches all systems), clear per-system caches.
        Object.keys(cachedData).forEach(k => {
            if (!k.startsWith('compare-')) delete cachedData[k];
        });
        loadTab(activeTab());
    });

    function loadTab(tab) {
        switch(tab) {
            case 'projections': loadProjections(); break;
            case 'compare': loadCompare(); break;
            case 'blend-curves': loadBlendCurves(); break;
            case 'lineup-diff': loadLineupDiff(); break;
        }
    }

    // --- Projections tab ---
    async function loadProjections() {
        const cacheKey = 'proj-' + selectedProj() + '-' + selectedDate();
        if (cachedData[cacheKey]) {
            renderProjections(cachedData[cacheKey]);
            return;
        }

        showLoading();
        try {
            const data = await fetchAPI('/api/projections?' + apiParams());
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

        const sysEl = document.getElementById('projection-system');
        if (data.projectionSystem) {
            sysEl.textContent = 'Projections: ' + data.projectionSystem;
            sysEl.classList.remove('hidden');
        } else {
            sysEl.classList.add('hidden');
        }

        renderHitterTable(data.hitters || [], data.projectionSystem);
        renderPitcherTable(data.pitchers || [], data.projectionSystem);
    }

    function renderHitterTable(hitters, projSystem) {
        const table = document.getElementById('hitter-table');
        const projHeader = table.querySelector('th[data-sort="steamer"]');
        if (projHeader && projSystem) projHeader.textContent = projSystem;

        const tbody = table.querySelector('tbody');
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

    function renderPitcherTable(pitchers, projSystem) {
        const table = document.getElementById('pitcher-table');
        const projHeader = table.querySelector('th[data-sort="steamer"]');
        if (projHeader && projSystem) projHeader.textContent = projSystem;

        const tbody = table.querySelector('tbody');
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

    // --- Compare tab ---
    async function loadCompare() {
        const date = selectedDate();
        const cacheKey = 'compare-' + date;
        if (cachedData[cacheKey]) {
            renderCompare(cachedData[cacheKey]);
            return;
        }

        showLoading();
        try {
            const data = await fetchAPI('/api/compare?date=' + date);
            cachedData[cacheKey] = data;
            renderCompare(data);
        } catch(e) {
            hideLoading();
            showWarnings(['Failed to load comparison: ' + e.message]);
        }
    }

    function renderCompare(data) {
        hideLoading();

        const systems = (data.systems || []).filter(s => !s.error);
        if (systems.length === 0) {
            showWarnings(['No projection systems returned data.']);
            return;
        }
        showWarnings(null);

        renderCompareHitters(systems);
        renderComparePitchers(systems);
    }

    function renderCompareHitters(systems) {
        const thead = document.getElementById('compare-hitter-head');
        const tbody = document.querySelector('#compare-hitter-table tbody');

        // Build header: Player | Team | [system: Proj  Blended  Final] ...
        let headerHtml = '<th data-sort="name">Player</th><th data-sort="team">Team</th>';
        for (const sys of systems) {
            headerHtml += '<th class="compare-sys-header num" colspan="3">' + escapeHtml(sys.projectionSystem) + '</th>';
        }
        headerHtml += '<th class="num" data-sort="spread">Spread</th>';
        thead.innerHTML = headerHtml;

        // Build a player-keyed map: name → { system → hitter }
        const playerMap = {};
        const playerOrder = [];
        for (const sys of systems) {
            for (const h of (sys.hitters || [])) {
                if (!playerMap[h.name]) {
                    playerMap[h.name] = { team: h.team };
                    playerOrder.push(h.name);
                }
                playerMap[h.name][sys.projectionSystem] = h;
            }
        }

        tbody.innerHTML = '';
        for (const name of playerOrder) {
            const info = playerMap[name];
            const tr = document.createElement('tr');

            // Collect finalPts across systems for highlighting.
            const finals = systems.map(sys => {
                const h = info[sys.projectionSystem];
                return h ? h.finalPts : null;
            }).filter(v => v !== null);

            const maxFinal = Math.max(...finals);
            const minFinal = Math.min(...finals);
            const spread = finals.length > 1 ? maxFinal - minFinal : 0;

            let cells = '<td>' + escapeHtml(name) + '</td>' +
                        '<td>' + escapeHtml(info.team) + '</td>';

            for (const sys of systems) {
                const h = info[sys.projectionSystem];
                if (!h) {
                    cells += '<td class="num">-</td><td class="num">-</td><td class="num">-</td>';
                    continue;
                }
                const projCls = (finals.length > 1 && h.steamerPts === Math.max(...systems.map(s => (info[s.projectionSystem] || {}).steamerPts || 0))) ? ' compare-best' : '';
                const blendCls = (finals.length > 1 && h.blendedPts === Math.max(...systems.map(s => (info[s.projectionSystem] || {}).blendedPts || 0))) ? ' compare-best' : '';
                const finalBest = h.finalPts === maxFinal && finals.length > 1;
                const finalWorst = h.finalPts === minFinal && finals.length > 1 && spread > 0.1;
                const finalCls = finalBest ? ' compare-best' : (finalWorst ? ' compare-worst' : '');

                cells += '<td class="num' + projCls + '">' + fmt(h.steamerPts) + '</td>';
                cells += '<td class="num' + blendCls + '">' + fmt(h.blendedPts) + '</td>';
                cells += '<td class="num' + finalCls + '"><strong>' + fmt(h.finalPts) + '</strong></td>';
            }

            const spreadCls = spread > 1.0 ? ' compare-worst' : (spread > 0.5 ? ' style="color: var(--yellow)"' : '');
            if (spreadCls.startsWith(' style')) {
                cells += '<td class="num"' + spreadCls + '>' + fmt(spread) + '</td>';
            } else {
                cells += '<td class="num' + spreadCls + '">' + fmt(spread) + '</td>';
            }

            tr.innerHTML = cells;
            tbody.appendChild(tr);
        }

        setupSorting('compare-hitter-table', []);
    }

    function renderComparePitchers(systems) {
        const thead = document.getElementById('compare-pitcher-head');
        const tbody = document.querySelector('#compare-pitcher-table tbody');

        let headerHtml = '<th data-sort="name">Player</th><th data-sort="team">Team</th><th>Role</th>';
        for (const sys of systems) {
            headerHtml += '<th class="compare-sys-header num" colspan="2">' + escapeHtml(sys.projectionSystem) + '</th>';
        }
        headerHtml += '<th class="num" data-sort="spread">Spread</th>';
        thead.innerHTML = headerHtml;

        const playerMap = {};
        const playerOrder = [];
        for (const sys of systems) {
            for (const p of (sys.pitchers || [])) {
                if (!playerMap[p.name]) {
                    playerMap[p.name] = { team: p.team, isSP: p.isSP };
                    playerOrder.push(p.name);
                }
                playerMap[p.name][sys.projectionSystem] = p;
            }
        }

        tbody.innerHTML = '';
        for (const name of playerOrder) {
            const info = playerMap[name];
            const tr = document.createElement('tr');

            const exps = systems.map(sys => {
                const p = info[sys.projectionSystem];
                return p ? p.expectedPts : null;
            }).filter(v => v !== null);

            const maxExp = Math.max(...exps);
            const minExp = Math.min(...exps);
            const spread = exps.length > 1 ? maxExp - minExp : 0;

            let cells = '<td>' + escapeHtml(name) + '</td>' +
                        '<td>' + escapeHtml(info.team) + '</td>' +
                        '<td>' + (info.isSP ? 'SP' : 'RP') + '</td>';

            for (const sys of systems) {
                const p = info[sys.projectionSystem];
                if (!p) {
                    cells += '<td class="num">-</td><td class="num">-</td>';
                    continue;
                }
                const bestCls = p.expectedPts === maxExp && exps.length > 1 ? ' compare-best' : '';
                const worstCls = p.expectedPts === minExp && exps.length > 1 && spread > 0.1 ? ' compare-worst' : '';
                const cls = bestCls || worstCls;

                cells += '<td class="num">' + fmt(p.steamerPts) + '</td>';
                cells += '<td class="num' + cls + '"><strong>' + fmt(p.expectedPts) + '</strong></td>';
            }

            const spreadCls = spread > 2.0 ? ' compare-worst' : (spread > 1.0 ? ' style="color: var(--yellow)"' : '');
            if (spreadCls.startsWith(' style')) {
                cells += '<td class="num"' + spreadCls + '>' + fmt(spread) + '</td>';
            } else {
                cells += '<td class="num' + spreadCls + '">' + fmt(spread) + '</td>';
            }

            tr.innerHTML = cells;
            tbody.appendChild(tr);
        }

        setupSorting('compare-pitcher-table', []);
    }

    // --- Sorting ---
    function setupSorting(tableId, data) {
        const table = document.getElementById(tableId);
        table.querySelectorAll('thead th[data-sort]').forEach(th => {
            // Remove old listeners by cloning.
            const clone = th.cloneNode(true);
            th.parentNode.replaceChild(clone, th);
            clone.addEventListener('click', () => {
                const key = clone.dataset.sort;
                const state = sortState[tableId] || {};
                const dir = (state.key === key && state.dir === 'desc') ? 'asc' : 'desc';
                sortState[tableId] = { key, dir };

                table.querySelectorAll('thead th').forEach(h => {
                    h.classList.remove('sorted-asc', 'sorted-desc');
                });
                clone.classList.add(dir === 'asc' ? 'sorted-asc' : 'sorted-desc');

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
            let va = a.cells[colIndex] ? a.cells[colIndex].textContent.trim() : '';
            let vb = b.cells[colIndex] ? b.cells[colIndex].textContent.trim() : '';

            const na = parseFloat(va.replace('%', ''));
            const nb = parseFloat(vb.replace('%', ''));
            if (!isNaN(na) && !isNaN(nb)) {
                return dir === 'asc' ? na - nb : nb - na;
            }

            return dir === 'asc' ? va.localeCompare(vb) : vb.localeCompare(va);
        });

        rows.forEach(r => tbody.appendChild(r));
    }

    // --- Blend Curves tab ---
    async function loadBlendCurves() {
        const cacheKey = 'blend-' + selectedProj() + '-' + selectedDate();
        if (cachedData[cacheKey]) {
            renderBlendCurves(cachedData[cacheKey]);
            return;
        }

        showLoading();
        try {
            const data = await fetchAPI('/api/blend-curve?' + apiParams());
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

        const legendEl = document.getElementById('blend-legend');
        legendEl.innerHTML = players.map(p => {
            const color = colorMap[p.type] || '#fff';
            return '<span class="player-dot"><span class="dot" style="background:' + color + '"></span>' +
                   escapeHtml(p.name) + ' (' + p.gp + ' GP)</span>';
        }).join('');
    }

    // --- Lineup Diff tab ---
    async function loadLineupDiff() {
        const cacheKey = 'diff-' + selectedProj() + '-' + selectedDate();
        if (cachedData[cacheKey]) {
            renderLineupDiff(cachedData[cacheKey]);
            return;
        }

        showLoading();
        try {
            const data = await fetchAPI('/api/lineup-diff?' + apiParams());
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

        renderDiffColumn('current-lineup', data.current, data.changedPlayerNames || []);
        renderDiffColumn('optimized-lineup', data.optimized, data.changedPlayerNames || []);

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
