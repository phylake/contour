package k8s

import (
	"k8s.io/client-go/informers"
)

func (c *Clients) NewInformerFactoryWithOptions(option informers.SharedInformerOption) informers.SharedInformerFactory {
	return informers.NewSharedInformerFactoryWithOptions(c.core, resyncInterval, option)
}
