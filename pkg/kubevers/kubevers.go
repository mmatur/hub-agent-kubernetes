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

package kubevers

import (
	"github.com/hashicorp/go-version"
)

// SupportsNetV1Ingresses reports whether the Kubernetes cluster supports net v1 Ingresses.
func SupportsNetV1Ingresses(ver string) bool {
	return atLeast(ver, "1.19")
}

// SupportsNetV1Beta1IngressClasses reports whether the Kubernetes cluster supports net v1beta1 Ingresses.
func SupportsNetV1Beta1IngressClasses(ver string) bool {
	return atLeast(ver, "1.18")
}

// SupportsNetV1IngressClasses reports whether the Kubernetes cluster supports net v1 IngressClasses.
func SupportsNetV1IngressClasses(ver string) bool {
	return atLeast(ver, "1.19")
}

// SupportsIngressClasses reports whether the Kubernetes cluster supports IngressClasses.
func SupportsIngressClasses(ver string) bool {
	return atLeast(ver, "1.18")
}

func atLeast(ver, minVer string) bool {
	kubeVersion := version.Must(version.NewSemver(ver))
	minVersion := version.Must(version.NewSemver(minVer))

	return kubeVersion.GreaterThanOrEqual(minVersion)
}
