package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Obtain(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	tests := []struct {
		desc       string
		statusCode int
		wantCert   Certificate
		wantErr    error
	}{
		{
			desc:       "obtain certificate succeed",
			statusCode: http.StatusOK,
			wantCert: Certificate{
				Domains:     []string{"test.localhost"},
				Certificate: []byte("cert"),
				PrivateKey:  []byte("key"),
				NotBefore:   now,
				NotAfter:    now.Add(24 * time.Hour),
			},
		},
		{
			desc:       "obtain pending certificate",
			statusCode: http.StatusAccepted,
			wantCert:   Certificate{},
			wantErr:    &PendingError{},
		},
		{
			desc:       "obtain certificate unexpected error",
			statusCode: http.StatusTeapot,
			wantCert:   Certificate{},
			wantErr: &APIError{
				StatusCode: http.StatusTeapot,
				Message:    "error",
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			var (
				callCount   int
				callDomains []string
			)
			mux := http.NewServeMux()
			mux.HandleFunc("/certificates", func(rw http.ResponseWriter, req *http.Request) {
				callCount++
				callDomains = strings.Split(req.URL.Query().Get("domains"), ",")

				if req.Method != http.MethodGet {
					http.Error(rw, fmt.Sprintf("unsupported method: %s", req.Method), http.StatusMethodNotAllowed)
					return
				}

				if req.Header.Get("Authorization") != "Bearer 123" {
					http.Error(rw, "Invalid token", http.StatusUnauthorized)
					return
				}

				rw.WriteHeader(test.statusCode)

				switch test.statusCode {
				case http.StatusAccepted:
				case http.StatusOK:
					_ = json.NewEncoder(rw).Encode(test.wantCert)

				default:
					_ = json.NewEncoder(rw).Encode(APIError{Message: "error"})
				}
			})

			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			c := New(srv.URL, "123")
			c.httpClient = srv.Client()

			wantDomains := []string{
				"test.localhost",
				"test2.localhost",
			}

			gotCert, err := c.Obtain(context.Background(), wantDomains)
			if test.wantErr != nil {
				require.ErrorAs(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, 1, callCount)
			assert.Equal(t, wantDomains, callDomains)
			assert.Equal(t, test.wantCert, gotCert)
		})
	}
}
