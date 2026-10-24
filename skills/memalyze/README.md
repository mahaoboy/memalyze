# memalyze as an agent skill

`SKILL.md` is a drop-in skill for harnesses that support the `SKILL.md` convention
(frontmatter `name` + `description`, Markdown body). It does not wrap the MCP server — it
tells the agent **when** to call the four tools and **how to read their results**, which is
the part models get wrong.

## Install

```bash
# project-local (recommended: the skill travels with the repo)
mkdir -p .skills && cp -r skills/memalyze .skills/

# user-global
mkdir -p ~/.skills && cp -r skills/memalyze ~/.skills/
```

Then make sure the MCP server itself is registered (see the repo README):

```bash
claude mcp add --scope project memalyze -- memalyze
```

## Why a skill and not more tools

The failure mode is not "the tool is missing", it is "the model answered from its own
memory instead of calling `memory_search`". S13 measured this: a soft hint produced 0/3
tool calls, a MUST-level contract produced 3/3. Prompting, not API surface, is the lever
(D-017).

## What the skill deliberately omits

- No "summarize the whole memory store" instruction — that is a context bomb.
- No automatic `memory_add` on every turn — writes stay explicit.
- No instruction to reconstruct redacted values — they are gone by design.
