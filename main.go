package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"embed" // Add this import
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

//go:embed static
var staticFiles embed.FS // Embed the static directory

var IsRecording bool               // Global variable to control recording state
var requestLogChan chan RequestLog // Channel for logging requests asynchronously

// ProxyHandler holds the reverse proxy and handles logging
type ProxyHandler struct {
	proxy *httputil.ReverseProxy
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Capture request details
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	// Restore body for proxy
	r.Body = ioutil.NopCloser(bytes.NewReader(requestBody))

	// Decompress request body if gzipped
	decompressedReqBody := requestBody
	if r.Header.Get("Content-Encoding") == "gzip" {
		decompressedReqBody, err = decompressGzip(requestBody)
		if err != nil {
			log.Printf("Error decompressing request body: %v", err)
			// Continue with compressed body if decompression fails
			decompressedReqBody = requestBody
		}
	}

	reqLog := RequestLog{
		Timestamp:      time.Now(),
		Method:         r.Method,
		URL:            r.URL.String(),
		RequestHeaders: HeadersToJSON(r.Header),
		RequestBody:    decompressedReqBody,
	}

	// Store request log in context for later use
	ctx := context.WithValue(r.Context(), "reqLog", &reqLog)
	newReq := r.WithContext(ctx)

	// Serve the request through the proxy
	h.proxy.ServeHTTP(w, newReq)
}

// decompressGzip decompresses a gzip compressed byte slice.
func decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return ioutil.ReadAll(reader)
}

// responseRecorder is a custom ResponseWriter to capture status code and body
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
	headers    http.Header
}

func (rec *responseRecorder) WriteHeader(statusCode int) {
	rec.statusCode = statusCode
	// Copy headers from the original response
	for k, v := range rec.ResponseWriter.Header() {
		rec.headers[k] = v
	}
	rec.ResponseWriter.WriteHeader(statusCode)
}

func (rec *responseRecorder) Write(buf []byte) (int, error) {
	rec.body.Write(buf) // Capture body
	return rec.ResponseWriter.Write(buf)
}

