package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	neov1alpha1 "github.com/traefik/neo-agent/pkg/crd/api/neo/v1alpha1"
	neokubemock "github.com/traefik/neo-agent/pkg/crd/generated/client/neo/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/neo-agent/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func TestFetcher_GetAccessControlPolicies(t *testing.T) {
	tests := []struct {
		desc    string
		objects []runtime.Object
		want    map[string]*AccessControlPolicy
	}{
		{
			desc: "Empty",
			want: make(map[string]*AccessControlPolicy),
		},
		{
			desc: "JWT access control policy",
			objects: []runtime.Object{
				&neov1alpha1.AccessControlPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myacp",
						Namespace: "myns",
					},
					Spec: neov1alpha1.AccessControlPolicySpec{
						JWT: &neov1alpha1.AccessControlPolicyJWT{
							SigningSecret:              "titi",
							SigningSecretBase64Encoded: true,
							PublicKey:                  "toto",
							JWKsFile:                   "tata",
							JWKsURL:                    "tete",
							StripAuthorizationHeader:   false,
							ForwardHeaders:             map[string]string{"Titi": "toto", "Toto": "titi"},
							TokenQueryKey:              "token",
							Claims:                     "iss=titi",
						},
					},
				},
			},
			want: map[string]*AccessControlPolicy{
				"myacp@myns": {
					Name:      "myacp",
					Namespace: "myns",
					ClusterID: "cluster-id",
					Method:    "jwt",
					JWT: &AccessControlPolicyJWT{
						SigningSecret:              "redacted",
						JWKsFile:                   "tata",
						JWKsURL:                    "tete",
						StripAuthorizationHeader:   false,
						PublicKey:                  "toto",
						SigningSecretBase64Encoded: true,
						ForwardHeaders:             map[string]string{"Titi": "toto", "Toto": "titi"},
						TokenQueryKey:              "token",
						Claims:                     "iss=titi",
					},
				},
			},
		},
		{
			desc: "Obfuscation doesn't run when fields are empty",
			objects: []runtime.Object{
				&neov1alpha1.AccessControlPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myacp",
						Namespace: "myns",
					},
					Spec: neov1alpha1.AccessControlPolicySpec{
						JWT: &neov1alpha1.AccessControlPolicyJWT{
							Claims: "iss=titi",
						},
					},
				},
			},
			want: map[string]*AccessControlPolicy{
				"myacp@myns": {
					Name:      "myacp",
					Namespace: "myns",
					ClusterID: "cluster-id",
					Method:    "jwt",
					JWT: &AccessControlPolicyJWT{
						Claims: "iss=titi",
					},
				},
			},
		},
		{
			desc: "Basic Auth access control policy",
			objects: []runtime.Object{
				&neov1alpha1.AccessControlPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myacp",
						Namespace: "myns",
					},
					Spec: neov1alpha1.AccessControlPolicySpec{
						BasicAuth: &neov1alpha1.AccessControlPolicyBasicAuth{
							Users:                    "toto:secret,titi:secret",
							Realm:                    "realm",
							StripAuthorizationHeader: true,
						},
					},
				},
			},
			want: map[string]*AccessControlPolicy{
				"myacp@myns": {
					Name:      "myacp",
					Namespace: "myns",
					ClusterID: "cluster-id",
					Method:    "basicauth",
					BasicAuth: &AccessControlPolicyBasicAuth{
						Users:                    "toto:redacted,titi:redacted",
						Realm:                    "realm",
						StripAuthorizationHeader: true,
					},
				},
			},
		},
		{
			desc: "Digest Auth access control policy",
			objects: []runtime.Object{
				&neov1alpha1.AccessControlPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myacp",
						Namespace: "myns",
					},
					Spec: neov1alpha1.AccessControlPolicySpec{
						DigestAuth: &neov1alpha1.AccessControlPolicyDigestAuth{
							Users:                    "toto:realm:secret,titi:realm:secret",
							Realm:                    "myrealm",
							StripAuthorizationHeader: true,
						},
					},
				},
			},
			want: map[string]*AccessControlPolicy{
				"myacp@myns": {
					Name:      "myacp",
					Namespace: "myns",
					ClusterID: "cluster-id",
					Method:    "digestauth",
					DigestAuth: &AccessControlPolicyDigestAuth{
						Users:                    "toto:realm:redacted,titi:realm:redacted",
						Realm:                    "myrealm",
						StripAuthorizationHeader: true,
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			clusterID := "cluster-id"
			kubeClient := kubemock.NewSimpleClientset()
			neoClient := neokubemock.NewSimpleClientset(test.objects...)
			traefikClient := traefikkubemock.NewSimpleClientset()

			f, err := watchAll(context.Background(), kubeClient, neoClient, traefikClient, "v1.20.1", clusterID)
			require.NoError(t, err)

			got, err := f.getAccessControlPolicies(clusterID)
			assert.NoError(t, err)

			assert.Equal(t, test.want, got)
		})
	}
}
