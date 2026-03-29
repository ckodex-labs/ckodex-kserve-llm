/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	// VectorImage is the Vector container image for log aggregation.
	VectorImage = "timberio/vector:0.43.1-distroless-static"

	// VectorConfigKey is the ConfigMap key for the Vector pipeline config.
	VectorConfigKey = "vector.yaml"

	// VectorLogVolumeName is the shared log volume between app and Vector sidecar.
	VectorLogVolumeName = "shared-logs"

	// VectorLogMountPath is the log directory in the app container.
	VectorLogMountPath = "/var/log/ckodex"
)

// VectorConfig defines Vector sidecar injection settings.
type VectorConfig struct {
	// Enabled controls whether Vector sidecar is injected.
	Enabled bool `json:"enabled"`

	// SinkType is the log destination ("loki", "elasticsearch", "otlp", "stdout").
	// "otlp" forwards logs as OTLP/HTTP to an OTel Collector, enabling trace↔log
	// correlation in Grafana Tempo + Loki via shared trace_id / span_id fields.
	SinkType string `json:"sinkType"`

	// SinkEndpoint is the sink's address (e.g., "http://loki.monitoring:3100").
	// For "otlp": the OTel Collector OTLP/HTTP endpoint, e.g. "http://otel-collector:4318".
	SinkEndpoint string `json:"sinkEndpoint"`

	// ExtraLabels are added to every log event.
	ExtraLabels map[string]string `json:"extraLabels,omitempty"`
}

// DefaultVectorConfig returns production defaults.
func DefaultVectorConfig() VectorConfig {
	return VectorConfig{
		Enabled:      false,
		SinkType:     "stdout",
		SinkEndpoint: "",
	}
}

// BuildVectorConfigMap generates a ConfigMap with the Vector pipeline YAML.
func BuildVectorConfigMap(name, namespace, modelName string, cfg VectorConfig) *corev1.ConfigMap {
	pipeline := buildVectorPipeline(modelName, cfg)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-vector-config",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
				"serving.ckodex.com/role":      "vector-config",
			},
		},
		Data: map[string]string{
			VectorConfigKey: pipeline,
		},
	}
}

// buildVectorPipeline generates the Vector YAML configuration with full OTel
// trace correlation. The pipeline:
//  1. Reads JSON logs from the shared volume.
//  2. Parses and enriches each event with model/tenant/cluster identity.
//  3. Extracts trace_id + span_id from the log body for Grafana Tempo ↔ Loki correlation.
//  4. Filters debug noise.
//  5. Routes to the configured sink (loki, elasticsearch, otlp, stdout).
func buildVectorPipeline(modelName string, cfg VectorConfig) string {
	sink := buildSinkConfig(cfg)

	return fmt.Sprintf(`# Auto-generated Vector pipeline for model: %s
# OTel trace correlation: trace_id + span_id extracted from log body and promoted
# to top-level fields so Grafana can link Loki log lines to Tempo traces.
sources:
  app_logs:
    type: file
    include:
      - "%s/*.log"
      - "%s/*.json"
    read_from: beginning

transforms:
  parse_json:
    type: remap
    inputs: ["app_logs"]
    source: |
      . = parse_json!(.message) ?? .
      .model = "%s"
      .component = "llm-inference"
      .timestamp = now()

  enrich:
    type: remap
    inputs: ["parse_json"]
    source: |
      .environment = get_env_var("ENVIRONMENT") ?? "production"
      .cluster     = get_env_var("CLUSTER_NAME") ?? "unknown"
      .tenant_id   = get_env_var("CKODEX_TENANT_ID") ?? "unknown"
      .namespace   = get_env_var("NAMESPACE") ?? "default"

  # Promote OTel trace context fields to the log root so sinks can index them.
  # Supports both flat (trace_id) and nested (span.trace_id) structures emitted
  # by the OTel Go SDK slog bridge and by vLLM's structured logger.
  otel_correlation:
    type: remap
    inputs: ["enrich"]
    source: |
      if exists(.span) {
        .trace_id = string(.span.trace_id) ?? ""
        .span_id  = string(.span.span_id)  ?? ""
        del(.span)
      }
      if exists(.traceId) {
        .trace_id = string(.traceId) ?? .trace_id
        .span_id  = string(.spanId)  ?? .span_id
        del(.traceId)
        del(.spanId)
      }
      # Normalise trace_id to lowercase 32-hex for Grafana Tempo link format
      .trace_id = downcase(string(.trace_id) ?? "")
      .span_id  = downcase(string(.span_id)  ?? "")

  filter_noise:
    type: filter
    inputs: ["otel_correlation"]
    condition:
      type: vrl
      source: '.level != "debug" && .level != "trace"'

%s
`, modelName, VectorLogMountPath, VectorLogMountPath, modelName, sink)
}

