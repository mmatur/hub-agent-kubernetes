/*
Copyright (C) 2022-2023 Traefik Labs

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package admission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

type fakeUpdater struct {
	policies []string
}

func (f *fakeUpdater) Update(polName string) {
	f.policies = append(f.policies, polName)
}

func createPolicy(uid, name string, sah bool) *hubv1alpha1.AccessControlPolicy {
	return &hubv1alpha1.AccessControlPolicy{
		ObjectMeta: metav1.ObjectMeta{UID: ktypes.UID(uid), Name: name},
		Spec: hubv1alpha1.AccessControlPolicySpec{
			JWT: &hubv1alpha1.AccessControlPolicyJWT{
				SigningSecret:            "secret",
				StripAuthorizationHeader: sah,
			},
		},
	}
}

func TestEventHandler_OnAdd(t *testing.T) {
	updater := fakeUpdater{}

	handler := NewEventHandler(&updater)

	handler.OnAdd(createPolicy("1", "my-policy-1", false))
	handler.OnAdd(createPolicy("2", "my-policy-2", false))

	expected := []string{"my-policy-1", "my-policy-2"}

	assert.Equal(t, expected, updater.policies)
}

func TestEventHandler_OnDelete(t *testing.T) {
	updater := fakeUpdater{}

	handler := NewEventHandler(&updater)

	handler.OnDelete(createPolicy("1", "my-policy-1", false))
	handler.OnDelete(createPolicy("2", "my-policy-2", false))

	expected := []string{"my-policy-1", "my-policy-2"}

	assert.Equal(t, expected, updater.policies)
}

func TestEventHandler_OnUpdate(t *testing.T) {
	updater := fakeUpdater{}

	handler := NewEventHandler(&updater)

	handler.OnUpdate(
		createPolicy("1", "my-policy-1", false),
		createPolicy("1", "my-policy-1", true),
	)

	handler.OnUpdate(
		createPolicy("2", "my-policy-2", false),
		createPolicy("2", "my-policy-2", false),
	)

	expected := []string{"my-policy-1"}

	assert.Equal(t, expected, updater.policies)
}
