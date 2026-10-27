// Package web serves the board: the page, the JSON API behind it, live updates, and MCP
// over HTTP at /mcp.
package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/azmainadel/kan-do/internal/home"
	"github.com/azmainadel/kan-do/internal/mcp"
)

// dist holds the built UI in dist/app; `make ui` builds it. Only dist/.gitkeep is
// tracked, so a build without Node serves placeholder.html instead.
//
//go:embed all:dist
var dist embed.FS

//go:embed placeholder.html
var placeholder []byte

// watchInterval is how often the board checks for changes while a page is open.
const watchInterval = 500 * time.Millisecond

// Handler is the whole board: UI, API, live updates and, if enabled, MCP.
func Handler(h *home.Home) http.Handler {
	return handler(h, newGuard(h.Config.Port, !h.Config.Listen.IsLoopback()))
}

func handler(h *home.Home, g *guard) http.Handler {
	mux := http.NewServeMux()
	(&api{home: h}).routes(mux)
	mux.HandleFunc("GET /api/events", newHub(h, watchInterval).events)
	if h.Config.MCPHTTP {
		m := mcp.HTTPHandler(h)
		// Per method: a bare /mcp would conflict with GET / below.
		for _, method := range []string{"GET", "POST", "DELETE"} {
			mux.Handle(method+" /mcp", m)
		}
	}
	mux.Handle("GET /", static(dist))
	return g.protect(mux)
}

// static serves the UI in root's dist/app. Vite names its assets by their content, so
// those are cached for good.
func static(root fs.FS) http.Handler {
	ui, err := fs.Sub(root, "dist/app")
	if err == nil {
		_, err = fs.Stat(ui, "index.html")
	}
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(placeholder)
		})
	}
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

// Serve runs the board until ctx is cancelled. port 0 uses the one in config.toml.
func Serve(ctx context.Context, h *home.Home, port uint16, open bool) error {
	if port != 0 {
		h.Config.Port = port
	}
	port = h.Config.Port
	listen := h.Config.Listen
	url := fmt.Sprintf("http://localhost:%d", port)
	ln, err := net.Listen("tcp", netip.AddrPortFrom(listen, port).String())
	if errors.Is(err, syscall.EADDRINUSE) {
		if isKando(port) {
			fmt.Printf("kan-do is already running at %s\n", url)
			if open {
				openBrowser(url)
			}
			return nil
		}
		return fmt.Errorf("port %d is used by another program; choose another with --port or in config.toml", port)
	}
	if err != nil {
		return fmt.Errorf("cannot listen on %s port %d: %w", listen, port, err)
	}
	listeners := []net.Listener{ln}
	// localhost is ::1 before 127.0.0.1 on many Linux systems, and not every client
	// falls back, so listen on ::1 as well.
	if listen.Is4() && (listen.IsLoopback() || listen.IsUnspecified()) {
		if v6, err := net.Listen("tcp", netip.AddrPortFrom(netip.IPv6Loopback(), port).String()); err == nil {
			listeners = append(listeners, v6)
		}
	}

	g := newGuard(port, !listen.IsLoopback())
	fmt.Printf("kan-do board at %s, data in %s\n", url, h.Dir)
	if g.lan {
		for _, n := range g.names {
			if strings.HasSuffix(n, ".local") {
				fmt.Printf("On your network: http://%s:%d (or this machine's IP address)\n", n, port)
				break
			}
		}
		fmt.Printf("Anyone who can reach port %d can read and change this board.\n", port)
	}
	if h.Config.MCPHTTP {
		fmt.Printf("MCP over HTTP at %s/mcp\n", url)
	}
	if open {
		openBrowser(url)
	}

	// Cancelling base ends open event and MCP streams, which would otherwise hold up shutdown.
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &http.Server{
		Handler:           handler(h, g),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return base },
	}
	errs := make(chan error, len(listeners))
	for _, l := range listeners {
		go func() { errs <- srv.Serve(l) }()
	}
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	cancel()
	stop, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	return srv.Shutdown(stop)
}

// isKando reports whether the program on port answers like a kan-do board.
func isKando(port uint16) bool {
	c := http.Client{Timeout: 2 * time.Second}
	res, err := c.Get("http://127.0.0.1:" + strconv.Itoa(int(port)) + "/api/ping")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64))
	return string(body) == "kando"
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", url)
	// A board without a screen has nothing to open; the URL is already printed.
	case os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	if cmd.Start() != nil {
		fmt.Fprintf(os.Stderr, "Open %s in your browser.\n", url)
		return
	}
	go cmd.Wait()
}
