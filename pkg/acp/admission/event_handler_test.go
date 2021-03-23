package admission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	neov1alpha1 "github.com/traefik/neo-agent/pkg/crd/api/neo/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type fakeUpdater struct {
	policies []string
}

func (f *fakeUpdater) Update(polName string) {
	f.policies = append(f.policies, polName)
}

func createPolicy(uid, name, ns string, sah bool) *neov1alpha1.AccessControlPolicy {
	return &neov1alpha1.AccessControlPolicy{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid), Name: name, Namespace: ns},
		Spec: neov1alpha1.AccessControlPolicySpec{
			JWT: &neov1alpha1.AccessControlPolicyJWT{
				SigningSecret:            "secret",
				StripAuthorizationHeader: sah,
			},
		},
	}
}

func TestEventHandler_OnAdd(t *testing.T) {
	updater := fakeUpdater{}

	handler := NewEventHandler(&updater)

	handler.OnAdd(createPolicy("1", "my-policy-1", "foo", false))
	handler.OnAdd(createPolicy("2", "my-policy-2", "bar", false))

	expected := []string{"my-policy-1@foo", "my-policy-2@bar"}

	assert.Equal(t, expected, updater.policies)
}

func TestEventHandler_OnDelete(t *testing.T) {
	updater := fakeUpdater{}

	handler := NewEventHandler(&updater)

	handler.OnDelete(createPolicy("1", "my-policy-1", "foo", false))
	handler.OnDelete(createPolicy("2", "my-policy-2", "bar", false))

	expected := []string{"my-policy-1@foo", "my-policy-2@bar"}

	assert.Equal(t, expected, updater.policies)
}

func TestEventHandler_OnUpdate(t *testing.T) {
	updater := fakeUpdater{}

	handler := NewEventHandler(&updater)

	handler.OnUpdate(
		createPolicy("1", "my-policy-1", "foo", false),
		createPolicy("1", "my-policy-1", "foo", true),
	)

	handler.OnUpdate(
		createPolicy("2", "my-policy-2", "bar", false),
		createPolicy("2", "my-policy-2", "bar", false),
	)

	expected := []string{"my-policy-1@foo"}

	assert.Equal(t, expected, updater.policies)
}
