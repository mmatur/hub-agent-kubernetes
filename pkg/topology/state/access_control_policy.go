package state

import (
	"strings"

	"k8s.io/apimachinery/pkg/labels"
)

func (f *Fetcher) getAccessControlPolicies(clusterID string) (map[string]*AccessControlPolicy, error) {
	policies, err := f.hub.Hub().V1alpha1().AccessControlPolicies().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	result := make(map[string]*AccessControlPolicy)
	for _, policy := range policies {
		acp := &AccessControlPolicy{
			Name:      policy.Name,
			Namespace: policy.Namespace,
			ClusterID: clusterID,
		}

		switch {
		case policy.Spec.JWT != nil:
			acp.Method = "jwt"
			acp.JWT = &AccessControlPolicyJWT{
				SigningSecretBase64Encoded: policy.Spec.JWT.SigningSecretBase64Encoded,
				PublicKey:                  policy.Spec.JWT.PublicKey,
				StripAuthorizationHeader:   policy.Spec.JWT.StripAuthorizationHeader,
				ForwardHeaders:             policy.Spec.JWT.ForwardHeaders,
				TokenQueryKey:              policy.Spec.JWT.TokenQueryKey,
				JWKsFile:                   policy.Spec.JWT.JWKsFile,
				JWKsURL:                    policy.Spec.JWT.JWKsURL,
				Claims:                     policy.Spec.JWT.Claims,
			}

			// TODO: policy.Spec.JWT.JWKsFile can be a huge file, maybe if it's too long we should truncate it.
			if policy.Spec.JWT.SigningSecret != "" {
				acp.JWT.SigningSecret = "redacted"
			}
		case policy.Spec.BasicAuth != nil:
			acp.Method = "basicauth"
			acp.BasicAuth = &AccessControlPolicyBasicAuth{
				Users:                    removePassword(policy.Spec.BasicAuth.Users),
				Realm:                    policy.Spec.BasicAuth.Realm,
				StripAuthorizationHeader: policy.Spec.BasicAuth.StripAuthorizationHeader,
				ForwardUsernameHeader:    policy.Spec.BasicAuth.ForwardUsernameHeader,
			}
		case policy.Spec.DigestAuth != nil:
			acp.Method = "digestauth"
			acp.DigestAuth = &AccessControlPolicyDigestAuth{
				Users:                    removePassword(policy.Spec.DigestAuth.Users),
				Realm:                    policy.Spec.DigestAuth.Realm,
				StripAuthorizationHeader: policy.Spec.DigestAuth.StripAuthorizationHeader,
				ForwardUsernameHeader:    policy.Spec.DigestAuth.ForwardUsernameHeader,
			}
		default:
			continue
		}

		result[objectKey(policy.Name, policy.Namespace)] = acp
	}

	return result, nil
}

func removePassword(rawUsers []string) string {
	var users []string

	for _, u := range rawUsers {
		parts := strings.Split(u, ":")

		// Digest format: user:realm:secret
		if len(parts) == 3 {
			users = append(users, parts[0]+":"+parts[1]+":redacted")
			continue
		}

		users = append(users, parts[0]+":redacted")
	}

	return strings.Join(users, ",")
}
