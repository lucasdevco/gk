#!/usr/bin/env python3
"""Generate bounded real HTTP traffic: normal → mixed client errors → recovery.

No telemetry is fabricated and no task data is created or changed.
Optional --demo runs order checkout scenarios with real SQL and local HTTP
payment, rolling back all demo inventory and order writes.
"""
import argparse
from collections import Counter
from concurrent.futures import ThreadPoolExecutor
import json
import math
import signal
import threading
import time
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def plan(sequence, phase, demo):
    if demo:
        scenario, body = "normal", {"quantity": sequence % 3 + 1}
        if phase == "mixed":
            pick = sequence % 10
            if pick == 0:
                scenario, body = "slow-payment", {"delayMs": 800, "quantity": 2}
            elif pick == 1:
                scenario, body = "payment-retry", {"failuresBeforeSuccess": 2}
            elif pick == 2:
                scenario = "out-of-stock"
            elif pick == 3:
                scenario = "payment-declined"
        return "POST", f"/api/v1/demo/orders/{scenario}", body
    if phase != "mixed":
        return "GET", "/api/v1/tasks", None
    pick = sequence % 10
    if pick in (2, 3):
        return "POST", "/api/v1/tasks", {"title": ""}
    if pick == 4:
        return "PATCH", f"/api/v1/tasks/load-missing-{sequence}", {"completed": True}
    return "GET", "/api/v1/tasks", None


def request(base, method, path, payload, timeout):
    started = time.monotonic()
    data = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(base + path, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    try:
        with urllib.request.build_opener(NoRedirect()).open(req, timeout=timeout) as response:
            while response.read(65536):
                pass
            status = response.status
    except urllib.error.HTTPError as error:
        status = error.code
        error.close()
    except (OSError, urllib.error.URLError):
        status = "network_error"
    return status, time.monotonic() - started


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://localhost:8080", help="API origin")
    parser.add_argument("--duration", type=int, default=180, help="total seconds (3–1800)")
    parser.add_argument("--rate", type=float, default=10, help="total target requests/s (0–100]")
    parser.add_argument("--concurrency", type=int, default=4, help="in-flight limit (1–32)")
    parser.add_argument("--demo", action="store_true", help="run dev-only order checkouts, slow payments, retries and business rejections")
    args = parser.parse_args()
    url = urllib.parse.urlsplit(args.url)
    if url.scheme not in ("http", "https") or not url.hostname or url.username or url.password or url.query or url.fragment:
        parser.error("--url must be an HTTP(S) origin without credentials, query or fragment")
    if url.path not in ("", "/"):
        parser.error("--url must be an origin, without a path")
    if not 3 <= args.duration <= 1800 or not math.isfinite(args.rate) or not 0 < args.rate <= 100 or not 1 <= args.concurrency <= 32:
        parser.error("invalid duration, rate or concurrency")
    base = args.url.rstrip("/")
    if args.demo:
        status, _ = request(base, "POST", "/api/v1/demo/orders/normal", {}, 5)
        if status != 200:
            parser.error("--demo requires a reachable API with APP_ENV=development")
    stopped = threading.Event()
    signal.signal(signal.SIGINT, lambda *_: stopped.set())
    signal.signal(signal.SIGTERM, lambda *_: stopped.set())
    started = time.monotonic()
    deadline = started + args.duration
    next_request = started
    counts, latencies = Counter(), []
    skipped = sequence = 0
    pending = set()
    phase = None
    print(f"Target: {base}; {args.duration}s; {args.rate:g} req/s; max {args.concurrency} in flight", flush=True)
    with ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        while time.monotonic() < deadline and not stopped.is_set():
            now = time.monotonic()
            current = ("normal", "mixed", "recovery")[min(2, int((now - started) * 3 / args.duration))]
            if current != phase:
                phase = current
                print(f"Phase: {phase}", flush=True)
            done = {future for future in pending if future.done()}
            pending -= done
            for future in done:
                status, duration = future.result()
                counts[str(status)] += 1
                latencies.append(duration)
            if now < next_request:
                stopped.wait(min(next_request - now, .1))
                continue
            next_request = now + 1 / args.rate  # Never burst to catch up after a delay.
            if len(pending) >= args.concurrency:
                skipped += 1
                continue
            method, path, body = plan(sequence, phase, args.demo)
            sequence += 1
            pending.add(pool.submit(request, base, method, path, body, 5))
        for future in pending:
            status, duration = future.result()
            counts[str(status)] += 1
            latencies.append(duration)
    latencies.sort()
    p95 = latencies[max(0, math.ceil(len(latencies) * .95) - 1)] * 1000 if latencies else 0
    elapsed = time.monotonic() - started
    print(json.dumps({"requests": sum(counts.values()), "statuses": dict(counts),
                      "elapsed_seconds": round(elapsed, 2), "actual_rps": round(sum(counts.values()) / elapsed, 2),
                      "p95_ms": round(p95, 2), "skipped_at_capacity": skipped}, indent=2))
    return 1 if counts["network_error"] or any(count and key.startswith("5") for key, count in counts.items()) else 0


if __name__ == "__main__":
    raise SystemExit(main())
