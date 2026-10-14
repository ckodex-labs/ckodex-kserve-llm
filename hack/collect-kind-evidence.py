#!/usr/bin/env python3
"""Read-only, best-effort diagnostics for a named disposable KIND context.

Raw kubectl output stays in memory. Never collect Secrets, ConfigMaps, pod
specs, kubeconfigs, or ambient environment values into the evidence directory.
"""

import argparse
import json
import re
import subprocess
import time
from pathlib import Path


REDACTED = "[REDACTED]"
# Bound identifier matching and start only at key boundaries. Otherwise a long
# ordinary log token makes the overlapping greedy prefixes scan quadratically.
SENSITIVE = r"(?:[\w.-]{0,96}(?:token|password|passwd|secret|api[_-]?key|access[_-]?key|credential|authorization|cookie|private[_-]?key|signature)[\w.-]{0,96})"


def redact(text, literals=()):
    """Scrub known pod env literals and common credentials in free-form text."""
    for value in sorted(set(literals), key=len, reverse=True):
        if value:
            if len(value) < 4:
                text = re.sub(r"(?<!\w)" + re.escape(value) + r"(?!\w)", REDACTED, text)
            else:
                text = text.replace(value, REDACTED)
    text = re.sub(r"-----BEGIN [^-]*PRIVATE KEY-----.*?(?:-----END [^-]*PRIVATE KEY-----|\Z)",
                  REDACTED, text, flags=re.S)
    text = re.sub(r"(?i)\b(?:Bearer|Basic)\s+[A-Za-z0-9+/=_.,-]+", REDACTED, text)
    text = re.sub(r"(?i)(https?://)[^\s/@]+:[^\s/@]+@", r"\1[REDACTED]@", text)
    # Quoted values can contain whitespace; unquoted values stop at delimiters.
    text = re.sub(r"(?i)(?<![\w.-])([\"']?" + SENSITIVE + r"[\"']?\s*[:=]\s*)"
                  r"(?:\"(?:\\.|[^\"\\\n])*\"|'(?:\\.|[^'\\\n])*'|[^\s,;&]+)", r"\1[REDACTED]", text)
    text = re.sub(r"(?i)(--" + SENSITIVE + r"\s+)(?:\"(?:\\.|[^\"\\\n])*\"|'(?:\\.|[^'\\\n])*'|\S+)",
                  r"\1[REDACTED]", text)
    text = re.sub(r"\b(?:hf_[A-Za-z0-9]+|gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+)\b",
                  REDACTED, text)
    text = re.sub(r"\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b", REDACTED, text)
    return text


def redact_json(value, literals):
    if isinstance(value, str):
        return redact(value, literals)
    if isinstance(value, list):
        return [redact_json(item, literals) for item in value]
    if isinstance(value, dict):
        return {key: redact_json(item, literals) for key, item in value.items()}
    return value


def redact_describe(text):
    """Remove sections that can expose arbitrary pod spec values."""
    sensitive_heading = re.compile(r"^(\s*)(?:Labels|Annotations|Command|Args|Environment(?: Variables from)?|Volumes):")
    lines, hidden_indent = [], None
    for line in text.splitlines():
        match = sensitive_heading.match(line)
        indent = len(line) - len(line.lstrip())
        if hidden_indent is not None and line.strip() and indent <= hidden_indent:
            hidden_indent = None
        if match:
            hidden_indent = len(match[1])
            lines.append(line.split(":", 1)[0] + ": " + REDACTED)
        elif hidden_indent is None:
            lines.append(line)
    return "\n".join(lines) + "\n"


def env_literals(pods):
    """Treat every literal env value as private, including custom variable names."""
    values = []
    for pod in pods:
        spec = pod.get("spec", {})
        for key in ("containers", "initContainers", "ephemeralContainers"):
            for container in spec.get(key, []):
                values.extend(env["value"] for env in container.get("env", []) if env.get("value"))
    return values


def container_status(status):
    result = {key: status[key] for key in ("name", "ready", "restartCount", "started") if key in status}
    for key in ("state", "lastState"):
        result[key] = {}
        for state, details in status.get(key, {}).items():
            result[key][state] = {field: details[field] for field in
                                  ("exitCode", "signal", "reason", "message", "startedAt", "finishedAt")
                                  if field in details}
    return result


def pod_status(pod):
    """Allowlist diagnostic status fields; exclude env, annotations and spec."""
    metadata, status = pod.get("metadata", {}), pod.get("status", {})
    result = {"name": metadata.get("name"), "namespace": metadata.get("namespace"),
              "phase": status.get("phase"), "reason": status.get("reason")}
    for key in ("containerStatuses", "initContainerStatuses", "ephemeralContainerStatuses"):
        result[key] = [container_status(item) for item in status.get(key, [])]
    return result


