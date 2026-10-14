"""Offline fixtures only: subprocess.run is replaced before collection."""

import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("collector", ROOT / "hack/collect-kind-evidence.py")
collector = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(collector)


def fixture_pod(name="cpu-smoke", controller=False):
    return {
        "metadata": {"name": name, "namespace": "default", "annotations": {"private": "annotation-private"},
                     "labels": {"app.kubernetes.io/component": "controller-manager" if controller else "inference"}},
        "spec": {"containers": [
            {"name": "manager" if controller else "vllm", "image": "fixture:v1",
             "env": [{"name": "CUSTOM_VALUE", "value": 'opaque-private-"quoted"'},
                     {"name": "OMP_NUM_THREADS", "value": "3"}],
             "args": ["command-private"]}, {"name": "vector"}],
            "initContainers": [{"name": "init", "env": [{"name": "CUSTOM_INIT", "value": "init-private"}]}]},
        "status": {"phase": "Running", "containerStatuses": [
            {"name": "vllm", "ready": False, "restartCount": 5, "started": False,
             "containerID": "container-private", "state": {"waiting": {"reason": "CrashLoopBackOff"}},
             "lastState": {"terminated": {"exitCode": 1, "signal": 0, "reason": "Error",
                                          "finishedAt": "2026-10-13T10:59:00Z",
                                          "message": 'failure: opaque-private-"quoted"'}}}],
                   "initContainerStatuses": [{"name": "init", "restartCount": 0,
                                              "state": {"terminated": {"exitCode": 0}}}]}}


DESCRIBE = """Name: cpu-smoke
Annotations: private: annotation-private
  arbitrary: another-private
Status: Running
Containers:
  vllm:
    Command:
      command-private
    Args:
      --custom
      argument-private
    State: Waiting
      Reason: CrashLoopBackOff
    Environment:
      CUSTOM_VALUE: opaque-private-"quoted"
      SOMETHING: describe-private
    Mounts:
      /cache from weights (rw)
Volumes:
  weights:
    value: volume-private
QoS Class: Burstable
Events:
  Warning BackOff restarting failed container
"""


class RedactionTests(unittest.TestCase):
    def test_common_log_credentials(self):
        cases = ["Authorization: Bearer bearer-private", "Authorization: Basic basic-private",
                 'password="private with spaces"', "TOKEN='private with spaces'",
                 '{"api_key": "json-private"}', "--api-key command-private",
                 "https://user:url-private@host/path?token=query-private&ok=yes",
                 "https://host/?X-Amz-Signature=signed-private&ok=yes",
                 "hf_fixtureprivate github_pat_fixtureprivate ghp_fixtureprivate",
                 "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.fixtureprivate",
                 "-----BEGIN PRIVATE KEY-----\npem-private\n-----END PRIVATE KEY-----",
                 "-----BEGIN RSA PRIVATE KEY-----\ntruncated-private"]
        for raw in cases:
            with self.subTest(raw=raw):
                output = collector.redact(raw)
                self.assertNotIn("private", output)
                self.assertIn(collector.REDACTED, output)

    def test_describe_excludes_arbitrary_spec_values(self):
        output = collector.redact_describe(DESCRIBE)
        self.assertNotIn("private", output)
        self.assertIn("CrashLoopBackOff", output)
        self.assertIn("Warning BackOff", output)
        self.assertIn("QoS Class: Burstable", output)

    def test_env_literals_and_short_values_preserve_numeric_json(self):
        literals = collector.env_literals([fixture_pod()])
        output = collector.redact_json(collector.pod_status(fixture_pod()), literals)
        serialized = json.dumps(output)
        self.assertNotIn("private", serialized)
        status = json.loads(serialized)["containerStatuses"][0]
        self.assertEqual(status["restartCount"], 5)
        self.assertEqual(status["lastState"]["terminated"]["exitCode"], 1)
        self.assertEqual(status["lastState"]["terminated"]["signal"], 0)
        self.assertEqual(status["lastState"]["terminated"]["finishedAt"], "2026-10-13T10:59:00Z")

    def test_nested_secret_names_and_escaped_values(self):
        output = collector.redact('HF_TOKEN=hf_fixtureprivate AWS_ACCESS_KEY_ID=access-private '
                                  'X-Amz-Credential=credential-private session_cookie=cookie-private')
        self.assertNotIn("private", output)
        self.assertEqual(output.count(collector.REDACTED), 4)

    def test_quoted_escaped_secret_is_removed_entirely(self):
        output = collector.redact(r'password="first\" remainder-private" normal=value')
        self.assertNotIn("private", output)
        self.assertNotIn("first", output)
        self.assertIn("normal=value", output)

    def test_large_plain_log_token_finishes_within_collection_budget(self):
        # A subprocess timeout bounds failure too; the old regex takes minutes
        # on this input. No kubectl, network, model or container is involved.
        code = "from runpy import run_path; m = run_path('hack/collect-kind-evidence.py'); " \
               "text = 'x' * 100000 + ' token=credential-private'; " \
               "out = m['redact'](text); assert out == 'x' * 100000 + ' token=[REDACTED]'"
        subprocess.run([sys.executable, "-c", code], cwd=ROOT, capture_output=True,
                       text=True, check=True, timeout=5)


class CollectionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.output = Path(self.temp.name) / "evidence"
        self.commands = []
        self.pods = [fixture_pod(), fixture_pod("old-replica"), fixture_pod("operator", True)]
        self.failure = None

    def fake_kubectl(self, command, **kwargs):
        self.commands.append(command)
        self.assertEqual(command[:4], ["kubectl", "--context", "kind-fixture", "--request-timeout=10s"])
        self.assertLessEqual(kwargs["timeout"], 15)
        args = command[4:]
        if self.failure:
            result = self.failure(args)
            if result is not None:
                return result
        if args[:2] == ["get", "pods"]:
            raw = json.dumps({"items": self.pods})
        elif args[0] == "describe":
            raw = DESCRIBE
        elif args[0] == "logs":
            raw = "\n".join("controller line " + str(i) for i in range(100))
            raw += '\nopaque-private-"quoted"\ninit-private\npassword=log-private\n'
        else:
            raw = 'resources/events opaque-private-"quoted"\n'
        return subprocess.CompletedProcess(command, 0, stdout=raw, stderr="stderr-private")

    def collect(self, **kwargs):
        with patch.object(collector.subprocess, "run", side_effect=self.fake_kubectl):
            collector.Collector("kind-fixture", self.output, **kwargs).collect()
        return json.loads((self.output / "collection.json").read_text())

    def test_current_previous_all_replicas_full_controller_logs_and_status(self):
        manifest = self.collect()
        for name in ("cpu-smoke", "old-replica"):
            for suffix in ("current", "previous"):
                self.assertTrue((self.output / f"default__{name}__vllm.{suffix}.log").exists())
            status = json.loads((self.output / f"default__{name}.status.json").read_text())
            self.assertEqual(status["containerStatuses"][0]["restartCount"], 5)
            self.assertEqual(status["initContainerStatuses"][0]["state"]["terminated"]["exitCode"], 0)
        for container in ("manager", "vector", "init"):
            logs = (self.output / f"default__operator__{container}.current.log").read_text()
            self.assertIn("controller line 99", logs)
        self.assertTrue((self.output / "events.txt").exists())
        self.assertTrue(all(r["status"] == "collected" for r in manifest["records"]))
        for command in self.commands:
            self.assertIn(command[4], ("get", "describe", "logs"))
            if command[4] == "logs":
                self.assertIn("--tail=-1", command)
        outputs = "".join(path.read_text() for path in self.output.iterdir())
        self.assertNotIn("private", outputs)
        self.assertNotIn('"spec"', outputs)

    def test_missing_previous_logs_do_not_stop_other_collection(self):
        self.failure = lambda args: subprocess.CompletedProcess(args, 1, "", "token=stderr-private") if "--previous" in args else None
        manifest = self.collect()
        self.assertTrue((self.output / "default__old-replica__vllm.current.log").exists())
        failed = [r for r in manifest["records"] if r["status"] == "command-failed"]
        self.assertEqual(len(failed), 5)
        self.assertTrue(all(r["exitCode"] == 1 for r in failed))
        self.assertFalse((self.output / "default__cpu-smoke__vllm.previous.log").exists())

    def test_timeout_discards_partial_output_and_continues(self):
        def fail(args):
            if args[0] == "describe":
                raise subprocess.TimeoutExpired(args, 15, output="timeout-private")
        self.failure = fail
        manifest = self.collect()
        self.assertTrue(any(r["status"] == "timeout" for r in manifest["records"]))
        self.assertTrue((self.output / "default__cpu-smoke__vllm.previous.log").exists())
        self.assertNotIn("private", (self.output / "collection.json").read_text())

    def test_discovery_failure_retains_only_safe_manifest(self):
        self.failure = lambda args: subprocess.CompletedProcess(args, 1, "stdout-private", "stderr-private")
        manifest = self.collect()
        self.assertEqual(len(self.commands), 1)
        self.assertEqual(list(self.output.iterdir()), [self.output / "collection.json"])
        self.assertEqual(manifest["records"][0]["status"], "command-failed")

    def test_invalid_discovery_fails_closed(self):
        for raw in ("invalid-private", '{"items": "invalid-private"}', '{"unknown": []}'):
            with self.subTest(raw=raw), tempfile.TemporaryDirectory() as temp:
                self.output = Path(temp) / "evidence"
                self.failure = lambda args: subprocess.CompletedProcess(args, 0, raw, "")
                manifest = self.collect()
                self.assertEqual(manifest["records"][0]["status"], "invalid-json")
                self.assertEqual(len(list(self.output.iterdir())), 1)

    def test_unavailable_kubectl_is_reported(self):
        def fail(args):
            raise FileNotFoundError("unavailable-private")
        self.failure = fail
        self.assertEqual(self.collect()["records"][0]["status"], "unavailable")

    def test_no_matching_pods_still_captures_events(self):
        self.pods = []
        manifest = self.collect()
        self.assertEqual(len(manifest["records"]), 3)
        self.assertTrue((self.output / "events.txt").exists())

    def test_exhausted_budget_makes_no_calls(self):
        manifest = self.collect(budget=0)
        self.assertEqual(self.commands, [])
        self.assertEqual(manifest["records"][0]["status"], "budget-exhausted")

    def test_non_kind_context_is_rejected_before_queries(self):
        with self.assertRaises(ValueError):
            self.collector = collector.Collector("production", self.output)
        self.assertEqual(self.commands, [])

    def test_stale_directory_or_symlink_is_rejected_before_queries(self):
        self.output.mkdir()
        with self.assertRaises(FileExistsError):
            self.collect()
        self.output = Path(self.temp.name) / "link"
        self.output.symlink_to(Path(self.temp.name) / "evidence")
        with self.assertRaises(FileExistsError):
            self.collect()
        self.assertEqual(self.commands, [])

    def test_invalid_pod_name_cannot_escape_evidence_directory(self):
        self.pods[0]["metadata"]["name"] = "../outside"
        manifest = self.collect()
        self.assertTrue(any(r["status"] == "invalid-pod-name" for r in manifest["records"]))
        self.assertFalse((Path(self.temp.name) / "outside.status.json").exists())

    def test_workflow_collects_before_upload_and_teardown_and_tests_are_wired(self):
        workflow = (ROOT / ".github/workflows/nightly-chaos.yml").read_text()
        capture = workflow.index("name: Collect cluster evidence on failure")
        upload = workflow.index("name: Upload failure evidence")
        teardown = workflow.index("name: Tear down KIND cluster")
        self.assertLess(capture, upload)
        self.assertLess(upload, teardown)
        self.assertIn("if: failure()", workflow[capture:upload])
        self.assertIn('python3 hack/collect-kind-evidence.py', workflow[capture:upload])
        self.assertIn('--context "kind-${KIND_CLUSTER_NAME}"', workflow[capture:upload])
        self.assertIn("if: always()", workflow[teardown:])
        self.assertIn("if: failure() && steps.evidence.outcome == 'success'", workflow[upload:teardown])
        self.assertIn("diagnosticTestBase(source).Sync(ctx)", (ROOT / "dagger/module.go").read_text())
        self.assertIn('"test/diagnostics", "-v"', (ROOT / "dagger/diagnostics.go").read_text())


if __name__ == "__main__":
    unittest.main()
