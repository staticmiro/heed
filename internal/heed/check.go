package heed

type State string

const (
	StateOK       State = "ok"
	StateWarning  State = "warning"
	StateCritical State = "critical"
	StateUnknown  State = "unknown"
)

type Result struct {
	Name      string
	State     State
	Value     string
	Message   string
	Numeric   float64
	Threshold float64
}

type Checker interface {
	Name() string
	Check() Result
}
