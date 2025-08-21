package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

// main starts the reverse proxy server.
// It reads the backend URL and port from environment variables.
// BACKEND_URL is required, PORT defaults to 8080 if not set.
func main() {
	// Get backend URL from environment variable
	backendURL := os.Getenv("BACKEND_URL")
	if backendURL == "" {
		log.Fatal("BACKEND_URL environment variable is required")
	}

	// Parse the backend URL
	target, err := url.Parse(backendURL)
	if err != nil {
		log.Fatalf("Invalid BACKEND_URL: %v", err)
	}

	// Create a reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Set error handler for the proxy
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error: %v", err)
		http.Error(w, "Proxy error", http.StatusBadGateway)
	}

	// Customize the Director to handle X-Forwarded-* headers properly
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Use the standard library's implementation for X-Forwarded-* headers
		// This is a more generic approach than manually setting each header
		if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			// Check if X-Forwarded-For already exists and append to it
			if prior, ok := req.Header["X-Forwarded-For"]; ok {
				clientIP = strings.Join(prior, ", ") + ", " + clientIP
			}
			req.Header.Set("X-Forwarded-For", clientIP)
		}

		// Set X-Forwarded-Proto if not already set
		if _, ok := req.Header["X-Forwarded-Proto"]; !ok {
			if req.TLS == nil {
				req.Header.Set("X-Forwarded-Proto", "http")
			} else {
				req.Header.Set("X-Forwarded-Proto", "https")
			}
		}

		// Set X-Forwarded-Host if not already set
		if _, ok := req.Header["X-Forwarded-Host"]; !ok {
			req.Header.Set("X-Forwarded-Host", req.Host)
		}
	}

	// Get port from environment variable, default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start the server
	log.Printf("Starting proxy server on port %s, forwarding to %s", port, backendURL)
	err = http.ListenAndServe(":"+port, proxy)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
