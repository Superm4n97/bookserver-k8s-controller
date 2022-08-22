package v1alpha1

type BookServerPhase string

const (
	BookServerPending BookServerPhase = "Pending"
	BookServerRunning BookServerPhase = "Running"
)
