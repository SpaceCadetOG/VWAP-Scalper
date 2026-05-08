package signing

// SignedRequest is a generic signed request payload for adapter transports.
type SignedRequest struct {
	QueryString string
	Headers     map[string]string
}
