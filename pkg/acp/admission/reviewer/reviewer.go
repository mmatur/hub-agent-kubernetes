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

package reviewer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IngressClasses allows to get the ingress controller type given an ingress class desc
// or the default ingress controller type.
type IngressClasses interface {
	GetController(name string) (string, error)
	GetDefaultController() (string, error)
}

func isNetV1Ingress(resource metav1.GroupVersionKind) bool {
	return resource.Group == "networking.k8s.io" && resource.Version == "v1" && resource.Kind == "Ingress"
}

func isNetV1Beta1Ingress(resource metav1.GroupVersionKind) bool {
	return resource.Group == "networking.k8s.io" && resource.Version == "v1beta1" && resource.Kind == "Ingress"
}

func isExtV1Beta1Ingress(resource metav1.GroupVersionKind) bool {
	return resource.Group == "extensions" && resource.Version == "v1beta1" && resource.Kind == "Ingress"
}

func isTraefikV1Alpha1IngressRoute(resource metav1.GroupVersionKind) bool {
	return resource.Group == "traefik.containo.us" && resource.Version == "v1alpha1" && resource.Kind == "IngressRoute"
}
