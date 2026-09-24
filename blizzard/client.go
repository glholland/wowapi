// Package blizzard is a small client for the Battle.net World of Warcraft APIs.
package blizzard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Namespace kinds used by the WoW APIs. The region suffix is added by the client.
const (
	Static  = "static"  // game data that changes with patches (items, recipes, talents)
	Dynamic = "dynamic" // game data that changes often (auctions, realms, tokens)
	Profile = "profile" // character data (refreshes when the character logs out)
)

// Client talks to the Battle.net API using the client-credentials OAuth flow.
type Client struct {
	ClientID     string
	ClientSecret string
	Region       string // us, eu, kr, tw
	Locale       string // en_US, en_GB, de_DE, ...

	// Overridable for tests.
	TokenURL string
	APIBase  string
	HTTP     *http.Client

	mu           sync.Mutex
	token        string
	tokenExp     time.Time
	cache        map[string]cacheEntry
	commodityIdx *commodityIndex
}

type cacheEntry struct {
	body []byte
	exp  time.Time
}

// NewFromEnv builds a client from BLIZZARD_CLIENT_ID, BLIZZARD_CLIENT_SECRET,
// WOW_REGION (default "us") and WOW_LOCALE (default "en_US").
func NewFromEnv() (*Client, error) {
	id, secret := os.Getenv("BLIZZARD_CLIENT_ID"), os.Getenv("BLIZZARD_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil, fmt.Errorf("BLIZZARD_CLIENT_ID and BLIZZARD_CLIENT_SECRET must be set (create a client at https://develop.battle.net/access/clients)")
	}
	return New(id, secret, envOr("WOW_REGION", "us"), envOr("WOW_LOCALE", "en_US")), nil
}

// New builds a client for the given region and locale.
func New(id, secret, region, locale string) *Client {
	region = strings.ToLower(region)
	return &Client{
		ClientID:     id,
		ClientSecret: secret,
		Region:       region,
		Locale:       locale,
		TokenURL:     "https://oauth.battle.net/token",
		APIBase:      fmt.Sprintf("https://%s.api.blizzard.com", region),
		HTTP:         &http.Client{Timeout: 60 * time.Second},
		cache:        map[string]cacheEntry{},
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// APIError is returned for non-2xx responses from the API.
type APIError struct {
	Status int
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("battle.net %s: HTTP %d", e.Path, e.Status)
	if e.Status == http.StatusNotFound && strings.HasPrefix(e.Path, "/profile/") {
		msg += " (character not found: check realm/name spelling, and note characters below level 10 or inactive for a long time are not exposed)"
	}
	if e.Body != "" {
		msg += ": " + truncate(e.Body, 300)
	}
	return msg
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL,
		strings.NewReader(url.Values{"grant_type": {"client_credentials"}}.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.ClientID, c.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request: HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("token response: %w", err)
	}
	c.token = tok.AccessToken
	// Refresh a minute early so a request never races the expiry.
	c.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second - time.Minute)
	return c.token, nil
}

// Get fetches an API path (e.g. "/data/wow/item/19019") in the given namespace
// kind (Static, Dynamic or Profile) and returns the raw JSON body.
func (c *Client) Get(ctx context.Context, path, namespace string, query url.Values) ([]byte, error) {
	return c.get(ctx, path, namespace, query, 0)
}

// get is Get with an optional in-memory cache TTL (0 disables caching).
func (c *Client) get(ctx context.Context, path, namespace string, query url.Values, ttl time.Duration) ([]byte, error) {
	if query == nil {
		query = url.Values{}
	}
	if query.Get("locale") == "" {
		query.Set("locale", c.Locale)
	}
	u := c.APIBase + path + "?" + query.Encode()
	ns := namespace + "-" + c.Region

	if ttl > 0 {
		c.mu.Lock()
		e, ok := c.cache[ns+" "+u]
		c.mu.Unlock()
		if ok && time.Now().Before(e.exp) {
			return e.body, nil
		}
	}

	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Battlenet-Namespace", ns)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		// Token revoked or expired early: drop it so the next call refreshes.
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{Status: resp.StatusCode, Path: path, Body: string(body)}
	}
	if ttl > 0 {
		c.mu.Lock()
		c.cache[ns+" "+u] = cacheEntry{body: body, exp: time.Now().Add(ttl)}
		c.mu.Unlock()
	}
	return body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
