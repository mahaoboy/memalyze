#!/usr/bin/env python3
"""Verify that the public MCP Registry serves the local server.json exactly."""

import json
import sys
import time
import urllib.parse
import urllib.request

REGISTRY = "https://registry.modelcontextprotocol.io"


def package_list(server):
    return sorted(server.get("packages", []), key=lambda p: p.get("identifier", ""))


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else "deploy/registry/server.json"
    with open(path, encoding="utf-8") as f:
        expected = json.load(f)
    query = urllib.parse.urlencode({"search": expected["name"].rsplit("/", 1)[-1]})
    url = f"{REGISTRY}/v0/servers?{query}"
    deadline = time.monotonic() + 90
    last = "not queried"
    while True:
        try:
            request = urllib.request.Request(
                url, headers={"Accept": "application/json", "User-Agent": "memalyze-registry-verify"}
            )
            with urllib.request.urlopen(request, timeout=20) as response:
                data = json.load(response)
            for item in data.get("servers", []):
                server = item.get("server", item)
                if server.get("name") != expected["name"]:
                    continue
                last = f"public version is {server.get('version')}"
                if server.get("version") != expected["version"]:
                    continue
                if package_list(server) != package_list(expected):
                    last = "matching version has different package metadata"
                    continue
                print(f"registry ok: {server['name']}@{server['version']} ({len(server['packages'])} packages)")
                return
        except Exception as exc:  # bounded retry for registry propagation/transient network errors
            last = f"{type(exc).__name__}: {exc}"
        if time.monotonic() >= deadline:
            raise SystemExit(f"registry verification failed for {expected['name']}@{expected['version']}: {last}")
        time.sleep(5)


if __name__ == "__main__":
    main()
