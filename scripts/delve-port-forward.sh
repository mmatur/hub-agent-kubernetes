#!/bin/bash

pf_pid=$(pgrep -fx "kubectl port-forward -n hub-agent svc/hub-agent 40000")

if [ -n "$pf_pid" ]
then
  echo 'Stopping existing port-forward'
  kill -SIGTERM $pf_pid
fi

kubectl port-forward -n hub-agent svc/hub-agent 40000 &>/dev/null &
