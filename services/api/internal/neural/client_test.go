package neural

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestVerifyKeepsRequestBodyUntilTransportReadsIt(t *testing.T) {
	const secret = "secret-value"
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), secret) {
			t.Fatalf("credential was cleared before the request body was sent: %q", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"valid":true,"provider":"openai"}`)),
		}, nil
	})
	client := NewHTTPClient("http://ai.internal", "internal-token", &http.Client{Transport: transport})
	if err := client.Verify(context.Background(), "openai", []byte(secret)); err != nil {
		t.Fatal(err)
	}
}
