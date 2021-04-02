package jwt_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/neo-agent/pkg/acp/jwt"
	jose "gopkg.in/square/go-jose.v2"
)

func TestContentKeySet_FetchesKeySet(t *testing.T) {
	var wantKeys jose.JSONWebKeySet
	err := json.Unmarshal([]byte(jwkeys), &wantKeys)
	require.NoError(t, err)

	ks, err := jwt.NewContentKeySet([]byte(jwkeys))
	require.NoError(t, err)

	gotFooKey, err := ks.Key(context.Background(), "foo-key")
	require.NoError(t, err)

	gotBarKey, err := ks.Key(context.Background(), "bar-key")
	require.NoError(t, err)

	assert.Equal(t, wantKeys.Key("foo-key")[0], *gotFooKey)
	assert.Equal(t, wantKeys.Key("bar-key")[0], *gotBarKey)
}

func TestContentKeySet_KeysReturnsNilWhenKeyIsUnknown(t *testing.T) {
	var wantKeys jose.JSONWebKeySet
	err := json.Unmarshal([]byte(jwkeys), &wantKeys)
	require.NoError(t, err)

	ks, err := jwt.NewContentKeySet([]byte(jwkeys))
	require.NoError(t, err)

	gotKey, err := ks.Key(context.Background(), "meh-key")
	require.NoError(t, err)

	assert.Nil(t, gotKey)
}

func TestFileKeySet_FetchesFile(t *testing.T) {
	var wantKeys jose.JSONWebKeySet
	err := json.Unmarshal([]byte(jwkeys), &wantKeys)
	require.NoError(t, err)

	ks := jwt.NewFileKeySet("./testdata/jwks.json")

	gotFooKey, err := ks.Key(context.Background(), "foo-key")
	require.NoError(t, err)

	gotBarKey, err := ks.Key(context.Background(), "bar-key")
	require.NoError(t, err)

	assert.Equal(t, wantKeys.Key("foo-key")[0], *gotFooKey)
	assert.Equal(t, wantKeys.Key("bar-key")[0], *gotBarKey)
}

func TestFileKeySet_KeysReturnsNilWhenKeyIsUnknown(t *testing.T) {
	var wantKeys jose.JSONWebKeySet
	err := json.Unmarshal([]byte(jwkeys), &wantKeys)
	require.NoError(t, err)

	ks := jwt.NewFileKeySet("./testdata/jwks.json")

	gotKey, err := ks.Key(context.Background(), "meh-key")
	require.NoError(t, err)

	assert.Nil(t, gotKey)
}

func TestRemoteKeySet_KeysFetchesKeySet(t *testing.T) {
	var wantKeys jose.JSONWebKeySet
	err := json.Unmarshal([]byte(jwkeys), &wantKeys)
	require.NoError(t, err)

	var hdlrCalled int
	hdlr := func(rw http.ResponseWriter, req *http.Request) {
		hdlrCalled++

		rw.Header().Add("Cache-Control", "max-age=600")
		_, _ = rw.Write([]byte(jwkeys))
	}

	srv := httptest.NewServer(http.HandlerFunc(hdlr))
	defer srv.Close()

	ks := jwt.NewRemoteKeySet(srv.URL)

	gotFooKey, err := ks.Key(context.Background(), "foo-key")
	require.NoError(t, err)

	gotBarKey, err := ks.Key(context.Background(), "bar-key")
	require.NoError(t, err)

	assert.Equal(t, 1, hdlrCalled)
	assert.Equal(t, wantKeys.Key("foo-key")[0], *gotFooKey)
	assert.Equal(t, wantKeys.Key("bar-key")[0], *gotBarKey)
}

func TestRemoteKeySet_KeysFetchesKeySetWithoutCache(t *testing.T) {
	var wantKeys jose.JSONWebKeySet
	err := json.Unmarshal([]byte(jwkeys), &wantKeys)
	require.NoError(t, err)

	var hdlrCalled int
	hdlr := func(rw http.ResponseWriter, req *http.Request) {
		hdlrCalled++

		_, _ = rw.Write([]byte(jwkeys))
	}

	srv := httptest.NewServer(http.HandlerFunc(hdlr))
	defer srv.Close()

	ks := jwt.NewRemoteKeySet(srv.URL)

	gotFooKey, err := ks.Key(context.Background(), "foo-key")
	require.NoError(t, err)

	gotBarKey, err := ks.Key(context.Background(), "bar-key")
	require.NoError(t, err)

	assert.Equal(t, 2, hdlrCalled)
	assert.Equal(t, wantKeys.Key("foo-key")[0], *gotFooKey)
	assert.Equal(t, wantKeys.Key("bar-key")[0], *gotBarKey)
}

func TestRemoteKeySet_KeysReturnsNilWhenKeyIsUnknown(t *testing.T) {
	hdlr := func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("Cache-Control", "max-age=600")
		_, _ = rw.Write([]byte(jwkeys))
	}

	srv := httptest.NewServer(http.HandlerFunc(hdlr))
	defer srv.Close()

	ks := jwt.NewRemoteKeySet(srv.URL)

	gotKey, err := ks.Key(context.Background(), "meh-key")
	require.NoError(t, err)

	assert.Nil(t, gotKey)
}

const jwkeys = `
{
  "keys": [
    {
      "e": "AQAB",
      "kty": "RSA",
      "alg": "RS256",
      "n": "1sUr077w2aaSnm08qFmuH1UON9e2n6vDNlUxm6WgM95n0_x1GwWTrhXtd_6U6x6R6m-50mVS_ki2BHZ9Fj3Y9W5zBww_TNyNLp4b1802gbXeGhVtQMcFQQ-hFne5HaTVTi1y6QNbu_3V1NW6nNAbpR_t79l1WzGiN4ilFiYFU0OVjk7isf7Dv3-6Trz9riHBExl34qhriu3x5pfipPT1rf4J6jMroJTEeU6L7zd9k_BwjNtptS8wAenYaK4FENR2gxvWWTX40i548Sh-3Ffprlu_9CZCswCkQCdhTq9lo3DbZYPEcW4aOLBEi3FfLiFm-DNDK_P_gBtNz8gW3VMQ2w",
      "use": "sig",
      "kid": "foo-key"
    },
    {
      "e": "AQAB",
      "kty": "RSA",
      "alg": "RS256",
      "n": "ya_7gVJrvqFp5xfYPOco8gBLY38kQDlTlT6ueHtUtbTkRVE1X5tFmPqChnX7wWd2fK7MS4-nclYaGLL7IvJtN9tjrD0h_3_HvnrRZTaVyS-yfWqCQDRq_0VW1LBEygwYRqbO2T0lOocTY-5qUosDvJfe-o-lQYMH7qtDAyiq9XprVzKYTfS545BTECXi0he9ikJl5Q_RAP1BZoaip8F0xX5Y_60G90VyXFWuy16nm5ASW8fwqzdn1lL_ogiO1LirgBFFEXz_t4PwmjWzfQwkoKv4Ab_l9u2FdAoKtFH2CwKaGB8hatIK3bOAJJgRebeU3w6Ah3gxRfi8HWPHbAGjtw",
      "use": "sig",
      "kid": "bar-key"
    }
  ]
}
`
