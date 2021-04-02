package state

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (f *Fetcher) getApps() (map[string]*App, error) {
	deployments, err := f.k8s.Apps().V1().Deployments().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	result := make(map[string]*App)
	for _, deployment := range deployments {
		key := "Deployment/" + objectKey(deployment.Name, deployment.Namespace)
		result[key] = appFromDeployment(deployment)
	}

	statefulSets, err := f.k8s.Apps().V1().StatefulSets().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, statefulSet := range statefulSets {
		key := "StatefulSet/" + objectKey(statefulSet.Name, statefulSet.Namespace)
		result[key] = appFromStatefulSet(statefulSet)
	}

	replicaSets, err := f.k8s.Apps().V1().ReplicaSets().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, replicaSet := range replicaSets {
		if isOwnedByDeployment(replicaSet) {
			continue
		}

		key := "ReplicaSet/" + objectKey(replicaSet.Name, replicaSet.Namespace)
		result[key] = appFromReplicaSet(replicaSet)
	}

	daemonSets, err := f.k8s.Apps().V1().DaemonSets().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, daemonSet := range daemonSets {
		key := "DaemonSet/" + objectKey(daemonSet.Name, daemonSet.Namespace)
		result[key] = appFromDaemonSet(daemonSet)
	}

	return result, nil
}

func isOwnedByDeployment(replicaSet *appsv1.ReplicaSet) bool {
	for _, ownerReference := range replicaSet.OwnerReferences {
		if ownerReference.Kind == "Deployment" {
			return true
		}
	}

	return false
}

func appFromDeployment(deployment *appsv1.Deployment) *App {
	return &App{
		Kind:          "Deployment",
		Name:          deployment.Name,
		Namespace:     deployment.Namespace,
		Replicas:      int(*deployment.Spec.Replicas),
		ReadyReplicas: int(deployment.Status.AvailableReplicas),
		Images:        getImages(deployment.Spec.Template.Spec.Containers),
		Labels:        deployment.Labels,
		podLabels:     deployment.Spec.Template.Labels,
	}
}

func appFromStatefulSet(statefulSet *appsv1.StatefulSet) *App {
	return &App{
		Kind:          "StatefulSet",
		Name:          statefulSet.Name,
		Namespace:     statefulSet.Namespace,
		Replicas:      int(*statefulSet.Spec.Replicas),
		ReadyReplicas: int(statefulSet.Status.ReadyReplicas),
		Images:        getImages(statefulSet.Spec.Template.Spec.Containers),
		Labels:        statefulSet.Labels,
		podLabels:     statefulSet.Spec.Template.Labels,
	}
}

func appFromReplicaSet(replicaSet *appsv1.ReplicaSet) *App {
	return &App{
		Kind:          "ReplicaSet",
		Name:          replicaSet.Name,
		Namespace:     replicaSet.Namespace,
		Replicas:      int(*replicaSet.Spec.Replicas),
		ReadyReplicas: int(replicaSet.Status.AvailableReplicas),
		Images:        getImages(replicaSet.Spec.Template.Spec.Containers),
		Labels:        replicaSet.Labels,
		podLabels:     replicaSet.Spec.Template.Labels,
	}
}

func appFromDaemonSet(daemonSet *appsv1.DaemonSet) *App {
	return &App{
		Kind:          "DaemonSet",
		Name:          daemonSet.Name,
		Namespace:     daemonSet.Namespace,
		Replicas:      int(daemonSet.Status.DesiredNumberScheduled),
		ReadyReplicas: int(daemonSet.Status.NumberAvailable),
		Images:        getImages(daemonSet.Spec.Template.Spec.Containers),
		Labels:        daemonSet.Labels,
		podLabels:     daemonSet.Spec.Template.Labels,
	}
}

func getImages(containers []corev1.Container) []string {
	var result []string

	known := make(map[string]struct{})
	for _, container := range containers {
		if _, ok := known[container.Image]; !ok {
			result = append(result, container.Image)
			known[container.Image] = struct{}{}
		}
	}

	return result
}
