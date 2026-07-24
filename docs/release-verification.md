# Release Verification

This repo has two distinct release checks:

- `make release-readiness` is the local snapshot rehearsal. It proves the repo can generate binary archives, checksums, and a Helm chart package without mutating tracked files.
- The tag-driven GitHub Actions workflow is the hosted release path. It is the only path that should be treated as authoritative for published images, GitHub release assets, and OIDC-backed provenance.

## Local Snapshot Rehearsal

Run:

```bash
make release-readiness
```

This checks:

- the release workflow still contains the Dagger image path, binary provenance, image provenance, and Helm publish steps,
- GoReleaser can produce snapshot archives and `dist/checksums.txt`,
- the Helm chart packages cleanly,
- the generated checksum manifest validates,
- and the rehearsal did not mutate tracked repo files.

The summary artifact is written to `bin/release-readiness.json`.

## Hosted Release Contract

On a real tag such as `v0.18.0-beta.8`, GitHub Actions is expected to:

1. build and push container images through the Dagger release pipeline,
2. build GitHub release binaries and checksums through GoReleaser,
3. generate image and binary provenance through the SLSA GitHub generator workflows,
4. package and push the Helm chart to GHCR with the tag version.

Starting with `v0.18.0-beta.6`, the image path also builds, scans, signs,
inventories, and emits provenance for the dedicated Xet-aware Hugging Face
initializer. This removes package installation from model-pod startup.

Local green checks are not enough to claim public release readiness unless that hosted path has also succeeded.

## Current Published Release

The current public release is `v0.18.0-beta.8`.

- GitHub release: <https://github.com/ckodex-labs/ckodex-kserve-llm/releases/tag/v0.18.0-beta.8>
- Source commit: `634a79b7fb91f2fbf95cb5fe17caf9061b0998aa`
- Hosted release run: <https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/28995564785>
- Published assets include manager archives, storage-initializer archives, `checksums.txt`, `checksums.txt.sigstore.json`, binary provenance, image provenance, container image signature, SBOM output, and the Helm chart package.

## Downstream Verification Commands

Replace `$TAG` with the release tag and run verification as a downstream consumer.

### Container Image Signature

```bash
cosign verify \
  --certificate-identity-regexp '.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ckodex-labs/ckodex-kserve-llm:$TAG
```

### Container Image Provenance

```bash
cosign verify-attestation \
  --type slsaprovenance1 \
  --certificate-identity-regexp '.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ckodex-labs/ckodex-kserve-llm:$TAG
```

### Container Image SBOM Attestation

```bash
cosign verify-attestation \
  --type cyclonedx \
  --certificate-identity-regexp '.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ckodex-labs/ckodex-kserve-llm:$TAG
```

### Binary Checksums

```bash
gh release download "$TAG" --repo ckodex-labs/ckodex-kserve-llm --dir release-assets
(cd release-assets && sha256sum -c checksums.txt)
```

### Helm Chart Retrieval

```bash
helm pull oci://ghcr.io/ckodex-labs/charts/ckodex-kserve-llm-operator --version "${TAG#v}"
```
