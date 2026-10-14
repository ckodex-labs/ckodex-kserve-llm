package main

import "dagger/ckodex-operator/internal/dagger"

// Reuse the Python image already pinned for the Hugging Face initializer.
const diagnosticPythonImage = "python:3.12.14-slim-trixie@sha256:7a8b475003c4fe15a2cd4e55e5cfc2f3560bdc9333d624f24cdd6d4340fd7a17"

func diagnosticTestBase(source *dagger.Directory) *dagger.Container {
	return dag.Container().From(diagnosticPythonImage).
		WithMountedDirectory("/src", filteredSource(source)).WithWorkdir("/src").
		WithExec([]string{"python3", "-m", "unittest", "discover", "-s", "test/diagnostics", "-v"})
}
