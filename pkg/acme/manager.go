package acme

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/acme/client"
	corev1 "k8s.io/api/core/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
)

// labelManagedBy is a label used to indicate that a secret is managed by the agent.
const labelManagedBy = "app.kubernetes.io/managed-by"

// Annotations used to store metadata about the certificate stored in a secret.
const (
	annotationCertificateNotAfter  = "neo.traefik.io/certificate-not-after"
	annotationCertificateNotBefore = "neo.traefik.io/certificate-not-before"
	annotationCertificateDomains   = "neo.traefik.io/certificate-domains"
)

// maxRetries is the number of times a certificate request will be retried before it is dropped out of the queue.
const maxRetries = 10

// defaultRetryAfter is the duration to wait before queueing again the certificate request when the certificate issuance is pending in the platform.
const defaultRetryAfter = 10 * time.Second

// CertResolver is responsible of resolving certificates.
type CertResolver interface {
	Obtain(ctx context.Context, domains []string) (client.Certificate, error)
}

// CertificateRequest represents the certificate needed by an Ingress.
type CertificateRequest struct {
	Domains    []string
	Namespace  string
	SecretName string
}

// Manager manages the Ingress certificates.
type Manager struct {
	certs         CertResolver
	reqsMu        sync.RWMutex
	reqs          map[string]CertificateRequest
	workqueue     workqueue.RateLimitingInterface
	kubeInformers informers.SharedInformerFactory
	kubeClient    clientset.Interface
}

// NewManager returns a new manager instance.
func NewManager(certs CertResolver, kubeClient clientset.Interface) *Manager {
	kubeInformers := informers.NewSharedInformerFactory(kubeClient, defaultResync)
	kubeInformers.Core().V1().Secrets().Informer()

	return &Manager{
		certs:         certs,
		workqueue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		reqs:          make(map[string]CertificateRequest),
		kubeClient:    kubeClient,
		kubeInformers: kubeInformers,
	}
}

// Run starts the manager routine.
func (m *Manager) Run(ctx context.Context) error {
	defer m.workqueue.ShutDown()

	m.kubeInformers.Start(ctx.Done())

	for typ, ok := range m.kubeInformers.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for k8s object caches to sync %s", typ)
		}
	}

	go m.runRenewer(ctx)
	go wait.Until(m.runWorker, time.Second, ctx.Done())

	<-ctx.Done()

	return nil
}

// ObtainCertificate enqueues a certificate request to be resolved asynchronously.
func (m *Manager) ObtainCertificate(req CertificateRequest) {
	key := req.SecretName + "@" + req.Namespace

	m.reqsMu.Lock()
	m.reqs[key] = req
	m.reqsMu.Unlock()

	m.workqueue.Add(key)
}

func (m *Manager) runWorker() {
	for m.processNextWorkItem() {
	}
}

func (m *Manager) processNextWorkItem() bool {
	obj, shutdown := m.workqueue.Get()
	if shutdown {
		return false
	}

	defer m.workqueue.Done(obj)

	key, ok := obj.(string)
	if !ok {
		m.workqueue.Forget(obj)
		return true
	}

	m.reqsMu.RLock()
	req, exists := m.reqs[key]
	m.reqsMu.RUnlock()

	if !exists {
		m.workqueue.Forget(obj)
		return true
	}

	err := m.resolveAndStoreCertificate(req)

	var pendingErr client.PendingError
	if errors.As(err, &pendingErr) {
		m.workqueue.AddAfter(key, defaultRetryAfter)
		return true
	}

	if err != nil && !kerror.HasStatusCause(err, corev1.NamespaceTerminatingCause) && m.workqueue.NumRequeues(key) < maxRetries {
		log.Error().
			Err(err).
			Str("namespace", req.Namespace).
			Str("secret", req.SecretName).
			Strs("domains", req.Domains).
			Msg("Unable to obtain certificate")

		m.workqueue.AddRateLimited(key)
		return true
	}

	m.reqsMu.Lock()
	delete(m.reqs, key)
	m.reqsMu.Unlock()

	m.workqueue.Forget(key)
	return true
}

// TODO Check that the created secret is still used, because the Ingress/IngressRoute could have been deleted/updated before the completion of the task.
func (m *Manager) resolveAndStoreCertificate(req CertificateRequest) error {
	cert, err := m.certs.Obtain(context.Background(), req.Domains)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		Type: "kubernetes.io/tls",
		ObjectMeta: metav1.ObjectMeta{
			Namespace: req.Namespace,
			Name:      req.SecretName,
			Labels: map[string]string{
				labelManagedBy: controllerName,
			},
			Annotations: map[string]string{
				annotationCertificateDomains:   strings.Join(cert.Domains, ","),
				annotationCertificateNotBefore: strconv.Itoa(int(cert.NotBefore.UTC().Unix())),
				annotationCertificateNotAfter:  strconv.Itoa(int(cert.NotAfter.UTC().Unix())),
			},
		},
		Data: map[string][]byte{
			"tls.crt": cert.Certificate,
			"tls.key": cert.PrivateKey,
		},
	}

	_, err = m.kubeClient.CoreV1().Secrets(req.Namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil && !kerror.IsAlreadyExists(err) {
		return fmt.Errorf("create secret: %w", err)
	}
	if err == nil {
		return nil
	}

	_, updateErr := m.kubeClient.CoreV1().Secrets(req.Namespace).Update(context.Background(), secret, metav1.UpdateOptions{})
	if updateErr != nil {
		return fmt.Errorf("update secret: %w", updateErr)
	}
	return nil
}

func (m *Manager) runRenewer(ctx context.Context) {
	if err := m.renewExpiringCertificates(); err != nil {
		log.Error().Err(err).Msg("Unable to renew expiring certificates")
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if err := m.renewExpiringCertificates(); err != nil {
				log.Error().Err(err).Msg("Unable to renew expiring certificates")
			}
		}
	}
}

func (m *Manager) renewExpiringCertificates() error {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			labelManagedBy: controllerName,
		},
	})
	if err != nil {
		return fmt.Errorf("create label selector: %w", err)
	}

	secrets, err := m.kubeInformers.Core().V1().Secrets().Lister().List(selector)
	if err != nil {
		return fmt.Errorf("list secrets: %w", err)
	}

	for _, secret := range secrets {
		notAfter, err := strconv.Atoi(secret.Annotations[annotationCertificateNotAfter])
		if err != nil {
			log.Error().
				Err(err).
				Str("namespace", secret.Namespace).
				Str("secret", secret.Name).
				Msg("Unable to parse notAfter timestamp")

			continue
		}

		notAfterTime := time.Unix(int64(notAfter), 0)

		// The stored certificate has not exceeded two third of its total lifetime (90days for let's encrypt).
		if time.Now().Add(30 * 24 * time.Hour).Before(notAfterTime) {
			continue
		}

		m.ObtainCertificate(CertificateRequest{
			Domains:    getCertificateDomains(secret),
			Namespace:  secret.Namespace,
			SecretName: secret.Name,
		})
	}

	return nil
}

func isManagedSecret(secret *corev1.Secret) bool {
	return secret.Labels[labelManagedBy] == controllerName
}

func getCertificateDomains(secret *corev1.Secret) []string {
	domains, exists := secret.Annotations[annotationCertificateDomains]
	if !exists {
		return nil
	}
	return strings.Split(domains, ",")
}
