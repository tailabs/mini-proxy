package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// loggedTransport wraps an http.RoundTripper to log requests and responses
type loggedTransport struct {
	http.RoundTripper
}

func (t *loggedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Log the request being sent to the backend
	log.Printf("Backend request: %s %s Host: %s X-Forwarded-For: %s", 
		req.Method, req.URL.String(), req.Host, req.Header.Get("X-Forwarded-For"))
	
	// Execute the request
	resp, err := t.RoundTripper.RoundTrip(req)
	
	// Log the response
	if err != nil {
		log.Printf("Backend error: %v for %s %s", err, req.Method, req.URL.String())
	} else {
		log.Printf("Backend response: %s %s -> %s", req.Method, req.URL.String(), resp.Status)
	}
	
	return resp, err
}

// getRealIP extracts the real IP address from an HTTP request
func getRealIP(r *http.Request) string {
	// Check X-Real-IP header first (highest priority)
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}
	
	// Check X-Forwarded-For header
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// The first IP in X-Forwarded-For is the client IP
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	
	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

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
	
	// Configure the transport for better performance
	proxy.Transport = &http.Transport{
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   100,
	}
	
	// Wrap transport to log backend requests
	originalTransport := proxy.Transport
	proxy.Transport = &loggedTransport{originalTransport}
	
	// Set error handler for the proxy
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error for %s %s: %v", r.Method, r.URL.String(), err)
		http.Error(w, "Proxy error", http.StatusBadGateway)
	}

	// Add request logging middleware
	logMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get real IP prioritizing X-Real-IP header
			realIP := getRealIP(r)
			xff := r.Header.Get("X-Forwarded-For")
			xfh := r.Header.Get("X-Forwarded-Host")
			xproto := r.Header.Get("X-Forwarded-Proto")
			xri := r.Header.Get("X-Real-IP")
			
			// Log incoming request details with emphasis on X-Real-IP
			log.Printf("Incoming request: %s %s, Client IP: %s (X-Real-IP: %s, X-Forwarded-For: %s), X-Forwarded-Host: %s, X-Forwarded-Proto: %s", 
				r.Method, r.URL.Path, realIP, xri, xff, xfh, xproto)
			
			// Wrap ResponseWriter to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)
			
			// Log response details
			log.Printf("Response: %s %s -> %d", r.Method, r.URL.Path, wrapped.statusCode)
		})
	}
	
	// Customize the Director to handle X-Forwarded-* headers properly
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		
		// Remove headers that shouldn't be forwarded to the backend
		req.Header.Del("Connection")
		req.Header.Del("Upgrade")
		req.Header.Del("Transfer-Encoding")
		
		// Save the original host before modifying it
		originalHost := req.Host
		
		// Set the Host header to the target host so the backend knows which site to serve
		req.Host = target.Host
		
		// Handle X-Forwarded-For - use X-Real-IP if available, otherwise use existing XFF or client IP
		xri := req.Header.Get("X-Real-IP")
		if xri != "" {
			// If X-Real-IP exists, use it as the X-Forwarded-For value
			req.Header.Set("X-Forwarded-For", xri)
		} else {
			// If no X-Real-IP, use the standard approach
			if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
				// Check if X-Forwarded-For already exists and append to it
				if prior, ok := req.Header["X-Forwarded-For"]; ok {
					clientIP = strings.Join(prior, ", ") + ", " + clientIP
				}
				req.Header.Set("X-Forwarded-For", clientIP)
			}
		}

		// Set X-Forwarded-Proto if not already set
		if _, ok := req.Header["X-Forwarded-Proto"]; !ok {
			// Check if the original request was HTTPS
			// First check if req.TLS is set (direct HTTPS connection)
			// If not, check the Forwarded header or X-Forwarded-Proto header from a load balancer
			if req.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else if proto := req.Header.Get("X-Forwarded-Proto"); proto != "" {
				// Use the proto from a previous proxy/load balancer
				req.Header.Set("X-Forwarded-Proto", proto)
			} else {
				// Default to http
				req.Header.Set("X-Forwarded-Proto", "http")
			}
		}

		// Set X-Forwarded-Host if not already set
		if _, ok := req.Header["X-Forwarded-Host"]; !ok {
			req.Header.Set("X-Forwarded-Host", originalHost)
		}
		
		// Log the modified request details before forwarding
		log.Printf("Forwarding request: %s %s to %s with Host: %s, X-Forwarded-For: %s", 
			req.Method, req.URL.Path, target.String(), req.Host, req.Header.Get("X-Forwarded-For"))
	}

	// Get port from environment variable, default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Add timeout middleware
	var handler http.Handler = proxy
	handler = logMiddleware(handler)
	handler = http.TimeoutHandler(handler, 30*time.Second, "Proxy timeout")

	// Start the server
	log.Printf("Starting proxy server on port %s, forwarding to %s", port, backendURL)
	err = http.ListenAndServe(":"+port, handler)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
