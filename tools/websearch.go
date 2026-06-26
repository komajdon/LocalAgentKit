package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"agent-gui/domain"
)

const (
	searchTimeout  = 20 * time.Second
	defaultResults = 5
	maxResults     = 10
)

// webSearchTool performs a web search via DuckDuckGo (no key), Brave, or SerpAPI.
type webSearchTool struct {
	provider string // duckduckgo | brave | serpapi
	apiKey   string
}

// NewWebSearch builds the search_web tool. provider defaults to duckduckgo.
func NewWebSearch(provider, apiKey string) domain.Tool {
	if provider == "" {
		provider = "duckduckgo"
	}
	return &webSearchTool{provider: strings.ToLower(provider), apiKey: apiKey}
}

func (t *webSearchTool) Name() string { return "search_web" }

func (t *webSearchTool) Description() string {
	return "Search the web for up-to-date information and return the top results (title, URL, snippet)"
}

func (t *webSearchTool) Parameters() string {
	return `{"query": "search query string", "count": "number of results 1-10 (optional, default 5)"}`
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func (t *webSearchTool) Execute(args map[string]string) string {
	query := strings.TrimSpace(args["query"])
	if query == "" {
		return "ERROR: 'query' is required"
	}
	count := defaultResults
	if c, err := strconv.Atoi(strings.TrimSpace(args["count"])); err == nil && c > 0 {
		count = c
	}
	if count > maxResults {
		count = maxResults
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	var (
		results []searchResult
		err     error
	)
	switch t.provider {
	case "brave":
		results, err = t.searchBrave(ctx, query, count)
	case "serpapi":
		results, err = t.searchSerpAPI(ctx, query, count)
	default: // duckduckgo
		results, err = t.searchDuckDuckGo(ctx, query, count)
	}
	if err != nil {
		return fmt.Sprintf("ERROR: web search failed: %v", err)
	}
	if len(results) == 0 {
		return "No results found for: " + query
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Top %d results for %q:\n", len(results), query)
	for i, r := range results {
		fmt.Fprintf(&sb, "\n%d. %s\n   %s\n", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			fmt.Fprintf(&sb, "   %s\n", r.Snippet)
		}
	}
	return sb.String()
}

// ── DuckDuckGo (HTML scrape, no API key) ──────────────────────────────────────

var (
	ddgLinkRe    = regexp.MustCompile(`(?s)<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	ddgSnippetRe = regexp.MustCompile(`(?s)<a[^>]+class="result__snippet"[^>]*>(.*?)</a>`)
	tagRe        = regexp.MustCompile(`<[^>]+>`)
)

func (t *webSearchTool) searchDuckDuckGo(ctx context.Context, query string, count int) ([]searchResult, error) {
	endpoint := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// DuckDuckGo blocks requests without a browser-like User-Agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; agent-gui/1.0)")

	body, err := doRequest(req)
	if err != nil {
		return nil, err
	}

	links := ddgLinkRe.FindAllStringSubmatch(body, -1)
	snippets := ddgSnippetRe.FindAllStringSubmatch(body, -1)

	var out []searchResult
	for i, m := range links {
		if len(out) >= count {
			break
		}
		href := decodeDDGRedirect(m[1])
		title := cleanHTML(m[2])
		snippet := ""
		if i < len(snippets) {
			snippet = cleanHTML(snippets[i][1])
		}
		if title == "" || href == "" {
			continue
		}
		out = append(out, searchResult{Title: title, URL: href, Snippet: snippet})
	}
	return out, nil
}

// DuckDuckGo wraps result URLs in a redirect like //duckduckgo.com/l/?uddg=<encoded>.
func decodeDDGRedirect(raw string) string {
	raw = html.UnescapeString(raw)
	if strings.Contains(raw, "uddg=") {
		if u, err := url.Parse(raw); err == nil {
			if target := u.Query().Get("uddg"); target != "" {
				return target
			}
		}
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

// ── Brave Search API ──────────────────────────────────────────────────────────

func (t *webSearchTool) searchBrave(ctx context.Context, query string, count int) ([]searchResult, error) {
	if t.apiKey == "" {
		return nil, fmt.Errorf("Brave Search requires an API key (set it in Settings)")
	}
	endpoint := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), count)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", t.apiKey)

	body, err := doRequest(req)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, fmt.Errorf("could not parse Brave response: %w", err)
	}
	var out []searchResult
	for _, r := range parsed.Web.Results {
		if len(out) >= count {
			break
		}
		out = append(out, searchResult{Title: cleanHTML(r.Title), URL: r.URL, Snippet: cleanHTML(r.Description)})
	}
	return out, nil
}

// ── SerpAPI (Google) ──────────────────────────────────────────────────────────

func (t *webSearchTool) searchSerpAPI(ctx context.Context, query string, count int) ([]searchResult, error) {
	if t.apiKey == "" {
		return nil, fmt.Errorf("SerpAPI requires an API key (set it in Settings)")
	}
	endpoint := fmt.Sprintf("https://serpapi.com/search.json?engine=google&q=%s&num=%d&api_key=%s",
		url.QueryEscape(query), count, url.QueryEscape(t.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	body, err := doRequest(req)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, fmt.Errorf("could not parse SerpAPI response: %w", err)
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("SerpAPI: %s", parsed.Error)
	}
	var out []searchResult
	for _, r := range parsed.OrganicResults {
		if len(out) >= count {
			break
		}
		out = append(out, searchResult{Title: cleanHTML(r.Title), URL: r.Link, Snippet: cleanHTML(r.Snippet)})
	}
	return out, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func doRequest(req *http.Request) (string, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	// Cap the response body to avoid loading a huge page into memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(firstLine(string(body))))
	}
	return string(body), nil
}

func cleanHTML(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
