package digestauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Generate password:

		htdigest -c <output-pwd-file> <realm> <username>
*/

func TestDigestAuthError(t *testing.T) {
	auth := &Config{
		Users: []string{"test"},
	}
	_, err := NewHandler(auth, "authName")
	assert.Error(t, err)
}

func TestDigestAuthUsers(t *testing.T) {
	testCases := []struct {
		desc               string
		givenUsers         []string
		username           string
		password           string
		expectedStatusCode int
		realm              string
	}{
		{
			desc:               "Should authenticate users",
			givenUsers:         []string{"test2:neo:c7a89caf0a2251b3e046e03115fa5afe", "test3:neo:4932cae9e61280f22b4c8d44f9cefe8e"},
			username:           "test2",
			password:           "test2",
			expectedStatusCode: http.StatusOK,
		},
		{
			desc:               "Should not authenticate unknown user",
			givenUsers:         []string{"test2:neo:c7a89caf0a2251b3e046e03115fa5afe", "test3:neo:4932cae9e61280f22b4c8d44f9cefe8e"},
			username:           "foo",
			password:           "bar",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			desc:               "Should authenticate the correct user based on the realm",
			givenUsers:         []string{"test:neo:d061460985b8212db4b9465a846615e2", "test:traefiker:a3d334dff2645b914918de78bec50bf4"},
			username:           "test",
			password:           "test2",
			realm:              "traefiker",
			expectedStatusCode: http.StatusOK,
		},
		{
			desc:               "Should not authenticate user from unknown realm",
			givenUsers:         []string{"test:neo:d061460985b8212db4b9465a846615e2", "test:traefiker:a3d334dff2645b914918de78bec50bf4"},
			username:           "test",
			password:           "test2",
			realm:              "otherRealm",
			expectedStatusCode: http.StatusUnauthorized,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			// Creates the configuration for our Authenticator.
			cfg := &Config{
				Users: test.givenUsers,
				Realm: test.realm,
			}

			handler, err := NewHandler(cfg, "acp@my-ns")
			require.NoError(t, err)

			ts := httptest.NewServer(handler)
			defer ts.Close()

			req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
			require.NoError(t, err)
			digestRequest := newDigestRequest(test.username, test.password, http.DefaultClient)

			resp, err := digestRequest.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, test.expectedStatusCode, resp.StatusCode)
		})
	}
}

func TestDigestAuthUserHeader(t *testing.T) {
	cfg := &Config{
		Users:                 []string{"test2:neo:c7a89caf0a2251b3e046e03115fa5afe"},
		ForwardUsernameHeader: "User",
	}

	handler, err := NewHandler(cfg, "acp@my-ns")
	require.NoError(t, err)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	require.NoError(t, err)
	digestRequest := newDigestRequest("test2", "test2", http.DefaultClient)

	resp, err := digestRequest.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, "test2", resp.Header.Get("User"))
}