func (rec *responseRecorder) Header() http.Header {
	return rec.headers
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil || cookie.Value != "valid_token" { // Simple check for now
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func getRequests(w http.ResponseWriter, r *http.Request) {
	// Get query parameters
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("page_size")
	urlFilter := r.URL.Query().Get("url")
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	// Parse pagination parameters
	page := 1
	pageSize := 50
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// Calculate offset
	offset := (page - 1) * pageSize

	// Build query with filters
	query := "SELECT id, timestamp, method, url, status_code FROM requests WHERE 1=1"
	countQuery := "SELECT COUNT(*) FROM requests WHERE 1=1"
	var args []interface{}

	// URL filter
	if urlFilter != "" {
		query += " AND url LIKE ?"
		countQuery += " AND url LIKE ?"
		args = append(args, "%"+urlFilter+"%")
	}

	// Date filters - convert date strings to datetime format
	if startDate != "" {
		// Convert YYYY-MM-DD to datetime format with start of day
		startDateTime := startDate + " 00:00:00"
		query += " AND timestamp >= ?"
		countQuery += " AND timestamp >= ?"
		args = append(args, startDateTime)
	}
	if endDate != "" {
		// Convert YYYY-MM-DD to datetime format with end of day
		endDateTime := endDate + " 23:59:59"
		query += " AND timestamp <= ?"
		countQuery += " AND timestamp <= ?"
		args = append(args, endDateTime)
	}

	// Add ordering and pagination
	query += " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)

	// Get total count
	var totalCount int
	err := db.QueryRow(countQuery, args[:len(args)-2]...).Scan(&totalCount)
	if err != nil {
		http.Error(w, "Failed to fetch request count", http.StatusInternalServerError)
		log.Printf("Error fetching request count: %v", err)
		return
	}

	// Execute query with pagination
	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, "Failed to fetch requests", http.StatusInternalServerError)
		log.Printf("Error fetching requests: %v", err)
		return
	}
	defer rows.Close()

	var requests []RequestLog
	for rows.Next() {
		var req RequestLog
		if err := rows.Scan(&req.ID, &req.Timestamp, &req.Method, &req.URL, &req.StatusCode); err != nil {
			log.Printf("Error scanning request: %v", err)
			continue
		}
		requests = append(requests, req)
	}

	// Prepare response with pagination info
	response := struct {
		Requests   []RequestLog `json:"requests"`
		Page       int          `json:"page"`
		PageSize   int          `json:"page_size"`
		TotalCount int          `json:"total_count"`
		TotalPages int          `json:"total_pages"`
	}{
		Requests:   requests,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		TotalPages: (totalCount + pageSize - 1) / pageSize,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getRequestDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/requests/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid request ID", http.StatusBadRequest)
		return
	}

	// Modified SQL query to fetch metadata instead of full bodies
	row := db.QueryRow("SELECT id, timestamp, method, url, request_headers, request_body_size, is_request_body_text, status_code, response_headers, response_body_size, is_response_body_text FROM requests WHERE id = ?", id)

	var req RequestLog
	// Scan into the new metadata fields
	if err := row.Scan(&req.ID, &req.Timestamp, &req.Method, &req.URL, &req.RequestHeaders, &req.RequestBodySize, &req.IsRequestBodyText, &req.StatusCode, &req.ResponseHeaders, &req.ResponseBodySize, &req.IsResponseBodyText); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Request not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch request details", http.StatusInternalServerError)
		log.Printf("Error fetching request details: %v", err)
		return
	}

	// Create a response struct that only includes metadata for bodies
	response := struct {
		ID                 int       `json:"id"`
		Timestamp          time.Time `json:"timestamp"`
		Method             string    `json:"method"`
		URL                string    `json:"url"`
		RequestHeaders     string    `json:"request_headers"`
		RequestBodySize    int       `json:"request_body_size"`
		IsRequestBodyText  bool      `json:"is_request_body_text"`
		StatusCode         int       `json:"status_code"`
		ResponseHeaders    string    `json:"response_headers"`
		ResponseBodySize   int       `json:"response_body_size"`
		IsResponseBodyText bool      `json:"is_response_body_text"`
	}{
		ID:                 req.ID,
		Timestamp:          req.Timestamp,
		Method:             req.Method,
		URL:                req.URL,
		RequestHeaders:     req.RequestHeaders,
		RequestBodySize:    req.RequestBodySize,
		IsRequestBodyText:  req.IsRequestBodyText,
		StatusCode:         req.StatusCode,
		ResponseHeaders:    req.ResponseHeaders,
		ResponseBodySize:   req.ResponseBodySize,
		IsResponseBodyText: req.IsResponseBodyText,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func replayRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var replayData struct {
		Method  string              `json:"method"`
		URL     string              `json:"url"`
		Headers map[string][]string `json:"headers"`
		Body    string              `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&replayData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		log.Printf("Error decoding replay data: %v", err)
		return
	}

	// --- Fix: Handle relative URLs ---
	// Retrieve the target URL from the global flag variable
	targetFlag := flag.Lookup("target")
	var targetStr string
	if targetFlag != nil {
		targetStr = targetFlag.Value.String()
	} else {
		// Fallback if flag is not found (should not happen in normal operation)
		targetStr = "http://localhost:8081"
		log.Println("Warning: '-target' flag not found, using default fallback for replay URL resolution.")
	}

	parsedTarget, err := url.Parse(targetStr)
	if err != nil {
		// Final fallback if parsing fails
		parsedTarget = &url.URL{Scheme: "http", Host: "localhost:8081"}
		log.Printf("Warning: Failed to parse target URL '%s', using fallback: %v", targetStr, parsedTarget)
	}

	// Parse the URL from replay data. If it's relative, resolve it against the target.
	parsedReplayURL, err := url.Parse(replayData.URL)
	if err != nil {
		http.Error(w, "Invalid URL in replay data", http.StatusBadRequest)
		log.Printf("Error parsing replay URL: %v", err)
		return
	}

	// If the URL from replay data is relative (no scheme), resolve it against the target
	finalURL := parsedTarget.ResolveReference(parsedReplayURL).String()

	// Create a new HTTP request
	replayReq, err := http.NewRequest(replayData.Method, finalURL, bytes.NewBuffer([]byte(replayData.Body)))
	if err != nil {
		http.Error(w, "Failed to create replay request", http.StatusInternalServerError)
		log.Printf("Error creating replay request to %s: %v", finalURL, err)
		return
	}

	// Add headers
	for k, v := range replayData.Headers {
		// Join multiple values with comma (standard for HTTP headers)
		// Special handling for Host header if needed, though Go usually handles it via req.Host
		replayReq.Header.Set(k, strings.Join(v, ", "))
	}

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(replayReq)
	if err != nil {
		http.Error(w, "Failed to execute replayed request", http.StatusInternalServerError)
		log.Printf("Error executing replayed request to %s: %v", finalURL, err)
		return
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read replayed response body", http.StatusInternalServerError)
		log.Printf("Error reading replayed response body from %s: %v", finalURL, err)
		return
	}

	// Decompress if necessary
	bodyBytes := respBody
	if resp.Header.Get("Content-Encoding") == "gzip" {
		decompressedBody, err := decompressGzip(respBody)
		if err != nil {
			log.Printf("Warning: Failed to decompress gzipped replay response from %s: %v", finalURL, err)
			// Keep original compressed body if decompression fails
			bodyBytes = respBody
		} else {
			bodyBytes = decompressedBody
		}
	}

	// Determine if the content is text or binary
	contentType := resp.Header.Get("Content-Type")
	isText := strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "xml") ||
		strings.Contains(contentType, "javascript")

	var finalRespBody string
	if isText {
		// If it's text, convert to string directly
		finalRespBody = string(bodyBytes)
	} else {
		// If it's binary, Base64 encode it
		finalRespBody = base64.StdEncoding.EncodeToString(bodyBytes)
	}

	// Return the replayed response details
	result := struct {
		StatusCode int         `json:"statusCode"`
		Headers    http.Header `json:"headers"`
		Body       string      `json:"body"`
	}{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       finalRespBody,
	}

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(result); err != nil {
		log.Printf("Error encoding replay response JSON: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // Delete the cookie
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Logged out"}`))
}

