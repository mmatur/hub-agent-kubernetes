# Traefik Hub Agent for Kubernetes

<p align="center">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="./traefik-hub-horizontal-dark-mode@3x.png">
      <source media="(prefers-color-scheme: light)" srcset="./traefik-hub-horizontal-light-mode@3x.png">
      <img alt="Traefik Hub Logo" src="./traefik-hub-horizontal-light-mode@3x.png">
    </picture>
</p>

## Usage

### Commands

```
NAME:
   Traefik Hub agent for Kubernetes - Manages a Traefik Hub agent installation

USAGE:
   Traefik Hub agent for Kubernetes [global options] command [command options] [arguments...]

VERSION:
   dev, build  on 

COMMANDS:
   controller      Runs the Hub agent controller
   auth-server     Runs the Hub agent authentication server
   refresh-config  Refresh agent configuration
   tunnel          Runs the Hub agent tunnel
   version         Shows the Hub Agent version information
   help, h         Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
```

### Controller

```
NAME:
   Traefik Hub agent for Kubernetes controller - Runs the Hub agent controller

USAGE:
   Traefik Hub agent for Kubernetes controller [command options] [arguments...]

OPTIONS:
   --acp-server.auth-server-addr value  Address the ACP server can reach the auth server on (default: "http://hub-agent-auth-server.hub.svc.cluster.local") [$ACP_SERVER_AUTH_SERVER_ADDR]
   --acp-server.cert value              Certificate used for TLS by the ACP server (default: "/var/run/hub-agent-kubernetes/cert.pem") [$ACP_SERVER_CERT]
   --acp-server.key value               Key used for TLS by the ACP server (default: "/var/run/hub-agent-kubernetes/key.pem") [$ACP_SERVER_KEY]
   --acp-server.listen-addr value       Address on which the access control policy server listens for admission requests (default: "0.0.0.0:443") [$ACP_SERVER_LISTEN_ADDR]
   --ingress-class-name value           The ingress class name used for ingresses managed by Hub [$INGRESS_CLASS_NAME]
   --log-level value                    Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
   --token value                        The token to use for Hub platform API calls [$TOKEN]
   --traefik.entryPoint value           The entry point used by Traefik to expose tunnels (default: "traefikhub-tunl") [$TRAEFIK_ENTRY_POINT]
   --traefik.metrics-url value          The url used by Traefik to expose metrics [$TRAEFIK_METRICS_URL]
```

### Auth Server

```
NAME:
   Traefik Hub agent for Kubernetes auth-server - Runs the Hub agent authentication server

USAGE:
   Traefik Hub agent for Kubernetes auth-server [command options] [arguments...]

OPTIONS:
   --listen-addr value  Address on which the auth server listens for auth requests (default: "0.0.0.0:80") [$AUTH_SERVER_LISTEN_ADDR]
   --log-level value    Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
```

### Refresh Config

```
NAME:
   Traefik Hub agent for Kubernetes refresh-config - Refresh agent configuration

USAGE:
   Traefik Hub agent for Kubernetes refresh-config [command options] [arguments...]

OPTIONS:
   --log-level value  Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
```

### Tunnel

```
NAME:
   Traefik Hub agent for Kubernetes tunnel - Runs the Hub agent tunnel

USAGE:
   Traefik Hub agent for Kubernetes tunnel [command options] [arguments...]

OPTIONS:
   --log-level value            Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
   --token value                The token to use for Hub platform API calls [$TOKEN]
   --traefik.tunnel-host value  The Traefik tunnel host [$TRAEFIK_TUNNEL_HOST]
   --traefik.tunnel-port value  The Traefik tunnel port (default: "9901") [$TRAEFIK_TUNNEL_PORT]
```

## Debugging the Agent

See [debug.md](./scripts/debug.md) for more information.
