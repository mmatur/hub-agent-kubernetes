# Remotely Debug the Agent

When using the dev image of the Agent (`make image-dev`), it actually runs the binary of the Agent wrapped in the
debugger to enable remote debugging.

## Configuring Goland

1. Edit your build configurations; add a `Go Remote` configuration.
2. Set the port to `40000`
3. Add a before launch script
4. Select `Run External Tool`
5. Add an external tool using the + sign, name it like you want (`setup debugging port-forward` for example)
6. Set the program to `/<where-you-cloned-the-agent>/neo-agent/scripts/delve-port-forward.sh`
7. Spam ok/apply
8. Everything should work™, meaning you can set breakpoints anywhere in the agent code and use the build configuration
   you just created.

> The port-forward script is here because restarting a pod breaks the port-forward for some reason (even if it's pointing to the service in front of the pod). Running it as a "before launch script" in GoLand will take care of everything for you when starting a debugging session.

> Also note that this requires the port 40000 to be exposed on the service of the agent, which should already be the case if you deployed the agent using the `github.com/traefik/neo` repository. If it's not the case, you can expose it with a command like: `kubectl patch svc -n neo-agent neo-agent -p '{"spec":{"ports":[{"name":"neo-agent-debug","port":40000}]}}'`.
