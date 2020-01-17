package dag

import (
	"testing"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/k8s"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
)

// customized version of TestAnnotationKindValidation()
// annotationIsKnown() isn't modified for now (because we warn only)
// validAnnotationForKind() invalidates annotations we don't want to support
func TestAdobeAnnotationKindValidation(t *testing.T) {
	type status struct {
		known bool
		valid bool
	}
	tests := map[string]struct {
		obj         Object
		annotations map[string]status
	}{
		"ingress": {
			obj: &v1beta1.Ingress{},
			annotations: map[string]status{
				"kubernetes.io/ingress.class": {
					known: true, valid: true,
				},
				"projectcontour.io/ingress.class": {
					known: true, valid: false,
				},
				"ingress.kubernetes.io/force-ssl-redirect": {
					known: true, valid: false,
				},
				"foo.bar.io": {
					known: false, valid: false,
				},
			},
		},
		"ingressroute": {
			obj: &ingressroutev1.IngressRoute{},
			annotations: map[string]status{
				"kubernetes.io/ingress.class": {
					known: true, valid: true,
				},
				"projectcontour.io/ingress.class": {
					known: true, valid: false,
				},
				"foo.bar.io": {
					known: false, valid: false,
				},
			},
		},
		"service": {
			obj: &v1.Service{},
			annotations: map[string]status{
				"foo.heptio.com/annotation": {
					known: false, valid: false,
				},
				"contour.heptio.com/annotation": {
					known: true, valid: false,
				},
				"projectcontour.io/annotation": {
					known: true, valid: false,
				},
				"contour.heptio.com/upstream-protocol.h2c": {
					known: true, valid: true,
				},
			},
		},
		"secrets": {
			obj: &v1.Secret{},
			annotations: map[string]status{
				// In contour.heptio.com namespace but not valid on this kind.
				"contour.heptio.com/ingress.class": {
					known: true, valid: false,
				},
				// Unknown, so potentially valid.
				"foo.io/secret-sauce": {
					known: false, valid: true,
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for k, s := range tc.annotations {
				assert.Equal(t, s.known, annotationIsKnown(k))
				assert.Equal(t, s.valid, validAnnotationForKind(k8s.KindOf(tc.obj), k))
			}
		})
	}
}
