#!/usr/bin/env python3
"""Minimal Gmail API client for memalyze operations (standard library only)."""

import argparse
import base64
import json
import os
import secrets
import subprocess
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from email.message import EmailMessage
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CLIENT = ROOT / ".secrets" / "gmail" / "client_secret.json"
DEFAULT_TOKEN = ROOT / ".secrets" / "gmail" / "token.json"
SCOPES = [
    "https://www.googleapis.com/auth/gmail.send",
    "https://www.googleapis.com/auth/gmail.readonly",
]
TOKEN_URI = "https://oauth2.googleapis.com/token"
API_ROOT = "https://gmail.googleapis.com/gmail/v1/users/me"
REPLY_DOMAINS = [
    "dify.ai", "openwebui.com", "librechat.ai", "roomote.dev", "menlo.ai",
    "block.xyz", "langflow.org", "zed.dev", "cherry-ai.com", "chatboxai.com",
    "all-hands.dev", "flowiseai.com", "manufact.com", "docker.com",
    "cursor.com", "arcade.dev",
]


def write_private(path: Path, data: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
    os.chmod(temporary, 0o600)
    temporary.replace(path)
    os.chmod(path, 0o600)


def read_client(path: Path) -> dict:
    data = json.loads(path.read_text(encoding="utf-8"))
    client = data.get("installed") or data.get("web")
    if not client or not client.get("client_id") or not client.get("client_secret"):
        raise RuntimeError(f"Invalid OAuth client file: {path}")
    return client


def post_form(url: str, fields: dict) -> dict:
    request = urllib.request.Request(
        url,
        data=urllib.parse.urlencode(fields).encode(),
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        method="POST",
    )
    return http_json(request)


def http_json(request: urllib.request.Request) -> dict:
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            raw = response.read()
    except urllib.error.HTTPError as error:
        detail = error.read().decode("utf-8", errors="replace")[:2000]
        raise RuntimeError(f"HTTP {error.code}: {detail}") from error
    return json.loads(raw) if raw else {}


def open_cdp_tab(port: int, url: str) -> None:
    endpoint = f"http://127.0.0.1:{port}/json/new?{urllib.parse.quote(url, safe='')}"
    request = urllib.request.Request(endpoint, method="PUT")
    with urllib.request.urlopen(request, timeout=10) as response:
        result = json.load(response)
    if not result.get("id"):
        raise RuntimeError("Chrome did not return a tab ID")


def authenticate(args: argparse.Namespace) -> None:
    client = read_client(Path(args.client))
    state = secrets.token_urlsafe(24)
    server = HTTPServer(("127.0.0.1", 0), BaseHTTPRequestHandler)
    port = server.server_address[1]
    redirect_uri = f"http://127.0.0.1:{port}"
    result = {}

    class Callback(BaseHTTPRequestHandler):
        def do_GET(self):  # noqa: N802 - stdlib callback API
            query = urllib.parse.parse_qs(urllib.parse.urlparse(self.path).query)
            result.update({key: values[0] for key, values in query.items()})
            if result.get("state") != state:
                self.send_response(400)
                body = b"OAuth state mismatch. Return to the terminal."
            elif result.get("error"):
                self.send_response(400)
                body = b"Authorization was denied. Return to the terminal."
            else:
                self.send_response(200)
                body = b"Gmail authorization complete. You can close this tab."
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            threading.Thread(target=server.shutdown, daemon=True).start()

        def log_message(self, *_):
            pass

    # Replace the handler class after retaining the bound server address.
    server.RequestHandlerClass = Callback
    params = {
        "client_id": client["client_id"],
        "redirect_uri": redirect_uri,
        "response_type": "code",
        "scope": " ".join(SCOPES),
        "access_type": "offline",
        "prompt": "consent",
        "include_granted_scopes": "true",
        "state": state,
    }
    auth_url = "https://accounts.google.com/o/oauth2/v2/auth?" + urllib.parse.urlencode(params)
    print(f"OAuth callback: {redirect_uri}", flush=True)
    print(f"Authorization URL: {auth_url}", flush=True)
    if args.open_cdp:
        open_cdp_tab(args.open_cdp, auth_url)
        print(f"Opened authorization in Chrome CDP {args.open_cdp}", flush=True)
    server.timeout = args.timeout
    started = time.monotonic()
    while not result and time.monotonic() - started < args.timeout:
        server.handle_request()
    server.server_close()
    if not result:
        raise RuntimeError("Timed out waiting for OAuth callback")
    if result.get("state") != state:
        raise RuntimeError("OAuth state mismatch")
    if result.get("error"):
        raise RuntimeError(f"OAuth error: {result['error']}")
    if not result.get("code"):
        raise RuntimeError("OAuth callback did not contain a code")
    token = post_form(TOKEN_URI, {
        "client_id": client["client_id"],
        "client_secret": client["client_secret"],
        "code": result["code"],
        "redirect_uri": redirect_uri,
        "grant_type": "authorization_code",
    })
    token["obtained_at"] = int(time.time())
    token["redirect_uri"] = redirect_uri
    write_private(Path(args.token), token)
    if not token.get("refresh_token"):
        raise RuntimeError("No refresh token returned; reauthorize with prompt=consent")
    print(f"Authorized scopes: {token.get('scope', '')}")
    print(f"Saved token privately: {args.token}")


def load_access_token(client_path: Path, token_path: Path) -> str:
    token = json.loads(token_path.read_text(encoding="utf-8"))
    obtained = int(token.get("obtained_at", 0))
    expires = int(token.get("expires_in", 0))
    if token.get("access_token") and time.time() < obtained + expires - 120:
        return token["access_token"]
    client = read_client(client_path)
    refreshed = post_form(TOKEN_URI, {
        "client_id": client["client_id"],
        "client_secret": client["client_secret"],
        "refresh_token": token["refresh_token"],
        "grant_type": "refresh_token",
    })
    token.update(refreshed)
    token["obtained_at"] = int(time.time())
    write_private(token_path, token)
    return token["access_token"]


def api_request(args: argparse.Namespace, path: str, method: str = "GET", payload=None) -> dict:
    token = load_access_token(Path(args.client), Path(args.token))
    data = json.dumps(payload).encode() if payload is not None else None
    request = urllib.request.Request(
        API_ROOT + path,
        data=data,
        method=method,
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json; charset=utf-8",
        },
    )
    return http_json(request)


