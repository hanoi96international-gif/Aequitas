package xsocial

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ms "github.com/hanoi96international-gif/marketsignals"
)

func at(h int) time.Time {
	return time.Date(2024, 6, 1, h, 0, 0, 0, time.UTC)
}

func post(id, author, text string, created time.Time, authorAge time.Duration) Post {
	return Post{
		ID: id, AuthorID: author, Text: text, Created: created,
		AuthorCreated: created.Add(-authorAge),
	}
}

// TestSnapshotOf_SeesThroughOneAccountPostingRepeatedly. Fifty posts from one
// account is not fifty people, and the post count alone cannot tell the
// difference.
func TestSnapshotOf_SeesThroughOneAccountPostingRepeatedly(t *testing.T) {
	var posts []Post
	for i := 0; i < 50; i++ {
		posts = append(posts, post(fmt.Sprint(i), "bot1",
			fmt.Sprintf("great project number %d", i), at(1), 400*24*time.Hour))
	}

	got := snapshotOf(at(1), posts)
	if got.Posts != 50 {
		t.Fatalf("posts %d, want 50", got.Posts)
	}
	if got.UniqueAuthors != 1 {
		t.Fatalf("unique authors %d, want 1", got.UniqueAuthors)
	}
	if cred := got.Credibility(); cred > 0.05 {
		t.Fatalf("credibility %.4f for one account posting fifty times", cred)
	}
}

// TestSnapshotOf_SeesThroughACopyPasteCampaign. Different accounts saying the
// same sentence is one message, and varying the handle or the tracking link
// does not change that.
func TestSnapshotOf_SeesThroughACopyPasteCampaign(t *testing.T) {
	var posts []Post
	for i := 0; i < 40; i++ {
		text := fmt.Sprintf("$TOKEN to the moon check https://t.co/%d @shill%d", i, i)
		posts = append(posts, post(fmt.Sprint(i), fmt.Sprintf("acct%d", i),
			text, at(1), 400*24*time.Hour))
	}

	got := snapshotOf(at(1), posts)
	if got.UniqueAuthors != 40 {
		t.Fatalf("unique authors %d, want 40", got.UniqueAuthors)
	}
	if got.DuplicateTextRatio < 0.9 {
		t.Fatalf("duplicate ratio %.3f — normalisation should strip the varying link and "+
			"handle and recognise one message", got.DuplicateTextRatio)
	}
	if cred := got.Credibility(); cred > 0.15 {
		t.Fatalf("credibility %.4f for forty accounts posting one sentence", cred)
	}
}

// TestSnapshotOf_SeesThroughAFreshlyMintedSwarm.
func TestSnapshotOf_SeesThroughAFreshlyMintedSwarm(t *testing.T) {
	var posts []Post
	for i := 0; i < 40; i++ {
		posts = append(posts, post(fmt.Sprint(i), fmt.Sprintf("acct%d", i),
			fmt.Sprintf("independent thought number %d about this token", i),
			at(1), 3*24*time.Hour))
	}

	got := snapshotOf(at(1), posts)
	if got.NewAccountRatio < 0.95 {
		t.Fatalf("new-account ratio %.3f for forty three-day-old accounts", got.NewAccountRatio)
	}
	if got.MedianAuthorAgeDays > 5 {
		t.Fatalf("median age %.1f days", got.MedianAuthorAgeDays)
	}
	if cred := got.Credibility(); cred > 0.1 {
		t.Fatalf("credibility %.4f for a swarm created last week", cred)
	}
}

func TestSnapshotOf_ScoresGenuineDiscussionHighly(t *testing.T) {
	var posts []Post
	for i := 0; i < 40; i++ {
		posts = append(posts, post(fmt.Sprint(i), fmt.Sprintf("person%d", i),
			fmt.Sprintf("a distinct opinion %d with its own wording and reasoning", i),
			at(1), time.Duration(300+i)*24*time.Hour))
	}

	got := snapshotOf(at(1), posts)
	if got.DuplicateTextRatio != 0 {
		t.Fatalf("duplicate ratio %.3f on forty distinct texts", got.DuplicateTextRatio)
	}
	if cred := got.Credibility(); cred < 0.8 {
		t.Fatalf("credibility %.4f for forty established accounts saying different things", cred)
	}
}

