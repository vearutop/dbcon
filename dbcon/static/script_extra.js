function render() {
    var props = arguments;
    return function (tok, i) {
        if (i % 2) {
            for (var a = 0; a < props.length; a++) {
                if (typeof props[a][tok] !== "undefined") {
                    return props[a][tok];
                }
            }
        }
        return tok;
    };
}

function get(url) {
    var res = ""

    $.ajax({
        async: false,
        type: 'GET',
        url: url,
        success: function(data) {
            res = data
        }
    });

    return res
}

// scriptSrcEndingWith finds the resolved src of a <script> tag already on the page whose URL
// ends with suffix, so the portable report bundles assets from wherever they are actually
// mounted (e.g. the plot package's own path prefix) instead of a hardcoded root path.
function scriptSrcEndingWith(suffix) {
    var scripts = document.getElementsByTagName('script')
    for (var i = 0; i < scripts.length; i++) {
        if (scripts[i].src && scripts[i].src.endsWith(suffix)) {
            return scripts[i].src
        }
    }
    return suffix
}

function downloadHTMLReport() {
    if (!results) {
        alert("No results.");
        return
    }

    var res = ""

    res += '<!DOCTYPE html>\n' +
        '<html lang="en">\n' +
        '<head>\n' +
        '    <meta charset="utf-8"/>\n' +
        '    <title>Report</title>\n' +
        '    <meta name="viewport" content="width=device-width, initial-scale=1">\n';

    var css = ""
    for (var i = 0; i< document.styleSheets.length;i++) {
        css += Array.from(document.styleSheets[i].cssRules).map(rule => rule.cssText).join(" ") + "\n"
    }

    res += '<style>' + css + '</style>\n'

    var rr = JSON.stringify(results)
    var rtemp = results

    var js = ""
    js += get("jquery-3.7.1.min.js") + "\n"
    js += get(scriptSrcEndingWith("uPlot.iife.min.js")) + "\n"
    js += get("uFuzzy.iife.js") + "\n"
    js += get(scriptSrcEndingWith("plot.js")) + "\n"
    js += get("script.js") + "\n"

    results = rtemp
    res += '<script>' + js + '</script>\n'

    res += '</head>\n' +
        '<body><div style="margin: 2em">' +
        '<h1>Report</h1>' +
        '<div id="query-results"></div>\n' +
        '<script>\n' +
        'isPortableReport = true;\n' +
        'results = ' +rr+';\n' +
        'renderResults();\n' +
        '</script>' +
        '\n' + '</div></body></html>'

    download_file("report.html", res, "text/html")
}

function download_file(name, contents, mime_type) {
    mime_type = mime_type || "text/plain";

    var blob = new Blob([contents], {type: mime_type});

    var dlink = document.createElement('a');
    dlink.download = name;
    dlink.href = window.URL.createObjectURL(blob);
    dlink.onclick = function(e) {
        // revokeObjectURL needs a delay to work properly
        var that = this;
        setTimeout(function() {
            window.URL.revokeObjectURL(that.href);
        }, 1500);
    };

    dlink.click();
    dlink.remove();
}

var editor

/**
 * Finds the instance for the end of text: the last `-- instance:name` comment, or the default one.
 * @param {String} text
 * @returns {String}
 */
function instanceAt(text) {
    var m, name = defaultInstance
    var re = /^[ \t]*--[ \t]*instance:[ \t]*(\S.*?)[ \t]*$/gm

    while ((m = re.exec(text))) {
        name = m[1]
    }

    return name
}

function initEditor() {
    editor = ace.edit('editor')
    editor.setTheme()
    editor.setOptions({
        enableBasicAutocompletion: true,
        enableLiveAutocompletion: true
    })

    // The editor box is user-resizable (CSS resize), let Ace follow.
    new ResizeObserver(function () {
        editor.resize()
    }).observe(document.getElementById('editor'))

    // Completions depend on the instance selected by the closest preceding `-- instance:` comment.
    editor.completers = [{
        getCompletions: function (editor, session, pos, prefix, callback) {
            callback(null, completions[instanceAt(session.getTextRange({start: {row: 0, column: 0}, end: pos}))] || [])
        }
    }]

    editor.session.setMode("ace/mode/sql")
}

