package annotation

import (
	"fmt"
	"testing"

	"github.com/projectcontour/contour/internal/assert"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Ensure "legacy" 'contour.heptio.com' annotations are supported forever
func TestAdobeAnnotationKindValidation(t *testing.T) {
	type status struct {
		known bool
		valid bool
	}
	tests := map[string]struct {
		obj         metav1.ObjectMetaAccessor
		annotations map[string]status
	}{
		// Ingress
		// contour.heptio.com/num-retries: deprecated form of projectcontour.io/num-retries.
		// contour.heptio.com/per-try-timeout: deprecated form of projectcontour.io/per-try-timeout.
		// contour.heptio.com/request-timeout: deprecated form of projectcontour.io/response-timeout. Note this is response-timeout.
		// contour.heptio.com/retry-on: deprecated form of projectcontour.io/retry-on.
		// contour.heptio.com/tls-minimum-protocol-version: deprecated form of projectcontour.io/tls-minimum-protocol-version.
		// contour.heptio.com/websocket-routes: deprecated form of projectcontour.io/websocket-routes.
		// NOTE: contour.heptio.com/response-timeout was never supported, but is the correct name, therefore we support it.
		"ingress": {
			obj: &v1beta1.Ingress{},
			annotations: map[string]status{
				"contour.heptio.com/num-retries": {
					known: true, valid: true,
				},
				"contour.heptio.com/per-try-timeout": {
					known: true, valid: true,
				},
				"contour.heptio.com/request-timeout": {
					known: true, valid: true,
				},
				"contour.heptio.com/retry-on": {
					known: true, valid: true,
				},
				"contour.heptio.com/tls-minimum-protocol-version": {
					known: true, valid: true,
				},
				"contour.heptio.com/websocket-routes": {
					known: true, valid: true,
				},
				"contour.heptio.com/response-timeout": {
					known: true, valid: true,
				},
			},
		},
		// Service
		// contour.heptio.com/max-connections: deprecated form of projectcontour.io/max-connections
		// contour.heptio.com/max-pending-requests: deprecated form of projectcontour.io/max-pending-requests.
		// contour.heptio.com/max-requests: deprecated form of projectcontour.io/max-requests.
		// contour.heptio.com/max-retries: deprecated form of projectcontour.io/max-retries.
		// contour.heptio.com/upstream-protocol.{protocol} : deprecated form of projectcontour.io/upstream-protocol.{protocol}. (h2, h2c, and tls)
		"service": {
			obj: &v1.Service{},
			annotations: map[string]status{
				"contour.heptio.com/max-connections": {
					known: true, valid: true,
				},
				"contour.heptio.com/max-pending-requests": {
					known: true, valid: true,
				},
				"contour.heptio.com/max-requests": {
					known: true, valid: true,
				},
				"contour.heptio.com/max-retries": {
					known: true, valid: true,
				},
				"contour.heptio.com/upstream-protocol.h2": {
					known: true, valid: true,
				},
				"contour.heptio.com/upstream-protocol.h2c": {
					known: true, valid: true,
				},
				"contour.heptio.com/upstream-protocol.tls": {
					known: true, valid: true,
				},
			},
		},
	}

	for name, tc := range tests {
		for k, s := range tc.annotations {
			t.Run(fmt.Sprintf("%s:%s", name, k), func(t *testing.T) {
				assert.Equal(t, s.known, IsKnown(k))
				assert.Equal(t, s.valid, ValidForKind(kindOf(tc.obj), k))
			})
		}
	}
}