// TestSnapshotOf_TreatsAnUnknownAuthorAgeAsSuspect. Absence of evidence is not
// evidence of a real account, and this is the one place where guessing
// generously would flatter a bot swarm.
func TestSnapshotOf_TreatsAnUnknownAuthorAgeAsSuspect(t *testing.T) {
	var posts []Post
	for i := 0; i < 20; i++ {
		p := post(fmt.Sprint(i), fmt.Sprintf("acct%d", i),
			fmt.Sprintf("thought %d", i), at(1), 0)
		p.AuthorCreated = time.Time{} // the API did not say
		posts = append(posts, p)
	}

	got := snapshotOf(at(1), posts)
	if got.NewAccountRatio != 1 {
		t.Fatalf("new-account ratio %.3f when no author age was returned; unknown must not "+
			"read as established", got.NewAccountRatio)
	}
	if got.Credibility() != 0 {
		t.Fatalf("credibility %.4f with no author ages at all", got.Credibility())
	}
}

// TestSnapshots_EmitsEveryBarIncludingTheQuietOnes. Skipping empty bars would
// misalign the social series against the price series, and the agent would
// then read one bar's attention against another's price — plausible output,
// no error.
func TestSnapshots_EmitsEveryBarIncludingTheQuietOnes(t *testing.T) {
	posts := []Post{
		post("1", "a", "hello", at(0).Add(10*time.Minute), 400*24*time.Hour),
		post("2", "b", "world", at(3).Add(20*time.Minute), 400*24*time.Hour),
	}

	got := Snapshots(posts, at(0), at(5), time.Hour)
	if len(got) != 5 {
		t.Fatalf("got %d snapshots for a five-hour window, want 5", len(got))
	}
	for i, s := range got {
		want := at(i)
		if !s.Time.Equal(want) {
			t.Fatalf("snapshot %d is stamped %s, want %s", i, s.Time, want)
		}
	}
	if got[0].Posts != 1 || got[3].Posts != 1 {
		t.Fatalf("posts landed in the wrong buckets: %+v", got)
	}
	if got[1].Posts != 0 || got[2].Posts != 0 || got[4].Posts != 0 {
		t.Fatal("a quiet bar was given posts it did not have")
	}
	if got[1].Credibility() != 0 {
		t.Fatalf("an empty bar scored %.3f credible", got[1].Credibility())
	}
}

func TestSnapshots_IgnoresPostsOutsideTheWindow(t *testing.T) {
	posts := []Post{
		post("early", "a", "before", at(0).Add(-time.Hour), 400*24*time.Hour),
		post("late", "b", "after", at(5), 400*24*time.Hour),
		post("in", "c", "inside", at(2), 400*24*time.Hour),
	}
	got := Snapshots(posts, at(0), at(5), time.Hour)
	total := 0
	for _, s := range got {
		total += s.Posts
	}
	if total != 1 {
		t.Fatalf("kept %d posts, want only the one inside [from, to)", total)
	}
}

func TestNormalise_StripsWhatACampaignVaries(t *testing.T) {
	a := normalise("$TOKEN is going to fly!! check https://t.co/abc123 @promoter1")
	b := normalise("$OTHER is going to fly!!   check https://t.co/zzz999 @promoter2")
	if a != b {
		t.Fatalf("normalisation left them different:\n  %q\n  %q", a, b)
	}
	if strings.Contains(a, "http") || strings.Contains(a, "@") {
		t.Fatalf("normalised text still carries a link or handle: %q", a)
	}
	// It must not flatten genuinely different sentences.
	if normalise("this project has real revenue") == normalise("this project has no product") {
		t.Fatal("normalisation collapsed two different statements")
	}
}

