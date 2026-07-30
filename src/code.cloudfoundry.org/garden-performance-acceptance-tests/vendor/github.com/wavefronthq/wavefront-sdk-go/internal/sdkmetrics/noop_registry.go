package sdkmetrics

func NewNoOpRegistry() Registry {
	return &noOpRegistry{}
}

type noOpRegistry struct{}

func (n *noOpRegistry) Flush() {
}

func (n *noOpRegistry) PointsTracker() SuccessTracker {
	return &noOpTracker{}
}

func (n *noOpRegistry) HistogramsTracker() SuccessTracker {
	return &noOpTracker{}
}

func (n *noOpRegistry) SpansTracker() SuccessTracker {
	return &noOpTracker{}
}

func (n *noOpRegistry) SpanLogsTracker() SuccessTracker {
	return &noOpTracker{}
}

func (n *noOpRegistry) EventsTracker() SuccessTracker {
	return &noOpTracker{}
}

func (n *noOpRegistry) Start() {
}

func (n *noOpRegistry) Stop() {
}

func (n *noOpRegistry) NewGauge(string, func() int64) *FunctionalGauge {
	return &FunctionalGauge{}
}

type noOpTracker struct{}

func (n noOpTracker) IncValid() {
}

func (n noOpTracker) IncInvalid() {
}

func (n noOpTracker) IncDropped() {
}
