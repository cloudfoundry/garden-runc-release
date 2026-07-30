package auth

type Err struct {
	error
}

func NewAuthError(err error) error {
	return &Err{error: err}
}

func (e *Err) Error() string {
	return e.error.Error()
}
