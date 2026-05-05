package apierror

type Error struct {
	Code    string
	Message string
	Status  int
}

func (e Error) Error() string {
	return e.Message
}

func New(code, message string, status int) Error {
	return Error{Code: code, Message: message, Status: status}
}