class Collector:
    def __init__(self, context, output, command_timeout=15, budget=240):
        if not re.fullmatch(r"kind-[a-z0-9][a-z0-9.-]*", context):
            raise ValueError("an explicit disposable KIND context is required")
        self.context, self.output = context, Path(output)
        self.command_timeout, self.deadline = command_timeout, time.monotonic() + budget
        self.records, self.literals = [], []

    def query(self, *args):
        remaining = self.deadline - time.monotonic()
        if remaining <= 0:
            return None, {"status": "budget-exhausted"}
        command = ["kubectl", "--context", self.context, "--request-timeout=10s", *args]
        try:
            result = subprocess.run(command, capture_output=True, text=True, errors="replace",
                                    timeout=min(self.command_timeout, remaining), check=False)
        except subprocess.TimeoutExpired:
            return None, {"status": "timeout"}
        except OSError:
            return None, {"status": "unavailable"}
        # Do not retain stderr: API errors can echo request specs or credentials.
        record = {"status": "collected" if result.returncode == 0 else "command-failed",
                  "exitCode": result.returncode}
        return result.stdout if result.returncode == 0 else None, record

    def write(self, filename, text):
        (self.output / filename).write_text(redact(text, self.literals), encoding="utf-8")

    def capture(self, filename, *args, describe=False):
        raw, record = self.query(*args)
        record["file"] = filename
        if raw is not None:
            self.write(filename, redact_describe(raw) if describe else raw)
        self.records.append(record)

    def discover(self):
        raw, record = self.query("get", "pods", "--all-namespaces", "-o", "json")
        record["operation"] = "pod-discovery"
        pods = []
        if raw is not None:
            try:
                pods = json.loads(raw)["items"]
                if not isinstance(pods, list) or not all(isinstance(pod, dict) for pod in pods):
                    raise ValueError("invalid pod list")
                self.literals = env_literals(pods)
            except (ValueError, KeyError, TypeError):
                record["status"] = "invalid-json"
                pods = []
        self.records.append(record)
        return pods, record["status"] == "collected"

    def capture_pod(self, pod, controller=False):
        metadata = pod["metadata"]
        namespace, name = metadata["namespace"], metadata["name"]
        # Kubernetes names cannot contain path separators; reject malformed discovery.
        if not all(re.fullmatch(r"[a-z0-9][a-z0-9.-]*", value) for value in (namespace, name)):
            self.records.append({"status": "invalid-pod-name"})
            return
        stem = namespace + "__" + name
        status = redact_json(pod_status(pod), self.literals)
        (self.output / (stem + ".status.json")).write_text(json.dumps(status, indent=2) + "\n", encoding="utf-8")
        self.records.append({"file": stem + ".status.json", "status": "collected"})
        self.capture(stem + ".describe.txt", "describe", "pod", name, "-n", namespace, describe=True)
        spec = pod.get("spec", {})
        containers = spec.get("containers", []) + spec.get("initContainers", [])
        for container in containers:
            cname = container["name"]
            if not re.fullmatch(r"[a-z0-9][a-z0-9-]*", cname):
                continue
            if not controller and cname != "vllm":
                continue
            for previous in (False, True):
                suffix = ".previous.log" if previous else ".current.log"
                args = ["logs", name, "-n", namespace, "-c", cname, "--tail=-1", "--prefix=true"]
                self.capture(stem + "__" + cname + suffix, *args, *(["--previous"] if previous else []))

    def collect(self):
        # Refuse stale outputs, including symlinks, so prior unredacted files cannot leak.
        self.output.mkdir(mode=0o700, parents=True, exist_ok=False)
        pods, discovered = self.discover()
        if discovered:
            self.capture("resources.txt", "get", "all", "--all-namespaces", "-o", "wide")
            self.capture("events.txt", "get", "events", "--all-namespaces", "--sort-by=.lastTimestamp")
            for pod in pods:
                controller = pod.get("metadata", {}).get("labels", {}).get(
                    "app.kubernetes.io/component") == "controller-manager"
                inference = any(c.get("name") == "vllm" for c in pod.get("spec", {}).get("containers", []))
                if controller or inference:
                    self.capture_pod(pod, controller)
        # Fail closed on discovery failure: no free text without env-literal scrubbing.
        (self.output / "collection.json").write_text(
            json.dumps({"schema": 1, "records": self.records}, indent=2) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", required=True)
    parser.add_argument("--output", default="kind-evidence")
    args = parser.parse_args()
    Collector(args.context, args.output).collect()


if __name__ == "__main__":
    main()
