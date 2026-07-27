// Package xsocial fetches attention data from X and turns it into the
// credibility-weighted snapshots the social agent actually uses.
//
// The API gives raw posts. What the agent needs is an assessment of whether
// those posts represent people, and that assessment is the entire job here —
// counting mentions measures somebody's marketing budget. So this package
// spends most of its code on three things the endpoint does not compute:
//
//   - how many DISTINCT accounts are talking, rather than how many posts exist
//   - how much of the text is copy-pasted from other posts in the same window
//   - how old those accounts are, and what share were created recently
//
// Each of those is a complete way to manufacture attention, which is why the
// agent multiplies them rather than averaging: passing three and failing one
// should not average out to fine.
//
// A limitation worth stating plainly: the recent-search endpoint most tiers
// have covers roughly the last seven days. That is enough to run live and not
// enough to backfill a history for a backtest. Anyone claiming a backtested
// social strategy on retail API access has either paid for archive access or
// is describing something they did not do.
package xsocial

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	ms "github.com/hanoi96international-gif/marketsignals"
)

// Client reads X's recent search endpoint.
type Client struct {
	// BaseURL is the API root; overridable for a proxy or for tests.
	BaseURL string
	// Bearer is the app-only token. Empty reads X_BEARER_TOKEN from the
	// environment — a token in a config file is a token in version control.
	Bearer string
	HTTP   *http.Client
}

