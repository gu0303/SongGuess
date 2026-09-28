package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed web/*
var embeddedWeb embed.FS

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func main() {
	webSubFS, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		log.Fatalf("failed to open embedded web files: %v", err)
	}

	mux := http.NewServeMux()

	// Serve Static Assets
	fileServer := http.FileServer(http.FS(webSubFS))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	// Serve Frontend Index
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		indexData, err := embeddedWeb.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexData)
	})

	// Audio Proxy Endpoint (RD-00, RD-02, RG-04)
	mux.HandleFunc("GET /audio/{token}", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		audioData, found := GetAudio(token)
		if !found {
			http.Error(w, "audio not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Cache-Control", "public, max-age=3600")

		http.ServeContent(w, r, "clip.mp3", time.Time{}, bytes.NewReader(audioData))
	})


	// Load Playlist by URL or ID (No login required!)
	mux.HandleFunc("POST /api/playlist/load", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "dados inválidos"})
			return
		}

		pl, err := LoadPlaylistFromInput(req.URL)
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		jsonResponse(w, http.StatusOK, map[string]any{
			"id":          pl.ID,
			"name":        pl.Name,
			"description": pl.Description,
			"coverUrl":    pl.CoverURL,
			"totalTracks": len(pl.Tracks),
			"source":      pl.Source,
		})
	})

	// Game API
	mux.HandleFunc("POST /api/game/start", func(w http.ResponseWriter, r *http.Request) {
		var cfg GameConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "dados inválidos"})
			return
		}

		pl, err := LoadPlaylistFromInput(cfg.PlaylistID)
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		cfg.PlaylistName = pl.Name
		if err := GlobalGame.StartNewGame(cfg, pl.Tracks); err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		jsonResponse(w, http.StatusOK, map[string]any{
			"status": "started",
			"config": GlobalGame.Config,
			"rounds": len(GlobalGame.TargetTracks),
		})
	})

	mux.HandleFunc("POST /api/game/round/next", func(w http.ResponseWriter, r *http.Request) {
		st, err := GlobalGame.NextRound()
		if err != nil {
			jsonResponse(w, http.StatusGone, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, st)
	})

	mux.HandleFunc("POST /api/game/round/choose", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OptionID string `json:"optionId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}

		st, err := GlobalGame.HandleChoose(req.OptionID)
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, st)
	})

	mux.HandleFunc("POST /api/game/round/guess", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Guess string `json:"guess"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}

		st, err := GlobalGame.HandleGuess(req.Guess)
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, st)
	})

	mux.HandleFunc("POST /api/game/round/reveal", func(w http.ResponseWriter, r *http.Request) {
		st, err := GlobalGame.HandleReveal()
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, st)
	})

	mux.HandleFunc("POST /api/game/round/timeout", func(w http.ResponseWriter, r *http.Request) {
		st, err := GlobalGame.HandleTimeout()
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, st)
	})

	mux.HandleFunc("GET /api/game/summary", func(w http.ResponseWriter, r *http.Request) {
		sum := GlobalGame.GetSummary()
		jsonResponse(w, http.StatusOK, sum)
	})

	addr := "127.0.0.1:8080"
	fmt.Println("==================================================")
	fmt.Println("🎵 Song Guess — Sistema Solo / Local Iniciado!")
	fmt.Println("🔓 Sem login: basta colar o link de qualquer playlist!")
	fmt.Printf("👉 Acesse no seu navegador: http://%s\n", addr)
	fmt.Println("==================================================")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

var _ = io.EOF
