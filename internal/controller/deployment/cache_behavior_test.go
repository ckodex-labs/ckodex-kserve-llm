package deployment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHardwareCacheGetRefreshesAndThenUsesCachedValue(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu"}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")}}}
	client := fake.NewClientBuilder().WithObjects(node).Build()
	cache := &HardwareCache{}
	assert.Equal(t, HardwareNVIDIA, cache.Get(context.Background(), client, nil))
	assert.Equal(t, HardwareNVIDIA, cache.Get(context.Background(), client, nil))
	assert.Equal(t, corev1.HostPathDirectory, *PtrToHostPath(corev1.HostPathDirectory))
}

func TestHardwareCachePrefersReaderAndReturnsCachedValueOnListError(t *testing.T) {
	reader := &nodeReader{err: errors.New("list failed")}
	cache := &HardwareCache{hardware: HardwareAMD, cacheTime: time.Now().Add(-hardwareCacheTTL)}
	assert.Equal(t, HardwareAMD, cache.Get(context.Background(), fake.NewClientBuilder().Build(), reader))
}

type nodeReader struct{ err error }

func (r *nodeReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return r.err
}
func (r *nodeReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return r.err
}
