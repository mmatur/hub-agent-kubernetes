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
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

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

func createSecret(namespace, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data: map[string][]byte{
			"clientSecret": []byte("1234567890123456"),
		},
	}
}

func TestWatcher_OnAddOIDC(t *testing.T) {
	var data string
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(data))
	}))
	t.Cleanup(srv.Close)

	data = fmt.Sprintf(`{"issuer":%q}`, srv.URL)

	switcher := NewHandlerSwitcher()
	watcher := NewWatcher(switcher, "1234567891234567")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)

	go watcher.Run(ctx)

	// Add oidc without secret
	watcher.OnAdd(createOIDCPolicy("1", "my-oidc", srv.URL, &corev1.SecretReference{Namespace: "ns", Name: "secret"}))

	time.Sleep(10 * time.Millisecond)

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/my-oidc", nil)

	switcher.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusNotFound, rw.Code)

	// Add secret for oidc
	watcher.OnAdd(createSecret("ns", "secret"))

	time.Sleep(100 * time.Millisecond)

	rw = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/my-oidc", nil)

	switcher.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusFound, rw.Code)
}

func TestWatcher_OnAdd(t *testing.T) {
	switcher := NewHandlerSwitcher()
	watcher := NewWatcher(switcher, "")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)

	go watcher.Run(ctx)

	watcher.OnAdd(createPolicy("1", "my-policy-1"))
	watcher.OnAdd(createPolicy("2", "my-policy-2"))
	watcher.OnAdd(createPolicy("3", "my-policy-3"))

	time.Sleep(10 * time.Millisecond)

	testCases := []struct {
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

	for _, test := range testCases {
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

func TestWatcher_OnUpdate(t *testing.T) {
	switcher := NewHandlerSwitcher()
	watcher := NewWatcher(switcher, "")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)

	go watcher.Run(ctx)

	watcher.OnAdd(createPolicyWithSecret("1", "my-policy-1", "Wrong secret"))

	watcher.OnUpdate(nil, createPolicy("1", "my-policy-1"))

	time.Sleep(10 * time.Millisecond)

	testCases := []struct {
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

	for _, test := range testCases {
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

func TestWatcher_OnDelete(t *testing.T) {
	switcher := NewHandlerSwitcher()
	watcher := NewWatcher(switcher, "")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)

	go watcher.Run(ctx)

	watcher.OnAdd(createPolicy("1", "my-policy-1"))
	watcher.OnAdd(createPolicy("2", "my-policy-2"))
	watcher.OnAdd(createPolicy("3", "my-policy-3"))

	watcher.OnDelete(createPolicy("1", "my-policy-1"))
	watcher.OnDelete(createPolicy("2", "my-policy-2"))
	watcher.OnDelete(createPolicy("3", "my-policy-3"))

	time.Sleep(10 * time.Millisecond)

	testCases := []struct {
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

	for _, test := range testCases {
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
