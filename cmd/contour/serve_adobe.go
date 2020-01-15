package main

import (
	clientset "github.com/projectcontour/contour/apis/generated/clientset/versioned"
	"github.com/projectcontour/contour/internal/contour"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func initCache(client *kubernetes.Clientset, contourClient *clientset.Clientset, eh *contour.EventHandler, et *contour.EndpointsTranslator) error {
	eh.Info("starting cache initialization")

	// Ingresses
	ingresses, err := client.ExtensionsV1beta1().Ingresses("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	iCached := 0
	for i := range ingresses.Items {
		if eh.Builder.Source.Insert(&ingresses.Items[i]) {
			iCached++
		}
	}
	eh.WithField("count", iCached).WithField("found", len(ingresses.Items)).Info("ingresses")

	// IngressRoutes
	irs, err := contourClient.ContourV1beta1().IngressRoutes("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	irCached := 0
	for i := range irs.Items {
		if eh.Builder.Source.Insert(&irs.Items[i]) {
			irCached++
		}
	}
	eh.WithField("count", irCached).WithField("found", len(irs.Items)).Info("ingressroutes")

	// Services
	services, err := client.CoreV1().Services("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	svcCached := 0
	for i := range services.Items {
		if eh.Builder.Source.Insert(&services.Items[i]) {
			svcCached++
		}
	}
	eh.WithField("count", svcCached).WithField("found", len(services.Items)).Info("services")

	// TLSCertificateDelegations
	tlscds, err := contourClient.ContourV1beta1().TLSCertificateDelegations("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	eh.WithField("count", len(tlscds.Items)).Info("tlscertdelegations")
	for i := range tlscds.Items {
		eh.Builder.Source.Insert(&tlscds.Items[i])
	}

	// Secrets
	secrets, err := client.CoreV1().Secrets("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	sCached := 0
	for i := range secrets.Items {
		if eh.Builder.Source.Insert(&secrets.Items[i]) {
			sCached++
		}
	}
	eh.WithField("count", sCached).WithField("found", len(secrets.Items)).Info("secrets")

	// Endpoints
	endpoints, err := client.CoreV1().Endpoints("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range endpoints.Items {
		et.OnAdd(&endpoints.Items[i])
	}
	eh.WithField("count", len(endpoints.Items)).Info("endpoints")

	// Now rebuild the dag
	eh.UpdateDAG()

	eh.Info("finished cache initialization")
	return nil
}
