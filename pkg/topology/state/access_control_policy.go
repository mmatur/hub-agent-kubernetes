package state

import "k8s.io/apimachinery/pkg/labels"

func (f *Fetcher) getAccessControlPolicies() (map[string]*AccessControlPolicy, error) {
	policies, err := f.acp.Neo().V1alpha1().AccessControlPolicies().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	result := make(map[string]*AccessControlPolicy)
	for _, policy := range policies {
		if policy.Spec.JWT == nil {
			continue
		}

		acp := &AccessControlPolicy{
			JWT: &JWTAccessControl{
				SigningSecretBase64Encoded: policy.Spec.JWT.SigningSecretBase64Encoded,
				PublicKey:                  policy.Spec.JWT.PublicKey,
				ForwardAuthorization:       policy.Spec.JWT.ForwardAuthorization,
				ForwardHeaders:             policy.Spec.JWT.ForwardHeaders,
				TokenQueryKey:              policy.Spec.JWT.TokenQueryKey,
				JWKsFile:                   policy.Spec.JWT.JWKsFile,
				JWKsURL:                    policy.Spec.JWT.JWKsURL,
				Claims:                     policy.Spec.JWT.Claims,
			},
		}

		if policy.Spec.JWT.SigningSecret != "" {
			acp.JWT.SigningSecret = "redacted"
		}

		// TODO: policy.Spec.JWT.JWKsFile can be a huge file, maybe if it's too long we should truncate it.

		result[objectKey(policy.Name, policy.Namespace)] = acp
	}

	return result, nil
}
