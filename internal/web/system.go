package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/azmainadel/kan-do/internal/gitsync"
	"github.com/azmainadel/kan-do/internal/update"
	"github.com/azmainadel/kan-do/internal/version"
)

// Sync and update: the board's sync button and its offer of new versions.
func (a *api) systemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sync", func(w http.ResponseWriter, _ *http.Request) {
		var remote *string
		if url := gitsync.RemoteURL(a.home); url != "" {
			remote = &url
		}
		writeJSON(w, http.StatusOK, map[string]any{"remote": remote})
	})
	mux.HandleFunc("PUT /api/sync", a.setSyncRepo)
	mux.HandleFunc("POST /api/sync", a.syncNow)
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, updateStatus(r.URL.Query().Has("now")))
	})
	mux.HandleFunc("POST /api/update", a.updateNow)
}

// setSyncRepo sets up sync with a repo, or moves it to another one.
func (a *api) setSyncRepo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	remote, err := gitsync.SetRemote(a.home, body.URL, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"remote": remote})
}

// syncNow is `kando sync` without git ever prompting. When both sides changed, it
// answers 409 with diverged, and the board asks which side to keep.
func (a *api) syncNow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Keep string `json:"keep"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	keep := map[string]gitsync.Keep{"local": gitsync.KeepLocal, "remote": gitsync.KeepRemote}[body.Keep]
	msg, err := gitsync.Sync(a.home, keep, false)
	if d, ok := errors.AsType[*gitsync.Diverged](err); ok {
		writeJSON(w, http.StatusConflict, map[string]any{"error": d.Message, "diverged": true, "first": d.First})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

type updateInfo struct {
	Current   string  `json:"current"`
	Latest    *string `json:"latest"`
	Available *string `json:"available"`
}

// updateStatus is this version, the latest release (nil when GitHub can't be reached)
// and, when that one is newer, available.
func updateStatus(now bool) updateInfo {
	info := updateInfo{Current: version.Version}
	if latest := update.Latest(now); latest != "" {
		info.Latest = &latest
		if update.Newer(latest) {
			info.Available = &latest
		}
	}
	return info
}

// updateNow installs the latest release over this program, answers, then starts it in place.
func (a *api) updateNow(w http.ResponseWriter, _ *http.Request) {
	exe, err := update.Install()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"restarting": true})
	go func() {
		// Let the answer reach the board first.
		time.Sleep(300 * time.Millisecond)
		err := update.Restart(exe)
		fmt.Fprintf(os.Stderr, "kando: updated, but cannot restart: %v\n", err)
	}()
}