def profile(args: argparse.Namespace) -> None:
    result = api_request(args, "/profile")
    print(json.dumps(result, ensure_ascii=False, indent=2))


def send(args: argparse.Namespace) -> None:
    message = EmailMessage()
    message["To"] = args.to
    message["Subject"] = args.subject
    if args.sender:
        message["From"] = args.sender
    message.set_content(Path(args.body_file).read_text(encoding="utf-8"))
    raw = base64.urlsafe_b64encode(message.as_bytes()).decode().rstrip("=")
    payload = {"raw": raw}
    if args.thread_id:
        payload["threadId"] = args.thread_id
    result = api_request(args, "/messages/send", "POST", payload)
    print(json.dumps({
        "id": result.get("id"),
        "threadId": result.get("threadId"),
        "to": args.to,
        "subject": args.subject,
    }, ensure_ascii=False, indent=2))


def search(args: argparse.Namespace) -> None:
    query = urllib.parse.urlencode({"q": args.query, "maxResults": args.max_results})
    result = api_request(args, "/messages?" + query)
    output = []
    for item in result.get("messages", []):
        detail = api_request(
            args,
            "/messages/" + item["id"] + "?format=metadata&metadataHeaders=From&metadataHeaders=To&metadataHeaders=Subject&metadataHeaders=Date",
        )
        headers = {h["name"]: h["value"] for h in detail.get("payload", {}).get("headers", [])}
        output.append({
            "id": item["id"],
            "threadId": item.get("threadId"),
            "date": headers.get("Date"),
            "from": headers.get("From"),
            "to": headers.get("To"),
            "subject": headers.get("Subject"),
            "snippet": detail.get("snippet"),
        })
    print(json.dumps(output, ensure_ascii=False, indent=2))


