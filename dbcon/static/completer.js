/**
 * Contextual SQL completion.
 *
 * Not a parser: a tolerant tokenizer plus heuristics for the clause under the cursor and the tables in scope
 * (aliases, CTEs, derived tables, parenthesis nesting), which works on incomplete statements.
 *
 * Schema is built from dbcon completion items: {value, table, column, meta, score}, names are already quoted
 * for the dialect (backticks for SQLite/MySQL, double quotes for Postgres), identifiers typed by the user are
 * compared unquoted and case-insensitively.
 */
(function (root) {
    'use strict'

    // Groups: 1 space, 2 line comment, 3 block comment, 4 string, 5 "ident", 6 `ident`, 7 word, 8 number, 9 other.
    var TOKEN = /(\s+)|(--[^\n]*)|(\/\*[\s\S]*?(?:\*\/|$))|('(?:[^']|'')*(?:'|$))|("(?:[^"]|"")*(?:"|$))|(`(?:[^`]|``)*(?:`|$))|([A-Za-z_$¢-￿][\w$¢-￿]*)|(\d[\w.]*)|([\s\S])/y
    var KINDS = [null, null, 'c', 'c', 's', 'q', 'q', 'w', 'n', 'p']

    // Clause keyword -> what is expected after it.
    var CLAUSES = {
        select: 'select', returning: 'select', from: 'from', join: 'join', on: 'on', using: 'on', where: 'where',
        having: 'where', limit: 'limit', offset: 'limit', set: 'set', into: 'into', update: 'update',
        values: 'values', union: 'start', intersect: 'start', except: 'start', with: 'start', insert: 'start',
        delete: 'start', create: 'start', drop: 'start', alter: 'start',
    }
    var NOT_ALIAS = set('where join inner left right full outer cross natural on using group order having limit ' +
        'offset union intersect except set values returning window select from as lateral fetch for')
    var TABLE_KW = set('from join into update only lateral table')
    var STARTERS = ['SELECT', 'WITH', 'INSERT INTO', 'UPDATE', 'DELETE FROM', 'CREATE TABLE', 'CREATE INDEX',
        'DROP TABLE', 'ALTER TABLE', 'EXPLAIN', 'PRAGMA', 'BEGIN', 'COMMIT']
    var FOLLOW = set('where join left inner on group order limit as union using from asc desc')
    // After these words an operand (column, expression) is expected, after any other word, name or value it is not.
    var OPERAND_KW = set('select distinct all and or not where on by when then else case in like ilike is between ' +
        'having set limit offset exists any some using returning values')

    function set(s) {
        var r = {}
        s.split(' ').forEach(function (w) {
            r[w] = true
        })
        return r
    }

    function tokenize(text) {
        var toks = [], m
        TOKEN.lastIndex = 0
        while (TOKEN.lastIndex < text.length && (m = TOKEN.exec(text))) {
            for (var i = 2; i < 10; i++) {
                if (m[i] !== undefined) {
                    toks.push({k: KINDS[i], v: m[0], s: m.index, e: m.index + m[0].length})
                    break
                }
            }
        }
        return toks
    }

    function closed(t) {
        if (t.k === 'c') return t.v[1] === '-' || (t.v.length >= 4 && t.v.slice(-2) === '*/')
        return t.v.length >= 2 && t.v[t.v.length - 1] === t.v[0]
    }

    function unq(v) {
        var q = v[0]
        if (q !== '`' && q !== '"') return v
        v = v.slice(1)
        if (v[v.length - 1] === q) v = v.slice(0, -1)
        return v.split(q + q).join(q)
    }

    function key(v) {
        return unq(v).toLowerCase()
    }

    function isId(t) {
        return !!t && (t.k === 'w' || t.k === 'q')
    }

    function is(t, v) {
        return !!t && t.k === 'p' && t.v === v
    }

    function lc(t) {
        return t.k === 'w' ? t.v.toLowerCase() : ''
    }

    /** Index of the parenthesis closing the one at code[i], code.length if unbalanced. */
    function close(code, i) {
        for (var d = 0; i < code.length; i++) {
            if (is(code[i], '(')) d++
            else if (is(code[i], ')') && --d === 0) return i
        }
        return code.length
    }

    /** Output column names (as typed) of the select list of code[a..b], used for CTEs and derived tables. */
    function selectCols(code, a, b) {
        var cols = [], item = [], d = 0, inSel = false
        var skip = set('distinct end null true false')

        function flush() {
            var l = item[item.length - 1]
            if (isId(l) && !skip[lc(l)] && cols.indexOf(l.v) < 0) cols.push(l.v)
            item = []
        }

        for (var i = a + 1; i < b; i++) {
            var t = code[i]
            if (is(t, '(')) d++
            else if (is(t, ')')) d--

            if (d === 0 && t.k === 'w') {
                var w = lc(t)
                if (!inSel) {
                    if (w === 'select') inSel = true
                    continue
                }
                if (w === 'from' || w === 'union') break
            }

            if (!inSel) continue
            if (d === 0 && is(t, ',')) {
                flush()
                continue
            }
            item.push(t)
        }
        flush()
        return cols
    }

    /**
     * @param {Array} items dbcon completion items of one instance
     */
    function buildSchema(items) {
        var schema = {tables: {}, statics: [], keywords: [], funcs: [], directives: []}

        function table(name) {
            var k = key(name)
            return schema.tables[k] || (schema.tables[k] = {name: name, cols: []})
        }

        items.forEach(function (it) {
            if (it.table && it.column) table(it.table).cols.push(it.column)
            else if (it.table) table(it.table)
            else if (it.value.slice(0, 3) === '-- ') schema.directives.push(it)
            else if (/func/i.test(it.meta || '')) schema.funcs.push(it)
            else schema.keywords.push(it)
        })

        return schema
    }

    /**
     * Finds what is around the cursor: the fragment to replace (prefix), the clause, and the tables in scope.
     */
    function analyze(text, offset, schema) {
        var res = {prefix: '', clause: 'start', refs: [], ctes: {}, qual: null, expectTable: false}
        var all = tokenize(text), i, t
        var lo = 0, hi = all.length

        // Current statement.
        for (i = 0; i < all.length; i++) {
            if (is(all[i], ';')) {
                if (all[i].e <= offset) lo = i + 1
                else {
                    hi = i
                    break
                }
            }
        }

        var cur = null
        for (i = lo; i < hi; i++) {
            if (all[i].s < offset && offset <= all[i].e) {
                cur = all[i]
                break
            }
        }

        if (cur) {
            if (cur.k === 'c') {
                if (cur.v[1] === '-') {
                    res.inComment = true
                    res.prefix = text.slice(cur.s, offset)
                } else if (offset < cur.e || !closed(cur)) res.none = true
                return res
            }
            if (cur.k === 'n' || (cur.k === 's' && (offset < cur.e || !closed(cur)))) {
                res.none = true
                return res
            }
        }

        // Code tokens with parenthesis groups.
        var code = [], groups = [{parent: -1}], stack = [0], gidByOpen = {}, curGroup = 0
        for (i = lo; i < hi; i++) {
            t = all[i]
            if (t.k === 'c') continue
            if (is(t, ')') && stack.length > 1) stack.pop()
            t.g = stack[stack.length - 1]
            code.push(t)
            if (is(t, '(')) {
                groups.push({parent: t.g})
                gidByOpen[code.length - 1] = groups.length - 1
                stack.push(groups.length - 1)
            }
            if (t.e <= offset) curGroup = stack[stack.length - 1]
        }

        var frag = cur && (cur.k === 'w' || (cur.k === 'q' && (offset < cur.e || !closed(cur)))) ? cur : null
        var fragStart = frag ? frag.s : offset
        var fragText = frag ? text.slice(frag.s, offset) : ''
        var before = code.filter(function (t) {
            return t.e <= fragStart
        })
        var prev = before[before.length - 1]

        // Tables in scope.
        function parseRef(j, g) {
            var t = code[j], ref = {g: g, cols: null}
            if (t && (lc(t) === 'only' || lc(t) === 'lateral')) t = code[++j]
            if (is(t, '(')) {
                var cl = close(code, j)
                ref.cols = selectCols(code, j, cl)
                j = cl + 1
            } else if (isId(t) && !(t.k === 'w' && CLAUSES[lc(t)])) {
                ref.name = key(t.v)
                ref.raw = t.v
                while (is(code[j + 1], '.') && isId(code[j + 2])) {
                    j += 2
                    ref.name = key(code[j].v)
                    ref.raw = code[j].v
                }
                j++
                var cte = res.ctes[ref.name], tb = schema.tables[ref.name]
                if (cte) ref.cols = cte
                else if (tb) {
                    ref.cols = tb.cols
                    ref.raw = tb.name
                }
            } else return null

            var a = code[j], alias = null
            if (a && lc(a) === 'as') {
                alias = code[j + 1]
                j += 2
            } else if (a && ((a.k === 'w' && !NOT_ALIAS[lc(a)] && !CLAUSES[lc(a)]) || a.k === 'q')) {
                alias = a
                j++
            }
            if (isId(alias)) {
                ref.alias = key(alias.v)
                ref.aliasRaw = alias.v
            } else {
                ref.alias = ref.name
                ref.aliasRaw = ref.raw
            }
            ref.meta = ref.name || ref.alias

            return {ref: ref, next: j}
        }

        var refs = []
        for (i = 0; i < code.length; i++) {
            t = code[i]
            if (t.k !== 'w') continue
            var w = lc(t), j

            if (w === 'with') {
                // WITH [RECURSIVE] name [(cols)] AS (...) [, ...]
                j = i + 1
                if (lc(code[j]) === 'recursive') j++
                while (isId(code[j])) {
                    var name = key(code[j].v), cols = null
                    j++
                    if (is(code[j], '(')) {
                        var cc = close(code, j)
                        cols = code.slice(j + 1, cc).filter(isId).map(function (t) {
                            return t.v
                        })
                        j = cc + 1
                    }
                    if (lc(code[j]) !== 'as') break
                    j++
                    if (lc(code[j]) === 'not') j++
                    if (lc(code[j]) === 'materialized') j++
                    if (!is(code[j], '(')) break
                    var end = close(code, j)
                    res.ctes[name] = cols || selectCols(code, j, end)
                    j = end + 1
                    if (!is(code[j], ',')) break
                    j++
                }
            } else if (w === 'from' || w === 'join' || w === 'update' || w === 'into') {
                j = i + 1
                for (; ;) {
                    var r = parseRef(j, t.g)
                    if (!r) break
                    refs.push(r.ref)
                    j = r.next
                    if (w === 'from' && is(code[j], ',')) {
                        j++
                        continue
                    }
                    break
                }
            }
        }

        function visible(g) {
            for (var x = curGroup; x !== -1; x = groups[x].parent) if (x === g) return true
            return false
        }

        res.refs = refs.filter(function (r) {
            return visible(r.g)
        })

        // Clause of the cursor group.
        var cl = {0: 'start'}, by = {}
        for (i = 0; i < before.length; i++) {
            t = before[i]
            if (is(t, '(')) {
                var p = before[i - 1]
                cl[gidByOpen[i]] = cl[t.g] === 'into' && isId(p) ? 'insertcols' : 'paren'
            } else if (t.k === 'w') {
                var l = lc(t)
                if (CLAUSES[l]) cl[t.g] = CLAUSES[l]
                else if (l === 'group' || l === 'order') by[t.g] = l + 'by'
                else if (l === 'by' && by[t.g]) cl[t.g] = by[t.g]
            }
        }
        res.clause = cl[curGroup] || 'start'
        res.operand = expectsOperand(before)
        res.afterAs = !!prev && lc(prev) === 'as'
        res.expectTable = !prev || is(prev, ',') || (prev.k === 'w' && !!TABLE_KW[lc(prev)])

        // Qualifier: `alias.<cursor>`.
        var dot = is(prev, '.') && prev.e === fragStart
        var qt = before[before.length - 2]
        if (dot && isId(qt) && qt.e === prev.s) {
            var q = key(qt.v)
            res.refs.forEach(function (r) {
                if (!res.qual && (r.alias === q || r.name === q)) res.qual = r
            })
            var tb = schema.tables[q]
            if (!res.qual && tb) res.qual = {cols: tb.cols, meta: tb.name}
        }

        if (res.qual || !dot) {
            res.prefix = fragText
        } else {
            // Not a known qualifier: `click.Track` is a search fragment of a (wide table, JSON path like) name.
            var start = fragStart, k = before.length - 1
            while (k >= 1 && is(before[k], '.') && before[k].e === start && isId(before[k - 1]) && before[k - 1].e === before[k].s) {
                start = before[k - 1].s
                k -= 2
            }
            res.prefix = text.slice(start, offset)
        }

        return res
    }

    /** Whether the cursor follows something that wants a column or an expression next, e.g. a comma or operator. */
    function expectsOperand(before) {
        var prev = before[before.length - 1]
        if (!prev) return true
        if (prev.k === 'w') return !!OPERAND_KW[lc(prev)]
        if (prev.k !== 'p') return false
        if (prev.v === ',' || prev.v === '(' || prev.v === '.') return true
        if (prev.v === '*') {
            // `SELECT *` is complete, `a * ` is not.
            var pp = before[before.length - 2]
            return !(!pp || is(pp, ',') || is(pp, '.') || is(pp, '(') || lc(pp) === 'select' || lc(pp) === 'distinct')
        }

        return '=<>+-/%|&!~^:'.indexOf(prev.v) >= 0
    }

    function item(value, meta, score, caption) {
        return {value: value, caption: caption || value, meta: meta, score: score}
    }

    function copy(it, score) {
        return {value: it.value, caption: it.caption, meta: it.meta, score: score === undefined ? it.score : score}
    }

    /**
     * @returns {{prefix: string, items: Array}} text before the cursor to replace, and ranked candidates
     */
    function suggest(text, offset, schema, prefixOnly) {
        var c = analyze(text, offset, schema), out = {prefix: c.prefix, items: []}
        if (c.none || prefixOnly) return out

        if (c.inComment) {
            out.items = schema.directives.map(function (it) {
                return copy(it, 1000)
            })
            return out
        }

        if (c.afterAs && !c.qual) return out // An alias name is up to the user.

        var items = out.items
        var kws = function () {
            schema.keywords.forEach(function (it) {
                items.push(copy(it, FOLLOW[it.value.toLowerCase()] ? 2000 : 1000))
            })
        }
        var funcs = function () {
            schema.funcs.forEach(function (it) {
                items.push(copy(it, 2000))
            })
        }
        var tables = function (score) {
            Object.keys(schema.tables).forEach(function (k) {
                items.push(item(schema.tables[k].name, 'table', score))
            })
            Object.keys(c.ctes).forEach(function (k) {
                items.push(item(k, 'cte', score))
            })
        }

        if (c.qual) {
            (c.qual.cols || []).forEach(function (col) {
                items.push(item(col, c.qual.meta, 30000))
            })
            return out
        }

        switch (c.clause) {
            case 'start':
                STARTERS.forEach(function (s) {
                    items.push(item(s, 'statement', 5000))
                })
                break
            case 'from':
            case 'join':
            case 'into':
            case 'update':
                if (c.expectTable) tables(30000)
                else kws()
                break
            case 'insertcols':
                if (!c.operand) break
                c.refs.forEach(function (r) {
                    (r.cols || []).forEach(function (col) {
                        items.push(item(col, r.meta, 30000))
                    })
                })
                break
            default: {
                if (!c.operand) {
                    // Previous item is complete, a comma is needed before the next one.
                    kws()
                    break
                }

                if (!c.refs.length) {
                    // Nothing known about the tables (e.g. SELECT before FROM), offer all columns.
                    Object.keys(schema.tables).forEach(function (k) {
                        var tb = schema.tables[k]
                        tb.cols.forEach(function (col) {
                            items.push(item(col, tb.name, 20000))
                        })
                    })
                    tables(10000)
                } else {
                    var count = {}
                    c.refs.forEach(function (r) {
                        (r.cols || []).forEach(function (col) {
                            count[col] = (count[col] || 0) + 1
                        })
                    })
                    c.refs.forEach(function (r) {
                        (r.cols || []).forEach(function (col) {
                            var qualify = c.clause === 'on' || count[col] > 1
                            items.push(item(qualify ? r.aliasRaw + '.' + col : col, r.meta, 30000))
                        })
                    })
                }
                funcs()
                kws()
            }
        }

        return out
    }

    var api = {tokenize: tokenize, buildSchema: buildSchema, analyze: analyze, suggest: suggest}
    if (typeof module !== 'undefined' && module.exports) module.exports = api
    else root.SQLComplete = api
})(this)
