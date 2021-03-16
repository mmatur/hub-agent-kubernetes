package state

import (
	"context"

	"sigs.k8s.io/external-dns/source"
)

func getExternalDNS(ctx context.Context, src source.Source) (map[string]*ExternalDNS, error) {
	endpoints, err := src.Endpoints(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*ExternalDNS)
	for _, endpoint := range endpoints {
		result[endpoint.DNSName] = &ExternalDNS{
			DNSName: endpoint.DNSName,
			Targets: endpoint.Targets,
			TTL:     endpoint.RecordTTL,
		}
	}

	return result, nil
}
