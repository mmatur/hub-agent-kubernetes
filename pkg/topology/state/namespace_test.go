package state

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	acpfake "github.com/traefik/neo-agent/pkg/crd/generated/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func TestFetcher_GetNamespaces(t *testing.T) {
	kubeClient := kubemock.NewSimpleClientset([]runtime.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "myns",
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "otherns",
			},
		},
	}...)
	acpClient := acpfake.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, acpClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getNamespaces()
	require.NoError(t, err)

	sort.Strings(got)

	assert.Equal(t, []string{"myns", "otherns"}, got)
}

func TestFetcher_GetClusterID(t *testing.T) {
	kubeClient := kubemock.NewSimpleClientset([]runtime.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: metav1.NamespaceSystem,
				UID:  "uid",
			},
		},
	}...)
	acpClient := acpfake.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, acpClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getClusterID()
	assert.NoError(t, err)

	assert.Equal(t, "uid", got)
}
