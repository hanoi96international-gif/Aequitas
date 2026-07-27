package marketsignals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Notifications.
//
// The runner emits a signal on every bar close, which is the right granularity
// for a log and the wrong one for a phone. An hourly strategy produces
// twenty-four messages a day, twenty-three of which say "no change" — and the
// reliable consequence of that is that the twenty-fourth gets ignored too.
//
// So notifications fire on a CHANGE IN THE TARGET POSITION, not on a bar. The
// question a notification answers is "is there something to do", and when the
// answer is no there is nothing to send.

// ChangeNotifier wraps a Sink and forwards only signals that represent a
// materially different position from the last one forwarded.
type ChangeNotifier struct {
	// Inner receives the signals that pass the filter.
	Inner Sink

	// MinChange is how much the target must move, in fractions of equity, to
	// be worth a message. Small drift as volatility targeting rebalances is
	// not news; a flip from long to flat is.
	MinChange float64

	// MinInterval throttles messages regardless of how much the target moves,
	// so a violent session cannot turn into a stream of alerts. Zero disables
	// the throttle.
	MinInterval time.Duration

	// AlwaysOnDirectionChange forwards a change of side even when it is below
	// MinChange or inside MinInterval. Going from long to short is the one
	// event that should never be suppressed for tidiness.
	AlwaysOnDirectionChange bool

	mu       sync.Mutex
	last     float64
	lastSent time.Time
	started  bool
}

// NewChangeNotifier wraps a sink with sensible filtering: a fifth of the
// account, at most one message per hour, but never suppressing a side change.
func NewChangeNotifier(inner Sink) *ChangeNotifier {
	return &ChangeNotifier{
		Inner:                   inner,
		MinChange:               0.20,
		MinInterval:             time.Hour,
		AlwaysOnDirectionChange: true,
	}
}

// Emit forwards the signal if it represents something to act on.
func (n *ChangeNotifier) Emit(s LiveSignal) error {
	n.mu.Lock()
	forward, reason := n.shouldForwardLocked(s)
	if forward {
		n.last = s.Target
		n.lastSent = s.Time
		n.started = true
	}
	n.mu.Unlock()

	if !forward {
		return nil
	}
	s.Reason = reason + " — " + s.Reason
	return n.Inner.Emit(s)
}

func (n *ChangeNotifier) shouldForwardLocked(s LiveSignal) (bool, string) {
	if !n.started {
		return true, "first signal since start"
	}

	sideChanged := sideOf(s.Target) != sideOf(n.last)
	if sideChanged && n.AlwaysOnDirectionChange {
		return true, fmt.Sprintf("side change: %s → %s", sideOf(n.last), sideOf(s.Target))
	}

	delta := math.Abs(s.Target - n.last)
	if delta < n.MinChange {
		return false, ""
	}
	if n.MinInterval > 0 && s.Time.Sub(n.lastSent) < n.MinInterval {
		return false, ""
	}
	return true, fmt.Sprintf("target moved %s (from %s to %s)",
		pct(delta), pct(n.last), pct(s.Target))
}

func sideOf(target float64) string {
	switch {
	case target > 0:
		return "long"
	case target < 0:
		return "short"
	default:
		return "flat"
	}
}

// WebhookSink posts each signal to an HTTP endpoint.
//
// It carries no provider-specific logic beyond a message template, because
// every service worth notifying through — Telegram's bot API, a Discord or
// Slack webhook, ntfy.sh, a self-hosted endpoint — accepts a POST with a JSON
// body. Which one is a matter of the URL and the field name, both of which are
// configuration rather than code.
//
// The endpoint URL usually contains a token. It is read from configuration and
// never logged, and nothing in this package writes it to disk.
type WebhookSink struct {
	URL string
	// Field is the JSON key the message goes into: "text" for Telegram and
	// Slack, "content" for Discord, "message" for many others. Empty posts the
	// whole LiveSignal as JSON instead, for endpoints that want structure.
	Field string
	// Extra fields merged into the body — Telegram needs "chat_id" here.
	Extra map[string]any
	HTTP  *http.Client
}

// NewWebhookSink returns a sink posting a text message to url.
func NewWebhookSink(url, field string) *WebhookSink {
	return &WebhookSink{URL: url, Field: field, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Emit posts the signal.
func (w *WebhookSink) Emit(s LiveSignal) error {
	var body []byte
	var err error

	if w.Field == "" {
		body, err = json.Marshal(s)
	} else {
		payload := map[string]any{w.Field: FormatSignal(s)}
		for k, v := range w.Extra {
			payload[k] = v
		}
		body, err = json.Marshal(payload)
	}
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := w.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		// The URL is deliberately not included: for most providers it
		// contains the bot token, and an error line is exactly the sort of
		// text that ends up pasted into an issue.
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

// FormatSignal renders a signal as a short human message.
//
// It leads with the action rather than the analysis. Somebody reading this on
// a phone needs to know what changed and how big the position should be; the
// agents' reasoning is there for afterwards, and the warning about the kill
// switch is there because it changes how much the number should be trusted.
func FormatSignal(s LiveSignal) string {
	var b strings.Builder

	side := strings.ToUpper(s.Direction)
	fmt.Fprintf(&b, "%s %s — %s\n", s.Symbol, side, pct(math.Abs(s.Target)))
	fmt.Fprintf(&b, "bar %s, close %.6g\n", s.BarTime.Format("2006-01-02 15:04 MST"), s.Close)

	if s.RiskReason != "" {
		fmt.Fprintf(&b, "risk cut to %s: %s\n", pct(s.RiskScale), s.RiskReason)
	}
	if strings.Contains(s.Reason, "kill switch cannot trip") {
		b.WriteString("WARNING: no account equity wired in — the drawdown stop is inert\n")
	}

	if len(s.Members) > 0 {
		b.WriteString("\n")
		for _, m := range s.Members {
			if m.Dir == Flat {
				continue
			}
			fmt.Fprintf(&b, "  %s %s: %s\n", m.Agent, m.Dir, m.Note)
		}
	}

	b.WriteString("\nThis is a signal, not an order. Nothing has been traded.")
	return b.String()
}
