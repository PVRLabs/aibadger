package github

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestFetchStargazersSuccessFortyTwo(t *testing.T) {
	originalEndpoint := stargazersEndpoint
	originalClient := httpClient
	stargazersEndpoint = "https://example.invalid/stargazers"
	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.Header.Get("Accept"), "application/vnd.github.v3+json"; got != want {
			t.Fatalf("Accept header = %q, want %q", got, want)
		}
		var b strings.Builder
		b.WriteByte('[')
		for i := 1; i <= 42; i++ {
			if i > 1 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"login":"user%02d"}`, i)
		}
		b.WriteByte(']')
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(b.String())),
			Request:    req,
		}, nil
	})}
	defer func() {
		stargazersEndpoint = originalEndpoint
		httpClient = originalClient
	}()

	logins, total, err := FetchStargazers()
	if err != nil {
		t.Fatalf("FetchStargazers() error = %v", err)
	}
	if total != 42 {
		t.Fatalf("total = %d, want 42", total)
	}
	if len(logins) != 10 {
		t.Fatalf("len(logins) = %d, want 10", len(logins))
	}
	for i, want := range []string{"user33", "user34", "user35", "user36", "user37", "user38", "user39", "user40", "user41", "user42"} {
		if logins[i] != want {
			t.Fatalf("logins[%d] = %q, want %q", i, logins[i], want)
		}
	}
}

func TestFetchStargazersSuccessGazillion(t *testing.T) {
	originalEndpoint := stargazersEndpoint
	originalClient := httpClient
	stargazersEndpoint = "https://example.invalid/stargazers"
	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		var b strings.Builder
		b.WriteByte('[')
		for i := 1; i <= 100; i++ {
			if i > 1 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"login":"user%03d"}`, i)
		}
		b.WriteByte(']')
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(b.String())),
			Request:    req,
		}, nil
	})}
	defer func() {
		stargazersEndpoint = originalEndpoint
		httpClient = originalClient
	}()

	logins, total, err := FetchStargazers()
	if err != nil {
		t.Fatalf("FetchStargazers() error = %v", err)
	}
	if total != 100 {
		t.Fatalf("total = %d, want 100", total)
	}
	if len(logins) != 10 {
		t.Fatalf("len(logins) = %d, want 10", len(logins))
	}
	if got, want := logins[0], "user091"; got != want {
		t.Fatalf("first login = %q, want %q", got, want)
	}
	if got, want := logins[9], "user100"; got != want {
		t.Fatalf("last login = %q, want %q", got, want)
	}
}

func TestFetchStargazersSuccessZero(t *testing.T) {
	originalEndpoint := stargazersEndpoint
	originalClient := httpClient
	stargazersEndpoint = "https://example.invalid/stargazers"
	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`[]`)),
			Request:    req,
		}, nil
	})}
	defer func() {
		stargazersEndpoint = originalEndpoint
		httpClient = originalClient
	}()

	logins, total, err := FetchStargazers()
	if err != nil {
		t.Fatalf("FetchStargazers() error = %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if len(logins) != 0 {
		t.Fatalf("len(logins) = %d, want 0", len(logins))
	}
}

func TestFetchStargazersRateLimit(t *testing.T) {
	originalEndpoint := stargazersEndpoint
	originalClient := httpClient
	stargazersEndpoint = "https://example.invalid/stargazers"
	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"rate limited"}`)),
			Request:    req,
		}, nil
	})}
	defer func() {
		stargazersEndpoint = originalEndpoint
		httpClient = originalClient
	}()

	_, _, err := FetchStargazers()
	if !errors.Is(err, ErrRateLimit) {
		t.Fatalf("error = %v, want ErrRateLimit", err)
	}
	if got, want := err.Error(), ErrRateLimit.Error(); got != want {
		t.Fatalf("error string = %q, want %q", got, want)
	}
}

func TestFetchStargazersMalformedJSON(t *testing.T) {
	originalEndpoint := stargazersEndpoint
	originalClient := httpClient
	stargazersEndpoint = "https://example.invalid/stargazers"
	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{not-json`)),
			Request:    req,
		}, nil
	})}
	defer func() {
		stargazersEndpoint = originalEndpoint
		httpClient = originalClient
	}()

	_, _, err := FetchStargazers()
	if err == nil {
		t.Fatal("FetchStargazers() error = nil, want malformed JSON error")
	}
	if got := err.Error(); !strings.Contains(got, "Could not fetch data: malformed response from GitHub API") {
		t.Fatalf("error = %q, want malformed JSON message", got)
	}
}

func TestFetchStargazersServerError(t *testing.T) {
	originalEndpoint := stargazersEndpoint
	originalClient := httpClient
	stargazersEndpoint = "https://example.invalid/stargazers"
	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"boom"}`)),
			Request:    req,
		}, nil
	})}
	defer func() {
		stargazersEndpoint = originalEndpoint
		httpClient = originalClient
	}()

	_, _, err := FetchStargazers()
	if err == nil {
		t.Fatal("FetchStargazers() error = nil, want server error")
	}
	if got := err.Error(); !strings.Contains(got, "Could not fetch data: GitHub API returned status 500") {
		t.Fatalf("error = %q, want readable server error", got)
	}
}
