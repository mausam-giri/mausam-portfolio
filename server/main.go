package main

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ContactRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
}

type VisitorInfo struct {
	IP        string   `json:"ip"`
	Location  Location `json:"location"`
	UserAgent string   `json:"user_agent"`
	Language  string   `json:"language"`
}

type Location struct {
	Country string `json:"country,omitempty"`
	City    string `json:"city,omitempty"`
	Region  string `json:"region,omitempty"`
}

// --------------------
// Visitor information
// --------------------

func getVisitorInfo(r *http.Request) VisitorInfo {
	return VisitorInfo{
		IP:        getIP(r),
		Location:  getLocation(r),
		UserAgent: r.UserAgent(),
		Language:  getLanguage(r),
	}
}

func getIP(r *http.Request) string {
	// Cloudflare
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}

	// CloudFront / other proxies
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}

	// Other reverse proxies
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Direct connection
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}

func getLocation(r *http.Request) Location {
	// CloudFront
	country := r.Header.Get("CloudFront-Viewer-Country")
	city := r.Header.Get("CloudFront-Viewer-City")
	region := r.Header.Get("CloudFront-Viewer-Country-Region-Name")

	// Cloudflare fallback
	if country == "" {
		country = r.Header.Get("CF-IPCountry")
	}

	if city == "" {
		city = r.Header.Get("CF-IPCity")
	}

	if region == "" {
		region = r.Header.Get("CF-Region")
	}

	return Location{
		Country: country,
		City:    city,
		Region:  region,
	}
}

func getLanguage(r *http.Request) string {
	language := r.Header.Get("Accept-Language")

	if language == "" {
		return ""
	}

	// Keep only the preferred language.
	if i := strings.Index(language, ","); i != -1 {
		language = language[:i]
	}

	// Remove quality value if present.
	if i := strings.Index(language, ";"); i != -1 {
		language = language[:i]
	}

	return strings.TrimSpace(language)
}

// --------------------
// Main
// --------------------

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	root := os.Getenv("DIST_DIR")
	if root == "" {
		root = filepath.Join("..", "ui", "dist")
	}

	content := os.DirFS(root)

	mux := http.NewServeMux()

	// --------------------
	// Health
	// --------------------

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
		})
	})

	// --------------------
	// Contact
	// --------------------

	mux.HandleFunc("/api/contact", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ContactRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(req.Email) == "" {
			http.Error(w, "email is required", http.StatusBadRequest)
			return
		}

		log.Printf(
			"contact from %s <%s>",
			req.Name,
			req.Email,
		)

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(map[string]bool{
			"ok": true,
		})
	})

	// --------------------
	// Visitor information
	// --------------------

	mux.HandleFunc("/api/visit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		info := getVisitorInfo(r)

		log.Printf(
			"[VISIT] IP: %s | Country: %s | City: %s | Region: %s | Language: %s | User-Agent: %s",
			info.IP,
			info.Location.Country,
			info.Location.City,
			info.Location.Region,
			info.Language,
			info.UserAgent,
		)

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(info)
	})

	// --------------------
	// Static Astro files
	// --------------------

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		path = strings.TrimSuffix(path, "/")

		if path == "" {
			path = "index.html"
		}

		// Disable browser caching during development
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

		candidates := []string{
			path + "/index.html",
			path + ".html",
			path,
		}

		for _, candidate := range candidates {
			f, err := content.Open(candidate)
			if err != nil {
				continue
			}

			info, err := f.Stat()
			if err != nil || info.IsDir() {
				f.Close()
				continue
			}

			seeker, ok := f.(io.ReadSeeker)
			if !ok {
				f.Close()
				continue
			}

			http.ServeContent(w, r, info.Name(), info.ModTime(), seeker)
			f.Close()
			return
		}

		http.NotFound(w, r)
	})

	// --------------------
	// Server
	// --------------------

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	log.Printf("serving %s on :%s", root, port)

	log.Fatal(server.ListenAndServe())
}