// generateCertificates generates CA and server certificates
func generateCertificates() {
	log.Println("Generating Root CA certificate and key...")

	// CA certificate
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2024),
		Subject: pkix.Name{
			Organization: []string{"dGateway CA"},
			CommonName:   "dGateway Root CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// CA private key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("Failed to generate CA private key: %v", err)
	}

	// Self-signed CA certificate
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		log.Fatalf("Failed to create CA certificate: %v", err)
	}

	// Save CA certificate
	caCertFile, err := os.Create("certs/ca.crt")
	if err != nil {
		log.Fatalf("Failed to create ca.crt: %v", err)
	}
	defer caCertFile.Close()
	err = pem.Encode(caCertFile, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	if err != nil {
		log.Fatalf("Failed to encode ca.crt: %v", err)
	}
	log.Println("Generated certs/ca.crt")

	// Save CA private key
	caKeyFile, err := os.Create("certs/ca.key")
	if err != nil {
		log.Fatalf("Failed to create ca.key: %v", err)
	}
	defer caKeyFile.Close()
	err = pem.Encode(caKeyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey)})
	if err != nil {
		log.Fatalf("Failed to encode ca.key: %v", err)
	}
	log.Println("Generated certs/ca.key")

	log.Println("Generating server certificate and key...")

	// Server certificate
	serverCert := &x509.Certificate{
		SerialNumber: big.NewInt(2025),
		Subject: pkix.Name{
			Organization: []string{"dGateway Server"},
			CommonName:   "localhost",
		},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0), // 1 year
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	// Server private key
	serverPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("Failed to generate server private key: %v", err)
	}

	// Create server certificate signed by CA
	serverBytes, err := x509.CreateCertificate(rand.Reader, serverCert, ca, &serverPrivKey.PublicKey, caPrivKey)
	if err != nil {
		log.Fatalf("Failed to create server certificate: %v", err)
	}

	// Save server certificate
	serverCertOut, err := os.Create("certs/server.crt")
	if err != nil {
		log.Fatalf("Failed to create server.crt: %v", err)
	}
	defer serverCertOut.Close()

	err = pem.Encode(serverCertOut, &pem.Block{Type: "CERTIFICATE", Bytes: serverBytes})
	if err != nil {
		log.Fatalf("Failed to encode server.crt: %v", err)
	}
	log.Println("Generated certs/server.crt")

	// Save server private key
	serverKeyOut, err := os.Create("certs/server.key")
	if err != nil {
		log.Fatalf("Failed to create server.key: %v", err)
	}
	defer serverKeyOut.Close()

	err = pem.Encode(serverKeyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey)})
	if err != nil {
		log.Fatalf("Failed to encode server.key: %v", err)
	}
	log.Println("Generated certs/server.key")

	log.Println("All certificates generated successfully.")
	log.Println("IMPORTANT: Install certs/ca.crt into your system/browser trust store to avoid certificate errors.")
}

func startRecordingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	IsRecording = true
	log.Println("Recording started.")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Recording started"}`))
}

func stopRecordingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	IsRecording = false
	log.Println("Recording stopped.")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Recording stopped"}`))
}

func getRecordingStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := "stopped"
	if IsRecording {
		status = "recording"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"status": "%s"}`, status)))
}

func getRequestBodyHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/requests/body/request/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid request ID", http.StatusBadRequest)
		return
	}

	var reqBody []byte
	var reqHeaders string
	row := db.QueryRow("SELECT request_body, request_headers FROM requests WHERE id = ?", id)
	if err := row.Scan(&reqBody, &reqHeaders); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Request not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch request body", http.StatusInternalServerError)
		log.Printf("Error fetching request body for ID %d: %v", id, err)
		return
	}

	// Try to set appropriate Content-Type
	contentType := getContentTypeFromHeaders(reqHeaders)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		// Default to plain text if content type is unknown
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Write(reqBody)
}

func getResponseBodyHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/requests/body/response/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid request ID", http.StatusBadRequest)
		return
	}

	var respBody []byte
	var respHeaders string
	row := db.QueryRow("SELECT response_body, response_headers FROM requests WHERE id = ?", id)
	if err := row.Scan(&respBody, &respHeaders); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Request not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch response body", http.StatusInternalServerError)
		log.Printf("Error fetching response body for ID %d: %v", id, err)
		return
	}

	// Try to set appropriate Content-Type
	contentType := getContentTypeFromHeaders(respHeaders)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		// Default to plain text if content type is unknown
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Write(respBody)
}

func exportHARHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get all requests from database
	rows, err := db.Query("SELECT id, timestamp, method, url, request_headers, request_body, status_code, response_headers, response_body FROM requests ORDER BY timestamp")
	if err != nil {
		http.Error(w, "Failed to fetch requests", http.StatusInternalServerError)
		log.Printf("Error fetching requests: %v", err)
		return
	}
	defer rows.Close()

	var requests []RequestLog
	for rows.Next() {
		var req RequestLog
		if err := rows.Scan(&req.ID, &req.Timestamp, &req.Method, &req.URL, &req.RequestHeaders, &req.RequestBody, &req.StatusCode, &req.ResponseHeaders, &req.ResponseBody); err != nil {
			log.Printf("Error scanning request: %v", err)
			continue
		}
		requests = append(requests, req)
	}

	// Convert to HAR format
	har, err := exportRequestsToHAR(requests)
	if err != nil {
		http.Error(w, "Failed to export requests to HAR format", http.StatusInternalServerError)
		log.Printf("Error exporting to HAR: %v", err)
		return
	}

	// Set response headers for file download
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dgateway-export.har\"")

	// Encode and send HAR file
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(har); err != nil {
		log.Printf("Error encoding HAR JSON: %v", err)
		http.Error(w, "Failed to encode HAR file", http.StatusInternalServerError)
		return
	}
}
func main() {
	port := flag.Int("port", 8080, "port to listen on for proxy")
	target := flag.String("target", "http://127.0.0.1:8081", "target to forward requests to")
	dbPath := flag.String("db", "requests.db", "path to SQLite database file")
	genCerts := flag.Bool("gen-certs", false, "generate CA and server certificates")
	enableHTTPS := flag.Bool("enable-https", false, "enable HTTPS support on the same port")
	recordOnStart := flag.Bool("record-on-start", true, "start recording requests by default")
	flag.Parse()

	IsRecording = *recordOnStart

	if *genCerts {
		generateCertificates()
		return
	}

	adminUsername := os.Getenv("ADMIN_USERNAME")
	if adminUsername == "" {
		adminUsername = "admin"
	}
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "admin"
	}

	// Initialize database
	InitDB(*dbPath)

	// Initialize the request log channel
	requestLogChan = make(chan RequestLog, 100) // Buffer up to 100 requests

	// Start a goroutine to process log entries from the channel
	go func() {
		for logEntry := range requestLogChan {
			LogRequest(logEntry)
		}
	}()

	// --- Proxy Server Setup ---
	remote, err := url.Parse(*target)
	if err != nil {
		log.Fatalf("Failed to parse target URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(remote)

	// Custom response modifier to capture, decompress, and ensure correct headers
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Get the request log from context
		reqLog, ok := resp.Request.Context().Value("reqLog").(*RequestLog)
		if !ok {
			log.Printf("Failed to get request log from context")
			return nil // Not an error for the client, just for our logging
		}

		// Capture response status code
		reqLog.StatusCode = resp.StatusCode

		// Capture response headers (do this early to preserve original headers for logging)
		reqLog.ResponseHeaders = HeadersToJSON(resp.Header)

		// Capture response body
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			// Log the error and return it to potentially abort the response
			log.Printf("Error reading response body: %v", err)
			return err
		}
		resp.Body.Close() // Important: Close the original body

		// Decompress response body if gzipped
		if resp.Header.Get("Content-Encoding") == "gzip" {
			decompressedBody, err := decompressGzip(body)
			if err != nil {
				log.Printf("Error decompressing response body: %v", err)
				// Continue with compressed body if decompression fails
				// Do not modify headers in this case
			} else {
				body = decompressedBody
				// Crucial: Remove the Content-Encoding header as the body is now decompressed
				resp.Header.Del("Content-Encoding")
				// Crucial: Update Content-Length header as the body size has changed
				resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
			}
		}

		// Store potentially modified body for logging
		reqLog.ResponseBody = body

		// Update response with the (possibly modified) body
		resp.Body = ioutil.NopCloser(bytes.NewReader(body))

		// Log to database if recording is enabled
		if IsRecording {
			select {
			case requestLogChan <- *reqLog:
				// Successfully sent to channel
			default:
				log.Println("Request log channel is full, dropping log entry.")
			}
		}

		return nil
	}

	proxyHandler := &ProxyHandler{proxy: proxy}

	// Start server with HTTPS support if enabled
	go func() {
		if *enableHTTPS {
			log.Printf("Proxy server listening on port %d with HTTPS support, forwarding to %s", *port, *target)

			// Certificate files
			certFile := "certs/server.crt"
			keyFile := "certs/server.key"

			// Check if certificate files exist
			if _, err := os.Stat(certFile); os.IsNotExist(err) {
				log.Println("Server certificate not found, using default certificates")
				certFile = "certs/ca.crt"
				keyFile = "certs/ca.key"
			} else if _, err := os.Stat(keyFile); os.IsNotExist(err) {
				log.Println("Server key not found, using default certificates")
				certFile = "certs/ca.crt"
				keyFile = "certs/ca.key"
			}

			// Create server
			server := &http.Server{
				Addr:    ":" + strconv.Itoa(*port),
				Handler: proxyHandler,
			}

			// Start TLS server
			log.Printf("Server is listening on port %d for HTTPS connections", *port)
			if err := server.ListenAndServeTLS(certFile, keyFile); err != nil {
				log.Fatalf("Failed to start HTTPS proxy server: %v", err)
			}
		} else {
			log.Printf("Proxy server listening on port %d (HTTP only), forwarding to %s", *port, *target)
			if err := http.ListenAndServe(":"+strconv.Itoa(*port), proxyHandler); err != nil {
				log.Fatalf("Failed to start HTTP proxy server: %v", err)
			}
		}
	}()

	// --- Admin Server Setup ---
	adminPort := *port + 1
	adminMux := http.NewServeMux()

	// Serve static files from embedded file system
	adminMux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))

	// Serve i18n files with proper content type from embedded file system
	adminMux.HandleFunc("/i18n/", func(w http.ResponseWriter, r *http.Request) {
		// Ensure the path is clean and within the expected directory
		upath := r.URL.Path
		if !strings.HasPrefix(upath, "/") {
			upath = "/" + upath
			r.URL.Path = upath
		}
		upath = path.Clean(upath)

		// Construct the file path relative to the embedded 'static' directory
		// The 'static' prefix is already handled by the embed directive
		filePath := path.Join("static/i18n", upath[len("/i18n/"):]) // Remove "/i18n/" prefix

		// Read the file from the embedded file system
		content, err := staticFiles.ReadFile(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Set content type for JSON files
		w.Header().Set("Content-Type", "application/json")

		// Serve the file content
		w.Write(content)
	})

	// Login page
	adminMux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			content, err := staticFiles.ReadFile("static/login.html")
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				log.Printf("Error reading embedded login.html: %v", err)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(content)
			return
		}

		if r.Method == "POST" {
			var creds struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			err := json.NewDecoder(r.Body).Decode(&creds)
			if err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}

			// Authenticate using environment variables or defaults
			if creds.Username == adminUsername && creds.Password == adminPassword {
				// In a real app, generate a secure token/session ID
				http.SetCookie(w, &http.Cookie{
					Name:     "session_token",
					Value:    "valid_token", // Placeholder
					Path:     "/",
					HttpOnly: true,
					Secure:   false, // Set to true in production with HTTPS
					SameSite: http.SameSiteLaxMode,
				})
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message": "Login successful"}`))
			} else {
				http.Error(w, `{"message": "Invalid credentials"}`, http.StatusUnauthorized)
			}
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	// Admin API endpoints (protected)
	adminMux.HandleFunc("/api/requests", authMiddleware(getRequests))
	adminMux.HandleFunc("/api/requests/body/request/", authMiddleware(getRequestBodyHandler))   // /api/requests/body/request/{id}
	adminMux.HandleFunc("/api/requests/body/response/", authMiddleware(getResponseBodyHandler)) // /api/requests/body/response/{id}
	adminMux.HandleFunc("/api/requests/", authMiddleware(getRequestDetail))                     // Trailing slash for ID
	adminMux.HandleFunc("/api/replay", authMiddleware(replayRequest))
	adminMux.HandleFunc("/api/start-recording", authMiddleware(startRecordingHandler))
	adminMux.HandleFunc("/api/stop-recording", authMiddleware(stopRecordingHandler))
	adminMux.HandleFunc("/api/recording-status", authMiddleware(getRecordingStatusHandler))
	adminMux.HandleFunc("/api/export/har", authMiddleware(exportHARHandler))
	adminMux.HandleFunc("/logout", logoutHandler)

	// Root handler for admin interface
	adminMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil || cookie.Value != "valid_token" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		content, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			log.Printf("Error reading embedded index.html: %v", err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(content)
	})

	// Print startup information
	log.Printf("dGateway Proxy Server listening on: http://localhost:%d", *port)
	log.Printf("Forwarding requests to: %s", *target)
	log.Printf("dGateway Admin Panel available at: http://localhost:%d", adminPort)
	if IsRecording {
		log.Println("Recording mode: ON (requests will be logged)")
	} else {
		log.Println("Recording mode: OFF (requests will NOT be logged)")
	}

	log.Printf("Admin server listening on port %d", adminPort)
	if err := http.ListenAndServe(":"+strconv.Itoa(adminPort), adminMux); err != nil {
		log.Fatalf("Failed to start admin server: %v", err)
	}
}
