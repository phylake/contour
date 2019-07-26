package main

import (
	clientset "github.com/heptio/contour/apis/generated/clientset/versioned"
	"github.com/heptio/contour/internal/contour"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func initCache(client *kubernetes.Clientset, contourClient *clientset.Clientset, reh *contour.ResourceEventHandler) error {
	// Services
	services, err := client.CoreV1().Services("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	reh.WithField("count", len(services.Items)).Info("caching services")
	for i := range services.Items {
		reh.Insert(&services.Items[i])
	}

	// Secrets
	secrets, err := client.CoreV1().Secrets("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	reh.WithField("count", len(secrets.Items)).Info("caching secrets")
	for i := range secrets.Items {
		reh.Insert(&secrets.Items[i])
	}

	// Endpoints
	endpoints, err := client.CoreV1().Endpoints("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	reh.WithField("count", len(endpoints.Items)).Info("caching endpoints")
	for i := range endpoints.Items {
		reh.Insert(&endpoints.Items[i])
	}

	// Ingresses
	ingresses, err := client.ExtensionsV1beta1().Ingresses("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	reh.WithField("count", len(ingresses.Items)).Info("caching ingresses")
	for i := range ingresses.Items {
		reh.Insert(&ingresses.Items[i])
	}

	// IngressRoutes
	irs, err := contourClient.ContourV1beta1().IngressRoutes("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	reh.WithField("count", len(irs.Items)).Info("caching ingressroutes")
	for i := range irs.Items {
		reh.Insert(&irs.Items[i])
	}

	// TLSCertificateDelegations
	tlscds, err := contourClient.ContourV1beta1().TLSCertificateDelegations("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	reh.WithField("count", len(tlscds.Items)).Info("caching tlscertdelegations")
	for i := range tlscds.Items {
		reh.Insert(&tlscds.Items[i])
	}

	// Now rebuild the dag
	reh.Update()
	return nil
}
