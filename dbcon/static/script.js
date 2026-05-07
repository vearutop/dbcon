function splitmix32(a) {
    return function () {
        a |= 0;
        a = a + 0x9e3779b9 | 0;
        let t = a ^ a >>> 16;
        t = Math.imul(t, 0x21f0aaad);
        t = t ^ t >>> 15;
        t = Math.imul(t, 0x735a2d97);
        return ((t = t ^ t >>> 15) >>> 0) / 4294967296;
    }
}

// Deterministic random number generator.
const prng = splitmix32(12345)

function getDarkColor() {
    var color = '#';
    for (var i = 0; i < 6; i++) {
        color += Math.floor(2 + prng() * 11).toString(16);
    }
    return color;
}

/**
 * @typedef Result
 * @type {Object}
 * @property {String} statement - SQL Statement.
 * @property {String} error - Error message.
 * @property {String} instance - DB instance name.
 * @property {String} elapsed - Time spent for a query.
 * @property {Array<String>} columns - List of columns.
 * @property {Array<Array<*>>} values - Data.
 */

/**
 * @param {Result} result
 * @param {Number} idx
 */
function renderResult(result, idx) {
    var desc = result.statement
    var res = ""

    // Header.
    if (desc.includes("-- #")) {
        let h = desc.match(/-- #(.+)/)
        if (h && h[1]) {
            res += '<h2>' + h[1].trim() + '</h2>'
        }
    }

    // Description.
    if (desc.includes("-- >")) {
        var b = ""
        let d = desc.matchAll(/-- >(.+)/g)
        for (var l of d) {
            if (l[1].trim()) {
                b += '<p>' + l[1].trim() + '</p>'
            }
        }

        if (b) {
            res += b
        }
    }

    var strip = desc.includes("-- strip")

    if (!strip) {
        res += '<pre>' + result.statement + '</pre>'
    }

    if (result.error) {
        res += '<p><a class="ai btn btn-info" onclick="return fixStatementAI(' + idx + ')">Fix 🤖</a> <code>' + result.error + '</code></p>';

        $('#query-results').append('<div>' + res + '</div>')

        return
    }

    var rowsCnt = 0
    if (result.values && !strip) {
        if (!isPortableReport) {
            res += '<a href="/query-db.csv?instance=' + encodeURIComponent(result.instance) + '&statement=' + encodeURIComponent(result.statement) + '" style="margin-bottom: 10px" class="btn btn-info" target="_blank">Download CSV</a> '
        }

        rowsCnt = result.values.length
    }
    if (!strip) {
        res += '<span>Rows: ' + rowsCnt + ', elapsed: ' + result.elapsed + '</span>\n'
    }

    let uplot_opts = null;
    let uplot_data = null;
    let pie_data = null;

    if (result.statement.includes("-- plot")) {
        res += '<div id="plot-' + idx + '"></div>'

        uplot_opts = uplotOpts()
        applyLegendSortOnHover(uplot_opts)

        let captionMatch = result.statement.match(/plot:caption=(.+)/)
        if (captionMatch && captionMatch[1]) {
            uplot_opts.title = captionMatch[1].trim()
        }

        let heightMatch = result.statement.match(/plot:height=(\d+)(?:px)?/)
        if (heightMatch && heightMatch[1]) {
            uplot_opts.height = parseInt(heightMatch[1], 10)
        }

        if (result.statement.includes('-- plot:time')) {
            uplot_opts.scales.x.time = true;
        }

        if (result.statement.includes('-- plot:stacked_bars')) {
            if (result.statement.includes('-- plot:rows')) {
                uplot_data = uplotBarsRowsData(result, uplot_opts)
            } else {
                uplot_data = uplotBarsColumnsData(result, uplot_opts)
            }
        } else if (result.statement.includes('-- plot:rows')) {
            uplot_data = uplotRowsData(result, uplot_opts)
        } else {
            uplot_data = uplotColumnsData(result, uplot_opts)
        }

        if (result.statement.includes('-- plot:time')) {
            let m = result.statement.match(/plot:time_axis_fmt=([^\s]+)/)
            let axisFmt = m && m[1] ? m[1] : null
            applyTimeAxis(uplot_opts, axisFmt)
        }
    }

    if (result.statement.includes("-- plot:pie") && result.values.length > 1) {
        res += '<div id="pie-' + idx + '"></div>'

        // Sorting data by first column (count) descending.
        let sortedValues = result.values.sort(function (a, b) {
            return b[0] - a[0]
        });

        pie_data = [];

        var v = 0 // 0 col for value
        var l = 1 // 1 col for label

        if (isNaN(parseFloat(sortedValues[0][v]))) {
            v = 1
            l = 0
        }

        // Transposing results from being an array of rows to array of columns.
        for (let i in sortedValues) {
            let item = sortedValues[i]

            pie_data.push(
                {label: item[l], value: parseFloat(item[v]), color: getDarkColor()}
            )
        }
    }

    if (!uplot_opts && !pie_data) {
        let transpose = result.statement.includes("-- transpose")

        if (transpose) {
            res += renderTransposedTable(result, result.statement.includes("-- transpose:skip_similar"))
        } else {
            res += renderTable(result)
        }
    }

    $('#query-results').append('<div>' + res + '</div>')

    if (uplot_opts && uplot_data) {
        // console.log("Plotting to", document.getElementById("plot-" + idx))
        // console.log(uplot_opts)
        // console.log(uplot_data)
        new uPlot(uplot_opts, uplot_data, document.getElementById("plot-" + idx));
    }

    if (pie_data) {
        var m = result.statement.match(/plot:pie:total=(\d+)/)
        var total = 0
        if (m && m[1]) {
            total = parseFloat(m[1])
        }
        // console.log("Plotting pie", JSON.stringify(pie_data), total, document.getElementById("pie-" + idx))
        drawPieChart(pie_data, total, document.getElementById("pie-" + idx))
    }
}

/**
 * @param {Result} result
 * @returns {string}
 */
function renderTable(result) {
    var res = ''

    res += '<table class="pure-table result"><thead><tr>';
    for (k in result.columns) {
        res += '<th>' + result.columns[k] + '</th>'
    }
    res += "</tr></thead>\n"

    res += "<tbody>"
    var odd = true
    for (const i in result.values) {
        const item = result.values[i]
        if (odd) {
            res += '<tr class="pure-table-odd">'
            odd = false
        } else {
            res += '<tr>'
            odd = true
        }

        for (const k in item) {
            res += '<td>' + renderValue(item[k]) + '</td>'
        }
        res += '</tr>'
    }
    res += "</tbody></table><hr/>"

    return res
}

/**
 * @param {Result} result
 * @param {Boolean} skipSimilar
 * @returns {string}
 */
function renderTransposedTable(result, skipSimilar) {
    let res = ''

    res += '<table class="pure-table result"><thead><tr>';
    res += '<th>column</th>'

    for (const i in result.values) {
        res += '<th>row '+i+'</th>'
    }

    res += "</tr></thead>\n"

    res += "<tbody>"
    let odd = true
    for (const k in result.columns) {
        if (skipSimilar) {
            let skip = true

            let prev = result.values[0][k]
            for (const i  in result.values) {
                if (i === 0) continue;

                let val = result.values[i][k]

                if (val !== prev) {
                    skip = false

                    break
                }

                prev = val
            }

            if (skip) {
                continue
            }
        }

        if (odd) {
            res += '<tr class="pure-table-odd">'
            odd = false
        } else {
            res += '<tr>'
            odd = true
        }

        res += '<td>' + result.columns[k] + '</td>'

        for (const i in result.values) {
            res += '<td>' + renderValue(result.values[i][k]) + '</td>'
        }

        res += '</tr>'
    }
    res += "</tbody></table><hr/>"

    return res
}

function renderValue(v) {
    if (v && typeof v === 'string' && (v.indexOf("\n") !== -1 || v.indexOf("\t") !== -1)) {
        return '<pre>' + v + '</pre>';
    }

    return v;
}


function uplotOpts() {
    return {
        width: document.getElementById("query-results").clientWidth,
        height: 300,
        // title: "Area Fill",
        tzDate: ts => uPlot.tzDate(new Date(ts * 1e3), 'Etc/UTC'),
        scales: {
            x: {
                time: false,
            },
        },
        series: [],
        axes: [
            {},
            {
                labelGap: 8,
                labelSize: 8 + 12 + 8,
                size(self, values, axisIdx, cycleNum) {
                    let axis = self.axes[axisIdx];

                    // bail out, force convergence
                    if (cycleNum > 1)
                        return axis._size;

                    let axisSize = axis.ticks.size + axis.gap;

                    // find longest value
                    let longestVal = (values ?? []).reduce((acc, val) => (
                        val.length > acc.length ? val : acc
                    ), "");

                    if (longestVal !== "") {
                        self.ctx.font = axis.font[0];
                        axisSize += self.ctx.measureText(longestVal).width / devicePixelRatio;
                    }

                    return Math.ceil(axisSize);
                },
            }
        ],
    };
}

function applyLegendSortOnHover(uplot_opts) {
    uplot_opts.hooks = uplot_opts.hooks || {};
    uplot_opts.hooks.init = uplot_opts.hooks.init || [];
    uplot_opts.hooks.init.push((u) => {
        const legend = u.root.querySelector(".u-legend");
        if (!legend) {
            return;
        }
        const rows = Array.from(legend.querySelectorAll(".u-series"));
        let needsIdx = false;
        for (let i = 0; i < rows.length; i++) {
            if (!rows[i].hasAttribute("data-idx")) {
                needsIdx = true;
                break;
            }
        }
        if (needsIdx) {
            for (let i = 0; i < rows.length && i < u.series.length; i++) {
                rows[i].setAttribute("data-idx", String(i));
            }
        }

        if (uplot_opts._legendSortData) {
            u._legendSortData = uplot_opts._legendSortData;
        }
    });

    uplot_opts.hooks.setLegend = uplot_opts.hooks.setLegend || [];
    uplot_opts.hooks.setLegend.push((u) => {
        if (!u.cursor || u.cursor.idx == null) {
            return;
        }

        const legend = u.root.querySelector(".u-legend");
        if (!legend) {
            return;
        }

        const rows = Array.from(legend.querySelectorAll(".u-series"));
        if (rows.length <= 2) {
            return;
        }

        const rowMap = new Map();
        rows.forEach((row) => {
            const idxAttr = row.getAttribute("data-idx");
            const idx = idxAttr != null ? parseInt(idxAttr, 10) : null;
            if (Number.isFinite(idx)) {
                rowMap.set(idx, row);
            }
        });

        const idxs = u.cursor.idxs || [];
        const hasValue = (si) => idxs[si] != null;
        const dataSource = u._legendSortData || u.data;
        const valueFor = (si) => {
            const idx = idxs[si];
            if (idx == null) {
                return null;
            }
            const v = dataSource[si] ? dataSource[si][idx] : null;
            if (v === null || typeof v === 'undefined') {
                return null;
            }
            const n = +v;
            return Number.isFinite(n) ? n : 0;
        };

        const order = [0];
        const withVals = [];
        const withoutVals = [];

        for (let si = 1; si < u.series.length; si++) {
            if (!rowMap.has(si)) {
                continue;
            }
            if (hasValue(si)) {
                withVals.push(si);
            } else {
                withoutVals.push(si);
            }
        }

        withVals.sort((a, b) => valueFor(b) - valueFor(a));
        order.push(...withVals, ...withoutVals);

        for (let i = 0; i < order.length; i++) {
            const row = rowMap.get(order[i]);
            if (row) {
                legend.appendChild(row);
            }
        }
    });
}

function toNumberOrNull(v) {
    if (v === null || typeof v === 'undefined') {
        return null;
    }
    if (typeof v === 'string' && v.trim() === '') {
        return null;
    }
    let n = parseFloat(v);
    return Number.isFinite(n) ? n : null;
}

function applyTimeAxis(uplot_opts, axisFmt) {
    function utcDate(ts) {
        if (!ts) {
            return null;
        }

        return uPlot.tzDate(new Date(ts * 1e3), 'Etc/UTC');
    }

    const axisDateFmt = axisFmt ? uPlot.fmtDate(axisFmt) : null;
    const fullDate = uPlot.fmtDate("{HH}:{mm}\n{YYYY}-{MM}-{DD}");
    const dayDate = uPlot.fmtDate("{HH}:{mm}\n{MM}-{DD}");
    const hourDate = uPlot.fmtDate("{HH}:{mm}");
    const legendDate = uPlot.fmtDate("{YYYY}-{MM}-{DD} {HH}:{mm}:{ss}");

    uplot_opts.axes[0].values = (u, vals, space) => {
        return vals.map((v, i) => {
            const d = utcDate(v);
            if (axisDateFmt) {
                return axisDateFmt(d)
            }
            const prev = i > 0 ? vals[i - 1] : null;
            if (!prev || !v) {
                return fullDate(d)
            }

            const prevUTCDate = utcDate(prev);

            if (d.getFullYear() !== prevUTCDate.getFullYear()) { // year changed
                return fullDate(d)
            } else if (d.getDate() !== prevUTCDate.getDate()) { // date changed
                return dayDate(d)
            } else { // same day
                return hourDate(d)
            }
        });
    };

    if (uplot_opts.series[0]) {
        uplot_opts.series[0].value = function(u, ts) {
            if (!ts) {
                return null;
            }

            return legendDate(utcDate(ts));
        };
    }
}

function stack(data, omit, fillNulls) {
    let data2 = [];
    let bands = [];
    let d0Len = data[0].length;
    let accum = Array(d0Len);

    for (let i = 0; i < d0Len; i++) {
        accum[i] = 0;
    }

    for (let i = 1; i < data.length; i++) {
        data2.push(omit(i) ? data[i] : data[i].map((v, i) => {
            if (v === null || typeof v === 'undefined') {
                return fillNulls ? accum[i] : null;
            }
            let n = +v;
            if (!Number.isFinite(n)) {
                return fillNulls ? accum[i] : null;
            }
            return (accum[i] += n);
        }));
    }

    for (let i = 1; i < data.length; i++) {
        !omit(i) && bands.push({
            series: [
                data.findIndex((s, j) => j > i && !omit(j)),
                i,
            ],
        });
    }

    bands = bands.filter(b => b.series[0] > -1 && b.series[1] > -1);

    return {
        data: [data[0]].concat(data2),
        bands,
    };
}

function getStackedOpts(uplot_opts, series, data) {
    uplot_opts.series = series;
    uplot_opts._legendSortData = data;

    let stacked = stack(data, i => false, true);
    uplot_opts.bands = stacked.bands;

    uplot_opts.cursor = uplot_opts.cursor || {};
    uplot_opts.cursor.dataIdx = (u, seriesIdx, closestIdx, xValue) => {
        return data[seriesIdx][closestIdx] == null ? null : closestIdx;
    };

    uplot_opts.series.forEach((s, si) => {
        s.value = (u, v, si, i) => data[si][i];

        s.points = s.points || {};

        // scan raw unstacked data to return only real points
        s.points.filter = (u, seriesIdx, show, gaps) => {
            if (show) {
                let pts = [];
                data[seriesIdx].forEach((v, i) => {
                    v != null && pts.push(i);
                });
                return pts;
            }
        }
    });

    // force 0 to be the sum minimum instead of the bottom series
    uplot_opts.scales.y = {
        range: (u, min, max) => {
            let minMax = uPlot.rangeNum(0, max, 0.1, true);
            return [0, minMax[1]];
        }
    };

    // restack on toggle
    uplot_opts.hooks = uplot_opts.hooks || {};
    uplot_opts.hooks.setSeries = uplot_opts.hooks.setSeries || [];
    uplot_opts.hooks.setSeries.push((u, i) => {
        let stacked = stack(data, i => !u.series[i].show, true);
        u.delBand(null);
        stacked.bands.forEach(b => u.addBand(b));
        u.setData(stacked.data);
    });

    return {opts: uplot_opts, data: stacked.data};
}

function buildStackedBarsSeries(columns) {
    const { bars } = uPlot.paths;
    const barsPath = bars({size: [0.95, 100]});

    let series = [{
        label: columns[0],
    }];

    for (let i = 1; i < columns.length; i++) {
        series.push({
            label: columns[i],
            width: 1,
            fill: getDarkColor(),
            paths: barsPath,
            points: {show: false},
        });
    }

    return series;
}

/**
 * Build rows-based data (x, y, label) into uPlot columnar data.
 * @param {Result} result
 * @returns {{uplot_data: Array<Array<*>>, labelsArr: Array<string>}}
 */
function buildRowsData(result) {
    let timedData = {}
    let labels = {}

    for (let i in result.values) {
        let row = result.values[i]
        let t = toNumberOrNull(row[0])
        let val = toNumberOrNull(row[1])
        let label = row[2]

        if (t === null || typeof label === 'undefined') {
            continue;
        }

        labels[label] = 1
        let key = String(t)
        if (!timedData[key]) {
            timedData[key] = {};
        }
        timedData[key][label] = val
    }

    let labelsArr = Object.keys(labels).sort()

    let uplot_data = []
    for (let i = 0; i < 1 + labelsArr.length; i++) {
        uplot_data.push([])
    }

    let xVals = Object.keys(timedData)
        .map(k => parseFloat(k))
        .filter(Number.isFinite)
    xVals.sort((a, b) => a - b)

    for (let xi = 0; xi < xVals.length; xi++) {
        let t = xVals[xi]
        uplot_data[0].push(t)

        let values = timedData[String(t)] || {}
        for (let li = 0; li < labelsArr.length; li++) {
            let label = labelsArr[li]
            let val = values[label]
            uplot_data[1 + li].push(typeof val === 'undefined' ? null : val)
        }
    }

    return {uplot_data, labelsArr}
}

/**
 *
 * @param {Result} result
 * @param uplot_opts
 * @returns {[]}
 */
function uplotRowsData(result, uplot_opts) {
    let built = buildRowsData(result)
    let uplot_data = built.uplot_data
    let labelsArr = built.labelsArr

    uplot_opts.series.push({
        label: result.columns[0],
    })

    for (let i = 0; i < labelsArr.length; i++) {
        uplot_opts.series.push({
            stroke: getDarkColor(),
            label: labelsArr[i],
        })
    }

    return uplot_data
}

/**
 *
 * @param {Result} result
 * @param uplot_opts
 * @returns {[]}
 */
function uplotBarsRowsData(result, uplot_opts) {
    let built = buildRowsData(result)
    let uplot_data = built.uplot_data
    let labelsArr = built.labelsArr
    let series = buildStackedBarsSeries([result.columns[0]].concat(labelsArr))

    let stacked = getStackedOpts(uplot_opts, series, uplot_data);

    return stacked.data;
}

/**
 *
 * @param {Result} result
 * @param uplot_opts
 * @returns {[]}
 */
function uplotBarsColumnsData(result, uplot_opts) {
    let series = buildStackedBarsSeries(result.columns);
    let uplot_data = [];
    for (let i = 0; i < result.columns.length; i++) {
        uplot_data.push([]) // Separate vector for each column (X + multiple Y).
    }

    let sortable = [];
    for (let i in result.values) {
        let item = result.values[i];
        let x = toNumberOrNull(item[0]);
        if (x === null) {
            continue;
        }
        sortable.push({x, item});
    }

    // Sorting data by first column (X axis) ascending.
    sortable.sort(function (a, b) {
        return a.x - b.x
    });

    for (let i = 0; i < sortable.length; i++) {
        let item = sortable[i].item;
        uplot_data[0].push(sortable[i].x);

        for (let j = 1; j < result.columns.length; j++) {
            let val = (j < item.length) ? toNumberOrNull(item[j]) : null;
            uplot_data[j].push(val);
        }
    }

    let stacked = getStackedOpts(uplot_opts, series, uplot_data);

    return stacked.data;
}

/**
 *
 * @param {Result} result
 * @param uplot_opts
 * @returns {[]}
 */
function uplotColumnsData(result, uplot_opts) {
    let uplot_data = [];
    for (let i = 0; i < result.columns.length; i++) {
        uplot_data.push([]) // Separate vector for each column (X + multiple Y).

        if (i === 0) {
            uplot_opts.series.push({
                label: result.columns[i],
            })
        } else {
            uplot_opts.series.push({
                stroke: getDarkColor(),
                label: result.columns[i],
            })
        }
    }

    // Sorting data by first column (X axis) ascending.
    let sortedValues = result.values.sort(function (a, b) {
        return a[0] - b[0]
    });

    for (let i in sortedValues) {
        let item = sortedValues[i]

        for (let j = 0; j < item.length; j++) {
            uplot_data[j].push(parseFloat(item[j]))
        }
    }

    return uplot_data
}

/**
 * @type {Array<Result>}
 */
var results = []
var isPortableReport = false;

function renderResults() {
    if (!results) {
        $('#query-results').html("<tr><td>No data.</td></tr>")
        return
    }

    $('#query-results').html('')

    for (var i in results) {
        renderResult(results[i], i)
    }

    if (!$.fn.fancyTable) {
        $.fn.fancyTable = fancyTable
    }

    $('#query-results table.result').fancyTable({
        sortable: true,
        searchable: true,
        fuzzySearch: true,
        pagination: false,
        globalSearch: true
    });
}

// drawPieChart renders a pie chart, courtesy of deepseek-r1:32b with minor changes.
function drawPieChart(data, total, container) {
    // Set chart dimensions
    const width = 500;
    const height = 500;
    const margin = 0;
    const chartWidth = width - 2 * margin;
    const chartHeight = height - 2 * margin;

    // Create SVG element
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute('width', document.getElementById("query-results").clientWidth);
    svg.setAttribute('height', height);
    container.appendChild(svg);

    // Calculate totals
    if (!total) {
        total = data.reduce((sum, item) => sum + item.value, 0);
    }

    // Create pie chart slices
    let currentAngle = 0;
    const centerX = margin + chartWidth / 2;
    const centerY = margin + chartHeight / 2;

    const radius = Math.min(chartWidth - margin * 2, chartHeight - margin * 2) / 2;

    data.forEach(item => {
        const percent = (item.value / total) * 100;
        const angle = (percent / 100) * 2 * Math.PI;

        // Create slice path
        const slice = document.createElementNS("http://www.w3.org/2000/svg", "path");
        slice.classList.add("slice");

        const d = [
            `M ${centerX} ${centerY}`,
            `L ${centerX + radius * Math.cos(currentAngle)} ${centerY + radius * Math.sin(currentAngle)}`,
            `A ${radius} ${radius} 0 ${(angle > Math.PI ? 1 : 0)} 1 ${centerX + radius * Math.cos(currentAngle + angle)} ${centerY + radius *
            Math.sin(currentAngle + angle)}`,
            `L ${centerX} ${centerY}`
        ].join(' ');

        slice.setAttributeNS(null, 'd', d);
        slice.style.fill = item.color;

        // Add title (tooltip)
        const tooltip = document.createElementNS("http://www.w3.org/2000/svg", "title");
        tooltip.textContent = `${item.label}: ${item.value} (${percent.toFixed(1)}%)`;
        slice.appendChild(tooltip);

        svg.appendChild(slice);

        currentAngle += angle;
    });

    // Add legend
    const legendG = document.createElementNS("http://www.w3.org/2000/svg", "g");
    legendG.setAttribute('transform', `translate(520,${margin})`);

    data.forEach((item, index) => {
        const legendItem = document.createElementNS("http://www.w3.org/2000/svg", 'g');
        legendItem.classList.add('legend-item');

        // Legend color swatch
        const rect = document.createElementNS("http://www.w3.org/2000/svg", "rect");
        rect.setAttributeNS(null, 'x', 0);
        rect.setAttributeNS(null, 'y', index * 20);
        rect.style.fill = item.color;

        // Legend text
        const text = document.createElementNS("http://www.w3.org/2000/svg", "text");
        text.setAttributeNS(null, 'x', 24);
        text.setAttributeNS(null, 'y', index * 20 + 12);
        const percent = (item.value / total) * 100;
        text.textContent = `${item.label}: ${item.value} (${percent.toFixed(1)}%)`;

        legendItem.appendChild(rect);
        legendItem.appendChild(text);
        legendG.appendChild(legendItem);
    });

    svg.appendChild(legendG);
}

function fancyTable(options) {
    var settings = $.extend({
        inputStyle: "",
        inputPlaceholder: "Search...",
        pagination: false,
        paginationClass: "btn btn-light",
        paginationClassActive: "active",
        pagClosest: 3,
        perPage: 10,
        sortable: true,
        searchable: true,
        fuzzySearch: false,
        fuzzySearchOptions: null,
        matchCase: false,
        exactMatch: false,
        localeCompare: false,
        onInit: function () {
        },
        beforeUpdate: function () {
        },
        onUpdate: function () {
        },
        sortFunction: function (a, b, fancyTableObject, rowA, rowB) {
            if (a == b && rowA && rowB) {
                // If sort values are of equal priority, sort by last order
                return (fancyTableObject.rowSortOrder[$(rowA).data("rowid")] > fancyTableObject.rowSortOrder[$(rowB).data("rowid")]);
            }
            if (fancyTableObject.sortAs[fancyTableObject.sortColumn] == 'numeric') {
                return (
                    (fancyTableObject.sortOrder > 0) ? (parseFloat(a) || 0) - (parseFloat(b) || 0) : (parseFloat(b) || 0) - (parseFloat(a) || 0) // NaN values will be sorted as 0
                );
            }
            if (fancyTableObject.sortAs[fancyTableObject.sortColumn] == 'datetime') {
                [a, b] = [a, b].map(x => {
                    return Date.parse(x) || 0; // NaN values will be sorted as epoch (1/1/1970 00:00:00 UTC)
                });
                return (fancyTableObject.sortOrder > 0) ? (a - b) : (b - a);
            } else {
                if (settings.localeCompare) {
                    return ((a.localeCompare(b) < 0) ? -fancyTableObject.sortOrder : (a.localeCompare(b) > 0) ? fancyTableObject.sortOrder : 0);
                } else {
                    return ((a < b) ? -fancyTableObject.sortOrder : (a > b) ? fancyTableObject.sortOrder : 0);
                }
            }
        },
        testing: false
    }, options);
    if (settings.fuzzySearch && typeof uFuzzy === "undefined") {
        settings.fuzzySearch = false;
    }
    var instance = this;
    this.settings = settings;
    function buildFuzzyIndex(elm) {
        if (!settings.fuzzySearch || !settings.globalSearch) {
            return;
        }
        let rows = $(elm).find("tbody tr").toArray();
        let haystack = rows.map(row => {
            let parts = [];
            $(row).find("td").each(function (idx) {
                if (Array.isArray(settings.globalSearchExcludeColumns) && settings.globalSearchExcludeColumns.includes(idx + 1)) {
                    return;
                }
                parts.push($(this).text());
            });
            return parts.join(" ");
        });
        elm.fancyTable.fuzzyRows = rows;
        elm.fancyTable.fuzzyHaystack = haystack;
    }
    this.tableUpdate = function (elm) {
        settings.beforeUpdate.call(this, elm);
        elm.fancyTable.matches = 0;
        let fuzzyMatches = null;
        if (settings.fuzzySearch && settings.globalSearch && elm.fancyTable.search) {
            if (!elm.fancyTable.fuzzy) {
                elm.fancyTable.fuzzy = new uFuzzy(settings.fuzzySearchOptions || {});
            }
            if (!elm.fancyTable.fuzzyHaystack || elm.fancyTable.fuzzyHaystack.length !== $(elm).find("tbody tr").length) {
                buildFuzzyIndex(elm);
            }
            let res = elm.fancyTable.fuzzy.search(elm.fancyTable.fuzzyHaystack || [], elm.fancyTable.search, 1);
            let idxs = (res && res[0]) ? res[0] : [];
            let order = res ? res[2] : null;
            let matched = order && order.length ? order.map(i => idxs[i]) : idxs;
            fuzzyMatches = new Set(matched);
        }
        $(elm).find("tbody tr").each(function (rowIdx) {
            var n = 0;
            var match = true;
            var globalMatch = false;
            if (settings.globalSearch && fuzzyMatches) {
                globalMatch = fuzzyMatches.has(rowIdx);
            } else {
                $(this).find("td").each(function () {
                    if (!settings.globalSearch && elm.fancyTable.searchArr[n] && !(instance.isSearchMatch($(this).html(), elm.fancyTable.searchArr[n]))) {
                        match = false;
                    } else if (settings.globalSearch && (!elm.fancyTable.search || (instance.isSearchMatch($(this).html(), elm.fancyTable.search)))) {
                        if (!Array.isArray(settings.globalSearchExcludeColumns) || !settings.globalSearchExcludeColumns.includes(n + 1)) {
                            globalMatch = true;
                        }
                    }
                    n++;
                });
            }
            if ((settings.globalSearch && globalMatch) || (!settings.globalSearch && match)) {
                elm.fancyTable.matches++
                if (!settings.pagination || (elm.fancyTable.matches > (elm.fancyTable.perPage * (elm.fancyTable.page - 1)) && elm.fancyTable.matches <= (elm.fancyTable.perPage * elm.fancyTable.page))) {
                    $(this).show();
                } else {
                    $(this).hide();
                }
            } else {
                $(this).hide();
            }
        });
        elm.fancyTable.pages = Math.ceil(elm.fancyTable.matches / elm.fancyTable.perPage);
        if (settings.pagination) {
            var paginationElement = (elm.fancyTable.paginationElement) ? $(elm.fancyTable.paginationElement) : $(elm).find(".pag");
            paginationElement.empty();
            for (var n = 1; n <= elm.fancyTable.pages; n++) {
                if (n == 1 || (n > (elm.fancyTable.page - (settings.pagClosest + 1)) && n < (elm.fancyTable.page + (settings.pagClosest + 1))) || n == elm.fancyTable.pages) {
                    var a = $("<a>", {
                        html: n,
                        "data-n": n,
                        style: "margin:0.2em",
                        class: settings.paginationClass + " " + ((n == elm.fancyTable.page) ? settings.paginationClassActive : "")
                    }).css("cursor", "pointer").bind("click", function () {
                        elm.fancyTable.page = $(this).data("n");
                        instance.tableUpdate(elm);
                    });
                    if (n == elm.fancyTable.pages && elm.fancyTable.page < (elm.fancyTable.pages - settings.pagClosest - 1)) {
                        paginationElement.append($("<span>...</span>"));
                    }
                    paginationElement.append(a);
                    if (n == 1 && elm.fancyTable.page > settings.pagClosest + 2) {
                        paginationElement.append($("<span>...</span>"));
                    }
                }
            }
        }
        settings.onUpdate.call(this, elm);
    };
    this.isSearchMatch = function (data, search) {
        if (!settings.matchCase) {
            data = data.toUpperCase();
            search = search.toUpperCase();
        }
        if (settings.exactMatch == "auto" && search.match(/^".*?"$/)) {
            // Exact match due to "quoted" value
            search = search.substring(1, search.length - 1);
            return (data == search);
        } else if (settings.exactMatch == "auto" && search.replace(/\s+/g, "").match(/^[<>]=?/)) {
            // Less < or greater > than
            var comp = search.replace(/\s+/g, "").match(/^[<>]=?/)[0];
            var val = search.replace(/\s+/g, "").substring(comp.length);
            return ((comp == '>' && data * 1 > val * 1) || (comp == '<' && data * 1 < val * 1) || (comp == '>=' && data * 1 >= val * 1) || (comp == '<=' && data * 1 <= val * 1))
        } else if (settings.exactMatch == "auto" && search.replace(/\s+/g, "").match(/^.+(\.\.|-).+$/)) {
            // Intervall 10..20 or 10-20
            var arr = search.replace(/\s+/g, "").split(/\.\.|-/);
            return (data * 1 >= arr[0] * 1 && data * 1 <= arr[1] * 1);
        }
        try {
            return (settings.exactMatch === true) ? (data == search) : (new RegExp(search).test(data));
        } catch {
            return false;
        }
    };
    this.reinit = function () {
        $(this).each(function () {
            $(this).find("th a").contents().unwrap();
            $(this).find("tr.fancySearchRow").remove();
        });
        $(this).fancyTable(this.settings);
    };
    this.tableSort = function (elm) {
        if (typeof elm.fancyTable.sortColumn !== "undefined" && elm.fancyTable.sortColumn < elm.fancyTable.nColumns) {
            var iElm = 0;
            $(elm).find("thead th").each(function () {
                $(this).attr("aria-sort",
                    (iElm == elm.fancyTable.sortColumn) ?
                        ((elm.fancyTable.sortOrder == 1) ? "ascending" : (elm.fancyTable.sortOrder == -1) ? "descending" : "other")
                        : null // "none" // Remove the attribute instead of setting to "none" to avoid spamming screen readers.
                );
                iElm++;
            });
            $(elm).find("thead th div.sortArrow").each(function () {
                $(this).remove();
            });
            var sortArrow = $("<div>", {"class": "sortArrow"}).css({
                "margin": "0.1em",
                "display": "inline-block",
                "width": 0,
                "height": 0,
                "border-left": "0.4em solid transparent",
                "border-right": "0.4em solid transparent"
            });
            sortArrow.css(
                (elm.fancyTable.sortOrder > 0) ?
                    {"border-top": "0.4em solid #000"} :
                    {"border-bottom": "0.4em solid #000"}
            );
            $(elm).find("thead th a").eq(elm.fancyTable.sortColumn).append(sortArrow);
            var rows = $(elm).find("tbody tr").toArray().sort(
                function (a, b) {
                    var elma = $(a).find("td").eq(elm.fancyTable.sortColumn);
                    var elmb = $(b).find("td").eq(elm.fancyTable.sortColumn);
                    var cmpa = typeof $(elma).data('sortvalue') !== 'undefined' ? $(elma).data('sortvalue') : elma.html();
                    var cmpb = typeof $(elmb).data('sortvalue') !== 'undefined' ? $(elmb).data('sortvalue') : elmb.html();
                    if (elm.fancyTable.sortAs[elm.fancyTable.sortColumn] == 'case-insensitive') {
                        cmpa = cmpa.toLowerCase();
                        cmpb = cmpb.toLowerCase();
                    }
                    return settings.sortFunction.call(this, cmpa, cmpb, elm.fancyTable, a, b);
                }
            );
            $(rows).each(function (index) {
                elm.fancyTable.rowSortOrder[$(this).data("rowid")] = index;
            });
            $(elm).find("tbody").empty().append(rows);
            buildFuzzyIndex(elm);
        }
    };
    this.each(function () {
        if ($(this).prop("tagName") !== "TABLE") {
            console.warn("fancyTable: Element is not a table.");
            return true;
        }
        var elm = this;
        elm.fancyTable = {
            nColumns: $(elm).find("td").first().parent().find("td").length,
            nRows: $(this).find("tbody tr").length,
            perPage: settings.perPage,
            page: 1,
            pages: 0,
            matches: 0,
            searchArr: [],
            search: "",
            sortColumn: settings.sortColumn,
            sortOrder: (typeof settings.sortOrder === "undefined") ? 1 : (new RegExp("desc", "i").test(settings.sortOrder) || settings.sortOrder == -1) ? -1 : 1,
            sortAs: [], // undefined, numeric, datetime, case-insensitive, or custom
            paginationElement: settings.paginationElement,
            fuzzy: settings.fuzzySearch ? new uFuzzy(settings.fuzzySearchOptions || {}) : null,
            fuzzyRows: null,
            fuzzyHaystack: null
        };
        elm.fancyTable.rowSortOrder = new Array(elm.fancyTable.nRows);
        if ($(elm).find("tbody").length == 0) {
            var content = $(elm).html();
            $(elm).empty();
            $(elm).append("<tbody>").append($(content));
        }
        if ($(elm).find("thead").length == 0) {
            $(elm).prepend($("<thead>"));
            // Maybe add generated headers at some point
            //var c=$(elm).find("tr").first().find("td").length;
            //for(var n=0; n<c; n++){
            //	$(elm).find("thead").append($("<th></th>"));
            //}
        }
        $(elm).find("tbody tr").each(function (index) {
            // $(this).attr("data-rowid", index);
            $(this).data("rowid", index);
        });
        if (settings.sortable) {
            var nAElm = 0;
            $(elm).find("thead th").each(function () {
                elm.fancyTable.sortAs.push($(this).data('sortas'));
                var content = $(this).html();
                var a = $("<a>", {
                    href: "#",
                    "aria-label": "Sort by " + $(this).text(),
                    html: content,
                    "data-n": nAElm,
                    class: ""
                }).css({
                    "cursor": "pointer",
                    "color": "inherit",
                    "text-decoration": "none",
                    "white-space": "nowrap"
                }).bind("click", function () {
                    if (elm.fancyTable.sortColumn == $(this).data("n")) {
                        elm.fancyTable.sortOrder = -elm.fancyTable.sortOrder;
                    } else {
                        elm.fancyTable.sortOrder = 1;
                    }
                    elm.fancyTable.sortColumn = $(this).data("n");
                    instance.tableSort(elm);
                    instance.tableUpdate(elm);
                    return false;
                });
                $(this).empty();
                $(this).append(a);
                nAElm++;
            });
        }
        if (settings.searchable) {
            var searchHeader = $("<tr>").addClass("fancySearchRow");
            if (settings.globalSearch) {
                var searchField = $("<input>", {
                    "aria-label": "Search table",
                    "placeholder": settings.inputPlaceholder,
                    style: "width:100%;box-sizing:border-box;" + settings.inputStyle
                }).bind("change paste keyup", function () {
                    elm.fancyTable.search = $(this).val();
                    elm.fancyTable.page = 1;
                    instance.tableUpdate(elm);
                });
                var th = $("<th>", {style: "padding:2px;"}).attr("colspan", elm.fancyTable.nColumns);
                $(searchField).appendTo($(th));
                $(th).appendTo($(searchHeader));
            } else {
                var nInputElm = 0;
                $(elm).find("td").first().parent().find("td").each(function () {
                    elm.fancyTable.searchArr.push("");
                    var searchField = $("<input>", {
                        "aria-label": "Search column",
                        "data-n": nInputElm,
                        "placeholder": settings.inputPlaceholder,
                        style: "width:100%;box-sizing:border-box;" + settings.inputStyle
                    }).bind("change paste keyup", function () {
                        elm.fancyTable.searchArr[$(this).data("n")] = $(this).val();
                        elm.fancyTable.page = 1;
                        instance.tableUpdate(elm);
                    });
                    var th = $("<th>", {style: "padding:2px;"});
                    $(searchField).appendTo($(th));
                    $(th).appendTo($(searchHeader));
                    nInputElm++;
                });
            }
            searchHeader.appendTo($(elm).find("thead"));
        }
        // Sort
        instance.tableSort(elm);
        buildFuzzyIndex(elm);
        if (settings.pagination && !settings.paginationElement) {
            $(elm).find("tfoot").remove();
            $(elm).append($("<tfoot><tr></tr></tfoot>"));
            $(elm).find("tfoot tr").append($("<td class='pag'></td>", {}).attr("colspan", elm.fancyTable.nColumns));
        }
        instance.tableUpdate(elm);
        settings.onInit.call(this, elm);
    });
    return this;
};

$.fn.fancyTable = fancyTable
