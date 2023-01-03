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

package state

import "k8s.io/apimachinery/pkg/labels"

func (f *Fetcher) getEdgeIngresses() (map[string]*EdgeIngress, error) {
	edgeIngresses, err := f.hub.Hub().V1alpha1().EdgeIngresses().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	result := make(map[string]*EdgeIngress)
	for _, edgeIngress := range edgeIngresses {
		status := EdgeIngressStatusDown
		if edgeIngress.Status.Connection == "UP" {
			status = EdgeIngressStatusUp
		}

		var acp *EdgeIngressACP
		if edgeIngress.Spec.ACP != nil {
			acp = &EdgeIngressACP{Name: edgeIngress.Spec.ACP.Name}
		}

		result[objectKey(edgeIngress.Name, edgeIngress.Namespace)] = &EdgeIngress{
			Name:      edgeIngress.Name,
			Namespace: edgeIngress.Namespace,
			Status:    status,
			Service: EdgeIngressService{
				Name: edgeIngress.Spec.Service.Name,
				Port: edgeIngress.Spec.Service.Port,
			},
			ACP: acp,
		}
	}

	return result, nil
}
