package main

import "net/http"

type testHTTPTransport func(*http.Request) (*http.Response, error)

func (transport testHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}
