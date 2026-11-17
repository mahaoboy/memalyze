#!/usr/bin/env node
// End-to-end stdio smoke test for the packaged memalyze binary.
//
// It deliberately points MEMALYZE_OLLAMA_URL at a closed port: this is the
// no-Ollama acceptance case that matters most for a plugin install. The test
// asserts that writes still succeed (embedding_failed) and that searches
// degrade to keyword within the 3s budget instead of hanging.

import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createInterface } from 'node:readline'
import { fileURLToPath } from 'node:url'
import { gunzipSync } from 'node:zlib'

const ASSETS = {
  'linux-x64': 'memalyze-linux-amd64',
  'linux-arm64': 'memalyze-linux-arm64',
  'darwin-x64': 'memalyze-darwin-amd64',
  'darwin-arm64': 'memalyze-darwin-arm64',
  'win32-x64': 'memalyze-windows-amd64.exe',
}

const work = mkdtempSync(join(tmpdir(), 'memalyze-smoke-'))
let binary = process.env.MEMALYZE_BIN
let decompressed
if (binary === undefined) {
  const key = `${process.platform}-${process.arch}`
  const asset = ASSETS[key]
  assert.ok(asset, `unsupported test platform ${key}`)
  const binRoot = fileURLToPath(new URL('../bin/', import.meta.url))
  const raw = join(binRoot, asset)
  const gz = `${raw}.gz`
  if (existsSync(raw)) {
    binary = raw
  } else {
    assert.ok(existsSync(gz), `missing packaged binary: ${raw} (or ${gz})`)
    decompressed = join(work, asset)
    writeFileSync(decompressed, gunzipSync(readFileSync(gz)))
    chmodSync(decompressed, 0o755)
    binary = decompressed
  }
}

const db = join(work, 'memalyze.db')
const child = spawn(binary, [], {
  stdio: ['pipe', 'pipe', 'pipe'],
  env: {
    ...process.env,
    MEMALYZE_DB: db,
    MEMALYZE_OLLAMA_URL: 'http://127.0.0.1:9',
    MEMALYZE_QUERY_TIMEOUT: '3s',
    MEMALYZE_AUDIT: 'on',
    MEMALYZE_REDACT_MODE: 'enforce',
    MEMALYZE_ACTOR: 'dsh-smoke',
  },
})

let stderr = ''
child.stderr.on('data', (chunk) => {
  stderr += chunk.toString()
})

const pending = new Map()
let nextId = 1
const lines = createInterface({ input: child.stdout })
lines.on('line', (line) => {
  if (line.trim() === '') return
  let message
  try {
    message = JSON.parse(line)
  } catch {
    return
  }
  if (message.id !== undefined && pending.has(message.id)) {
    const { resolve, reject, timer } = pending.get(message.id)
    pending.delete(message.id)
    clearTimeout(timer)
    if (message.error !== undefined) {
      reject(new Error(`${message.error.code}: ${message.error.message}`))
    } else {
      resolve(message.result)
    }
  }
})

child.on('exit', (code, signal) => {
  for (const { reject, timer } of pending.values()) {
    clearTimeout(timer)
    reject(new Error(`memalyze exited early (code=${code}, signal=${signal})\n${stderr}`))
  }
  pending.clear()
})

function request(method, params = {}, timeoutMs = 15000) {
  const id = nextId++
  const payload = JSON.stringify({ jsonrpc: '2.0', id, method, params })
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id)
      reject(new Error(`${method} timed out after ${timeoutMs}ms\n${stderr}`))
    }, timeoutMs)
    pending.set(id, { resolve, reject, timer })
    child.stdin.write(`${payload}\n`)
  })
}

function notify(method, params = {}) {
  child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', method, params })}\n`)
}

function toolResult(result) {
  if (result?.structuredContent !== undefined) return result.structuredContent
  const text = (result?.content ?? [])
    .filter((block) => block.type === 'text')
    .map((block) => block.text)
    .join('\n')
  try {
    return JSON.parse(text)
  } catch {
    return { text }
  }
}

async function call(name, args) {
  const result = await request('tools/call', { name, arguments: args })
  if (result?.isError === true) {
    throw new Error(`${name} returned isError: ${JSON.stringify(toolResult(result))}`)
  }
  return toolResult(result)
}

