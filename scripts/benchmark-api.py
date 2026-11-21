#!/usr/bin/env python3
"""Bounded synthetic API latency sampler. Never prints credentials or response bodies."""
import argparse
import concurrent.futures
import json
import os
import time
import urllib.error
import urllib.request


def percentile(values, pct):
    ordered = sorted(values)
    return ordered[min(len(ordered) - 1, round((len(ordered) - 1) * pct))]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--path", default="/health")
    parser.add_argument("--count", type=int, default=30)
    parser.add_argument("--concurrency", type=int, default=1)
    parser.add_argument("--method", choices=("GET", "POST"), default="GET")
    parser.add_argument("--body-file")
    args = parser.parse_args()
    if args.count < 1 or args.count > 10000 or args.concurrency < 1 or args.concurrency > 10:
        parser.error("count must be 1..10000 and concurrency 1..10")
    body = None
    if args.body_file:
        with open(args.body_file, "rb") as source:
            body = source.read(128 * 1024 + 1)
        if len(body) > 128 * 1024:
            parser.error("request body exceeds 128 KiB")
    headers = {"Accept": "application/json"}
    if body is not None:
        headers["Content-Type"] = "application/json"
    key = os.environ.get("AMBATU_PARTNER_KEY")
    if key:
        headers["X-API-Key"] = key
    url = args.base_url.rstrip("/") + "/" + args.path.lstrip("/")

    def sample(_):
        request = urllib.request.Request(url, data=body, headers=headers, method=args.method)
        started = time.perf_counter()
        try:
            with urllib.request.urlopen(request, timeout=10) as response:
                response.read(1024 * 1024)
                return response.status, time.perf_counter() - started
        except urllib.error.HTTPError as error:
            error.read(1024 * 1024)
            return error.code, time.perf_counter() - started
        except Exception:
            return 0, time.perf_counter() - started

    started = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        results = list(pool.map(sample, range(args.count)))
    elapsed = time.perf_counter() - started
    durations = [duration for status, duration in results]
    statuses = {}
    for status, _ in results:
        label = str(status) if status else "transport_error"
        statuses[label] = statuses.get(label, 0) + 1
    print(json.dumps({
        "url": url,
        "method": args.method,
        "requests": args.count,
        "concurrency": args.concurrency,
        "elapsed_seconds": round(elapsed, 3),
        "throughput_rps": round(args.count / elapsed, 2),
        "statuses": statuses,
        "latency_ms": {
            "p50": round(percentile(durations, .50) * 1000, 2),
            "p95": round(percentile(durations, .95) * 1000, 2),
            "p99": round(percentile(durations, .99) * 1000, 2),
        },
    }, indent=2))
    return 0 if all(200 <= status < 400 for status, _ in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
