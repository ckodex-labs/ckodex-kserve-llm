package main

const (
	goBuilderImage     = "golang:1.26.5-bookworm"
	golangciLintImage  = "golangci/golangci-lint:v2.12.2"
	trivyVersion       = "0.72.0"
	cosignVersion      = "v3.1.1"
	distrolessImage    = "gcr.io/distroless/static:nonroot"
	envtestToolVersion = "v0.0.0-20260318145839-6c9615a2a166"
	envtestK8sVersion  = "1.35.x!"

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