async function main() {
  const init = await request('initialize', {
    protocolVersion: '2025-06-18',
    capabilities: {},
    clientInfo: { name: 'memalyze-dsh-smoke', version: '0.2.0' },
  })
  assert.equal(init.serverInfo?.name !== undefined, true, 'initialize returned no serverInfo')
  notify('notifications/initialized')

  const list = await request('tools/list')
  const names = new Set((list.tools ?? []).map((tool) => tool.name))
  for (const name of ['memory_add', 'memory_search', 'memory_delete', 'echo']) {
    assert.ok(names.has(name), `missing tool ${name}; got ${[...names].join(', ')}`)
  }

  const stamp = Date.now().toString(36)
  const first = await call('memory_add', {
    content: `The launch code is ORANGE-42. smoke=${stamp}`,
    scope: 'project:dsh-smoke',
    kind: 'fact',
  })
  assert.ok(first.id, 'memory_add returned no id')
  assert.equal(first.duplicate, false)
  assert.equal(first.embedded, false, 'a closed Ollama port must not embed')
  assert.equal(first.embedding_failed, true, 'write should still succeed with embedding_failed')

  const duplicate = await call('memory_add', {
    content: `The launch code is ORANGE-42. smoke=${stamp}`,
    scope: 'project:dsh-smoke',
    kind: 'fact',
  })
  assert.equal(duplicate.duplicate, true, 'identical content should dedupe')
  assert.equal(duplicate.id, first.id)

  const started = Date.now()
  const search = await call('memory_search', {
    query: 'launch code',
    scope: 'project:dsh-smoke',
    limit: 5,
  })
  const searchMs = Date.now() - started
  assert.ok(searchMs < 5000, `search took ${searchMs}ms with Ollama down`)
  assert.equal(search.degraded, true, 'search should degrade without Ollama')
  assert.ok(search.hits.length >= 1, 'keyword search returned no hits')
  assert.ok(
    search.hits.some((hit) => hit.content.includes('ORANGE-42')),
    `keyword hit missing the stored value: ${JSON.stringify(search.hits)}`,
  )

  const globalAdd = await call('memory_add', {
    content: `Workspace-independent preference: use tabs in Go. smoke=${stamp}`,
    kind: 'preference',
  })
  const globalSearch = await call('memory_search', { query: 'tabs Go', limit: 5 })
  assert.ok(
    globalSearch.hits.some((hit) => hit.content.includes('use tabs in Go')),
    'default-scope memory was not searchable without a scope',
  )

  const redacted = await call('memory_add', {
    content: `Call 13800138000 about smoke=${stamp}`,
    scope: 'project:dsh-smoke',
    kind: 'note',
  })
  assert.equal(redacted.redacted, true, 'phone number should be redacted')
  const redactedSearch = await call('memory_search', {
    query: `smoke=${stamp}`,
    scope: 'project:dsh-smoke',
    limit: 10,
  })
  const redactedHit = redactedSearch.hits.find((hit) => hit.content.includes(`smoke=${stamp}`))
  assert.ok(redactedHit, 'redacted memory not found')
  assert.ok(!redactedHit.content.includes('13800138000'), 'original phone number leaked into storage')
  assert.match(redactedHit.content, /\[REDACTED:phone\]/)

  const deleted = await call('memory_delete', { id: first.id })
  assert.equal(deleted.deleted, true)
  const afterDelete = await call('memory_search', {
    query: 'launch code',
    scope: 'project:dsh-smoke',
    limit: 5,
  })
  assert.ok(
    !afterDelete.hits.some((hit) => hit.id === first.id),
    'deleted memory still appears in search',
  )

  child.stdin.end()
  await new Promise((resolve) => {
    const timer = setTimeout(() => {
      child.kill('SIGTERM')
      resolve()
    }, 3000)
    child.once('exit', () => {
      clearTimeout(timer)
      resolve()
    })
  })

  const audit = execFileSync(
    binary,
    ['audit', 'export', '--db', db, '--format', 'json'],
    { encoding: 'utf8' },
  )
  assert.match(audit, /"action"/)
  assert.match(audit, /"search"/)
  assert.match(audit, /"add"/)

  console.log('mcp smoke ok')
  console.log(`  tools: ${[...names].sort().join(', ')}`)
  console.log(`  search: ${searchMs}ms, degraded=${search.degraded}, hits=${search.hits.length}`)
  console.log(`  db: ${db}`)
}

try {
  await main()
} catch (error) {
  console.error('mcp smoke failed')
  console.error(stderr)
  console.error(error.stack ?? error)
  process.exitCode = 1
} finally {
  if (child.exitCode === null) child.kill('SIGTERM')
  rmSync(work, { recursive: true, force: true })
}
