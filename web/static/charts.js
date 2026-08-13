(function (global) {
    const COLORS = {
        accent: '#21D198',
        accentMuted: 'rgba(33, 209, 152, 0.22)',
        warning: '#f59e0b',
        error: '#ef4444',
        blue: '#60a5fa',
        purple: '#a78bfa',
        text: '#f0f0f0',
        muted: '#888888',
        grid: '#2a2a2a',
        surface: '#141414',
    };

    const PALETTE = [COLORS.accent, COLORS.blue, COLORS.warning, COLORS.purple, COLORS.error, '#2dd4bf', '#fb7185'];

    function clear(el) {
        el.innerHTML = '';
    }

    function emptyState(el, message) {
        clear(el);
        const div = document.createElement('div');
        div.className = 'chart-empty';
        div.textContent = message || 'No data in this range';
        el.appendChild(div);
    }

    function svgEl(name, attrs) {
        const el = document.createElementNS('http://www.w3.org/2000/svg', name);
        Object.entries(attrs || {}).forEach(([k, v]) => el.setAttribute(k, String(v)));
        return el;
    }

    function tooltip() {
        let node = document.getElementById('chart-tooltip');
        if (!node) {
            node = document.createElement('div');
            node.id = 'chart-tooltip';
            node.className = 'chart-tooltip';
            document.body.appendChild(node);
        }
        return node;
    }

    function showTip(ev, html) {
        const node = tooltip();
        node.innerHTML = html;
        node.style.opacity = '1';
        const x = ev.clientX + 12;
        const y = ev.clientY + 12;
        node.style.left = x + 'px';
        node.style.top = y + 'px';
    }

    function hideTip() {
        const node = document.getElementById('chart-tooltip');
        if (node) node.style.opacity = '0';
    }

    function niceMax(n) {
        if (n <= 0) return 1;
        const exp = Math.pow(10, Math.floor(Math.log10(n)));
        const f = n / exp;
        const nice = f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10;
        return nice * exp;
    }

    function donut(container, items, opts) {
        opts = opts || {};
        const filtered = (items || []).filter(i => i.value > 0);
        if (!filtered.length) {
            emptyState(container, opts.empty);
            return;
        }
        clear(container);
        const wrap = document.createElement('div');
        wrap.className = 'donut-wrap';

        const size = 196;
        const cx = size / 2;
        const cy = size / 2;
        const r = 64;
        const stroke = 22;
        const C = 2 * Math.PI * r;
        const total = filtered.reduce((s, i) => s + i.value, 0);

        const svg = svgEl('svg', { viewBox: `0 0 ${size} ${size}`, class: 'donut-svg' });
        svg.appendChild(svgEl('circle', {
            cx, cy, r,
            fill: 'none',
            stroke: COLORS.grid,
            'stroke-width': stroke,
        }));

        let offset = 0;
        filtered.forEach((item, idx) => {
            const frac = item.value / total;
            const len = frac * C;
            const circle = svgEl('circle', {
                cx, cy, r,
                fill: 'none',
                stroke: item.color || PALETTE[idx % PALETTE.length],
                'stroke-width': stroke,
                'stroke-dasharray': `${len} ${C - len}`,
                'stroke-dashoffset': String(C * 0.25 - offset),
                'stroke-linecap': frac === 1 ? 'butt' : 'butt',
                class: 'donut-seg',
            });
            circle.addEventListener('mousemove', (ev) => {
                showTip(ev, `<strong>${item.label}</strong><br>${item.value.toLocaleString()} (${Math.round(frac * 100)}%)`);
            });
            circle.addEventListener('mouseleave', hideTip);
            svg.appendChild(circle);
            offset += len;
        });

        const label = opts.center || String(total);
        const sub = opts.centerSub || 'jobs';
        const center = svgEl('text', { x: cx, y: cy - 4, 'text-anchor': 'middle', class: 'donut-center' });
        center.textContent = label;
        const centerSub = svgEl('text', { x: cx, y: cy + 16, 'text-anchor': 'middle', class: 'donut-sub' });
        centerSub.textContent = sub;
        svg.appendChild(center);
        svg.appendChild(centerSub);

        const legend = document.createElement('div');
        legend.className = 'chart-legend';
        filtered.forEach((item, idx) => {
            const row = document.createElement('div');
            row.className = 'chart-legend-row';
            row.innerHTML = `<span class="chart-swatch" style="background:${item.color || PALETTE[idx % PALETTE.length]}"></span><span>${item.label}</span><span class="chart-legend-val">${item.value.toLocaleString()}</span>`;
            legend.appendChild(row);
        });

        wrap.appendChild(svg);
        wrap.appendChild(legend);
        container.appendChild(wrap);
    }

    function bars(container, items, opts) {
        opts = opts || {};
        const data = items || [];
        if (!data.length || data.every(i => !i.value)) {
            emptyState(container, opts.empty);
            return;
        }
        clear(container);

        const width = 640;
        const height = 240;
        const pad = { l: 52, r: 12, t: 16, b: 36 };
        const innerW = width - pad.l - pad.r;
        const innerH = height - pad.t - pad.b;
        const max = niceMax(Math.max(...data.map(i => i.value)));
        const gap = data.length > 40 ? 1 : data.length > 20 ? 2 : 4;
        const barW = Math.max(2, (innerW - gap * (data.length - 1)) / data.length);
        const formatY = opts.formatY || (v => String(v));
        const formatTip = opts.formatTip || formatY;

        const svg = svgEl('svg', { viewBox: `0 0 ${width} ${height}`, class: 'bar-svg' });

        for (let i = 0; i <= 4; i++) {
            const y = pad.t + innerH - (innerH * i) / 4;
            const val = (max * i) / 4;
            svg.appendChild(svgEl('line', {
                x1: pad.l, x2: width - pad.r, y1: y, y2: y,
                stroke: COLORS.grid, 'stroke-width': 1,
            }));
            const lab = svgEl('text', { x: pad.l - 8, y: y + 4, 'text-anchor': 'end', class: 'chart-axis' });
            lab.textContent = formatY(val);
            svg.appendChild(lab);
        }

        data.forEach((item, idx) => {
            const h = max ? (item.value / max) * innerH : 0;
            const x = pad.l + idx * (barW + gap);
            const y = pad.t + innerH - h;
            const rect = svgEl('rect', {
                x, y, width: barW, height: Math.max(h, item.value > 0 ? 2 : 0),
                rx: Math.min(3, barW / 2),
                fill: opts.color || COLORS.accent,
                class: 'bar-seg',
            });
            rect.addEventListener('mousemove', (ev) => {
                showTip(ev, `<strong>${item.label}</strong><br>${formatTip(item.value)}`);
            });
            rect.addEventListener('mouseleave', hideTip);
            svg.appendChild(rect);
        });

        const labelEvery = Math.ceil(data.length / 7);
        data.forEach((item, idx) => {
            if (idx % labelEvery !== 0 && idx !== data.length - 1) return;
            const x = pad.l + idx * (barW + gap) + barW / 2;
            const lab = svgEl('text', { x, y: height - 10, 'text-anchor': 'middle', class: 'chart-axis' });
            lab.textContent = item.shortLabel || item.label;
            svg.appendChild(lab);
        });

        container.appendChild(svg);
    }

    function hbars(container, items, opts) {
        opts = opts || {};
        const data = (items || []).slice(0, 8);
        if (!data.length || data.every(i => !i.value)) {
            emptyState(container, opts.empty);
            return;
        }
        clear(container);
        const format = opts.format || (v => v.toLocaleString());
        const max = Math.max(...data.map(i => i.value)) || 1;
        const list = document.createElement('div');
        list.className = 'hbar-list';
        data.forEach((item, idx) => {
            const row = document.createElement('div');
            row.className = 'hbar-row';
            const pct = Math.max(4, (item.value / max) * 100);
            row.innerHTML = `
                <div class="hbar-label" title="${item.label}">${item.label}</div>
                <div class="hbar-track">
                    <div class="hbar-fill" style="width:${pct}%; background:${item.color || PALETTE[idx % PALETTE.length]}"></div>
                </div>
                <div class="hbar-val">${format(item.value)}</div>
            `;
            list.appendChild(row);
        });
        container.appendChild(list);
    }

    global.GoEncodeCharts = { donut, bars, hbars, COLORS, PALETTE, emptyState };
})(window);
