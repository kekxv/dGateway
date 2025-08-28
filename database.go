
package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http" // Added for http.Header
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type RequestLog struct {
	ID             int
	Timestamp      time.Time
	Method         string
	URL            string
	RequestHeaders string // JSON string
	RequestBody    []byte
	StatusCode     int
	ResponseHeaders string // JSON string
	ResponseBody   []byte
}

var db *sql.DB

func InitDB(dataSourceName string) {
	var err error
	db, err = sql.Open("sqlite3", dataSourceName)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS requests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		method TEXT,
		url TEXT,
		request_headers TEXT,
		request_body BLOB,
		status_code INTEGER,
		response_headers TEXT,
		response_body BLOB
	);
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	log.Println("Database initialized successfully.")
}

func LogRequest(logEntry RequestLog) {
	stmt, err := db.Prepare(`
	INSERT INTO requests(
		timestamp, method, url, request_headers, request_body,
		status_code, response_headers, response_body
	)
	VALUES(?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		log.Printf("Failed to prepare statement: %v", err)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		logEntry.Timestamp,
		logEntry.Method,
		logEntry.URL,
		logEntry.RequestHeaders,
		logEntry.RequestBody,
		logEntry.StatusCode,
		logEntry.ResponseHeaders,
		logEntry.ResponseBody,
	)
	if err != nil {
		log.Printf("Failed to insert log entry: %v", err)
	}
}

// Helper to convert http.Header to JSON string
func HeadersToJSON(headers http.Header) string {
	jsonBytes, err := json.Marshal(headers)
	if err != nil {
		log.Printf("Error marshalling headers to JSON: %v", err)
		return "{}"
	}
	return string(jsonBytes)
}

// isTextData determines if the given data is text or binary
func isTextData(data []byte, contentType string) bool {
	// If content type indicates text, treat as text
	if strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "application/javascript") ||
		strings.Contains(contentType, "application/xhtml+xml") {
		return true
	}

	// If no content type or not explicitly text, check the data
	if len(data) == 0 {
		return true // Empty data is considered text
	}

	// Check if data contains mostly printable characters
	textChars := 0
	for _, b := range data[:min(len(data), 512)] { // Check first 512 bytes or less
		if b == 0x09 || b == 0x0A || b == 0x0D || (b >= 0x20 && b <= 0x7E) {
			textChars++
		}
	}
	
	// If more than 70% of characters are printable, treat as text
	return float64(textChars)/float64(min(len(data), 512)) > 0.7
}

// getContentTypeFromHeaders extracts content type from JSON headers string
func getContentTypeFromHeaders(headersJSON string) string {
	var headers http.Header
	if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
		return ""
	}
	return headers.Get("Content-Type")
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
