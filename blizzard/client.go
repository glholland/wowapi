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

// Namespace kinds used by the WoW APIs. The client turns a kind into the
// configured game's namespace for its region (Game.Namespace).
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
	Game         Game   // which WoW game's namespaces to use; the zero value is Retail

	// Overridable for tests.
	TokenURL string
	APIBase  string
	HTTP     *http.Client

	mu           sync.Mutex
	token        string
	tokenExp     time.Time
	cache        map[string]cacheEntry
	commodityIdx *commodityIndex
	dropIdx      map[int]map[int][]string  // journal expansion ID -> item ID -> "Boss (Instance)"
	auctionIdx   map[int]realmAuctionIndex // connected realm ID -> listings
}

type cacheEntry struct {
	body []byte
	exp  time.Time
}

// NewFromEnv builds a client from BLIZZARD_CLIENT_ID, BLIZZARD_CLIENT_SECRET,
// WOW_REGION (default "us"), WOW_LOCALE (default "en_US") and WOW_GAME
// (default "retail"; see ParseGame).
func NewFromEnv() (*Client, error) {
	game, err := GameFromEnv()
	if err != nil {
		return nil, err
	}
	id, secret := Env("BLIZZARD_CLIENT_ID"), Env("BLIZZARD_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil, fmt.Errorf("BLIZZARD_CLIENT_ID and BLIZZARD_CLIENT_SECRET must be set (create a client at https://develop.battle.net/access/clients)")
	}
	return New(id, secret, envOr("WOW_REGION", "us"), envOr("WOW_LOCALE", "en_US"), WithGame(game)), nil
}

// Option customizes a Client built by New.
type Option func(*Client)

// WithGame points the client at a game other than retail.
func WithGame(g Game) Option { return func(c *Client) { c.Game = g } }

// New builds a client for the given region and locale (retail unless
// WithGame says otherwise).
func New(id, secret, region, locale string, opts ...Option) *Client {
	region = strings.ToLower(region)
	c := &Client{
		ClientID:     id,
		ClientSecret: secret,
		Region:       region,
		Locale:       locale,
		Game:         Retail,
		TokenURL:     "https://oauth.battle.net/token",
		APIBase:      fmt.Sprintf("https://%s.api.blizzard.com", region),
		HTTP:         &http.Client{Timeout: 60 * time.Second},
		cache:        map[string]cacheEntry{},
		dropIdx:      map[int]map[int][]string{},
		auctionIdx:   map[int]realmAuctionIndex{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func envOr(key, def string) string {
	if v := Env(key); v != "" {
		return v
	}
	return def
}

// Env reads an environment variable, treating an unsubstituted placeholder
// such as "${user_config.realm}" as unset. MCP clients fill these in from
// bundle settings and may pass them through verbatim when a setting is empty.
func Env(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		return ""
	}
	return v
}

// APIError is returned for non-2xx responses from the API.
type APIError struct {
	Status    int
	Path      string
	Namespace string // the namespace the request used, e.g. "static-forever-us"
	Body      string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("battle.net %s: HTTP %d", e.Path, e.Status)
	switch {
	case e.Status == http.StatusNotFound && strings.HasPrefix(e.Path, "/profile/") && isForeverNamespace(e.Namespace):
		msg += " (WoW: Forever character lookups are not served by Blizzard's API yet, and its realm slugs and name format are not published)"
	case e.Status == http.StatusNotFound && strings.HasPrefix(e.Path, "/profile/"):
		msg += " (character not found: check realm/name spelling, and note characters below level 10 or inactive for a long time are not exposed)"
	case e.Status == http.StatusForbidden && e.Namespace != "":
		msg += fmt.Sprintf(" (unless this is a user-scoped endpoint, Blizzard does not recognize the namespace %q: unknown namespaces return 403, while a known namespace without the data returns 404)", e.Namespace)
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

// Get fetches an API path (e.g. "/data/wow/item/19019") and returns the raw
// JSON body. namespace is a kind (Static, Dynamic or Profile), resolved to the
// configured game's namespace, or a full namespace string such as
// "static-classicann-us", which is used as-is (see ValidNamespace).
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
	ns := c.resolveNamespace(namespace)

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
		return nil, &APIError{Status: resp.StatusCode, Path: path, Namespace: ns, Body: string(body)}
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
