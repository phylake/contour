package contour

// Used during synchronous cache initialization so that update() is called
// only once instead of after every inserts
func (reh *EventHandler) UpdateDAG() {
	reh.updateDAG()
}
