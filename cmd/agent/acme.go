package main

import (
	"context"
	"fmt"

	"github.com/traefik/hub-agent-kubernetes/pkg/acme"
	"github.com/traefik/hub-agent-kubernetes/pkg/acme/client"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	traefikclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned"
	"github.com/traefik/hub-agent-kubernetes/pkg/kube"
	"golang.org/x/sync/errgroup"
	clientset "k8s.io/client-go/kubernetes"
)

func runACME(ctx context.Context, platformURL, token string) error {
	config, err := kube.InClusterConfigWithRetrier(2)
	if err != nil {
		return fmt.Errorf("create Kubernetes in-cluster configuration: %w", err)
	}

	kubeClient, err := clientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("create Kubernetes clientset: %w", err)
	}

	hubClient, err := hubclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("create Hub clientset")
	}

	traefikClient, err := traefikclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("create Traefik clientset")
	}

	certs := client.New(platformURL, token)
	mgr := acme.NewManager(certs, kubeClient)

	ctrl, err := acme.NewController(mgr, kubeClient, hubClient, traefikClient)
	if err != nil {
		return fmt.Errorf("create controller: %w", err)
	}

	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		return mgr.Run(ctx)
	})

	group.Go(func() error {
		return ctrl.Run(ctx)
	})

	return group.Wait()
}