// buildSinkConfig generates the sink section for the Vector pipeline.
func buildSinkConfig(cfg VectorConfig) string {
	switch cfg.SinkType {
	case "loki":
		// Loki sink: include trace_id + span_id as structured metadata fields so
		// Grafana can render a "View trace in Tempo" button on each log line.
		return fmt.Sprintf(`sinks:
  loki:
    type: loki
    inputs: ["filter_noise"]
    endpoint: "%s"
    labels:
      app: ckodex-llm
      model: "{{ model }}"
      tenant_id: "{{ tenant_id }}"
      namespace: "{{ namespace }}"
      source: vector
    structured_metadata:
      trace_id: "{{ trace_id }}"
      span_id: "{{ span_id }}"
    encoding:
      codec: json`, cfg.SinkEndpoint)

	case "elasticsearch":
		return fmt.Sprintf(`sinks:
  elasticsearch:
    type: elasticsearch
    inputs: ["filter_noise"]
    endpoints:
      - "%s"
    bulk:
      index: "ckodex-llm-%%Y.%%m.%%d"
    encoding:
      codec: json`, cfg.SinkEndpoint)

	case "otlp":
		// OTLP/HTTP logs sink — forwards to an OTel Collector which can fan-out
		// to Tempo (traces), Loki (logs), and Prometheus (metrics) from a single pipeline.
		// trace_id and span_id become OTLP LogRecord.trace_id / span_id, enabling
		// native Grafana Explore trace↔log correlation without any configuration.
		return fmt.Sprintf(`sinks:
  otlp:
    type: opentelemetry
    inputs: ["filter_noise"]
    endpoint: "%s"
    protocol: http
    encoding:
      codec: json
    resource:
      service.name: ckodex-llm-operator
      service.namespace: ckodex`, cfg.SinkEndpoint)

	default: // stdout
		return `sinks:
  console:
    type: console
    inputs: ["filter_noise"]
    encoding:
      codec: json`
	}
}

// InjectVectorSidecar adds a Vector sidecar container and shared log volume
// to a PodSpec for structured log collection.
func InjectVectorSidecar(podSpec *corev1.PodSpec, configMapName string) {
	// Add shared log volume
	hasLogVolume := false
	for _, v := range podSpec.Volumes {
		if v.Name == VectorLogVolumeName {
			hasLogVolume = true
			break
		}
	}
	if !hasLogVolume {
		podSpec.Volumes = append(podSpec.Volumes,
			corev1.Volume{
				Name: VectorLogVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			corev1.Volume{
				Name: "vector-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					},
				},
			},
		)
	}

	// Mount shared log volume on first app container
	if len(podSpec.Containers) > 0 {
		hasMount := false
		for _, m := range podSpec.Containers[0].VolumeMounts {
			if m.Name == VectorLogVolumeName {
				hasMount = true
				break
			}
		}
		if !hasMount {
			podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
				Name:      VectorLogVolumeName,
				MountPath: VectorLogMountPath,
			})
		}
	}

	// Add Vector sidecar container
	hasVector := false
	for _, c := range podSpec.Containers {
		if c.Name == "vector" {
			hasVector = true
			break
		}
	}
	if !hasVector {
		podSpec.Containers = append(podSpec.Containers, corev1.Container{
			Name:  "vector",
			Image: VectorImage,
			Args:  []string{"--config-dir", "/etc/vector"},
			VolumeMounts: []corev1.VolumeMount{
				{Name: VectorLogVolumeName, MountPath: VectorLogMountPath, ReadOnly: true},
				{Name: "vector-config", MountPath: "/etc/vector", ReadOnly: true},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
			SecurityContext: &corev1.SecurityContext{
				RunAsNonRoot:             ptr.To(true),
				AllowPrivilegeEscalation: ptr.To(false),
				ReadOnlyRootFilesystem:   ptr.To(true),
			},
		})
	}
}

// ----- High-Throughput Socket Sink -----

// VectorLogger provides a thread-safe structured logger that streams to Vector.
type VectorLogger struct {
	mu    sync.Mutex
	conn  net.Conn
	addr  string
	model string
}

// NewVectorLogger creates a logger that writes to a Vector socket.
func NewVectorLogger(addr, model string) *VectorLogger {
	return &VectorLogger{
		addr:  addr,
		model: model,
	}
}

// Log sends a structured log event to Vector.
func (l *VectorLogger) Log(level, message string, fields map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		var err error
		l.conn, err = net.DialTimeout("tcp", l.addr, 2*time.Second)
		if err != nil {
			return fmt.Errorf("connect to vector: %w", err)
		}
	}

	event := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"level":     level,
		"message":   message,
		"model":     l.model,
	}
	for k, v := range fields {
		event[k] = v
	}

	data, _ := json.Marshal(event)
	_, err := fmt.Fprintf(l.conn, "%s\n", data)
	if err != nil {
		l.conn.Close()
		l.conn = nil
		return fmt.Errorf("write to vector: %w", err)
	}

	return nil
}

// Close closes the logger connection.
func (l *VectorLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn != nil {
		err := l.conn.Close()
		l.conn = nil
		return err
	}
	return nil
}
