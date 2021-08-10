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
