// Run with: node testdata/completer_test.js (also from `go test`, when node is available).
const assert = require('assert')
const C = require('../static/completer.js')

const col = (table, column) => ({value: '`' + column + '`', table: '`' + table + '`', column: '`' + column + '`', meta: table, score: 20000})
const schema = C.buildSchema([
    {value: 'users', table: 'users', meta: 'table'}, col('users', 'id'), col('users', 'name'),
    {value: 'orders', table: 'orders', meta: 'table'}, col('orders', 'id'), col('orders', 'user_id'), col('orders', 'total'),
    {value: 'events', table: 'events', meta: 'table'}, col('events', '.context.click.CallbackData.Tracker'), col('events', '.context.click.Url'),
    {value: 'SELECT', meta: 'keyword', score: 1000}, {value: 'FROM', meta: 'keyword', score: 1000}, {value: 'WHERE', meta: 'keyword', score: 1000}, {value: 'count(', meta: 'function', score: 1000},
    {value: '-- histogram', meta: 'directive', score: 1000}, {value: '-- plot:pie', meta: 'directive', score: 1000},
])

// `|` marks the cursor.
function at(sql) {
    const offset = sql.indexOf('|')
    const r = C.suggest(sql.replace('|', ''), offset, schema)
    r.values = r.items.map(i => i.value)
    return r
}

let r

r = at('SELECT | FROM users u')
assert.deepStrictEqual(r.values.filter(v => v[0] === '`'), ['`id`', '`name`'])
assert(!r.values.includes('users'))

r = at('SELECT u.| FROM users u JOIN orders o ON o.user_id = u.id')
assert.strictEqual(r.prefix, '')
assert.deepStrictEqual(r.values, ['`id`', '`name`'])

r = at('SELECT u.na| FROM users u')
assert.strictEqual(r.prefix, 'na')

r = at('SELECT | FROM users u JOIN orders o ON 1')
assert(r.values.includes('u.`id`') && r.values.includes('o.`id`') && r.values.includes('`total`') && !r.values.includes('`id`'))

r = at('SELECT * FROM |')
assert(r.values.includes('users') && r.values.includes('orders') && !r.values.includes('`id`'))
r = at('SELECT * FROM users u JOIN |')
assert(r.values.includes('orders'))
r = at('SELECT * FROM users |')
assert(r.values.includes('WHERE') && !r.values.includes('orders'))

r = at('SELECT * FROM users u JOIN orders o ON |')
assert(r.values.includes('u.`id`') && r.values.includes('o.`user_id`'))

r = at('WITH x AS (SELECT id AS uid, name FROM users) SELECT | FROM x')
assert.deepStrictEqual(r.values.filter(v => !/^[A-Z]+$|\($/.test(v)), ['uid', 'name'])

r = at('SELECT * FROM events WHERE click.Tr|')
assert.strictEqual(r.prefix, 'click.Tr')
assert(r.values.includes('`.context.click.CallbackData.Tracker`'))

r = at('-- hist|')
assert.strictEqual(r.prefix, '-- hist')
assert.deepStrictEqual(r.values, ['-- histogram', '-- plot:pie'])

assert.strictEqual(at("SELECT 'abc|").items.length, 0)

r = at('SELECT 1; SELECT | FROM orders')
assert(r.values.includes('`total`') && !r.values.includes('`name`'))

r = at('SELECT * FROM users u WHERE id IN (SELECT user_id FROM orders o WHERE o.|)')
assert.deepStrictEqual(r.values, ['`id`', '`user_id`', '`total`'])

r = at('INSERT INTO users (|')
assert.deepStrictEqual(r.values, ['`id`', '`name`'])

r = at('|')
assert(r.values.includes('SELECT'))

r = at('select * from `users` as "u" where u.|')
assert.deepStrictEqual(r.values, ['`id`', '`name`'])

// Comma awareness: a complete item is followed by keywords, not by columns.
const cols = v => v.filter(x => x[0] === '`')
r = at('SELECT id |')
assert.strictEqual(cols(r.values).length, 0)
assert(r.values.includes('FROM') && r.values.includes('WHERE'))
assert(cols(at('SELECT id, |').values).length > 0)
assert.strictEqual(cols(at('SELECT * | FROM users').values).length, 0)
assert(cols(at('SELECT 2 * | FROM users').values).length > 0)
assert.strictEqual(cols(at('SELECT * FROM users GROUP BY id |').values).length, 0)
assert(cols(at('SELECT * FROM users GROUP BY id, |').values).length > 0)
assert(cols(at('SELECT * FROM users ORDER BY |').values).length > 0)
assert(cols(at('SELECT * FROM users WHERE id = |').values).length > 0)
assert.strictEqual(cols(at('SELECT * FROM users WHERE id = 1 |').values).length, 0)
assert(cols(at('SELECT * FROM users WHERE id = 1 AND |').values).length > 0)
assert(cols(at('SELECT count(|) FROM users').values).length > 0)
assert.strictEqual(cols(at('SELECT count(*) | FROM users').values).length, 0)
assert.strictEqual(at('SELECT id AS |').items.length, 0)
assert.strictEqual(cols(at('INSERT INTO users (id |').values).length, 0)
assert(cols(at('INSERT INTO users (id, |').values).length > 0)
assert(cols(at('SELECT u.| FROM users u').values).length > 0)

console.log('ok')
