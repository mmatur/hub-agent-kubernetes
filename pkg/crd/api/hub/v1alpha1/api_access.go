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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// APIAccess defines which group of consumers can access APIs and APICollections.
type APIAccess struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec APIAccessSpec `json:"spec,omitempty"`

	// The current status of this APIAccess.
	// +optional
	Status APIAccessStatus `json:"status,omitempty"`
}

// APIAccessSpec configures an APIAccess.
type APIAccessSpec struct {
	Groups                []string             `json:"groups"`
	APISelector           metav1.LabelSelector `json:"apiSelector"`
	APICollectionSelector metav1.LabelSelector `json:"apiCollectionSelector"`
}

// APIAccessStatus is the status of an APIAccess.
type APIAccessStatus struct {
	Version  string      `json:"version,omitempty"`
	SyncedAt metav1.Time `json:"syncedAt,omitempty"`
	// Hash is a hash representing the APIAccess.
	Hash string `json:"hash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// APIAccessList defines a list of APIAccesses.
type APIAccessList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []APIAccess `json:"items"`
}
