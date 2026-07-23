/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// ---- DefaultVectorConfig --------------------------------------------------------

func TestDefaultVectorConfig_Defaults(t *testing.T) {
	cfg := observability.DefaultVectorConfig()
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "stdout", cfg.SinkType)
	assert.Empty(t, cfg.SinkEndpoint)
}

// ---- BuildVectorConfigMap -------------------------------------------------------

func TestBuildVectorConfigMap_NotNil(t *testing.T) {
	cfg := observability.DefaultVectorConfig()
	cm := observability.BuildVectorConfigMap("my-svc", "prod", "llama3", cfg)
	require.NotNil(t, cm)
	assert.Equal(t, "my-svc-vector-config", cm.Name)
	assert.Equal(t, "prod", cm.Namespace)
	assert.Contains(t, cm.Labels, "app.kubernetes.io/managed-by")
	assert.NotEmpty(t, cm.Data[observability.VectorConfigKey])
}

func TestBuildVectorConfigMap_StdoutSink_ContainsConsole(t *testing.T) {
	cfg := observability.VectorConfig{SinkType: "stdout"}
	cm := observability.BuildVectorConfigMap("svc", "ns", "model", cfg)
	assert.Contains(t, cm.Data[observability.VectorConfigKey], "console")
}

func TestBuildVectorConfigMap_LokiSink_ContainsLoki(t *testing.T) {
	cfg := observability.VectorConfig{
		SinkType:     "loki",
		SinkEndpoint: "http://loki.monitoring:3100",
	}
	cm := observability.BuildVectorConfigMap("svc", "ns", "model", cfg)
	data := cm.Data[observability.VectorConfigKey]
	assert.Contains(t, data, "loki")
	assert.Contains(t, data, "http://loki.monitoring:3100")
	assert.Contains(t, data, "trace_id")
}

func TestBuildVectorConfigMap_ElasticsearchSink_ContainsElasticsearch(t *testing.T) {
	cfg := observability.VectorConfig{
		SinkType:     "elasticsearch",
		SinkEndpoint: "http://es.logging:9200",
	}
	cm := observability.BuildVectorConfigMap("svc", "ns", "model", cfg)
	data := cm.Data[observability.VectorConfigKey]
	assert.Contains(t, data, "elasticsearch")
	assert.Contains(t, data, "http://es.logging:9200")
}

func TestBuildVectorConfigMap_OTLPSink_ContainsOpentelemetry(t *testing.T) {
	cfg := observability.VectorConfig{
		SinkType:     "otlp",
		SinkEndpoint: "http://otel-collector:4318",
	}
	cm := observability.BuildVectorConfigMap("svc", "ns", "model", cfg)
	data := cm.Data[observability.VectorConfigKey]
	assert.Contains(t, data, "opentelemetry")
	assert.Contains(t, data, "protocol:")
	assert.Contains(t, data, "type: http")
	assert.Contains(t, data, "uri: \"http://otel-collector:4318\"")
	assert.Contains(t, data, "max_events: 1")
	assert.Contains(t, data, "codec: json")
}

func TestBuildVectorConfigMap_ModelNameInjected(t *testing.T) {
	cfg := observability.DefaultVectorConfig()
	cm := observability.BuildVectorConfigMap("svc", "ns", "llama3-70b", cfg)
	assert.Contains(t, cm.Data[observability.VectorConfigKey], "llama3-70b")
}

// ---- InjectVectorSidecar -------------------------------------------------------

func emptyPodSpec() *corev1.PodSpec {
	return &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "app", Image: "app:latest"},
		},
	}
}

func TestInjectVectorSidecar_AddsVectorContainer(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "my-svc-vector-config")

	names := make([]string, 0, len(spec.Containers))
	for _, c := range spec.Containers {
		names = append(names, c.Name)
	}
	assert.Contains(t, names, "vector")
}

func TestInjectVectorSidecar_AddsSharedLogVolume(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "cfg")

	var found bool
	for _, v := range spec.Volumes {
		if v.Name == observability.VectorLogVolumeName {
			found = true
		}
	}
	assert.True(t, found, "shared-logs volume must be present")
}

func TestInjectVectorSidecar_AddsWritableDataVolume(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "cfg")

	var found bool
	for _, v := range spec.Volumes {
		if v.Name == observability.VectorDataVolumeName {
			found = true
		}
	}
	assert.True(t, found, "vector-data volume must be present")
}

func TestInjectVectorSidecar_AddsVectorConfigVolume(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "my-vector-cm")

	var found bool
	for _, v := range spec.Volumes {
		if v.Name == "vector-config" {
			found = true
			assert.Equal(t, "my-vector-cm", v.ConfigMap.Name)
			// DefaultMode must equal the API server default (0644) so the
			// reconciler's desired volume matches the persisted (defaulted) one;
			// otherwise the Deployment loops forever on "volumes changed".
			if assert.NotNil(t, v.ConfigMap.DefaultMode, "vector-config DefaultMode must be set") {
				assert.Equal(t, int32(0o644), *v.ConfigMap.DefaultMode)
			}
		}
	}
	assert.True(t, found, "vector-config volume must be present")
}