func TestSource_RefusesASymbolWithNoQuery(t *testing.T) {
	src := Source{Client: &Client{}, Query: map[string]string{}}
	_, err := src.Attention(context.Background(),
		ms.Instrument{Symbol: "SOL"}, time.Hour, at(0), at(5))
	if err == nil {
		t.Fatal("expected a refusal rather than a search for the bare ticker")
	}
	if !strings.Contains(err.Error(), "wrong subject") {
		t.Fatalf("error %q should explain why a bare symbol is not a query", err)
	}
}

func TestSource_TrendingSaysWhyItIsNotImplemented(t *testing.T) {
	_, err := Source{}.Trending("solana", 10)
	if err == nil {
		t.Fatal("expected an error rather than a plausible list from the wrong data")
	}
	if !strings.Contains(err.Error(), "on-chain indexer") {
		t.Fatalf("error %q should point at what does answer the question", err)
	}
}

func TestClient_SearchParsesPostsAndAuthors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer TESTTOKEN" {
			t.Errorf("authorization header %q", got)
		}
		fmt.Fprint(w, `{
		  "data":[
		    {"id":"1","author_id":"u1","text":"first","created_at":"2024-06-01T01:10:00Z"},
		    {"id":"2","author_id":"u2","text":"second","created_at":"2024-06-01T01:20:00Z"}
		  ],
		  "includes":{"users":[
		    {"id":"u1","created_at":"2020-01-01T00:00:00Z","public_metrics":{"followers_count":5000}},
		    {"id":"u2","created_at":"2024-05-30T00:00:00Z","public_metrics":{"followers_count":3}}
		  ]},
		  "meta":{}
		}`)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Bearer: "TESTTOKEN", HTTP: srv.Client()}
	posts, err := c.Search(context.Background(), "$TOKEN lang:en", at(0), 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}
	if posts[0].AuthorFollowers != 5000 {
		t.Fatalf("author metrics not attached: %+v", posts[0])
	}
	if posts[1].AuthorCreated.IsZero() {
		t.Fatal("author creation date not attached")
	}

	// The two accounts differ by years; the snapshot must reflect that.
	snap := snapshotOf(at(1), posts)
	if snap.NewAccountRatio != 0.5 {
		t.Fatalf("new-account ratio %.2f with one established and one two-day-old account",
			snap.NewAccountRatio)
	}
}

func TestClient_ErrorsCarryTheAPIsReasonNotTheQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"title":"Too Many Requests","detail":"Rate limit exceeded"}`)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Bearer: "TESTTOKEN", HTTP: srv.Client()}
	_, err := c.Search(context.Background(), "0xSECRETCONTRACTADDRESS", at(0), 1)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "SECRETCONTRACT") || strings.Contains(err.Error(), "TESTTOKEN") {
		t.Fatalf("error leaked the query or the token: %v", err)
	}
	if !strings.Contains(err.Error(), "Rate limit") {
		t.Fatalf("error %q should carry the API's own reason", err)
	}
}

func TestNew_RefusesWithoutAToken(t *testing.T) {
	t.Setenv("X_BEARER_TOKEN", "")
	if _, err := New(); err == nil {
		t.Fatal("expected a refusal rather than a client that fails one request at a time")
	}
}

// TestCredibility_ComponentsMultiply confirms the property the agent relies on:
// passing three checks and failing one must not average out to fine.
func TestCredibility_ComponentsMultiply(t *testing.T) {
	good := ms.SocialSnapshot{
		Posts: 100, UniqueAuthors: 90, DuplicateTextRatio: 0.05,
		MedianAuthorAgeDays: 400, NewAccountRatio: 0.05,
	}
	if got := good.Credibility(); got < 0.7 {
		t.Fatalf("a clean window scored %.3f", got)
	}
	broken := good
	broken.DuplicateTextRatio = 0.97
	if got := broken.Credibility(); got > 0.1 {
		t.Fatalf("failing one component scored %.3f; the components must multiply", got)
	}
	if math.Abs(good.Credibility()-broken.Credibility()) < 0.5 {
		t.Fatal("one failed component barely moved the score")
	}
}
