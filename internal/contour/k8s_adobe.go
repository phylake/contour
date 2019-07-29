package contour

// Used during synchronous cache initialization so that update() is called
// only once instead of after every inserts
func (reh *ResourceEventHandler) Update() {
	reh.update()
}

// Exposes IngressClass validation to synchronous cache initialization
func (reh *ResourceEventHandler) ValidIngressClass(obj interface{}) bool {
	return reh.validIngressClass(obj)
}