func TestInjectVectorSidecar_MountsSharedLogVolumeOnFirstContainer(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "cfg")

	var found bool
	for _, m := range spec.Containers[0].VolumeMounts {
		if m.Name == observability.VectorLogVolumeName {
			found = true
			assert.Equal(t, observability.VectorLogMountPath, m.MountPath)
		}
	}
	assert.True(t, found, "app container must have shared-logs volume mount")
}

func TestInjectVectorSidecar_Idempotent_NoDuplicates(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "cfg")
	observability.InjectVectorSidecar(spec, "cfg") // second call

	vectorCount := 0
	for _, c := range spec.Containers {
		if c.Name == "vector" {
			vectorCount++
		}
	}
	assert.Equal(t, 1, vectorCount, "vector container must appear exactly once")

	logVolCount := 0
	for _, v := range spec.Volumes {
		if v.Name == observability.VectorLogVolumeName {
			logVolCount++
		}
	}
	assert.Equal(t, 1, logVolCount, "shared-logs volume must appear exactly once")

	dataVolCount := 0
	for _, v := range spec.Volumes {
		if v.Name == observability.VectorDataVolumeName {
			dataVolCount++
		}
	}
	assert.Equal(t, 1, dataVolCount, "vector-data volume must appear exactly once")
}

func TestInjectVectorSidecar_EmptyPodSpec_NoPanic(t *testing.T) {
	spec := &corev1.PodSpec{} // no containers
	assert.NotPanics(t, func() {
		observability.InjectVectorSidecar(spec, "cfg")
	})
}

func TestInjectVectorSidecar_VectorContainerHasResourceLimits(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "cfg")

	for _, c := range spec.Containers {
		if c.Name == "vector" {
			assert.NotEmpty(t, c.Resources.Requests)
			assert.NotEmpty(t, c.Resources.Limits)
			return
		}
	}
	t.Fatal("vector container not found")
}

func TestInjectVectorSidecar_VectorContainerUsesExpectedImage(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "cfg")

	for _, c := range spec.Containers {
		if c.Name == "vector" {
			assert.Equal(t, observability.VectorImage, c.Image)
			return
		}
	}
	t.Fatal("vector container not found")
}

func TestInjectVectorSidecar_VectorContainerMountsWritableDataDir(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "cfg")

	for _, c := range spec.Containers {
		if c.Name != "vector" {
			continue
		}
		for _, m := range c.VolumeMounts {
			if m.Name == observability.VectorDataVolumeName {
				assert.Equal(t, observability.VectorDataMountPath, m.MountPath)
				assert.False(t, m.ReadOnly)
				return
			}
		}
		t.Fatal("vector-data volume mount not found")
	}
	t.Fatal("vector container not found")
}

func TestInjectVectorSidecar_VectorContainerRunsAsNonRoot(t *testing.T) {
	spec := emptyPodSpec()
	observability.InjectVectorSidecar(spec, "cfg")

	for _, c := range spec.Containers {
		if c.Name == "vector" {
			require.NotNil(t, c.SecurityContext)
			require.NotNil(t, c.SecurityContext.RunAsUser)
			require.NotNil(t, c.SecurityContext.RunAsGroup)
			assert.True(t, *c.SecurityContext.RunAsNonRoot)
			assert.Equal(t, int64(65532), *c.SecurityContext.RunAsUser)
			assert.Equal(t, int64(65532), *c.SecurityContext.RunAsGroup)
			return
		}
	}
	t.Fatal("vector container not found")
}

// ---- NewVectorLogger / Log / Close -------------------------------------------

// startEchoServer starts a local TCP server that reads one newline-terminated
// message and then closes the connection. Returns the address it binds to.
func startEchoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("TCP binding unavailable in this environment: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		buf := make([]byte, 4096)
		_, _ = conn.Read(buf)
	}()

	return ln.Addr().String()
}

func TestNewVectorLogger_NotNil(t *testing.T) {
	l := observability.NewVectorLogger("127.0.0.1:9999", "llama3")
	assert.NotNil(t, l)
}

func TestVectorLogger_Log_SuccessWithServer(t *testing.T) {
	addr := startEchoServer(t)
	l := observability.NewVectorLogger(addr, "llama3")

	err := l.Log("info", "test message", map[string]any{"tenant": "acme"})
	assert.NoError(t, err)

	assert.NoError(t, l.Close())
}

func TestVectorLogger_Log_ConnectionRefused_ReturnsError(t *testing.T) {
	// Use a port that is not listening.
	l := observability.NewVectorLogger("127.0.0.1:1", "llama3")
	err := l.Log("info", "will fail", nil)
	assert.Error(t, err)
}

func TestVectorLogger_Close_WithoutLog_NoPanic(t *testing.T) {
	l := observability.NewVectorLogger("127.0.0.1:9999", "model")
	assert.NotPanics(t, func() {
		_ = l.Close()
	})
}

func TestVectorLogger_Close_WithoutLog_ReturnsNil(t *testing.T) {
	l := observability.NewVectorLogger("127.0.0.1:9999", "model")
	err := l.Close()
	assert.NoError(t, err)
}

func TestVectorLogger_Log_EmptyFields_NoPanic(t *testing.T) {
	addr := startEchoServer(t)
	l := observability.NewVectorLogger(addr, "llama3")

	assert.NotPanics(t, func() {
		_ = l.Log("debug", "no fields", nil)
	})
	_ = l.Close()
}
