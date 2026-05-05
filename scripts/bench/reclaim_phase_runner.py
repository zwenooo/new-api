#!/usr/bin/env python3
import argparse
import concurrent.futures
import json
import pathlib
import statistics
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone


def utc_now():
    return datetime.now(timezone.utc)


def utc_stamp():
    return utc_now().strftime("%Y%m%dT%H%M%SZ")


def normalize_label(label: str) -> str:
    cleaned = []
    for ch in label.strip():
        if ch.isalnum() or ch in ("-", "_"):
            cleaned.append(ch)
        else:
            cleaned.append("-")
    out = "".join(cleaned).strip("-")
    return out or "run"


def percentile(sorted_values: list[float], pct: float) -> float:
    if not sorted_values:
        return 0.0
    if len(sorted_values) == 1:
        return round(sorted_values[0], 2)
    rank = (len(sorted_values) - 1) * pct
    lower = int(rank)
    upper = min(lower + 1, len(sorted_values) - 1)
    if lower == upper:
        return round(sorted_values[lower], 2)
    weight = rank - lower
    return round(sorted_values[lower] * (1 - weight) + sorted_values[upper] * weight, 2)


def build_headers(item: dict, bearer_token: str) -> dict:
    headers = {}
    item_headers = item.get("headers", {})
    if isinstance(item_headers, dict):
        headers.update(item_headers)
    if "Content-Type" not in headers:
        headers["Content-Type"] = item.get("content_type", "application/json")
    if bearer_token:
        headers["Authorization"] = f"Bearer {bearer_token}"
    return headers


def run_http_request(base_url: str, item: dict, timeout_sec: float, bearer_token: str, iteration: int) -> dict:
    method = item["method"].upper()
    url = base_url.rstrip("/") + item["path"]
    body = item.get("body")
    headers = build_headers(item, bearer_token)
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")

    request = urllib.request.Request(url=url, method=method, data=data, headers=headers)
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=timeout_sec) as response:
            payload = response.read()
            ended = time.perf_counter()
            elapsed_ms = round((ended - started) * 1000, 2)
            return {
                "id": item["id"],
                "kind": item["kind"],
                "iteration": iteration,
                "status": "ok",
                "http_status": response.status,
                "elapsed_ms": elapsed_ms,
                "bytes": len(payload),
                "started_at_perf": started,
                "ended_at_perf": ended,
            }
    except urllib.error.HTTPError as exc:
        payload = exc.read()
        ended = time.perf_counter()
        elapsed_ms = round((ended - started) * 1000, 2)
        return {
            "id": item["id"],
            "kind": item["kind"],
            "iteration": iteration,
            "status": "http_error",
            "http_status": exc.code,
            "elapsed_ms": elapsed_ms,
            "bytes": len(payload),
            "error": str(exc),
            "started_at_perf": started,
            "ended_at_perf": ended,
        }
    except Exception as exc:  # noqa: BLE001
        ended = time.perf_counter()
        elapsed_ms = round((ended - started) * 1000, 2)
        return {
            "id": item["id"],
            "kind": item["kind"],
            "iteration": iteration,
            "status": "error",
            "elapsed_ms": elapsed_ms,
            "error": str(exc),
            "started_at_perf": started,
            "ended_at_perf": ended,
        }


def summarize_timing(results: list[dict]) -> tuple[float, float]:
    timed = [
        item for item in results
        if "started_at_perf" in item and "ended_at_perf" in item
    ]
    if not timed:
        return 0.0, 0.0
    started = min(item["started_at_perf"] for item in timed)
    ended = max(item["ended_at_perf"] for item in timed)
    wall_clock_ms = round(max(ended - started, 0.0) * 1000, 2)
    latency_sum_ms = round(sum(item.get("elapsed_ms", 0.0) for item in timed), 2)
    return wall_clock_ms, latency_sum_ms


def summarize_http_results(results: list[dict]) -> dict:
    http_results = [item for item in results if item.get("kind") == "http"]
    ok_results = [item for item in http_results if item.get("status") == "ok"]
    elapsed_values = sorted(item["elapsed_ms"] for item in ok_results if "elapsed_ms" in item)
    wall_clock_ms, latency_sum_ms = summarize_timing(http_results)
    rpm = 0.0
    if wall_clock_ms > 0:
        rpm = round((len(ok_results) * 60000.0) / wall_clock_ms, 2)
    return {
        "http_total": len(http_results),
        "http_success": len(ok_results),
        "http_failure": sum(1 for item in http_results if item.get("status") in ("http_error", "error")),
        "elapsed_total_ms": wall_clock_ms,
        "latency_sum_ms": latency_sum_ms,
        "rpm": rpm,
        "p50_ms": percentile(elapsed_values, 0.50),
        "p95_ms": percentile(elapsed_values, 0.95),
        "p99_ms": percentile(elapsed_values, 0.99),
        "mean_ms": round(statistics.fmean(elapsed_values), 2) if elapsed_values else 0.0
    }


