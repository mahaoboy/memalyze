#!/usr/bin/env python3
"""Generate deploy/registry/server.json from the built .mcpb bundles.

The registry requires a SHA-256 for every MCPB package. Typing four hashes by hand is
how you ship a metadata file that points at the wrong bytes, so this script derives
them from dist/ and writes the JSON in one shot.

    python3 scripts/gen-server-json.py --version 0.1.0

Re-run it whenever the bundles are rebuilt.
"""

import argparse
import hashlib
import json
import pathlib
import sys

# One bundle per platform; the MCPB manifest has no "pick the right binary" field.
TARGETS = [
    ("darwin", "arm64"),
    ("darwin", "amd64"),
    ("windows", "amd64"),
    ("linux", "amd64"),
]

SCHEMA = "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--version", required=True, help="release version, e.g. 0.1.0")
    ap.add_argument("--repo", default="mahaoboy/memalyze", help="GitHub owner/repo")
    ap.add_argument("--dist", default="dist", help="directory holding the .mcpb files")
    ap.add_argument("--out", default="deploy/registry/server.json")
    ap.add_argument(
        "--check",
        action="store_true",
        help="verify the existing file matches dist/ instead of rewriting it",
    )
    args = ap.parse_args()

    version = args.version[1:] if args.version.startswith("v") else args.version
    base = f"https://github.com/{args.repo}/releases/download/v{version}"
    dist = pathlib.Path(args.dist)

    packages = []
    for os_name, arch in TARGETS:
        bundle = dist / f"memalyze-{version}-{os_name}-{arch}.mcpb"
        if not bundle.is_file():
            print(f"error: missing {bundle}", file=sys.stderr)
            return 1
        digest = hashlib.sha256(bundle.read_bytes()).hexdigest()
        packages.append(
            {
                "registryType": "mcpb",
                "identifier": f"{base}/{bundle.name}",
                "fileSha256": digest,
                "transport": {"type": "stdio"},
            }
        )

    doc = {
        "$schema": SCHEMA,
        "name": f"io.github.{args.repo.split('/')[0]}/memalyze",
        "title": "memalyze",
        "description": (
            "Self-hosted agent memory: redaction before write, append-only audit, "
            "local embeddings."
        ),
        "version": version,
        "repository": {"url": f"https://github.com/{args.repo}", "source": "github"},
        "websiteUrl": f"https://github.com/{args.repo}",
        "packages": packages,
    }

    out = pathlib.Path(args.out)
    if args.check:
        if not out.is_file():
            print(f"error: {out} does not exist", file=sys.stderr)
            return 1
        current = json.loads(out.read_text(encoding="utf-8"))
        have = {p["identifier"]: p["fileSha256"] for p in current.get("packages", [])}
        want = {p["identifier"]: p["fileSha256"] for p in packages}
        if have != want or current.get("version") != version:
            print("error: server.json does not match dist/ (rebuild it)", file=sys.stderr)
            for key in sorted(set(have) | set(want)):
                if have.get(key) != want.get(key):
                    print(f"  {key}\n    file: {have.get(key)}\n    dist: {want.get(key)}", file=sys.stderr)
            return 1
        print(f"{out} matches dist/ ({len(packages)} packages, version {version})")
        return 0

    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(doc, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"wrote {out} ({len(packages)} packages, version {version})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
