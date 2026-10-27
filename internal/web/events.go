package web

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/azmainadel/kan-do/internal/home"
)

// changeToken is what the page compares to see if anything changed: the change counter,
// plus the database file's identity, so a board pulled by sync always counts as a change.
func changeToken(h *home.Home) (string, error) {
	s, err := h.Store()
	if err != nil {
		return "", err
	}
	defer s.Close()
	n, err := s.Changes()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d-%d", n, h.DBIdentity()), nil
}

// hub tells connected pages when the board changes. Agents write through their own
// `kando mcp` processes, so the only way to see their changes is to watch the change
// token. One watcher runs while at least one page is connected.
type hub struct {
	home     *home.Home
	interval time.Duration

	mu    sync.Mutex
	subs  map[chan string]struct{}
	token string
	stop  context.CancelFunc
	// done closes when the running watcher has exited.
	done chan struct{}
}

func newHub(h *home.Home, interval time.Duration) *hub {
	return &hub{home: h, interval: interval, subs: map[chan string]struct{}{}}
}

// subscribe returns a channel that receives the latest token after each change, and a
// function to unsubscribe.
func (b *hub) subscribe() (<-chan string, func()) {
	ch := make(chan string, 1)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[ch] = struct{}{}
	if b.stop == nil {
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan struct{})
		b.stop, b.done = stop, done
		go b.watch(ctx, done)
	}
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		var done chan struct{}
		if len(b.subs) == 0 && b.stop != nil {
			b.stop()
			b.stop, done = nil, b.done
		}
		b.mu.Unlock()
		// Wait outside the lock: the watcher may be publishing.
		if done != nil {
			<-done
		}
	}
}

func (b *hub) watch(ctx context.Context, done chan struct{}) {
	defer close(done)
	tick := time.NewTicker(b.interval)
	defer tick.Stop()
	for {
		// A failed read, like sync holding the folder, is retried on the next tick.
		if token, err := changeToken(b.home); err == nil {
			b.publish(token)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (b *hub) publish(token string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if token == b.token {
		return
	}
	b.token = token
	for ch := range b.subs {
		// Keep only the newest token for a slow page.
		select {
		case <-ch:
		default:
		}
		ch <- token
	}
}

// keepAlive is how often an idle stream sends a comment, so proxies keep it open.
const keepAlive = 25 * time.Second

// events streams change tokens as Server-Sent Events.
func (b *hub) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	// The current token first, so a page that reconnects catches up at once.
	last, err := changeToken(b.home)
	if err == nil {
		fmt.Fprintf(w, "event: change\ndata: %s\n\n", last)
	}
	if rc.Flush() != nil {
		return
	}
	ch, unsubscribe := b.subscribe()
	defer unsubscribe()
	ping := time.NewTicker(keepAlive)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case token := <-ch:
			if token == last {
				continue
			}
			last = token
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", token)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		if rc.Flush() != nil {
			return
		}
	}
}
