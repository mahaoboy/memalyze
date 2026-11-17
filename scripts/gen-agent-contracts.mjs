#!/usr/bin/env node
// Render docs/agent-contract.md into every harness instruction file.
//
// The shared block lives between the memalyze:contract markers; everything outside them
// stays hand-written (frontmatter, setup, DSH-specific notes). `--check` writes nothing and
// exits 1 when a file is out of date, so CI catches drift between harnesses.

import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const SOURCE = 'docs/agent-contract.md'
const BEGIN = '<!-- memalyze:contract:begin -->'
const END = '<!-- memalyze:contract:end -->'

const TOOL_SETS = {
  bare: {
    memory_search: 'memory_search',
    memory_add: 'memory_add',
    memory_delete: 'memory_delete',
  },
  dsh: {
    memory_search: 'mcp__memalyze__memory_search',
    memory_add: 'mcp__memalyze__memory_add',
    memory_delete: 'mcp__memalyze__memory_delete',
  },
}

const TARGETS = [
  { file: 'examples/claude-code/CLAUDE.md', tools: 'bare' },
  { file: 'examples/cline/.clinerules', tools: 'bare' },
  { file: 'examples/custom-harness/system-prompt.md', tools: 'bare' },
  { file: 'integrations/dsh/SKILL.md', tools: 'dsh' },
  { file: 'skills/memalyze/SKILL.md', tools: 'bare' },
]

function renderContract(toolSet) {
  const names = TOOL_SETS[toolSet]
  if (names === undefined) throw new Error(`unknown tool set: ${toolSet}`)
  let text = readFileSync(join(root, SOURCE), 'utf8')
  // Drop the leading editing note; it is for maintainers, not agents.
  text = text.replace(/^<!--[\s\S]*?-->\n?/, '')
  text = text.replace(/\{\{(memory_search|memory_add|memory_delete)\}\}/g, (_, key) => names[key])
  return text.trim()
}

function replaceBlock(source, block, file) {
  const start = source.indexOf(BEGIN)
  const end = source.indexOf(END)
  if (start === -1 || end === -1 || end < start) {
    throw new Error(`${file}: missing ${BEGIN} / ${END} markers`)
  }
  return `${source.slice(0, start + BEGIN.length)}\n${block}\n${source.slice(end)}`
}

function main() {
  const check = process.argv.includes('--check')
  const stale = []
  for (const target of TARGETS) {
    const path = join(root, target.file)
    const before = readFileSync(path, 'utf8')
    const after = replaceBlock(before, renderContract(target.tools), target.file)
    if (after === before) continue
    stale.push(relative(root, path))
    if (!check) writeFileSync(path, after)
  }

  if (check) {
    if (stale.length > 0) {
      console.error('agent contracts out of date (run `make contracts`):')
      for (const file of stale) console.error(`  ${file}`)
      process.exitCode = 1
      return
    }
    console.log(`agent contracts ok (${TARGETS.length} files)`)
    return
  }

  for (const file of stale) console.log(`updated ${file}`)
  console.log(`agent contracts rendered from ${SOURCE} (${TARGETS.length} files)`)
}

main()
