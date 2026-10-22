// A stand-in for Chrome used by cdp.test.mjs. It accepts Chrome's flags and
// speaks just enough of the DevTools protocol for cdp.mjs to connect:
// DevToolsActivePort, /json/version, /json/new and a minimal WebSocket.
// FAKE_BROWSER_PIDFILE, if set, receives the process ID so tests can check
// that the harness stopped it. FAKE_BROWSER_MODE selects the behaviour:
//   ready             start at once
//   delay:<ms>        start after a delay
//   crash             print an error and exit with code 3
//   hang              never become ready
//   drop:<n>          close the DevTools socket after answering n commands
//   fail-once:<file>  crash if <file> is missing (creating it), else start
import { createHash } from 'node:crypto'
import { appendFileSync, existsSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { join } from 'node:path'

const [mode, value] = (process.env.FAKE_BROWSER_MODE || 'ready').split(/:(.*)/s)
const profile = process.argv.find(arg => arg.startsWith('--user-data-dir='))?.slice('--user-data-dir='.length)
if (!profile) { console.error('fake browser: --user-data-dir is required'); process.exit(2) }
if (process.env.FAKE_BROWSER_PIDFILE) appendFileSync(process.env.FAKE_BROWSER_PIDFILE, `${process.pid}
`)

function crash() {
  console.error('fake browser: cannot open display (simulated startup crash)')
  process.exit(3)
}
if (mode === 'crash') crash()
if (mode === 'fail-once') {
  if (!existsSync(value)) { writeFileSync(value, 'failed once'); crash() }
}
if (mode === 'hang') setInterval(() => {}, 1 << 30)
else setTimeout(start, mode === 'delay' ? Number(value) : 0)

function start() {
  const server = createServer((req, res) => {
    const { port } = server.address()
    res.setHeader('Content-Type', 'application/json')
    if (req.url === '/json/version') return res.end(JSON.stringify({ Browser: 'FakeChrome/1.0' }))
    if (req.url.startsWith('/json/new')) return res.end(JSON.stringify({ webSocketDebuggerUrl: `ws://127.0.0.1:${port}/devtools/page/1` }))
    res.statusCode = 404
    res.end('{}')
  })
  server.on('upgrade', (req, socket) => {
    const accept = createHash('sha1').update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64')
    socket.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`)
    let buffer = Buffer.alloc(0)
    let answered = 0
    socket.on('data', chunk => {
      buffer = Buffer.concat([buffer, chunk])
      for (let frame = readFrame(buffer); frame; frame = readFrame(buffer)) {
        buffer = buffer.subarray(frame.size)
        if (frame.opcode === 8) return socket.end()
        if (frame.opcode !== 1) continue
        const { id, method } = JSON.parse(frame.text)
        if (mode === 'drop' && answered === Number(value)) return socket.destroy()
        const result = method === 'Runtime.evaluate' ? { result: { value: 2 } } : {}
        socket.write(textFrame(JSON.stringify({ id, result })))
        answered++
      }
    })
    socket.on('error', () => {})
  })
  server.listen(0, '127.0.0.1', () => {
    // Chrome writes this file once DevTools is listening.
    writeFileSync(join(profile, 'DevToolsActivePort'), `${server.address().port}\n/devtools/browser/fake\n`)
  })
}

// Client frames are masked; this handles the short text frames cdp.mjs sends.
function readFrame(buffer) {
  if (buffer.length < 2) return null
  const opcode = buffer[0] & 0x0f
  let length = buffer[1] & 0x7f
  let offset = 2
  if (length === 126) { if (buffer.length < 4) return null; length = buffer.readUInt16BE(2); offset = 4 }
  else if (length === 127) { if (buffer.length < 10) return null; length = Number(buffer.readBigUInt64BE(2)); offset = 10 }
  const masked = buffer[1] & 0x80
  const size = offset + (masked ? 4 : 0) + length
  if (buffer.length < size) return null
  const payload = Buffer.from(buffer.subarray(size - length, size))
  if (masked) for (let i = 0; i < payload.length; i++) payload[i] ^= buffer[offset + (i % 4)]
  return { opcode, text: payload.toString('utf8'), size }
}

function textFrame(text) {
  const payload = Buffer.from(text)
  const header = payload.length < 126 ? Buffer.from([0x81, payload.length]) : Buffer.from([0x81, 126, payload.length >> 8, payload.length & 0xff])
  return Buffer.concat([header, payload])
}