function runQuery() {
    $("#link-form").hide()
    $("#download-report").hide()
    $('#query-status').addClass("spinner").html("<span style='font-size: 15px'>loading...</span>").show()

    $.ajax({
        type: 'POST',
        url: 'query-db',
        contentType: 'application/json',
        data: JSON.stringify({statement: editor.getValue()}),
        success: function (response) {
            results = response.results

            $("#link-form").attr("href", "?form=" + encodeURIComponent(response.form)).show()
            $("#download-report").show()

            renderResults()
        },
        error: function (x) {
            $('#query-status').text(x.responseText || x.statusText)
        },
        complete: function (x) {
            $('#query-status').removeClass("spinner")
            if (x.status === 200) {
                $('#query-status').hide()
            }
        }
    })

    return false
}

var askInstance = null

function sendPrompt() {
    $('#ask-ai-result').addClass("spinner").html("<span style='font-size: 15px'>loading...</span>").show()

    $.ajax({
        type: 'POST',
        url: 'prompt',
        contentType: 'application/json',
        data: JSON.stringify({
            instance: askInstance || instanceAt(editor.getValue()),
            question: $('#ask-ai-question').val()
        }),
        success: function (response) {
            $('#ask-ai-result').html(response.message)
        },
        error: function (x) {
            $('#ask-ai-result').text(x.responseText || x.statusText)
        },
        complete: function () {
            askInstance = null
            $('#ask-ai-result').removeClass("spinner")
        }
    })

    return false
}

var completions = {}

function renderColumnsDirectory() {
    var rows = ''
    var multi = Object.keys(completions).length > 1
    var inst = function (name) {
        return multi ? '<td>' + name + '</td>' : ''
    }

    for (var instance in completions) {
        var cmp = completions[instance]
        for (var i in cmp) {
            var item = cmp[i]

            if (item.table && item.column) {
                rows += '<tr>' + inst(instance) + '<td>' + item.table + '</td><td>' + item.column + "</td><td>column</td></tr>\n"
                continue
            }

            if (item.table) {
                rows += '<tr>' + inst(instance) + '<td>' + item.table + "</td><td></td><td>table</td></tr>\n"
                continue
            }
        }
    }

    if (!rows) {
        return
    }

    var res = '<table class="pure-table result"><thead><tr>';
    res += (multi ? '<th>instance</th>' : '') + '<th>table</th><th>column</th><th>entity</th>'
    res += "</tr></thead>\n"

    res += "<tbody>"
    res += rows
    res += "</tbody></table>"

    $('#columns-directory').html('<div class="btn btn-info" onclick="return toggleColumnsDirectory();">Columns Directory</div>' +
        '<div class="overlay" style="display: none">' + res + '</div>')
    $('#columns-directory table').fancyTable({
        sortable: true,
        searchable: true,
        pagination: false,
        globalSearch: true,
        fuzzySearch: true,
    });
}

function fixStatementAI(resultIdx) {
    var result = results[resultIdx]

    $('#columns-directory .overlay').hide();
    $('#ask-ai').show();

    $('#ask-ai-question').val("```\n" + result.statement + "\n```\nfailed with\n```\n"+result.error+"\n```\n\nPlease fix or explain the error.")
    askInstance = result.instance

    return false
}

function toggleAskAI() {
    $('#columns-directory .overlay').hide();
    $('#ask-ai').toggle();
    return false;
}

function toggleColumnsDirectory() {
    $('#ask-ai').hide();
    $('#columns-directory .overlay').toggle();
    return false;
}

function refreshSchema() {
    var btn = $('#refresh-schema')
    btn.addClass('spinner').text('Refreshing...')

    $.ajax({
        type: 'POST',
        url: 'refresh-schema',
        success: function (data) {
            completions = data.completions

            renderColumnsDirectory();
        },
        error: function (x) {
            alert("Failed to refresh schema: " + x.responseText);
        },
        complete: function () {
            btn.removeClass('spinner').text('Refresh Schema');
        }
    });

    return false;
}
