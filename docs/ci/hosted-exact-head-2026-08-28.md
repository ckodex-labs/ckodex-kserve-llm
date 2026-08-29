# Hosted exact-head evidence — 2026-08-28

## Recorded hosted state

- **C — CI:** run [33006888854](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33006888854) passed on main commit `c3f6b83d932ca0779339e73df67b8866e0157806`.
- **C — pull-request CI:** run [33005025176](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33005025176) passed on pull-request head `2ef53bd55156f21b9abb4f446d7d3409e239ecac`; that head was merged as `c3f6b83d932ca0779339e73df67b8866e0157806`.
- **C — Nightly failure:** run [33081386331](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33081386331) failed on the same main commit before the API server started. KIND v0.32.0 selected `kindest/node:v1.35.0`; kubeadm generated `v1beta3` configuration and rejected the list-shaped `etcd.local.extraArgs` patch with `json: cannot unmarshal array into Go struct field LocalEtcd.etcd.local.extraArgs of type map[string]string`.

## Remediation contract

- **C — local contract:** Nightly installs the KIND version from `deploy/kind/acceptance-kind-version.txt` and loads the node image from `deploy/kind/acceptance-node-image.txt`. The files pin KIND v0.33.0 and its release image `kindest/node:v1.36.4` by digest; local setup fails early on a different KIND CLI, and local setup plus Make use the same node source.
- **C — local contract:** the kubeadm patch retains the `name`/`value` list required by the v1beta4 schema used by the selected node profile.
- **C — release guard:** a tag workflow verifies its checkout SHA, requires the two hosted CI jobs to have succeeded on that SHA, injects the tag into the manager binary, and executes `manager --version` before publication.
- **S — hosted acceptance:** the KIND and release guards remain unaccepted until a hosted run executes the committed remediation head. No local contract check is runtime proof.

## Exact commands used to collect the hosted state

```text
gh run view 33081386331 --json status,conclusion,headSha,headBranch,event,url,jobs
gh run view 33081386331 --log-failed
gh run list --commit c3f6b83d932ca0779339e73df67b8866e0157806 --limit 30 --json databaseId,name,workflowName,event,status,conclusion,headSha,url,createdAt
gh pr view 54 --json mergeCommit,headRefOid,statusCheckRollup,url
```
