#!/bin/bash

ctrlr_pf_pid=$(pgrep -fx "kubectl port-forward -n hub-agent svc/hub-agent-controller 40000:40000 &>/dev/null")
authsrvr_pf_pid=$(pgrep -fx "kubectl port-forward -n hub-agent svc/hub-agent-auth-server 39999:40000 &>/dev/null")

if [ -n "$ctrlr_pf_pid" ]
then
  echo 'Stopping existing controller port-forward'
  kill -SIGTERM $ctrlr_pf_pid
fi

if [ -n "authsrvr_pf_pid" ]
then
  echo 'Stopping existing auth server port-forward'
  kill -SIGTERM $authsrvr_pf_pid
fi

kubectl port-forward -n hub-agent svc/hub-agent-controller 40000:40000 &>/dev/null &
kubectl port-forward -n hub-agent svc/hub-agent-auth-server 39999:40000 &>/dev/null &
