package main

import "dagger/ckodex-operator/internal/dagger"

func goModBase(source *dagger.Directory) *dagger.Container {
	return dag.Container().From(goBuilderImage).WithWorkdir("/src").
		WithMountedFile("/src/go.mod", source.File("go.mod")).
		WithMountedFile("/src/go.sum", source.File("go.sum")).
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("go-build")).
		WithExec([]string{"mkdir", "-p", "/root/.cache/go-build/tmp"}).
		WithEnvVariable("GOCACHE", "/root/.cache/go-build").
		WithEnvVariable("GOTMPDIR", "/root/.cache/go-build/tmp").
		WithEnvVariable("GOFLAGS", "-mod=readonly").
		WithExec([]string{"go", "mod", "download"})
}

func goBase(source *dagger.Directory) *dagger.Container {
	return goModBase(source).WithMountedDirectory("/src", filteredSource(source))
}

func testBase(source *dagger.Directory, args []string) *dagger.Container {
	return goBase(source).WithExec(args)
}

func golangciBase(source *dagger.Directory) *dagger.Container {
	return dag.Container().From(goBuilderImage).WithWorkdir("/src").
		WithMountedFile("/src/go.mod", source.File("go.mod")).
		WithMountedFile("/src/go.sum", source.File("go.sum")).
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("go-build")).
		WithMountedCache("/root/.cache/golangci-lint", dag.CacheVolume("golangci-lint")).
		WithExec([]string{"mkdir", "-p", "/root/.cache/go-build/tmp"}).
		WithEnvVariable("GOBIN", "/usr/local/bin").
		WithExec([]string{"go", "install", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@" + golangciLintVersion}).
		WithEnvVariable("GOCACHE", "/root/.cache/go-build").
		WithEnvVariable("GOLANGCI_LINT_CACHE", "/root/.cache/golangci-lint").
		WithEnvVariable("GOTMPDIR", "/root/.cache/go-build/tmp").
		WithEnvVariable("GOFLAGS", "-mod=readonly").
		WithExec([]string{"go", "mod", "download"}).
		WithMountedDirectory("/src", filteredSource(source))
}

func filteredSource(source *dagger.Directory) *dagger.Directory {
	return source.Filter(dagger.DirectoryFilterOpts{Exclude: sourceExcludes, Gitignore: true})
}
