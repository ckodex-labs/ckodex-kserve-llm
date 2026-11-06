#!/usr/bin/env python3
"""Exercise a real vLLM external cache and record bounded parity evidence."""

import argparse
import datetime
import hashlib
import ipaddress
import json
import math
import os
import pathlib
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid

HIT_METRIC = "vllm:external_prefix_cache_hits_total"
MAX_RESPONSE = 8 * 1024 * 1024
PHRASES = (
    "The quick brown fox jumps over the lazy dog while a patient scientist observes the result. ",
    "A sailor reads the northern stars while a blue vessel crosses a distant ocean at night. ",
    "Gardeners cultivate bright flowers and tall trees beside a river in a quiet green valley. ",
    "A musician practices careful notes beneath the windows of an old library during winter. ",
)


class VerificationError(Exception):
    """A bounded verification condition failed."""


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise VerificationError("HTTP redirects are not accepted")


def timestamp():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def endpoint_url(value):
    parts = urllib.parse.urlsplit(value)
    if parts.scheme not in ("http", "https") or not parts.hostname:
        raise argparse.ArgumentTypeError("endpoint must be an HTTP(S) URL")
    if parts.username is not None or parts.password is not None or parts.query or parts.fragment:
        raise argparse.ArgumentTypeError("endpoint must not contain credentials, query, or fragment")
    try:
        parts.port
    except ValueError as exc:
        raise argparse.ArgumentTypeError("invalid endpoint port") from exc
    if parts.scheme == "http":
        try:
            loopback = ipaddress.ip_address(parts.hostname).is_loopback
        except ValueError:
            loopback = parts.hostname == "localhost"
        if not loopback:
            raise argparse.ArgumentTypeError("HTTP endpoints must be loopback; use HTTPS otherwise")
    return value.rstrip("/")


def read_limited(response):
    body = response.read(MAX_RESPONSE + 1)
    if len(body) > MAX_RESPONSE:
        raise VerificationError("response exceeded size limit")
    return body


def fetch(opener, url, headers, timeout, payload=None):
    req = urllib.request.Request(url, data=payload, headers=headers)
    try:
        with opener.open(req, timeout=timeout) as response:
            if response.status != 200:
                raise VerificationError(f"HTTP status {response.status}")
            return read_limited(response)
    except urllib.error.HTTPError as exc:
        raise VerificationError(f"HTTP status {exc.code}") from exc
    except urllib.error.URLError as exc:
        raise VerificationError("endpoint connection failed") from exc


def metric_hits(opener, endpoint, headers, timeout):
    body = fetch(opener, endpoint + "/metrics", headers, timeout).decode("utf-8")
    values = []
    for line in body.splitlines():
        if line.startswith(HIT_METRIC + "{"):
            fields = line.split()
            if len(fields) != 2:
                raise VerificationError("unexpected cache metric format")
            values.append(float(fields[1]))
    if len(values) != 1 or not math.isfinite(values[0]) or values[0] < 0:
        raise VerificationError("expected exactly one finite nonnegative external-cache metric series")
    return values[0]


def completion(opener, endpoint, headers, timeout, model, prompt):
    payload = json.dumps({"model": model, "prompt": prompt, "max_tokens": 8,
                          "temperature": 0, "seed": 0, "return_token_ids": True}).encode()
    response = json.loads(fetch(opener, endpoint + "/v1/completions", headers, timeout, payload))
    if not isinstance(response, dict):
        raise VerificationError("completion response is not an object")
    choices = response.get("choices")
    if not isinstance(choices, list) or len(choices) != 1 or not isinstance(choices[0], dict):
        raise VerificationError("expected exactly one completion choice")
    choice = choices[0]
    prompt_ids, token_ids, request_id = choice.get("prompt_token_ids"), choice.get("token_ids"), response.get("id")
    if not isinstance(request_id, str) or not request_id:
        raise VerificationError("completion request ID missing")
    for ids, limit in ((prompt_ids, 32768), (token_ids, 8)):
        if not isinstance(ids, list) or not ids or len(ids) > limit or any(type(i) is not int or i < 0 for i in ids):
            raise VerificationError("completion token IDs missing or invalid")
    return {"request_id": request_id, "prompt_sha256": hashlib.sha256(prompt.encode()).hexdigest(),
            "prompt_token_ids": prompt_ids, "generated_token_ids": token_ids,
            "finish_reason": choice.get("finish_reason"), "observed_at": timestamp()}


