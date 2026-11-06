// Command deployment-assessment checks explicit container runAsNonRoot settings.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

const maxInputBytes = 8 << 20

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (retErr error) {
	path := flag.String("input", "", "Deployment JSON or YAML file; - reads stdin")
	flag.Parse()
	if *path == "" || flag.NArg() != 0 {
		return errors.New("specify exactly one --input file")
	}
	var reader io.Reader = os.Stdin
	if *path != "-" {
		f, err := os.Open(*path)
		if err != nil {
			return fmt.Errorf("open deployment: %w", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close deployment: %w", err))
			}
		}()
		reader = f
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxInputBytes+1))
	if err != nil {
		return fmt.Errorf("read deployment: %w", err)
	}
	if len(data) > maxInputBytes {
		return errors.New("deployment exceeds 8 MiB")
	}
	deployment, err := decodeDeployment(data)
	if err != nil {
		return err
	}
	if err := checkDeployment(deployment); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{
		"check": "container-run-as-non-root", "deployment": deployment.Name,
		"namespace": deployment.Namespace, "result": "passed",
	})
}

func decodeDeployment(data []byte) (*appsv1.Deployment, error) {
	reader := yamlutil.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	doc, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read deployment document: %w", err)
	}
	if _, err := reader.Read(); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("read additional document: %w", err)
		}
		return nil, errors.New("expected exactly one deployment document")
	}
	data, err = yaml.YAMLToJSONStrict(doc)
	if err != nil {
		return nil, fmt.Errorf("decode deployment YAML: %w", err)
	}
	var deployment appsv1.Deployment
	if err := json.Unmarshal(data, &deployment); err != nil {
		return nil, fmt.Errorf("decode deployment: %w", err)
	}
	if deployment.APIVersion != "apps/v1" || deployment.Kind != "Deployment" || deployment.Name == "" {
		return nil, errors.New("expected an apps/v1 Deployment with metadata.name")
	}
	return &deployment, nil
}

func checkDeployment(deployment *appsv1.Deployment) error {
	pod := deployment.Spec.Template.Spec
	if len(pod.Containers) == 0 {
		return errors.New("deployment has no containers")
	}
	for _, group := range []struct {
		name       string
		containers []corev1.Container
	}{{"container", pod.Containers}, {"init container", pod.InitContainers}} {
		for _, container := range group.containers {
			if container.Name == "" {
				return fmt.Errorf("%s has no name", group.name)
			}
			if container.SecurityContext == nil || container.SecurityContext.RunAsNonRoot == nil || !*container.SecurityContext.RunAsNonRoot {
				return fmt.Errorf("%s %q must explicitly set securityContext.runAsNonRoot to true", group.name, container.Name)
			}
		}
	}
	return nil
}
