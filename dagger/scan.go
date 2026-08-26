package main

import (
	"fmt"

	"dagger/ckodex-operator/internal/dagger"
)

func trivyBase() *dagger.Container {
	return dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).
		From(fmt.Sprintf("aquasec/trivy:%s", trivyVersion)).
		WithMountedCache("/root/.cache/trivy", dag.CacheVolume("trivy-db")).
		WithEnvVariable("TRIVY_CACHE_DIR", "/root/.cache/trivy")
}

func scanRootfs(rootfs *dagger.Directory) *dagger.Container {
	return scanRootfsForArch(rootfs, "amd64")
}

func scanRootfsForArch(rootfs *dagger.Directory, arch string) *dagger.Container {
	return trivyBaseForArch(arch).WithMountedDirectory("/rootfs", rootfs).WithExec([]string{
		"trivy", "rootfs", "--severity", "CRITICAL,HIGH", "--scanners", "vuln", "--exit-code", "1",
		"--ignore-unfixed", "--format", "table", "/rootfs",
	})
}

func trivyBaseForArch(arch string) *dagger.Container {
	if arch != "amd64" && arch != "arm64" {
		arch = "amd64"
	}
	return dag.Container(dagger.ContainerOpts{Platform: dagger.Platform("linux/" + arch)}).
		From(fmt.Sprintf("aquasec/trivy:%s", trivyVersion)).
		WithMountedCache("/root/.cache/trivy", dag.CacheVolume("trivy-db-"+arch)).
		WithEnvVariable("TRIVY_CACHE_DIR", "/root/.cache/trivy")
}
