#!/usr/bin/env python3
"""Generate deploy/registry/server.json from MCPB bundles.

Two sources:

    # local build (for the release you just packed yourself)
    python3 scripts/gen-server-json.py --version 0.1.0

    # published release (authoritative: uses GitHub's own asset digests)
    python3 scripts/gen-server-json.py --from-release v0.1.1

The registry requires a SHA-256 for every MCPB package. Typing four hashes by hand is
how you ship metadata that points at the wrong bytes, so this script derives them and
writes the JSON in one shot.

`--check` compares the existing file against the selected source instead of rewriting.
"""

import argparse
import hashlib
import json
import os
import pathlib
import sys
import urllib.request

# Preferred order for the desktop bundles. The MCPB manifest has no "pick the right
# binary" field, so one bundle per platform is the only honest packaging.
TARGETS = [
    ("darwin", "arm64"),
    ("darwin", "amd64"),
    ("windows", "amd64"),
    ("linux", "amd64"),
]

SCHEMA = "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json"


def api_get(url: str) -> dict:
    req = urllib.request.Request(url)
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("User-Agent", "memalyze-gen-server-json")
    token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)


def packages_from_local(dist: pathlib.Path, version: str, base: str) -> list[dict]:
    packages = []
    for os_name, arch in TARGETS:
        bundle = dist / f"memalyze-{version}-{os_name}-{arch}.mcpb"
        if not bundle.is_file():
            sys.exit(f"error: missing {bundle}")
        packages.append(
            {
                "registryType": "mcpb",
                "identifier": f"{base}/{bundle.name}",
                "fileSha256": hashlib.sha256(bundle.read_bytes()).hexdigest(),
                "transport": {"type": "stdio"},
            }
        )
    return packages


def packages_from_release(repo: str, tag: str) -> tuple[str, list[dict]]:
    """Read the published release and trust GitHub's asset digests."""
    release = api_get(f"https://api.github.com/repos/{repo}/releases/tags/{tag}")
    version = tag[1:] if tag.startswith("v") else tag

    by_name = {}
    for asset in release.get("assets", []):
        name = asset["name"]
        if not name.startswith("memalyze-") or not name.endswith(".mcpb"):
            continue
        digest = asset.get("digest") or ""
        if not digest.startswith("sha256:"):
            sys.exit(
                f"error: asset {name} has no sha256 digest; "
                "GitHub only exposes digests for newer uploads"
            )
        by_name[name] = (asset["browser_download_url"], digest.split(":", 1)[1])

    if not by_name:
        sys.exit(f"error: no .mcpb assets in release {tag}")

    order = {
        f"memalyze-{version}-{os_name}-{arch}.mcpb": i
        for i, (os_name, arch) in enumerate(TARGETS)
    }
    names = sorted(by_name, key=lambda n: (order.get(n, len(order)), n))
    packages = [
        {
            "registryType": "mcpb",
            "identifier": by_name[name][0],
            "fileSha256": by_name[name][1],
            "transport": {"type": "stdio"},
        }
        for name in names
    ]
    return version, packages


def build_doc(repo: str, version: str, packages: list[dict]) -> dict:
    return {
        "$schema": SCHEMA,
        "name": f"io.github.{repo.split('/')[0]}/memalyze",
        "title": "memalyze",
        "description": (
            "Self-hosted agent memory: redaction before write, append-only audit, "
            "local embeddings."
        ),
        "version": version,
        "repository": {"url": f"https://github.com/{repo}", "source": "github"},
        "websiteUrl": f"https://github.com/{repo}",
        "packages": packages,
    }


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--version", help="release version, e.g. 0.1.0 (local mode)")
    ap.add_argument("--from-release", metavar="TAG", help="derive hashes from a published release")
    ap.add_argument("--repo", default="mahaoboy/memalyze", help="GitHub owner/repo")
    ap.add_argument("--dist", default="dist", help="directory holding the .mcpb files")
    ap.add_argument("--out", default="deploy/registry/server.json")
    ap.add_argument(
        "--check",
        action="store_true",
        help="verify the existing file matches the source instead of rewriting it",
    )
    args = ap.parse_args()

    if args.from_release:
        if args.version:
            sys.exit("error: --version and --from-release are mutually exclusive")
        version, packages = packages_from_release(args.repo, args.from_release)
    else:
        if not args.version:
            sys.exit("error: --version is required unless --from-release is used")
        version = args.version[1:] if args.version.startswith("v") else args.version
        base = f"https://github.com/{args.repo}/releases/download/v{version}"
        packages = packages_from_local(pathlib.Path(args.dist), version, base)

    doc = build_doc(args.repo, version, packages)
    out = pathlib.Path(args.out)

    if args.check:
        if not out.is_file():
            sys.exit(f"error: {out} does not exist")
        current = json.loads(out.read_text(encoding="utf-8"))
        have = {p["identifier"]: p["fileSha256"] for p in current.get("packages", [])}
        want = {p["identifier"]: p["fileSha256"] for p in packages}
        if have != want or current.get("version") != version:
            print("error: server.json does not match the source (regenerate it)", file=sys.stderr)
            for key in sorted(set(have) | set(want)):
                if have.get(key) != want.get(key):
                    print(f"  {key}\n    file: {have.get(key)}\n    want: {want.get(key)}", file=sys.stderr)
            return 1
        print(f"{out} matches {len(packages)} packages, version {version}")
        return 0

    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(doc, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"wrote {out} ({len(packages)} packages, version {version})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
