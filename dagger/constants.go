package main

const (
	goBuilderImage      = "golang:1.26.9-bookworm@sha256:d9c68c2c51161e12fd77e4c6320687c9cd86e1af1e3ad6e6cd63ff970641453c"
	golangciLintVersion = "v2.13.1"
	trivyVersion        = "0.72.0"
	cosignVersion       = "v3.1.3"
	distrolessImage     = "gcr.io/distroless/static:nonroot"
	// setup-envtest is published as a release binary. Pin the release and
	// verify its platform-specific digest so CI does not compile its dependency
	// graph inside the integration timeout.
	envtestToolVersion       = "v0.24.1"
	envtestToolBaseURL       = "https://github.com/kubernetes-sigs/controller-runtime/releases/download/" + envtestToolVersion
	envtestToolSHA256AMD64   = "a9a78fadfc338a38188332f36863c76877f1c86df1a83d2241d2bfc3935297d2"
	envtestToolSHA256ARM64   = "c5d8968ec3f2a120b66bc13bd36f80fe4150c34aae7cc491bf9624c8680296c7"
	envtestK8sVersion        = "1.36.2"
	envtestAssetsBaseURL     = "https://github.com/kubernetes-sigs/controller-tools/releases/download/envtest-v" + envtestK8sVersion
	envtestAssetsSHA512AMD64 = "ea743186c8a799f5cf8faf16969f86189d003cb7d130e0ac4b58789f1e5748dcf30ebe91c837a10d5ac415383da3e10b9e64d65785c938c23e739781cfb76f08"
	envtestAssetsSHA512ARM64 = "2d72ee985a8e262a3c57dc9f7f0fd891f6a8c7bf7ebaa2db6dc6d8eac8ae28181afe51c1f368b67756cdb40b10de9b205609e1726e5f27f7c6d824dd9c6649ac"

	// Coverage thresholds enforced by the Dagger test gate.
	coverageController = 80
	coverageGateway    = 80
	coverageStorage    = 80
	coverageAuth       = 80
	coverageInference  = 80
	coverageObs        = 80
)

var sourceExcludes = []string{
	".git/", ".dagger/", ".cache/", ".cocoindex_code/", ".tmp/", "bin/",
	"console/.next/", "console/node_modules/", "dist/", "scratch/bin/", "target/",
	"node_modules/", "*.log", "*.out",
}
