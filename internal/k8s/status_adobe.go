package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
)

func (irs *StatusWriter) setIngressRouteStatusAdobe(updated *ingressroutev1.IngressRoute) error {
	usUpdated, err := irs.Converter.ToUnstructured(updated)
	if err != nil {
		return fmt.Errorf("unable to convert status update to IngressRoute: %s", err)
	}

	_, err = irs.Client.Resource(ingressroutev1.IngressRouteGVR).Namespace(updated.GetNamespace()).
		UpdateStatus(context.TODO(), usUpdated, metav1.UpdateOptions{})

	return err
}