def persist(output, result):
    output.seek(0)
    json.dump(result, output, indent=2)
    output.write("\n")
    output.truncate()
    output.flush()
    os.fsync(output.fileno())


def verify(args, output):
    result = {"schema": "ckodex.kv-cache.parity-diagnostic/v1", "run_id": uuid.uuid4().hex,
              "started_at": timestamp(), "endpoint": args.endpoint, "model": args.model,
              "outcome": "in_progress", "cases": []}
    persist(output, result)
    try:
        with pathlib.Path(args.api_key_file).open(encoding="utf-8") as key_file:
            key = key_file.read(16385).strip()
        if len(key) > 16384:
            raise VerificationError("API key file exceeds size limit")
        if not key or "\n" in key or "\r" in key:
            raise VerificationError("API key file is empty or contains a newline")
        headers = {"Authorization": "Bearer " + key, "Content-Type": "application/json"}
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
        ids_seen = set()
        for case in range(args.cases):
            prompts = [f"Run {result['run_id']} case {case} source {i}. " + phrase * 30 +
                       "Summarize the passage in one sentence:" for i, phrase in enumerate(PHRASES)]
            item = {"case": case, "started_at": timestamp(), "outcome": "in_progress"}
            result["cases"].append(item)
            persist(output, result)
            item["hits_before_cold"] = metric_hits(opener, args.endpoint, headers, args.timeout)
            item["cold"] = completion(opener, args.endpoint, headers, args.timeout, args.model, prompts[0])
            item["hits_after_cold"] = metric_hits(opener, args.endpoint, headers, args.timeout)
            if item["hits_after_cold"] != item["hits_before_cold"]:
                raise VerificationError("external-cache hits changed during the cold request")
            completions = [item["cold"]]
            item["eviction_requests"] = []
            for prompt in prompts[1:]:
                filler = completion(opener, args.endpoint, headers, args.timeout, args.model, prompt)
                item["eviction_requests"].append(filler)
                completions.append(filler)
            item["hits_before_reload"] = metric_hits(opener, args.endpoint, headers, args.timeout)
            item["reload"] = completion(opener, args.endpoint, headers, args.timeout, args.model, prompts[0])
            item["hits_after_reload"] = metric_hits(opener, args.endpoint, headers, args.timeout)
            completions.append(item["reload"])
            for c in completions:
                if c["request_id"] in ids_seen:
                    raise VerificationError("completion request IDs were not unique")
                ids_seen.add(c["request_id"])
            if item["hits_after_reload"] <= item["hits_before_reload"]:
                raise VerificationError("external-cache hits did not increase on reload")
            if item["cold"]["prompt_token_ids"] != item["reload"]["prompt_token_ids"]:
                raise VerificationError("prompt token IDs changed")
            if item["cold"]["generated_token_ids"] != item["reload"]["generated_token_ids"]:
                raise VerificationError("generated token IDs changed after reload")
            item["outcome"] = "pass"
            item["finished_at"] = timestamp()
            persist(output, result)
        result["outcome"] = "pass"
    except Exception as exc:
        result["outcome"] = "fail"
        # VerificationError messages are locally authored; other exception
        # text may contain a URL or private service response, so omit it.
        result["error"] = str(exc) if isinstance(exc, VerificationError) else type(exc).__name__
        if result["cases"] and result["cases"][-1]["outcome"] == "in_progress":
            result["cases"][-1]["outcome"] = "fail"
    result["finished_at"] = timestamp()
    persist(output, result)
    return result["outcome"] == "pass"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--endpoint", required=True, type=endpoint_url)
    parser.add_argument("--model", required=True)
    parser.add_argument("--api-key-file", required=True)
    parser.add_argument("--output", required=True, help="new JSON output file; existing files are never overwritten")
    parser.add_argument("--cases", type=int, default=3, choices=range(1, 8))
    parser.add_argument("--timeout", type=int, default=120, choices=range(1, 121), metavar="SECONDS",
                        help="per socket operation, from 1 to 120 seconds")
    args = parser.parse_args()
    if not args.model.strip():
        parser.error("model must not be empty")
    try:
        fd = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as output:
            passed = verify(args, output)
    except OSError as exc:
        print(f"cannot create or write evidence file: {type(exc).__name__}", file=sys.stderr)
        return 1
    print("parity diagnostic " + ("passed" if passed else "failed") + "; see evidence file")
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
