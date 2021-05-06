package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
)

// hardcoded to secret for now
func (c *Clients) NewInformerFactoryWithOptions() informers.SharedInformerFactory {
	return informers.NewSharedInformerFactoryWithOptions(c.core, resyncInterval,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = "type=kubernetes.io/tls"
		}),
	)
}
