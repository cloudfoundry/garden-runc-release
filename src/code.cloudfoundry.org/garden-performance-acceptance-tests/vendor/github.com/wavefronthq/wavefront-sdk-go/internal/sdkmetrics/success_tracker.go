package sdkmetrics

type SuccessTracker interface {
	IncValid()
	IncInvalid()
	IncDropped()
}

type realSuccessTracker struct {
	Valid   *DeltaCounter
	Invalid *DeltaCounter
	Dropped *DeltaCounter
}

func (f *realSuccessTracker) IncValid() {
	f.Valid.Inc()
}

func (f *realSuccessTracker) IncInvalid() {
	f.Invalid.Inc()
}

func (f *realSuccessTracker) IncDropped() {
	f.Dropped.Inc()
}
