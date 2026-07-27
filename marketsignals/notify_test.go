package marketsignals

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type capture struct{ got []LiveSignal }

func (c *capture) Emit(s LiveSignal) error { c.got = append(c.got, s); return nil }

func sig(at time.Time, target float64) LiveSignal {
	return LiveSignal{
		Time: at, BarTime: at, Symbol: "TEST", Target: target,
		Direction: sideOf(target), Close: 100, Reason: "test",
	}
}

// TestChangeNotifier_StaysQuietWhenNothingChanged is the whole point. An
// hourly strategy emits twenty-four signals a day, and a notifier that
// forwards all of them trains its reader to ignore the one that mattered.
func TestChangeNotifier_StaysQuietWhenNothingChanged(t *testing.T) {
	c := &capture{}
	n := NewChangeNotifier(c)
	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// Twelve bars holding the same position, with only tiny rebalancing drift.
	for i := 0; i < 12; i++ {
		if err := n.Emit(sig(base.Add(time.Duration(i)*time.Hour), 0.40+float64(i)*0.001)); err != nil {
			t.Fatalf("emit: %v", err)
		}
	}
	if len(c.got) != 1 {
		t.Fatalf("forwarded %d of 12 unchanged signals; only the first should have gone out",
			len(c.got))
	}
	if !strings.Contains(c.got[0].Reason, "first signal") {
		t.Fatalf("reason %q should say why the first one was sent", c.got[0].Reason)
	}
}

func TestChangeNotifier_ForwardsAMaterialMove(t *testing.T) {
	c := &capture{}
	n := NewChangeNotifier(c)
	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	_ = n.Emit(sig(base, 0.10))
	// Same side, but a large increase, and outside the throttle window.
	_ = n.Emit(sig(base.Add(2*time.Hour), 0.60))

	if len(c.got) != 2 {
		t.Fatalf("forwarded %d signals, want 2", len(c.got))
	}
	if !strings.Contains(c.got[1].Reason, "target moved") {
		t.Fatalf("reason %q should quantify the move", c.got[1].Reason)
	}
}

// TestChangeNotifier_NeverSuppressesASideChange: going from long to short is
// the one event that must not be filtered away for tidiness, however small the
// numbers or however recent the last message.
func TestChangeNotifier_NeverSuppressesASideChange(t *testing.T) {
	c := &capture{}
	n := NewChangeNotifier(c)
	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	_ = n.Emit(sig(base, 0.05))
	// One minute later — well inside the throttle — and a move far below
	// MinChange. But it crosses to the other side.
	_ = n.Emit(sig(base.Add(time.Minute), -0.05))

	if len(c.got) != 2 {
		t.Fatalf("forwarded %d signals; a long-to-short flip was suppressed by the throttle",
			len(c.got))
	}
	if !strings.Contains(c.got[1].Reason, "side change") {
		t.Fatalf("reason %q should name the side change", c.got[1].Reason)
	}

	// Going flat is also a side change — it means close the position.
	_ = n.Emit(sig(base.Add(2*time.Minute), 0))
	if len(c.got) != 3 {
		t.Fatal("a move to flat was suppressed; that instruction is 'close the position'")
	}
}

func TestChangeNotifier_ThrottlesRepeatedLargeMoves(t *testing.T) {
	c := &capture{}
	n := NewChangeNotifier(c)
	n.AlwaysOnDirectionChange = false
	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	_ = n.Emit(sig(base, 0.10))
	// Three big same-side moves within the hour: only the throttle decides.
	for i := 1; i <= 3; i++ {
		_ = n.Emit(sig(base.Add(time.Duration(i)*time.Minute), 0.10+float64(i)*0.30))
	}
	if len(c.got) != 1 {
		t.Fatalf("forwarded %d messages in one minute-by-minute burst, want 1", len(c.got))
	}

	// Once the window passes, the next material move goes out.
	_ = n.Emit(sig(base.Add(2*time.Hour), 0.95))
	if len(c.got) != 2 {
		t.Fatalf("forwarded %d after the throttle expired, want 2", len(c.got))
	}
}

func TestWebhookSink_PostsATextMessage(t *testing.T) {
	var body []byte
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		contentType = r.Header.Get("Content-Type")
	}))
	defer srv.Close()

	sink := NewWebhookSink(srv.URL, "text")
	sink.Extra = map[string]any{"chat_id": "12345"}

	s := sig(time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC), 0.42)
	s.Members = []Signal{{Agent: "breakout", Dir: Long, Note: "closed above channel"}}
	if err := sink.Emit(s); err != nil {
		t.Fatalf("emit: %v", err)
	}

	if contentType != "application/json" {
		t.Fatalf("content type %q", contentType)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if payload["chat_id"] != "12345" {
		t.Fatalf("extra fields were dropped: %v", payload)
	}
	text, _ := payload["text"].(string)
	for _, want := range []string{"TEST", "LONG", "42%", "breakout", "not an order"} {
		if !strings.Contains(text, want) {
			t.Fatalf("message %q missing %q", text, want)
		}
	}
}

func TestWebhookSink_PostsStructuredJSONWhenNoFieldIsSet(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	if err := NewWebhookSink(srv.URL, "").Emit(sig(time.Now(), -0.3)); err != nil {
		t.Fatalf("emit: %v", err)
	}
	var got LiveSignal
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body did not round-trip as a LiveSignal: %v", err)
	}
	if got.Target != -0.3 {
		t.Fatalf("target %v, want -0.3", got.Target)
	}
}

// TestWebhookSink_ErrorDoesNotLeakTheURL. For most providers the endpoint
// contains a bot token, and an error string is exactly the sort of text that
// gets pasted into an issue tracker.
func TestWebhookSink_ErrorDoesNotLeakTheURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	secret := srv.URL + "/bot123456:SUPERSECRETTOKEN/sendMessage"
	err := NewWebhookSink(secret, "text").Emit(sig(time.Now(), 0.5))
	if err == nil {
		t.Fatal("expected an error for a 401")
	}
	if strings.Contains(err.Error(), "SUPERSECRETTOKEN") || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("error leaked the endpoint: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error %q should still say what went wrong", err)
	}
}

func TestFormatSignal_LeadsWithTheAction(t *testing.T) {
	s := sig(time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC), -0.25)
	s.RiskScale = 0.5
	s.RiskReason = "macro: ahead of CPI"
	s.Reason = "vol 40% vs target 20% | WARNING: no account drawdown wired in, so the kill switch cannot trip"

	got := FormatSignal(s)
	first := strings.SplitN(got, "\n", 2)[0]
	if !strings.Contains(first, "SHORT") || !strings.Contains(first, "25%") {
		t.Fatalf("first line %q should carry the side and the size", first)
	}
	if !strings.Contains(got, "risk cut to 50%") {
		t.Fatalf("message should surface the risk cut:\n%s", got)
	}
	if !strings.Contains(got, "drawdown stop is inert") {
		t.Fatalf("message should surface the inert kill switch:\n%s", got)
	}
	if !strings.Contains(got, "not an order") {
		t.Fatalf("message must say it is not an order:\n%s", got)
	}
}