// New builds a client, failing rather than starting without a token.
func New() (*Client, error) {
	token := os.Getenv("X_BEARER_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("no X credentials: set X_BEARER_TOKEN in the environment")
	}
	return &Client{
		BaseURL: "https://api.x.com/2",
		Bearer:  token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Post is one message with the author facts the credibility score needs.
type Post struct {
	ID       string
	AuthorID string
	Text     string
	Created  time.Time

	AuthorCreated   time.Time
	AuthorFollowers int
}

// Search fetches recent posts matching a query.
//
// The query should be narrow. "bitcoin" returns the entire internet and the
// resulting credibility score describes X rather than the asset; a cashtag
// plus a language filter, or a token's contract address, is the shape that
// measures something.
func (c *Client) Search(ctx context.Context, query string, since time.Time, maxPages int) ([]Post, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("max_results", "100")
	params.Set("tweet.fields", "created_at,author_id")
	params.Set("expansions", "author_id")
	params.Set("user.fields", "created_at,public_metrics")
	if !since.IsZero() {
		params.Set("start_time", since.UTC().Format(time.RFC3339))
	}

	var out []Post
	next := ""
	if maxPages <= 0 {
		maxPages = 5
	}

	for page := 0; page < maxPages; page++ {
		if next != "" {
			params.Set("next_token", next)
		}
		body, err := c.get(ctx, "/tweets/search/recent?"+params.Encode())
		if err != nil {
			return nil, err
		}

		var resp struct {
			Data []struct {
				ID        string `json:"id"`
				AuthorID  string `json:"author_id"`
				Text      string `json:"text"`
				CreatedAt string `json:"created_at"`
			} `json:"data"`
			Includes struct {
				Users []struct {
					ID            string `json:"id"`
					CreatedAt     string `json:"created_at"`
					PublicMetrics struct {
						Followers int `json:"followers_count"`
					} `json:"public_metrics"`
				} `json:"users"`
			} `json:"includes"`
			Meta struct {
				NextToken string `json:"next_token"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}

		authors := map[string]struct {
			created   time.Time
			followers int
		}{}
		for _, u := range resp.Includes.Users {
			t, _ := time.Parse(time.RFC3339, u.CreatedAt)
			authors[u.ID] = struct {
				created   time.Time
				followers int
			}{t, u.PublicMetrics.Followers}
		}

		for _, d := range resp.Data {
			created, _ := time.Parse(time.RFC3339, d.CreatedAt)
			p := Post{ID: d.ID, AuthorID: d.AuthorID, Text: d.Text, Created: created}
			if a, ok := authors[d.AuthorID]; ok {
				p.AuthorCreated, p.AuthorFollowers = a.created, a.followers
			}
			out = append(out, p)
		}

		next = resp.Meta.NextToken
		if next == "" {
			break
		}
	}
	return out, nil
}

// Snapshots buckets posts into bars and scores each bucket's credibility.
//
// Every bar in [from, to) gets a snapshot, including empty ones. Skipping the
// quiet bars would leave the social series misaligned with the price series,
// and the agent would then read one bar's attention against another's price —
// a mistake that produces plausible output and no error.
func Snapshots(posts []Post, from, to time.Time, interval time.Duration) []ms.SocialSnapshot {
	if interval <= 0 || !to.After(from) {
		return nil
	}
	buckets := map[time.Time][]Post{}
	for _, p := range posts {
		if p.Created.Before(from) || !p.Created.Before(to) {
			continue
		}
		slot := from.Add(p.Created.Sub(from).Truncate(interval))
		buckets[slot] = append(buckets[slot], p)
	}

	var out []ms.SocialSnapshot
	for t := from; t.Before(to); t = t.Add(interval) {
		out = append(out, snapshotOf(t, buckets[t]))
	}
	return out
}

// snapshotOf scores one bar's worth of posts.
//
// The three quality figures are the whole point of this package, and each
// answers a way attention is manufactured: one account posting fifty times,
// fifty accounts posting one sentence, and fifty accounts created last week.
func snapshotOf(at time.Time, posts []Post) ms.SocialSnapshot {
	out := ms.SocialSnapshot{Time: at, Posts: len(posts)}
	if len(posts) == 0 {
		return out
	}

	// Distinct voices, and their ages. Authors are counted once however often
	// they posted — that difference is precisely what the diversity term
	// measures.
	authors := map[string]Post{}
	for _, p := range posts {
		if _, seen := authors[p.AuthorID]; !seen {
			authors[p.AuthorID] = p
		}
	}
	out.UniqueAuthors = len(authors)

	var ages []float64
	newAccounts := 0
	for _, p := range authors {
		if p.AuthorCreated.IsZero() {
			// An author whose creation date the API did not return is treated
			// as unknown rather than as established: absence of evidence is
			// not evidence of a real account, and this is the one place where
			// guessing generously would flatter a bot swarm.
			ages = append(ages, 0)
			newAccounts++
			continue
		}
		age := at.Sub(p.AuthorCreated).Hours() / 24
		ages = append(ages, age)
		if age < 30 {
			newAccounts++
		}
	}
	sort.Float64s(ages)
	out.MedianAuthorAgeDays = ages[len(ages)/2]
	out.NewAccountRatio = float64(newAccounts) / float64(len(authors))

	// Copy-paste detection. Texts are normalised first so that a campaign
	// varying only the handle or the tracking link is still one message.
	counts := map[string]int{}
	for _, p := range posts {
		counts[normalise(p.Text)]++
	}
	duplicates := 0
	for _, n := range counts {
		if n > 1 {
			duplicates += n
		}
	}
	out.DuplicateTextRatio = float64(duplicates) / float64(len(posts))

	return out
}

// whitespace and mentions/links are stripped before comparing texts, so that a
// campaign posting the same sentence with a different handle or tracking link
// is still recognised as one message.
var (
	linkRe    = regexp.MustCompile(`https?://\S+`)
	mentionRe = regexp.MustCompile(`[@$#]\w+`)
	spaceRe   = regexp.MustCompile(`\s+`)
)

func normalise(text string) string {
	s := strings.ToLower(text)
	s = linkRe.ReplaceAllString(s, "")
	s = mentionRe.ReplaceAllString(s, "")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Bearer)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// The token is in a header rather than the URL, so it cannot leak
		// here — but the query can contain a contract address somebody would
		// rather not publish, so only the status and the API's own message go
		// into the error.
		var e struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}
		if json.Unmarshal(body, &e) == nil && e.Title != "" {
			return nil, fmt.Errorf("x api returned %d: %s — %s", resp.StatusCode, e.Title, e.Detail)
		}
		return nil, fmt.Errorf("x api returned %d", resp.StatusCode)
	}
	return body, nil
}

// Source adapts the client to marketsignals.SocialSource.
type Source struct {
	Client *Client
	// Query maps a symbol to the search query that measures it. A symbol with
	// no entry is refused rather than guessed: searching "SOL" returns posts
	// about the sun in Spanish, and the resulting credibility score would be
	// a real number about the wrong thing.
	Query map[string]string
	// MaxPages caps the API calls per request.
	MaxPages int
}

// Attention returns per-bar snapshots for an instrument.
func (s Source) Attention(ctx context.Context, i ms.Instrument, interval time.Duration,
	from, to time.Time) ([]ms.SocialSnapshot, error) {

	q, ok := s.Query[i.Symbol]
	if !ok || q == "" {
		return nil, fmt.Errorf("no search query configured for %s; searching the bare symbol "+
			"returns the wrong subject and scores it confidently", i.Symbol)
	}
	posts, err := s.Client.Search(ctx, q, from, s.MaxPages)
	if err != nil {
		return nil, err
	}
	return Snapshots(posts, from, to, interval), nil
}

// Trending is not implemented: X's endpoints report what is popular globally,
// which is a different question from which newly deployed tokens are drawing
// credible attention. Returning a plausible list built from the wrong data
// would be worse than returning nothing.
func (s Source) Trending(string, int) ([]ms.ProjectAttention, error) {
	return nil, fmt.Errorf("trending discovery is not implemented on this source; use an " +
		"on-chain indexer, which knows what was deployed rather than what is popular")
}
