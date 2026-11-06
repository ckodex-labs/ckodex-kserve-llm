// Command oscal-assessment evaluates the operator's control checks against
// supplied Kubernetes resources or a live Kubernetes API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	inputPath := flag.String("input", "", "JSON resource snapshot")
	live := flag.Bool("live", false, "read resources from the Kubernetes API")
	kubeconfig := flag.String("kubeconfig", "", "kubeconfig path (defaults to Kubernetes client configuration)")
	control := flag.String("control", "", "one control ID; default evaluates all")
	identityMode := flag.String("identity-mode", "", "caller-asserted IA-9 identity mode: direct or vcluster")
	hostNamespace := flag.String("host-namespace", "", "caller-asserted vcluster physical host namespace")
	flag.Parse()
	if (*inputPath == "") == !*live {
		return errors.New("select exactly one of --input or --live")
	}
	if !*live && *kubeconfig != "" {
		return errors.New("--kubeconfig requires --live")
	}
	if *control != "" && len(evaluate(input{}, *control)) == 0 {
		return fmt.Errorf("unknown control %q", *control)
	}
	identity := identityContext{Mode: *identityMode, HostNamespace: *hostNamespace}
	if err := validateIdentityForControl(*control, identity); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var in input
	var err error
	if *live {
		in, err = readCluster(ctx, *kubeconfig, *control)
	} else {
		in, err = readSnapshot(*inputPath)
	}
	if err != nil {
		return err
	}
	if !*live {
		if err := validateSnapshotSources(in, *control); err != nil {
			return err
		}
	}
	if *control == "" || *control == "ac-4" {
		if err := validatePolicyIdentities(in.Policies); err != nil {
			return err
		}
	}
	in.Identity = identity
	results := evaluate(in, *control)
	if len(results) == 0 {
		return fmt.Errorf("unknown control %q", *control)
	}
	if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
		return fmt.Errorf("write results: %w", err)
	}
	for _, result := range results {
		if !result.Passed {
			return errors.New("one or more controls did not pass")
		}
	}
	return nil
}

// A non-nil slice represents an explicitly supplied JSON array, including [].
func validateSnapshotSources(in input, control string) error {
	if in.Services == nil {
		return errors.New("snapshot requires a non-null llmServices array")
	}
	if (control == "" || control == "ac-4") && in.Policies == nil {
		return errors.New("snapshot requires a non-null networkPolicies array")
	}
	if (control == "" || control == "ia-9") && in.Entries == nil {
		return errors.New("snapshot requires a non-null spireEntries array")
	}
	return nil
}

func validatePolicyIdentities(policies []object) error {
	for i, policy := range policies {
		if strings.TrimSpace(policy.Metadata.Name) == "" || strings.TrimSpace(policy.Metadata.Namespace) == "" {
			return fmt.Errorf("NetworkPolicy at index %d is missing name or namespace", i)
		}
	}
	return nil
}

func readSnapshot(path string) (_ input, retErr error) {
	f, err := os.Open(path) // #nosec G304 -- The operator explicitly supplies the snapshot path.
	if err != nil {
		return input{}, fmt.Errorf("open resource snapshot: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close resource snapshot: %w", err))
		}
	}()
	data, err := io.ReadAll(io.LimitReader(f, 32<<20+1))
	if err != nil {
		return input{}, fmt.Errorf("read resource snapshot: %w", err)
	}
	if len(data) > 32<<20 {
		return input{}, errors.New("resource snapshot exceeds 32 MiB")
	}
	var in input
	if err := unmarshalUniqueJSON(data, &in); err != nil {
		return input{}, fmt.Errorf("decode resource snapshot: %w", err)
	}
	return in, nil
}

func readCluster(ctx context.Context, kubeconfig, control string) (input, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return input{}, fmt.Errorf("load Kubernetes configuration: %w", err)
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return input{}, fmt.Errorf("create Kubernetes client: %w", err)
	}
	var in input
	type resourceRequest struct {
		gvr       schema.GroupVersionResource
		namespace string
		dest      *[]object
	}
	resources := []resourceRequest{
		{schema.GroupVersionResource{Group: "serving.ckodex.com", Version: "v1alpha2", Resource: "llminferenceservices"}, "", &in.Services},
	}
	if control == "" || control == "ac-4" {
		resources = append(resources, resourceRequest{schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, "", &in.Policies})
	}
	if control == "" || control == "ia-9" {
		resources = append(resources, resourceRequest{schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, "spire", &in.Entries})
	}
	for _, resource := range resources {
		list, err := client.Resource(resource.gvr).Namespace(resource.namespace).List(ctx, metav1.ListOptions{Limit: 5000})
		if err != nil {
			return input{}, fmt.Errorf("list %s: %w", resource.gvr.Resource, err)
		}
		if list.GetContinue() != "" {
			return input{}, fmt.Errorf("list %s exceeds 5000 resources", resource.gvr.Resource)
		}
		data, err := json.Marshal(list.Items)
		if err != nil {
			return input{}, fmt.Errorf("encode %s: %w", resource.gvr.Resource, err)
		}
		if err := json.Unmarshal(data, resource.dest); err != nil {
			return input{}, fmt.Errorf("decode %s: %w", resource.gvr.Resource, err)
		}
	}
	return in, nil
}
