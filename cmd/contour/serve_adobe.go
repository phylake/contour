package main

import (
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// func initCache(client *kubernetes.Clientset, contourClient *clientset.Clientset, eh *contour.EventHandler, et *contour.EndpointsTranslator) error {
func initCache(clients *k8s.Clients, eh *contour.EventHandler, et *contour.EndpointsTranslator) error {
	eh.Info("starting cache initialization")

	client := clients.ClientSet()
	contourClient := clients.DynamicClient()
	converter, err := k8s.NewUnstructuredConverter()
	if err != nil {
		return err
	}

	// Ingresses
	ingresses, err := client.NetworkingV1beta1().Ingresses("").List(metav1.ListOptions{})
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
	irs, err := contourClient.Resource(ingressroutev1.IngressRouteGVR).Namespace("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	irCached := 0
	for i := range irs.Items {
		ir, err := converter.Convert(&irs.Items[i])
		if err != nil {
			return err
		}
		if eh.Builder.Source.Insert(ir) {
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
	tlscds, err := contourClient.Resource(ingressroutev1.TLSCertificateDelegationGVR).Namespace("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for i := range tlscds.Items {
		tlscd, err := converter.Convert(&tlscds.Items[i])
		if err != nil {
			return err
		}
		eh.Builder.Source.Insert(tlscd)
	}
	eh.WithField("count", len(tlscds.Items)).Info("tlscertdelegations")

	// Secrets
	// only eager load tls secrets as they are the most used (if not the only one)
	// opaque and other generic secrets will still be discovered via the informer
	// see internal/dag/secrets.go:isValidSecret()
	secrets, err := client.CoreV1().Secrets("").List(metav1.ListOptions{
		FieldSelector: "type=" + string(v1.SecretTypeTLS),
	})
	if err != nil {
		return err
	}
	sCached := 0
	for i := range secrets.Items {
		if eh.Builder.Source.Insert(&secrets.Items[i]) {
			sCached++
		}
	}
	eh.WithField("count", sCached).WithField("found", len(secrets.Items)).WithField("type", v1.SecretTypeTLS).Info("secrets")

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