def check_replies(args: argparse.Namespace) -> None:
    """Record and optionally notify about unread replies from outreach domains."""
    ledger = Path(args.ledger_dir)
    seen_path = ledger / "gmail-seen.json"
    report_path = ledger / "收件箱检查.md"
    try:
        seen = json.loads(seen_path.read_text(encoding="utf-8"))
    except (FileNotFoundError, json.JSONDecodeError):
        seen = {}

    senders = " ".join("from:" + domain for domain in REPLY_DOMAINS)
    query = f"is:unread newer_than:{args.days}d in:anywhere {{{senders}}}"
    params = {"q": query, "maxResults": 100}
    hits = []
    while True:
        result = api_request(args, "/messages?" + urllib.parse.urlencode(params))
        for item in result.get("messages", []):
            detail = api_request(
                args,
                "/messages/" + item["id"] + "?format=metadata&metadataHeaders=From&metadataHeaders=Subject&metadataHeaders=Date",
            )
            headers = {h["name"]: h["value"] for h in detail.get("payload", {}).get("headers", [])}
            sender = headers.get("From", "")
            if not any(domain in sender.lower() for domain in REPLY_DOMAINS):
                continue
            hits.append({
                "id": item["id"],
                "threadId": item.get("threadId"),
                "date": headers.get("Date"),
                "from": sender,
                "subject": headers.get("Subject", ""),
                "snippet": detail.get("snippet", ""),
            })
        page_token = result.get("nextPageToken")
        if not page_token:
            break
        params["pageToken"] = page_token

    fresh = [hit for hit in hits if hit["id"] not in seen]
    now = datetime.now(timezone.utc).isoformat(timespec="seconds")
    if fresh:
        ledger.mkdir(parents=True, exist_ok=True)
        if not report_path.exists():
            report_path.write_text("# 收件箱检查\n", encoding="utf-8")
        lines = [f"\n## {now} — {len(fresh)} new repl{'y' if len(fresh) == 1 else 'ies'}\n"]
        for hit in fresh:
            snippet = " ".join(hit["snippet"].split())[:180]
            lines.append(f"- **{hit['from']}** — {hit['subject']}\n  > {snippet}")
            seen[hit["id"]] = now
        with report_path.open("a", encoding="utf-8") as report:
            report.write("\n".join(lines) + "\n")
        write_private(seen_path, seen)
        if not args.no_notify:
            try:
                subprocess.run(
                    ["notify-send", "-u", "normal", "memalyze: Gmail 有新回复",
                     f"{len(fresh)} 封，打开 收件箱检查.md"],
                    timeout=5,
                    check=False,
                )
            except (FileNotFoundError, subprocess.SubprocessError):
                pass

    printable = [{key: hit[key] for key in ("id", "from", "subject")} for hit in fresh]
    print(json.dumps({
        "checked_at": now,
        "query": query,
        "matched": len(hits),
        "new": len(fresh),
        "fresh": printable,
    }, ensure_ascii=False))


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--client", default=str(DEFAULT_CLIENT))
    parser.add_argument("--token", default=str(DEFAULT_TOKEN))
    sub = parser.add_subparsers(dest="command", required=True)

    auth = sub.add_parser("auth", help="Authorize Gmail via loopback OAuth")
    auth.add_argument("--open-cdp", type=int, default=9230)
    auth.add_argument("--timeout", type=int, default=600)
    auth.set_defaults(func=authenticate)

    sub.add_parser("profile", help="Verify read access").set_defaults(func=profile)

    send_parser = sub.add_parser("send", help="Send a plain-text email")
    send_parser.add_argument("--to", required=True)
    send_parser.add_argument("--subject", required=True)
    send_parser.add_argument("--body-file", required=True)
    send_parser.add_argument("--sender")
    send_parser.add_argument("--thread-id")
    send_parser.set_defaults(func=send)

    search_parser = sub.add_parser("search", help="Search Gmail metadata")
    search_parser.add_argument("--query", required=True)
    search_parser.add_argument("--max-results", type=int, default=10)
    search_parser.set_defaults(func=search)

    check_parser = sub.add_parser("check-replies", help="Record unread outreach replies")
    check_parser.add_argument("--days", type=int, default=3)
    check_parser.add_argument("--ledger-dir", default=str(ROOT / ".ledger-updates" / "08-销售"))
    check_parser.add_argument("--no-notify", action="store_true")
    check_parser.set_defaults(func=check_replies)
    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
