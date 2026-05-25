package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var stargazersEndpoint = "https://api.github.com/repos/PVRLabs/aibadger/stargazers?per_page=100"

var ErrRateLimit = errors.New("GitHub API rate limit hit. Try again in an hour.")

var httpClient = &http.Client{Timeout: 10 * time.Second}

type stargazer struct {
	Login string `json:"login"`
}

// FetchStargazers fetches GitHub stargazers for the repository and returns the
// last 10 logins plus the total count signal used by the badge renderer.
func FetchStargazers() ([]string, int, error) {
	req, err := http.NewRequest(http.MethodGet, stargazersEndpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("Could not fetch data: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Could not fetch data: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// Continue below.
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, 0, ErrRateLimit
	default:
		return nil, 0, fmt.Errorf("Could not fetch data: GitHub API returned status %d", resp.StatusCode)
	}

	payload, err := decodeStargazers(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("Could not fetch data: malformed response from GitHub API: %w", err)
	}

	logins := make([]string, 0, len(payload))
	for _, stargazer := range payload {
		login := strings.TrimSpace(stargazer.Login)
		if login == "" {
			continue
		}
		logins = append(logins, login)
	}

	total := len(logins)
	if total == 0 {
		return []string{}, 0, nil
	}
	if total > 10 {
		logins = append([]string(nil), logins[total-10:]...)
	} else {
		logins = append([]string(nil), logins...)
	}
	if total >= 100 {
		total = 100
	}
	return logins, total, nil
}

func decodeStargazers(r io.Reader) ([]stargazer, error) {
	var payload []stargazer
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}
