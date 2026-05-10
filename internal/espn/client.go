package espn

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultBaseURL is ESPN's read-API host for fantasy. It moved from
// fantasy.espn.com to lm-api-reads.fantasy.espn.com in April 2024.
const DefaultBaseURL = "https://lm-api-reads.fantasy.espn.com/apis/v3/games"

// SportFLB is ESPN's identifier for fantasy baseball. Football is "ffl",
// basketball is "fba", hockey is "fhl".
const SportFLB = "flb"

// Client is a thin HTTP wrapper for the ESPN v3 fantasy baseball API.
//
// Auth is two cookies (SWID, espn_s2) the user grabs from a logged-in
// browser session — vastly simpler than Fantrax's chromedp dance.
// Both cookies are sent on every request; ESPN rejects private-league
// reads without them.
type Client struct {
	baseURL  string
	sport    string
	leagueID string
	teamID   int
	year     int
	swid     string
	espnS2   string
	http     *http.Client
}

// NewClient constructs a client for the given league. teamID is the user's
// fantasy team ID inside that league (0 if not needed for the call site).
// year defaults to the current calendar year if 0.
func NewClient(leagueID string, teamID int, year int, swid, espnS2 string) (*Client, error) {
	if leagueID == "" {
		return nil, fmt.Errorf("espn: leagueID required")
	}
	if swid == "" || espnS2 == "" {
		return nil, fmt.Errorf("espn: SWID and espn_s2 cookies required (export from your ESPN browser session)")
	}
	if year == 0 {
		year = time.Now().Year()
	}
	return &Client{
		baseURL:  DefaultBaseURL,
		sport:    SportFLB,
		leagueID: leagueID,
		teamID:   teamID,
		year:     year,
		swid:     swid,
		espnS2:   espnS2,
		http:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// SetBaseURL overrides the API host. Tests use this to point at httptest.
func (c *Client) SetBaseURL(u string) { c.baseURL = u }

// LeagueID returns the configured league ID.
func (c *Client) LeagueID() string { return c.leagueID }

// TeamID returns the configured team ID.
func (c *Client) TeamID() int { return c.teamID }

// Year returns the configured season year.
func (c *Client) Year() int { return c.year }

// leagueURL builds the per-league endpoint with optional ?view= params.
// ESPN supports passing multiple view= keys in one request; this returns
// the URL with each view appended in order.
func (c *Client) leagueURL(views []string) string {
	u := fmt.Sprintf("%s/%s/seasons/%d/segments/0/leagues/%s",
		c.baseURL, c.sport, c.year, c.leagueID)
	if len(views) == 0 {
		return u
	}
	q := url.Values{}
	for _, v := range views {
		q.Add("view", v)
	}
	return u + "?" + q.Encode()
}

// freeAgentsURL is the league endpoint used to load FAs. Critical: this is
// the per-league URL (NOT the global `/seasons/{year}/players` endpoint).
// ESPN's `filterStatus: FREEAGENT/WAIVERS` filter only honors per-league
// queries — hitting the global pool returns every MLB+MiLB player as if
// they were free agents because there's no league context to consult for
// ownership.
//
// `view=kona_player_info` is the canonical view that returns a top-level
// `players` array on the league response. cwendt94/espn-api uses the same.
func (c *Client) freeAgentsURL() string {
	u := fmt.Sprintf("%s/%s/seasons/%d/segments/0/leagues/%s",
		c.baseURL, c.sport, c.year, c.leagueID)
	q := url.Values{}
	q.Set("view", "kona_player_info")
	return u + "?" + q.Encode()
}

// get issues a GET, attaches auth cookies, optionally sets X-Fantasy-Filter,
// and decodes the body into out (which must be a pointer).
func (c *Client) get(rawURL string, fantasyFilter string, out any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("espn: build request: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: "SWID", Value: c.swid})
	req.AddCookie(&http.Cookie{Name: "espn_s2", Value: c.espnS2})
	req.Header.Set("Accept", "application/json")
	if fantasyFilter != "" {
		req.Header.Set("X-Fantasy-Filter", fantasyFilter)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("espn: request %s: %w", redactedURL(rawURL), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("espn: %d from %s — check SWID/espn_s2 cookies", resp.StatusCode, redactedURL(rawURL))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("espn: %d from %s: %s", resp.StatusCode, redactedURL(rawURL), string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("espn: decode %s: %w", redactedURL(rawURL), err)
	}
	return nil
}

// redactedURL strips query strings from a URL for error logging — keeps
// cookies (which aren't in the URL anyway) and X-Fantasy-Filter contents
// off stderr.
func redactedURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	parsed.RawQuery = ""
	return parsed.String()
}

// itoa is an internal alias for strconv.Itoa to keep call sites concise.
func itoa(i int) string { return strconv.Itoa(i) }
