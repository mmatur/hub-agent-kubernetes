/*
Copyright (C) 2022 Traefik Labs

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
		i := strings.Index(u, ":")
		if i <= 0 {
			continue
		}

		users = append(users, u[:i]+":redacted")
	}

	return strings.Join(users, ",")
}
