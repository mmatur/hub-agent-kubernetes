module github.com/traefik/hub-agent

go 1.16

require (
	github.com/abbot/go-http-auth v0.4.0
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/ettle/strcase v0.1.1
	github.com/go-chi/chi/v5 v5.0.3
	github.com/go-logr/logr v0.4.0
	github.com/google/go-cmp v0.5.4 // indirect
	github.com/gravitational/trace v1.1.14 // indirect
	github.com/hamba/avro v1.5.4
	github.com/hashicorp/go-retryablehttp v0.6.8
	github.com/hashicorp/go-version v1.2.1
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/ldez/go-git-cmd-wrapper/v2 v2.0.0
	github.com/pquerna/cachecontrol v0.0.0-20201205024021-ac21108117ac
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.15.0
	github.com/rs/zerolog v1.20.0
	github.com/sirupsen/logrus v1.8.0 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli/v2 v2.3.0
	github.com/vulcand/predicate v1.1.0
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	gopkg.in/square/go-jose.v2 v2.5.1
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/klog/v2 v2.5.0
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
)

replace github.com/abbot/go-http-auth => github.com/containous/go-http-auth v0.4.1-0.20210329152427-e70ce7ef1ade
