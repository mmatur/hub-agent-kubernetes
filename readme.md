# Traefik Hub Agent for Kubernetes

## Usage

### Commands

```
NAME:
   Traefik Hub agent for Kubernetes - Manages a Traefik Hub agent installation

USAGE:
   hub-agent-kubernetes [global options] command [command options] [arguments...]

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
   hub-agent-kubernetes controller - Runs the Hub agent controller

USAGE:
   hub-agent-kubernetes controller [command options] [arguments...]

OPTIONS:
   --token value                        The token to use for Hub platform API calls [$TOKEN]
   --log-level value                    Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
   --acp-server.listen-addr value       Address on which the access control policy server listens for admission requests (default: "0.0.0.0:443") [$ACP_SERVER_LISTEN_ADDR]
   --acp-server.cert value              Certificate used for TLS by the ACP server (default: "/var/run/hub-agent-kubernetes/cert.pem") [$ACP_SERVER_CERT]
   --acp-server.key value               Key used for TLS by the ACP server (default: "/var/run/hub-agent-kubernetes/key.pem") [$ACP_SERVER_KEY]
   --acp-server.auth-server-addr value  Address the ACP server can reach the auth server on (default: "http://hub-agent-auth-server.hub.svc.cluster.local") [$ACP_SERVER_AUTH_SERVER_ADDR]
   --help, -h                           show help (default: false)
```

### Auth Server

```
NAME:
   hub-agent-kubernetes auth-server - Runs the Hub agent authentication server

USAGE:
   hub-agent-kubernetes auth-server [command options] [arguments...]

OPTIONS:
   --listen-addr value  Address on which the auth server listens for auth requests (default: "0.0.0.0:80") [$AUTH_SERVER_LISTEN_ADDR]
   --log-level value    Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
   --help, -h           show help (default: false)
```

### Refresh Config

```
NAME:
   hub-agent-kubernetes refresh-config - Refresh agent configuration

USAGE:
   hub-agent-kubernetes refresh-config [command options] [arguments...]

OPTIONS:
   --log-level value  Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
   --help, -h         show help (default: false)
```

### Tunnel

```
NAME:
   hub-agent-kubernetes tunnel - Runs the Hub agent tunnel

USAGE:
   hub-agent-kubernetes tunnel [command options] [arguments...]

OPTIONS:
   --token value      The token to use for Hub platform API calls [$TOKEN]
   --log-level value  Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
   --help, -h         show help (default: false)
```

## Debugging the Agent

See [debug.md](./scripts/debug.md) for more information.
