/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type registryFixture struct {
	server              *httptest.Server
	operatorManifest    manifest
	initializerManifest manifest
	chartManifest       manifest
	tokenStatus         int
}

func TestRunVerifiesAnonymousMultiArchImagesAndHelmChart(t *testing.T) {
	fixture := newRegistryFixture(t)
	var output bytes.Buffer

	err := run(context.Background(), fixture.arguments(), &output)

	require.NoError(t, err)
	assert.Contains(t, output.String(), "public release contract passed for v0.18.0-beta.6")
}

func TestRunFailsWhenAnonymousTokenIsDenied(t *testing.T) {
	fixture := newRegistryFixture(t)
	fixture.tokenStatus = http.StatusForbidden

	err := run(context.Background(), fixture.arguments(), &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestRunRejectsTagDigestMismatch(t *testing.T) {
	fixture := newRegistryFixture(t)
	arguments := fixture.arguments()
	arguments[len(arguments)-3] = digestFor([]byte("wrong operator"))

	err := run(context.Background(), arguments, &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag resolves to")
}

func TestRunRejectsMissingArm64Image(t *testing.T) {
	fixture := newRegistryFixture(t)
	fixture.initializerManifest.Manifests = fixture.initializerManifest.Manifests[:1]

	err := run(context.Background(), fixture.arguments(), &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no linux/arm64 manifest")
}

func TestRunRejectsNonHelmOCIArtifact(t *testing.T) {
	fixture := newRegistryFixture(t)
	fixture.chartManifest.Layers[0].MediaType = "application/octet-stream"

	err := run(context.Background(), fixture.arguments(), &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Helm chart content layer")
}

func newRegistryFixture(t *testing.T) *registryFixture {
	t.Helper()
	index := manifest{
		MediaType: ociIndexMediaType,
		Manifests: []descriptor{
			{Platform: &platform{OS: "linux", Architecture: "amd64"}},
			{Platform: &platform{OS: "linux", Architecture: "arm64"}},
		},
	}
	fixture := &registryFixture{
		operatorManifest:    index,
		initializerManifest: index,
		chartManifest: manifest{
			Config: descriptor{MediaType: helmConfigMediaType},
			Layers: []descriptor{{MediaType: helmContentMediaType}},
		},
		tokenStatus: http.StatusOK,
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *registryFixture) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/token" {
		fixture.serveToken(writer)
		return
	}
	if request.Header.Get("Authorization") != "Bearer anonymous-token" {
		writer.Header().Set("WWW-Authenticate",
			fmt.Sprintf(`Bearer realm="%s/token",service="test-registry"`, fixture.server.URL))
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	document, found := fixture.manifestForPath(request.URL.Path)
	if !found {
		http.NotFound(writer, request)
		return
	}
	payload, _ := json.Marshal(document)
	writer.Header().Set("Content-Type", mediaTypeFor(request.URL.Path))
	writer.Header().Set("Content-Length", fmt.Sprint(len(payload)))
	writer.Header().Set("Docker-Content-Digest", digestFor(payload))
	if request.Method != http.MethodHead {
		_, _ = writer.Write(payload)
	}
}

func (fixture *registryFixture) serveToken(writer http.ResponseWriter) {
	writer.WriteHeader(fixture.tokenStatus)
	if fixture.tokenStatus == http.StatusOK {
		_ = json.NewEncoder(writer).Encode(map[string]string{"token": "anonymous-token"})
	}
}

func (fixture *registryFixture) manifestForPath(path string) (manifest, bool) {
	switch {
	case strings.Contains(path, "huggingface-initializer"):
		return fixture.initializerManifest, true
	case strings.Contains(path, "charts/"):
		return fixture.chartManifest, true
	case strings.Contains(path, "ckodex-kserve-llm"):
		return fixture.operatorManifest, true
	default:
		return manifest{}, false
	}
}

func (fixture *registryFixture) arguments() []string {
	host := strings.TrimPrefix(fixture.server.URL, "http://")
	operatorPayload, _ := json.Marshal(fixture.operatorManifest)
	initializerPayload, _ := json.Marshal(fixture.initializerManifest)
	return []string{
		"--plain-http",
		"--repository", host + "/ckodex-labs/ckodex-kserve-llm",
		"--chart-repository", host + "/ckodex-labs/charts/ckodex-kserve-llm-operator",
		"--version", "v0.18.0-beta.6",
		"--operator-digest", digestFor(operatorPayload),
		"--initializer-digest", digestFor(initializerPayload),
	}
}

func digestFor(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", sum)
}

func mediaTypeFor(path string) string {
	if strings.Contains(path, "charts/") {
		return ociManifestMediaType
	}
	return ociIndexMediaType
}
