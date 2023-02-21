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

package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func TestWatcher_CreateOIDCACP(t *testing.T) {
	var data string
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(data))
	}))
	t.Cleanup(srv.Close)

	data = fmt.Sprintf(`{"issuer":%q}`, srv.URL)

	switcher := NewHandlerSwitcher()

	// Initialize this client set with the required "hub-secret".
	kubeClientSet := kubemock.NewSimpleClientset(
		createSecret("default", "hub-secret", "key", "1234567891234567"),
	)
	hubClientSet := hubkubemock.NewSimpleClientset()
	startWatcher(t, switcher, kubeClientSet, hubClientSet)

	// Create an ACP referencing a secret that does not exist yet.
	_, err := hubClientSet.HubV1alpha1().AccessControlPolicies().Create(
		context.Background(),
		createOIDCPolicy("1", "my-oidc", srv.URL, &corev1.SecretReference{Namespace: "ns", Name: "secret"}),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// ACP has been created, but not the secret it references, so calling the handler should give a 404.
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/my-oidc", nil)

	switcher.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusNotFound, rw.Code)

	// Now, create the missing secret, it should be processed by the watcher and the ACP handler should be created.
	_, err = kubeClientSet.CoreV1().Secrets("ns").Create(
		context.Background(),
		createSecret("ns", "secret", "clientSecret", "secret"),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	// Give it some time, just to be sure.
	time.Sleep(10 * time.Millisecond)

	// Calling the ACP endpoint should now work.
	rw = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/my-oidc", nil)

	switcher.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusFound, rw.Code)
}

func TestWatcher_CreateACP(t *testing.T) {
	switcher := NewHandlerSwitcher()

	// Initialize this client set with the required "hub-secret".
	kubeClientSet := kubemock.NewSimpleClientset(
		createSecret("default", "hub-secret", "key", "1234567891234567"),
	)
	hubClientSet := hubkubemock.NewSimpleClientset()
	startWatcher(t, switcher, kubeClientSet, hubClientSet)

	_, err := hubClientSet.HubV1alpha1().AccessControlPolicies().Create(
		context.Background(),
		createPolicy("1", "my-policy-1"),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)
	_, err = hubClientSet.HubV1alpha1().AccessControlPolicies().Create(
		context.Background(),
		createPolicy("2", "my-policy-2"),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)
	_, err = hubClientSet.HubV1alpha1().AccessControlPolicies().Create(
		context.Background(),
		createPolicy("3", "my-policy-3"),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	tests := []struct {
		desc     string
		path     string
		jwt      string
		expected int
	}{
		{
			desc:     "my-policy-1",
			path:     "/my-policy-1",
			expected: http.StatusUnauthorized,
		},
		{
			desc:     "my-policy-1",
			path:     "/my-policy-1",
			jwt:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.XbPfbIHMI6arZ3Y922BhjWgQzWXcXNrz0ogtVhfEd2o",
			expected: http.StatusOK,
		},

		{
			desc:     "my-policy-2",
			path:     "/my-policy-2",
			expected: http.StatusUnauthorized,
		},
		{
			desc:     "my-policy-3",
			path:     "/my-policy-3",
			expected: http.StatusUnauthorized,
		},
		{
			desc:     "unknown resource",
			path:     "/my-policy",
			expected: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://localhost"+test.path, nil)

			if test.jwt != "" {
				req.Header.Set("Authorization", "Bearer "+test.jwt)
			}

			switcher.ServeHTTP(rw, req)

			assert.Equal(t, test.expected, rw.Code)
		})
	}
}

func TestWatcher_UpdateACP(t *testing.T) {
	switcher := NewHandlerSwitcher()

	// Initialize this client set with the required "hub-secret".
	kubeClientSet := kubemock.NewSimpleClientset(
		createSecret("default", "hub-secret", "key", "1234567891234567"),
	)
	hubClientSet := hubkubemock.NewSimpleClientset()
	startWatcher(t, switcher, kubeClientSet, hubClientSet)

	_, err := hubClientSet.HubV1alpha1().AccessControlPolicies().Create(
		context.Background(),
		createPolicyWithSecret("1", "my-policy-1", "Wrong secret"),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	_, err = hubClientSet.HubV1alpha1().AccessControlPolicies().Update(
		context.Background(),
		createPolicy("1", "my-policy-1"),
		metav1.UpdateOptions{},
	)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	tests := []struct {
		desc     string
		path     string
		jwt      string
		expected int
	}{
		{
			desc:     "my-policy-1",
			path:     "/my-policy-1",
			jwt:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.XbPfbIHMI6arZ3Y922BhjWgQzWXcXNrz0ogtVhfEd2o",
			expected: http.StatusOK,
		},
		{
			desc:     "unknown resource",
			path:     "/my-policy",
			expected: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://localhost"+test.path, nil)

			if test.jwt != "" {
				req.Header.Set("Authorization", "Bearer "+test.jwt)
			}

			switcher.ServeHTTP(rw, req)

			assert.Equal(t, test.expected, rw.Code)
		})
	}
}

