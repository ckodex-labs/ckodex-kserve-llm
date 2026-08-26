package main

const (
	goBuilderImage    = "golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36"
	golangciLintImage = "golangci/golangci-lint:v2.12.2"
	trivyVersion      = "0.72.0"
	cosignVersion     = "v3.1.1"
	distrolessImage   = "gcr.io/distroless/static:nonroot"
	// setup-envtest is published as a release binary. Pin the release and
	// verify its platform-specific digest so CI does not compile its dependency
	// graph inside the integration timeout.
	envtestToolVersion       = "v0.24.1"
	envtestToolBaseURL       = "https://github.com/kubernetes-sigs/controller-runtime/releases/download/" + envtestToolVersion
	envtestToolSHA256AMD64   = "a9a78fadfc338a38188332f36863c76877f1c86df1a83d2241d2bfc3935297d2"
	envtestToolSHA256ARM64   = "c5d8968ec3f2a120b66bc13bd36f80fe4150c34aae7cc491bf9624c8680296c7"
	envtestK8sVersion        = "1.35.0"
	envtestAssetsBaseURL     = "https://github.com/kubernetes-sigs/controller-tools/releases/download/envtest-v" + envtestK8sVersion
	envtestAssetsSHA512AMD64 = "130369c16f076e724d089189afaede960316f5f5dea6cf57be7a4fc6f09c77342893192509790e4056e116e232dff832ed863f5bd55dcb55d38f3ab834828a11"
	envtestAssetsSHA512ARM64 = "e53e2b88398f5b9503e3f074d82a2dcb090c708b34940848607ce658138a5d4a25962e042ab683ccc026a8a6c90c0be7f658e42dde0887369d73c3b68e2fc86c"

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
