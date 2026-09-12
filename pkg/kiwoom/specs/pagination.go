package specs

// Continuation carries Kiwoom's HTTP continuation headers. A Y response and its
// next key must be sent together when requesting the next page.
type Continuation struct {
	ContYN  string `json:"cont_yn,omitempty"`
	NextKey string `json:"next_key,omitempty"`
}

// EndpointPage contains one endpoint response and its continuation metadata.
// Data retains the same generated response type as the legacy CallEndpoint API.
type EndpointPage struct {
	Data any `json:"data"`
	Continuation
}
