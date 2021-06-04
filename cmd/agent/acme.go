package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/acme"
	"github.com/traefik/hub-agent/pkg/acme/client"
	hubclientset "github.com/traefik/hub-agent/pkg/crd/generated/client/hub/clientset/versioned"
	traefikclientset "github.com/traefik/hub-agent/pkg/crd/generated/client/traefik/clientset/versioned"
	"golang.org/x/sync/errgroup"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func runACME(ctx context.Context, platformURL, token string) error {
	config, err := setupKubeConfig()
	if err != nil {
		return fmt.Errorf("create Kubernetes configuration: %w", err)
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

func setupKubeConfig() (*rest.Config, error) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		log.Debug().Msg("Creating in-cluster k8s certificates")
		return rest.InClusterConfig()
	}

	log.Debug().Str("kubeconfig", os.Getenv("KUBECONFIG")).Msg("Creating external k8s certificates")
	return clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
}
