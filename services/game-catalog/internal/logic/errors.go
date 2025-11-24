package logic

type apiError struct {
	code    int
	message string
}

func (e *apiError) Error() string {
	return e.message
}

func (e *apiError) StatusCode() int {
	return e.code
}

func newAPIError(code int, message string) error {
	return &apiError{code: code, message: message}
}