func TestWatcher_DeleteACP(t *testing.T) {
	switcher := NewHandlerSwitcher()

	// Initialize this client set with the required "hub-secret".
	kubeClientSet := kubemock.NewSimpleClientset(
		createSecret("default", "hub-secret", "key", "1234567891234567"),
	)
	hubClientSet := hubkubemock.NewSimpleClientset()
	startWatcher(t, switcher, kubeClientSet, hubClientSet)

	_, err := hubClientSet.HubV1alpha1().AccessControlPolicies().Create(
		context.Background(),
		createPolicy("1", "my-policy-1"),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)
	_, err = hubClientSet.HubV1alpha1().AccessControlPolicies().Create(
		context.Background(),
		createPolicy("2", "my-policy-2"),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)
	_, err = hubClientSet.HubV1alpha1().AccessControlPolicies().Create(
		context.Background(),
		createPolicy("3", "my-policy-3"),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	err = hubClientSet.HubV1alpha1().AccessControlPolicies().Delete(
		context.Background(),
		"my-policy-1",
		metav1.DeleteOptions{},
	)
	require.NoError(t, err)
	err = hubClientSet.HubV1alpha1().AccessControlPolicies().Delete(
		context.Background(),
		"my-policy-2",
		metav1.DeleteOptions{},
	)
	require.NoError(t, err)
	err = hubClientSet.HubV1alpha1().AccessControlPolicies().Delete(
		context.Background(),
		"my-policy-3",
		metav1.DeleteOptions{},
	)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	tests := []struct {
		desc     string
		path     string
		expected int
	}{
		{
			desc:     "my-policy-1",
			path:     "/my-policy-1",
			expected: http.StatusNotFound,
		},
		{
			desc:     "my-policy-2",
			path:     "/my-policy-2",
			expected: http.StatusNotFound,
		},
		{
			desc:     "my-policy-3",
			path:     "/my-policy-3@foo",
			expected: http.StatusNotFound,
		},
		{
			desc:     "unknown resource",
			path:     "/my-policy",
			expected: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://localhost"+test.path, nil)

			switcher.ServeHTTP(rw, req)

			assert.Equal(t, test.expected, rw.Code)
		})
	}
}

func startWatcher(t *testing.T, switcher *HTTPHandlerSwitcher, kubeClientSet *kubemock.Clientset, hubClientSet *hubkubemock.Clientset) {
	t.Helper()

	hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 5*time.Minute)
	kubeInformer := informers.NewSharedInformerFactory(kubeClientSet, 5*time.Minute)

	watcher := NewWatcher(
		switcher,
		hubInformer.Hub().V1alpha1().AccessControlPolicies().Lister(),
		acp.NewKubeSecretValueGetter(kubeInformer.Core().V1().Secrets().Lister()),
	)

	_, err := hubInformer.Hub().V1alpha1().AccessControlPolicies().Informer().AddEventHandler(watcher)
	require.NoError(t, err)
	_, err = kubeInformer.Core().V1().Secrets().Informer().AddEventHandler(watcher)
	require.NoError(t, err)

	hubInformer.Start(context.Background().Done())
	for typ, ok := range hubInformer.WaitForCacheSync(context.Background().Done()) {
		if !ok {
			t.Fatalf("wait for cache sync: %s", typ)
		}
	}

	kubeInformer.Start(context.Background().Done())
	for typ, ok := range kubeInformer.WaitForCacheSync(context.Background().Done()) {
		if !ok {
			t.Fatalf("wait for cache sync: %s", typ)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go watcher.Run(ctx)
}

func createPolicy(uid, name string) *hubv1alpha1.AccessControlPolicy {
	return createPolicyWithSecret(uid, name, "secret")
}

func createPolicyWithSecret(uid, name, secret string) *hubv1alpha1.AccessControlPolicy {
	return &hubv1alpha1.AccessControlPolicy{
		ObjectMeta: metav1.ObjectMeta{UID: ktypes.UID(uid), Name: name},
		Spec: hubv1alpha1.AccessControlPolicySpec{
			JWT: &hubv1alpha1.AccessControlPolicyJWT{
				SigningSecret: secret,
			},
		},
	}
}

func createOIDCPolicy(uid, name, issuer string, secret *corev1.SecretReference) *hubv1alpha1.AccessControlPolicy {
	return &hubv1alpha1.AccessControlPolicy{
		ObjectMeta: metav1.ObjectMeta{UID: ktypes.UID(uid), Name: name},
		Spec: hubv1alpha1.AccessControlPolicySpec{
			OIDC: &hubv1alpha1.AccessControlPolicyOIDC{
				Issuer:   issuer,
				ClientID: "ID",
				Secret:   secret,
			},
		},
	}
}

func createSecret(namespace, name, key, value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
}
