//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientAuth) DeepCopyInto(out *ClientAuth) {
	*out = *in
	if in.SecretNames != nil {
		in, out := &in.SecretNames, &out.SecretNames
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientAuth.
func (in *ClientAuth) DeepCopy() *ClientAuth {
	if in == nil {
		return nil
	}
	out := new(ClientAuth)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientTLS) DeepCopyInto(out *ClientTLS) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientTLS.
func (in *ClientTLS) DeepCopy() *ClientTLS {
	if in == nil {
		return nil
	}
	out := new(ClientTLS)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Cookie) DeepCopyInto(out *Cookie) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Cookie.
func (in *Cookie) DeepCopy() *Cookie {
	if in == nil {
		return nil
	}
	out := new(Cookie)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Domain) DeepCopyInto(out *Domain) {
	*out = *in
	if in.SANs != nil {
		in, out := &in.SANs, &out.SANs
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Domain.
func (in *Domain) DeepCopy() *Domain {
	if in == nil {
		return nil
	}
	out := new(Domain)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ForwardAuth) DeepCopyInto(out *ForwardAuth) {
	*out = *in
	if in.AuthResponseHeaders != nil {
		in, out := &in.AuthResponseHeaders, &out.AuthResponseHeaders
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.AuthRequestHeaders != nil {
		in, out := &in.AuthRequestHeaders, &out.AuthRequestHeaders
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.TLS != nil {
		in, out := &in.TLS, &out.TLS
		*out = new(ClientTLS)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ForwardAuth.
func (in *ForwardAuth) DeepCopy() *ForwardAuth {
	if in == nil {
		return nil
	}
	out := new(ForwardAuth)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressRoute) DeepCopyInto(out *IngressRoute) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IngressRoute.
func (in *IngressRoute) DeepCopy() *IngressRoute {
	if in == nil {
		return nil
	}
	out := new(IngressRoute)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *IngressRoute) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressRouteList) DeepCopyInto(out *IngressRouteList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]IngressRoute, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IngressRouteList.
func (in *IngressRouteList) DeepCopy() *IngressRouteList {
	if in == nil {
		return nil
	}
	out := new(IngressRouteList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *IngressRouteList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressRouteSpec) DeepCopyInto(out *IngressRouteSpec) {
	*out = *in
	if in.Routes != nil {
		in, out := &in.Routes, &out.Routes
		*out = make([]Route, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.EntryPoints != nil {
		in, out := &in.EntryPoints, &out.EntryPoints
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.TLS != nil {
		in, out := &in.TLS, &out.TLS
		*out = new(TLS)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IngressRouteSpec.
func (in *IngressRouteSpec) DeepCopy() *IngressRouteSpec {
	if in == nil {
		return nil
	}
	out := new(IngressRouteSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LoadBalancerSpec) DeepCopyInto(out *LoadBalancerSpec) {
	*out = *in
	if in.Sticky != nil {
		in, out := &in.Sticky, &out.Sticky
		*out = new(Sticky)
		(*in).DeepCopyInto(*out)
	}
	out.Port = in.Port
	if in.PassHostHeader != nil {
		in, out := &in.PassHostHeader, &out.PassHostHeader
		*out = new(bool)
		**out = **in
	}
	if in.ResponseForwarding != nil {
		in, out := &in.ResponseForwarding, &out.ResponseForwarding
		*out = new(ResponseForwarding)
		**out = **in
	}
	if in.Weight != nil {
		in, out := &in.Weight, &out.Weight
		*out = new(int)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LoadBalancerSpec.
func (in *LoadBalancerSpec) DeepCopy() *LoadBalancerSpec {
	if in == nil {
		return nil
	}
	out := new(LoadBalancerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Middleware) DeepCopyInto(out *Middleware) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Middleware.
func (in *Middleware) DeepCopy() *Middleware {
	if in == nil {
		return nil
	}
	out := new(Middleware)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Middleware) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MiddlewareList) DeepCopyInto(out *MiddlewareList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Middleware, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MiddlewareList.
func (in *MiddlewareList) DeepCopy() *MiddlewareList {
	if in == nil {
		return nil
	}
	out := new(MiddlewareList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MiddlewareList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MiddlewareRef) DeepCopyInto(out *MiddlewareRef) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MiddlewareRef.
func (in *MiddlewareRef) DeepCopy() *MiddlewareRef {
	if in == nil {
		return nil
	}
	out := new(MiddlewareRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MiddlewareSpec) DeepCopyInto(out *MiddlewareSpec) {
	*out = *in
	if in.ForwardAuth != nil {
		in, out := &in.ForwardAuth, &out.ForwardAuth
		*out = new(ForwardAuth)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MiddlewareSpec.
func (in *MiddlewareSpec) DeepCopy() *MiddlewareSpec {
	if in == nil {
		return nil
	}
	out := new(MiddlewareSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MirrorService) DeepCopyInto(out *MirrorService) {
	*out = *in
	in.LoadBalancerSpec.DeepCopyInto(&out.LoadBalancerSpec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MirrorService.
func (in *MirrorService) DeepCopy() *MirrorService {
	if in == nil {
		return nil
	}
	out := new(MirrorService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Mirroring) DeepCopyInto(out *Mirroring) {
	*out = *in
	in.LoadBalancerSpec.DeepCopyInto(&out.LoadBalancerSpec)
	if in.MaxBodySize != nil {
		in, out := &in.MaxBodySize, &out.MaxBodySize
		*out = new(int64)
		**out = **in
	}
	if in.Mirrors != nil {
		in, out := &in.Mirrors, &out.Mirrors
		*out = make([]MirrorService, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Mirroring.
func (in *Mirroring) DeepCopy() *Mirroring {
	if in == nil {
		return nil
	}
	out := new(Mirroring)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResponseForwarding) DeepCopyInto(out *ResponseForwarding) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResponseForwarding.
func (in *ResponseForwarding) DeepCopy() *ResponseForwarding {
	if in == nil {
		return nil
	}
	out := new(ResponseForwarding)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Route) DeepCopyInto(out *Route) {
	*out = *in
	if in.Services != nil {
		in, out := &in.Services, &out.Services
		*out = make([]Service, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Middlewares != nil {
		in, out := &in.Middlewares, &out.Middlewares
		*out = make([]MiddlewareRef, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Route.
func (in *Route) DeepCopy() *Route {
	if in == nil {
		return nil
	}
	out := new(Route)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Service) DeepCopyInto(out *Service) {
	*out = *in
	in.LoadBalancerSpec.DeepCopyInto(&out.LoadBalancerSpec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Service.
func (in *Service) DeepCopy() *Service {
	if in == nil {
		return nil
	}
	out := new(Service)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServiceSpec) DeepCopyInto(out *ServiceSpec) {
	*out = *in
	if in.Weighted != nil {
		in, out := &in.Weighted, &out.Weighted
		*out = new(WeightedRoundRobin)
		(*in).DeepCopyInto(*out)
	}
	if in.Mirroring != nil {
		in, out := &in.Mirroring, &out.Mirroring
		*out = new(Mirroring)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServiceSpec.
func (in *ServiceSpec) DeepCopy() *ServiceSpec {
	if in == nil {
		return nil
	}
	out := new(ServiceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Sticky) DeepCopyInto(out *Sticky) {
	*out = *in
	if in.Cookie != nil {
		in, out := &in.Cookie, &out.Cookie
		*out = new(Cookie)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Sticky.
func (in *Sticky) DeepCopy() *Sticky {
	if in == nil {
		return nil
	}
	out := new(Sticky)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLS) DeepCopyInto(out *TLS) {
	*out = *in
	if in.Options != nil {
		in, out := &in.Options, &out.Options
		*out = new(TLSOptionRef)
		**out = **in
	}
	if in.Store != nil {
		in, out := &in.Store, &out.Store
		*out = new(TLSStoreRef)
		**out = **in
	}
	if in.Domains != nil {
		in, out := &in.Domains, &out.Domains
		*out = make([]Domain, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLS.
func (in *TLS) DeepCopy() *TLS {
	if in == nil {
		return nil
	}
	out := new(TLS)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSOption) DeepCopyInto(out *TLSOption) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSOption.
func (in *TLSOption) DeepCopy() *TLSOption {
	if in == nil {
		return nil
	}
	out := new(TLSOption)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TLSOption) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSOptionList) DeepCopyInto(out *TLSOptionList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]TLSOption, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSOptionList.
func (in *TLSOptionList) DeepCopy() *TLSOptionList {
	if in == nil {
		return nil
	}
	out := new(TLSOptionList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TLSOptionList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSOptionRef) DeepCopyInto(out *TLSOptionRef) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSOptionRef.
func (in *TLSOptionRef) DeepCopy() *TLSOptionRef {
	if in == nil {
		return nil
	}
	out := new(TLSOptionRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSOptionSpec) DeepCopyInto(out *TLSOptionSpec) {
	*out = *in
	if in.CipherSuites != nil {
		in, out := &in.CipherSuites, &out.CipherSuites
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.CurvePreferences != nil {
		in, out := &in.CurvePreferences, &out.CurvePreferences
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	in.ClientAuth.DeepCopyInto(&out.ClientAuth)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSOptionSpec.
func (in *TLSOptionSpec) DeepCopy() *TLSOptionSpec {
	if in == nil {
		return nil
	}
	out := new(TLSOptionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSStoreRef) DeepCopyInto(out *TLSStoreRef) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSStoreRef.
func (in *TLSStoreRef) DeepCopy() *TLSStoreRef {
	if in == nil {
		return nil
	}
	out := new(TLSStoreRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TraefikService) DeepCopyInto(out *TraefikService) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TraefikService.
func (in *TraefikService) DeepCopy() *TraefikService {
	if in == nil {
		return nil
	}
	out := new(TraefikService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TraefikService) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TraefikServiceList) DeepCopyInto(out *TraefikServiceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]TraefikService, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TraefikServiceList.
func (in *TraefikServiceList) DeepCopy() *TraefikServiceList {
	if in == nil {
		return nil
	}
	out := new(TraefikServiceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TraefikServiceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WeightedRoundRobin) DeepCopyInto(out *WeightedRoundRobin) {
	*out = *in
	if in.Services != nil {
		in, out := &in.Services, &out.Services
		*out = make([]Service, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Sticky != nil {
		in, out := &in.Sticky, &out.Sticky
		*out = new(Sticky)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WeightedRoundRobin.
func (in *WeightedRoundRobin) DeepCopy() *WeightedRoundRobin {
	if in == nil {
		return nil
	}
	out := new(WeightedRoundRobin)
	in.DeepCopyInto(out)
	return out
}
