package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/external-dns/endpoint"
)

func TestFetcher_GetExternalDNS(t *testing.T) {
	tests := []struct {
		desc   string
		source fakeSource
		want   map[string]*ExternalDNS
	}{
		{
			desc: "Empty",
			want: make(map[string]*ExternalDNS),
		},
		{
			desc: "Endpoint from fake source",
			source: fakeSource{
				endpoints: []*endpoint.Endpoint{
					{
						DNSName:   "foo.com",
						Targets:   []string{"1.2.3.4"},
						RecordTTL: 180,
					},
				},
			},
			want: map[string]*ExternalDNS{
				"foo.com": {
					DNSName: "foo.com",
					Targets: []string{"1.2.3.4"},
					TTL:     180,
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			got, err := getExternalDNS(context.Background(), test.source)
			assert.NoError(t, err)

			assert.Equal(t, test.want, got)
		})
	}
}

type fakeSource struct {
	endpoints []*endpoint.Endpoint
}

func (f fakeSource) Endpoints(_ context.Context) ([]*endpoint.Endpoint, error) {
	return f.endpoints, nil
}

func (f fakeSource) AddEventHandler(context.Context, func()) {
	panic("implement me")
}
