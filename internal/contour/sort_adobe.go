// utility to sort various objects
package contour

import (
	v1 "k8s.io/api/core/v1"
)

type endpointByIP []v1.EndpointAddress

func (e endpointByIP) Len() int           { return len(e) }
func (e endpointByIP) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }
func (e endpointByIP) Less(i, j int) bool { return e[i].IP < e[i].IP }
