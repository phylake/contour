package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func (irs *StatusWriter) setIngressRouteStatusAdobe(updated *ingressroutev1.IngressRoute) error {
	err := irs.setIngressRouteStatusAdobeImpl(updated)

	if err != nil {
		usLatest, err1 := irs.Client.Resource(ingressroutev1.IngressRouteGVR).Namespace(updated.Namespace).
			Get(context.TODO(), updated.Name, metav1.GetOptions{})
		if err1 != nil {
			// error while retriving the current one - abort
			return err
		}
		intLatest, err2 := irs.Converter.FromUnstructured(usLatest)
		if err2 != nil {
			// same as before - abort
			return err
		}
		irLatest := intLatest.(*ingressroutev1.IngressRoute)
		if irs.updateNeeded(updated.CurrentStatus, updated.Description, irLatest.Status) {
			irLatest.Status = projcontour.Status{
				CurrentStatus: updated.CurrentStatus,
				Description:   updated.Description,
			}
			err = irs.setIngressRouteStatusAdobeImpl(irLatest)
		}
	}
	return err
}

func (irs *StatusWriter) setIngressRouteStatusAdobeImpl(updated *ingressroutev1.IngressRoute) error {
	usUpdated, err := irs.Converter.ToUnstructured(updated)
	if err != nil {
		return fmt.Errorf("unable to convert status update to IngressRoute: %s", err)
	}

	_, err = irs.Client.Resource(ingressroutev1.IngressRouteGVR).Namespace(updated.GetNamespace()).
		UpdateStatus(context.TODO(), usUpdated, metav1.UpdateOptions{})

	return err
}
