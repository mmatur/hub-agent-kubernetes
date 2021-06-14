# Hub Agent

## Usage

### Commands

```
NAME:
   Hub agent CLI - Manages a Traefik Hub agent installation

USAGE:
   hub-agent [global options] command [command options] [arguments...]

COMMANDS:
   controller   Runs the Hub agent controller
   auth-server  Runs the Hub agent authentication server
   help, h      Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h  show help (default: false)
```

### Controller

```
NAME:
   hub-agent controller - Runs the Hub agent controller

USAGE:
   hub-agent controller [command options] [arguments...]

OPTIONS:
   --token value                        The token to use for Hub platform API calls [$TOKEN]
   --log-level value                    Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
   --acp-server.listen-addr value       Address on which the access control policy server listens for admission requests (default: "0.0.0.0:443") [$ACP_SERVER_LISTEN_ADDR]
   --acp-server.cert value              Certificate used for TLS by the ACP server (default: "/var/run/hub-agent/cert.pem") [$ACP_SERVER_CERT]
   --acp-server.key value               Key used for TLS by the ACP server (default: "/var/run/hub-agent/key.pem") [$ACP_SERVER_KEY]
   --acp-server.auth-server-addr value  Address the ACP server can reach the auth server on (default: "http://hub-agent-auth-server.hub.svc.cluster.local") [$ACP_SERVER_AUTH_SERVER_ADDR]
   --help, -h                           show help (default: false)
```

### Auth Server

```
NAME:
   hub-agent auth-server - Runs the Hub agent authentication server

USAGE:
   hub-agent auth-server [command options] [arguments...]

OPTIONS:
   --listen-addr value  Address on which the auth server listens for auth requests (default: "0.0.0.0:80") [$AUTH_SERVER_LISTEN_ADDR]
   --log-level value    Log level to use (debug, info, warn, error or fatal) (default: "info") [$LOG_LEVEL]
   --help, -h           show help (default: false)
```

## Debugging the Agent

See [debug.md](./scripts/debug.md) for more information.
