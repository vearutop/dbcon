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
    js += get("/json-form/jquery-3.7.1.min.js") + "\n"
    js += get("/uPlot.iife.min.js") + "\n"
    js += get("/script.js") + "\n"

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

/**
 * @param {XMLHttpRequest} x
 */
function onQuerySQLSuccess(x) {
    // console.log(x.responseText)

    var response = JSON.parse(x.responseText)

    results = response.results

    var url = "?form=" + encodeURIComponent(response.form)
    // console.log(url)

    $("#link-form").attr("href", url).show()
    $("#download-report").show()

    // console.log(results)

    renderResults();
}

function onQuerySQLBeforeSubmit(values) {
    $("#link-form").hide()
    $("#download-report").hide()


    $('#form-title-0').addClass("spinner")

    $('#query-result').html("<tr><td>Running query:</td></tr><tr><td>" + values.statement + "</td></tr>")
}

function onQuerySQLFinished() {
    $('#form-title-0').removeClass("spinner")
}

var completions = {}

function renderColumnsDirectory() {
    var rows = ''

    for (var instance in completions) {
        var cmp = completions[instance]
        for (var i in cmp) {
            var item = cmp[i]

            if (item.table && item.column) {
                rows += '<tr><td>' + instance + '</td><td>' + item.table + '</td><td>' + item.column + "</td><td>column</td></tr>\n"
                continue
            }

            if (item.table) {
                rows += '<tr><td>' + instance + '</td><td>' + item.table + "</td><td></td><td>table</td></tr>\n"
                continue
            }
        }
    }

    if (!rows) {
        return
    }

    var res = '<table class="pure-table result" style="display: none;margin-top:2em;background: #fff"><thead><tr>';
    res += '<th>instance</th><th>table</th><th>column</th><th>entity</th>'
    res += "</tr></thead>\n"

    res += "<tbody>"
    res += rows
    res += "</tbody></table>"

    $('#form-title-0').parent().parent().append('<div id="columns-directory" class="pure-u-2-5" style="position: absolute;margin-top: 4em;"></div>')

    $('#columns-directory').html('<div class="btn btn-info" onclick="$(this).next().toggle();return false;">Columns Directory</div>' + res)
    $('#columns-directory table').fancyTable({
        sortable: true,
        searchable: true,
        pagination: false,
        globalSearch: true
    });
}


window.jsonform_ace_setup = function(setup){
    setup()

    $(".sql-statement").each(function(i, el){
        var instanceSelect = $(el).parent().find('select')
        var id = $(el).find('.ace_editor').parent().attr('id')

        function prepare() {
            var instance = instanceSelect.find(":selected").val()

            var editor = ace.edit(id + '__ace')
            editor.setTheme();
            editor.setOptions({
                enableBasicAutocompletion: true,
                enableLiveAutocompletion: true
            });

            var cmp = completions[instance]

            editor.completers = []

            if (cmp) {
                editor.completers.push({
                    getCompletions: function(editor, session, pos, prefix, callback) {
                        callback(null, cmp);
                    }
                });
            }

            editor.session.setMode("ace/mode/sql");
        }

        prepare()

        instanceSelect.on('change', function() {
            prepare()
        });
    })
}