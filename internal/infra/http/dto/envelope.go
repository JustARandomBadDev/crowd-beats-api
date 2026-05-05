package dto

type Envelope struct {
	Data  any            `json:"data"`
	Error *APIError      `json:"error"`
	Meta  map[string]any `json:"meta"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
