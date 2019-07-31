package main

import (
	clientset "github.com/heptio/contour/apis/generated/clientset/versioned"
	"github.com/heptio/contour/internal/contour"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func initCache(client *kubernetes.Clientset, contourClient *clientset.Clientset, reh *contour.ResourceEventHandler) error {
	reh.Info("starting cache initialization")

	// Services
	services, err := client.CoreV1().Services("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range services.Items {
		reh.Insert(&services.Items[i])
	}
	reh.WithField("count", len(services.Items)).Info("services")

	// Secrets
	secrets, err := client.CoreV1().Secrets("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range secrets.Items {
		reh.Insert(&secrets.Items[i])
	}
	reh.WithField("count", len(secrets.Items)).Info("secrets")

	// Endpoints
	endpoints, err := client.CoreV1().Endpoints("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range endpoints.Items {
		reh.Insert(&endpoints.Items[i])
	}
	reh.WithField("count", len(endpoints.Items)).Info("endpoints")

	// Ingresses
	ingresses, err := client.ExtensionsV1beta1().Ingresses("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	iCached := 0
	for i := range ingresses.Items {
		if reh.ValidIngressClass(&ingresses.Items[i]) {
			reh.Insert(&ingresses.Items[i])
			iCached++
		}
	}
	reh.WithField("count", iCached).WithField("found", len(ingresses.Items)).Info("ingresses")

	// IngressRoutes
	irs, err := contourClient.ContourV1beta1().IngressRoutes("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	irCached := 0
	for i := range irs.Items {
		if reh.ValidIngressClass(&irs.Items[i]) {
			reh.Insert(&irs.Items[i])
			irCached++
		}
	}
	reh.WithField("count", irCached).WithField("found", len(irs.Items)).Info("ingressroutes")

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

	reh.Info("finished cache initialization")
	return nil
}
