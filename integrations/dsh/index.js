// dsh-plugin-memalyze — setup half.
//
// A DSH bundle patch can only insert plugin rows; it cannot run shell commands.
// This plugin does the two things a user would otherwise do by hand:
//   1. install the matching static memalyze binary under ~/.memalyze/bin;
//   2. register the memalyze skill with ctx.skills.
// The MCP row in cordis.patch.yml starts `~/.memalyze/bin/memalyze` on stdio.
//
// The copy is synchronous on purpose: it runs before the MCP row spawns the
// child process, and it must not depend on a shell PATH (GUI DSH has none).

import {
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import { homedir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gunzipSync } from 'node:zlib'

/** Cordis plugin name; keep stable after publishing. */
export const name = 'memalyze-setup'

const PACKAGE = JSON.parse(
  readFileSync(new URL('./package.json', import.meta.url), 'utf8'),
)

/** npm tarball asset name for each supported Node platform/arch pair. */
const PLATFORM_ASSETS = {
  'linux-x64': 'memalyze-linux-amd64',
  'linux-arm64': 'memalyze-linux-arm64',
  'darwin-x64': 'memalyze-darwin-amd64',
  'darwin-arm64': 'memalyze-darwin-arm64',
  'win32-x64': 'memalyze-windows-amd64.exe',
}

const SUPPORTED = Object.keys(PLATFORM_ASSETS).join(', ')

function isLocked(error) {
  return error && ['EBUSY', 'EPERM', 'EACCES'].includes(error.code)
}

/**
 * Install the packaged binary into `<home>/.memalyze/bin`.
 * @param {string} home - user home directory (injectable for tests).
 * @returns {string} absolute path to the executable.
 */
function ensureBinary(home) {
  const root = join(home, '.memalyze')
  const binDir = join(root, 'bin')
  const binaryName = process.platform === 'win32' ? 'memalyze.exe' : 'memalyze'
  const dest = join(binDir, binaryName)
  const marker = join(binDir, '.installed-version')

  mkdirSync(binDir, { recursive: true, mode: 0o700 })
  if (process.platform !== 'win32') {
    for (const dir of [root, binDir]) {
      try {
        chmodSync(dir, 0o700)
      } catch {
        // A shared/read-only home is the user's choice; the binary still works.
      }
    }
  }

  let installed = ''
  try {
    installed = readFileSync(marker, 'utf8').trim()
  } catch {
    // First run.
  }
  if (installed === PACKAGE.version && existsSync(dest) && statSync(dest).size > 0) {
    return dest
  }

  const key = `${process.platform}-${process.arch}`
  const asset = PLATFORM_ASSETS[key]
  if (asset === undefined) {
    throw new Error(
      `[memalyze] unsupported platform ${key}; this package ships ${SUPPORTED}. ` +
        'Build memalyze from source or run DSH under one of those platforms.',
    )
  }

  const binRoot = fileURLToPath(new URL('./bin/', import.meta.url))
  const raw = join(binRoot, asset)
  const gz = `${raw}.gz`
  let bytes
  if (existsSync(raw)) {
    bytes = readFileSync(raw)
  } else if (existsSync(gz)) {
    bytes = gunzipSync(readFileSync(gz))
  } else {
    throw new Error(
      `[memalyze] packaged binary missing: ${raw} (or ${gz}). ` +
        'Reinstall dsh-plugin-memalyze; a source checkout needs `make dsh-package` first.',
    )
  }

  const tmp = `${dest}.tmp-${process.pid}-${Date.now()}`
  writeFileSync(tmp, bytes, { mode: 0o755 })
  if (process.platform !== 'win32') {
    try {
      chmodSync(tmp, 0o755)
    } catch {
      // Windows and some filesystems do not expose POSIX modes.
    }
  }

  const backup = `${dest}.old-${process.pid}`
  let moved = false
  if (existsSync(dest)) {
    try {
      renameSync(dest, backup)
      moved = true
    } catch (error) {
      try {
        rmSync(tmp, { force: true })
      } catch {
        // Best effort.
      }
      if (isLocked(error)) {
        console.error(
          `[memalyze] cannot replace ${dest}: ${error.message}. ` +
            'Another DSH instance may be using it; close it and restart.',
        )
        return dest
      }
      throw error
    }
  }

  try {
    renameSync(tmp, dest)
  } catch (error) {
    if (moved) {
      try {
        renameSync(backup, dest)
      } catch {
        // The old binary is gone; the error below is the actionable one.
      }
    }
    try {
      rmSync(tmp, { force: true })
    } catch {
      // Best effort.
    }
    throw error
  }
  if (moved) {
    try {
      rmSync(backup, { force: true })
    } catch {
      // A stale backup does not affect the new binary.
    }
  }

  try {
    writeFileSync(marker, `${PACKAGE.version}\n`)
  } catch (error) {
    console.error(`[memalyze] installed ${dest} but could not write ${marker}: ${error.message}`)
  }
  return dest
}

/** Parse SKILL.md frontmatter; ctx.skills wants the body without it. */
function loadSkill() {
  const raw = readFileSync(new URL('./SKILL.md', import.meta.url), 'utf8')
  const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/.exec(raw)
  if (match === null) {
    return {
      name: 'memalyze',
      description: 'Persistent, self-hosted memory for agents via memalyze MCP.',
      content: raw,
    }
  }
  const field = (key, fallback) => {
    const found = new RegExp(`^${key}:\\s*(.+)$`, 'm').exec(match[1])
    return found === null ? fallback : found[1].trim().replace(/^["']|["']$/g, '')
  }
  return {
    name: field('name', 'memalyze'),
    description: field('description', 'Persistent, self-hosted memory for agents via memalyze MCP.'),
    content: match[2].trim(),
  }
}

/**
 * Register the runtime skill once the skill service is available. Row order is
 * not a load-order promise in DSH, so a short bounded retry is safer than
 * requiring the skill plugin or failing the setup plugin when it is absent.
 */
function registerSkill(ctx, skill) {
  let attempts = 0
  let timer

  const stop = () => {
    if (timer !== undefined) {
      clearTimeout(timer)
      timer = undefined
    }
  }

  const attempt = () => {
    attempts += 1
    let skills
    try {
      skills = ctx.get?.('skills')
    } catch {
      skills = undefined
    }
    if (skills !== undefined && typeof skills.register === 'function') {
      try {
        const registration = {
          name: skill.name,
          description: skill.description,
          content: skill.content,
          // SkillDefinition.source is required by the loader; `runtime` is the
          // registry's source bucket for in-process contributions.
          source: 'runtime',
          provider: 'memalyze',
        }
        if (typeof ctx.effect === 'function') {
          ctx.effect(() => skills.register(registration), 'memalyze: runtime skill')
        } else {
          skills.register(registration)
        }
        console.error(`[memalyze] skill registered: ${skill.name}`)
      } catch (error) {
        console.error(`[memalyze] skill registration failed: ${error.message}`)
      }
      return
    }
    if (attempts < 40) {
      timer = setTimeout(attempt, 250)
    } else {
      console.error('[memalyze] skill registry unavailable after 10s; MCP tools still work')
    }
  }

  attempt()
  if (typeof ctx.effect === 'function') {
    ctx.effect(() => stop, 'memalyze: skill retry timer')
  }
}

/**
 * Cordis entry point.
 * @param {import('@deepseek-ai/cordis').Context} ctx
 * @param {{ home?: string }} [config] - `home` is for tests only; the bundle
 *   patch deliberately fixes the production path at ~/.memalyze.
 */
export function apply(ctx, config = {}) {
  const home = config.home === undefined ? homedir() : resolve(config.home)
  const binary = ensureBinary(home)
  console.error(`[memalyze] binary ready: ${binary}`)
  registerSkill(ctx, loadSkill())
}