def summarize_by_request(results: list[dict]) -> dict:
    by_id = {}
    for item in results:
        if item.get("kind") != "http":
            continue
        bucket = by_id.setdefault(item["id"], [])
        bucket.append(item)
    summary = {}
    for key, items in by_id.items():
        ok_items = [item for item in items if item.get("status") == "ok"]
        elapsed_values = sorted(item["elapsed_ms"] for item in ok_items if "elapsed_ms" in item)
        wall_clock_ms, latency_sum_ms = summarize_timing(items)
        rpm = 0.0
        if wall_clock_ms > 0:
            rpm = round((len(ok_items) * 60000.0) / wall_clock_ms, 2)
        summary[key] = {
            "total": len(items),
            "success": len(ok_items),
            "failure": sum(1 for item in items if item.get("status") in ("http_error", "error")),
            "elapsed_total_ms": wall_clock_ms,
            "latency_sum_ms": latency_sum_ms,
            "rpm": rpm,
            "p50_ms": percentile(elapsed_values, 0.50),
            "p95_ms": percentile(elapsed_values, 0.95),
            "p99_ms": percentile(elapsed_values, 0.99),
            "mean_ms": round(statistics.fmean(elapsed_values), 2) if elapsed_values else 0.0
        }
    return summary


def main():
    parser = argparse.ArgumentParser(description="Run the fixed reclaim benchmark corpus and emit JSON evidence.")
    parser.add_argument("--label", required=True, help="Label for the binary/build under test")
    parser.add_argument("--base-url", required=True, help="Base URL for HTTP requests")
    parser.add_argument("--output-dir", required=True, help="Directory for JSON evidence output")
    parser.add_argument("--dataset-note", default="", help="Dataset/environment note stored in the artifact")
    parser.add_argument(
        "--corpus",
        default="scripts/bench/reclaim_phase0_corpus.json",
        help="Corpus JSON path"
    )
    parser.add_argument("--timeout-sec", type=float, default=30.0, help="Per-request timeout in seconds")
    parser.add_argument("--bearer-token", default="", help="Optional bearer token added to every HTTP request")
    parser.add_argument("--repeat", type=int, default=1, help="How many times to run each HTTP request")
    parser.add_argument("--concurrency", type=int, default=1, help="Concurrent HTTP workers")
    args = parser.parse_args()

    corpus_path = pathlib.Path(args.corpus)
    output_dir = pathlib.Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    with corpus_path.open("r", encoding="utf-8") as handle:
        corpus = json.load(handle)

    started = time.perf_counter()
    results = []
    http_tasks = []
    for item in corpus.get("requests", []):
        if item.get("kind") == "http":
            for iteration in range(1, max(args.repeat, 1) + 1):
                http_tasks.append((item, iteration))
        else:
            results.append(
                {
                    "id": item["id"],
                    "kind": item["kind"],
                    "status": "manual",
                    "path": item.get("path", ""),
                    "notes": item.get("notes", "")
                }
            )

    if args.concurrency <= 1:
        for item, iteration in http_tasks:
            results.append(run_http_request(args.base_url, item, args.timeout_sec, args.bearer_token, iteration))
    else:
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as executor:
            futures = [
                executor.submit(run_http_request, args.base_url, item, args.timeout_sec, args.bearer_token, iteration)
                for item, iteration in http_tasks
            ]
            for future in concurrent.futures.as_completed(futures):
                results.append(future.result())

    elapsed_total_ms = round((time.perf_counter() - started) * 1000, 2)
    total = len(results)
    success = sum(1 for item in results if item["status"] == "ok")
    failures = sum(1 for item in results if item["status"] in ("http_error", "error"))

    artifact = {
        "generated_at_utc": utc_now().isoformat(),
        "label": args.label,
        "normalized_label": normalize_label(args.label),
        "base_url": args.base_url,
        "dataset_note": args.dataset_note,
        "corpus_file": str(corpus_path),
        "corpus_version": corpus.get("version", 0),
        "repeat": max(args.repeat, 1),
        "concurrency": max(args.concurrency, 1),
        "results": results,
        "summary": {
          "total": total,
          "success": success,
          "failure": failures,
          "elapsed_total_ms": elapsed_total_ms
        },
        "http_summary": summarize_http_results(results),
        "by_request": summarize_by_request(results)
    }

    output_path = output_dir / f"run-{utc_stamp()}-{normalize_label(args.label)}.json"
    with output_path.open("w", encoding="utf-8") as handle:
        for item in artifact["results"]:
            item.pop("started_at_perf", None)
            item.pop("ended_at_perf", None)
        json.dump(artifact, handle, ensure_ascii=True, indent=2)
        handle.write("\n")

    print(output_path)


if __name__ == "__main__":
    main()
