import assert from 'node:assert/strict'
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { apply } from '../index.js'

const home = mkdtempSync(join(tmpdir(), 'memalyze-plugin-'))
const registered = []
const ctx = {
  get(name) {
    if (name !== 'skills') return undefined
    return {
      register(skill) {
        registered.push(skill)
        return () => {}
      },
    }
  },
  effect(callback) {
    return callback()
  },
}

try {
  apply(ctx, { home })

  const binary = join(
    home,
    '.memalyze',
    'bin',
    process.platform === 'win32' ? 'memalyze.exe' : 'memalyze',
  )
  assert.ok(existsSync(binary), `binary not installed at ${binary}`)
  assert.ok(readFileSync(binary).length > 1_000_000, 'binary is suspiciously small')

  const marker = join(home, '.memalyze', 'bin', '.installed-version')
  assert.match(readFileSync(marker, 'utf8'), /^0\.2\.0$/m)

  assert.equal(registered.length, 1, 'expected exactly one skill registration')
  const skill = registered[0]
  assert.equal(skill.name, 'memalyze')
  assert.equal(skill.source, 'runtime', 'skill source is required by the loader')
  assert.equal(skill.provider, 'memalyze')
  assert.match(skill.description, /self-hosted memory/i)
  assert.match(skill.content, /mcp__memalyze__memory_search/)
  assert.match(skill.content, /Search first/)
  assert.ok(!skill.content.startsWith('---'), 'frontmatter leaked into the skill body')
  assert.doesNotMatch(skill.content, /claude mcp add/, 'Claude-specific setup leaked into DSH skill')

  console.log('setup plugin ok')
  console.log(`  binary: ${binary} (${readFileSync(binary).length} bytes)`)
  console.log(`  skill: ${skill.name} (${skill.content.length} chars)`)
} finally {
  rmSync(home, { recursive: true, force: true })
}
