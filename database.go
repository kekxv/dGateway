
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http" // Added for http.Header
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type RequestLog struct {
	ID             int
	Timestamp      time.Time
	Method         string
	URL            string
	RequestHeaders string // JSON string
	RequestBody    []byte
	RequestBodySize int // New field
	IsRequestBodyText bool // New field
	StatusCode     int
	ResponseHeaders string // JSON string
	ResponseBody   []byte
	ResponseBodySize int // New field
	IsResponseBodyText bool // New field
}

var db *sql.DB

func InitDB(dataSourceName string) {
	var err error
	db, err = sql.Open("sqlite", dataSourceName)
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

	// Add new columns if they don't exist
	// SQLite doesn't have IF NOT EXISTS for ADD COLUMN, so we check manually
	// Start a transaction for schema modifications
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("Failed to begin transaction for schema migration: %v", err)
	}
	defer tx.Rollback() // Rollback on error or if not committed

	addColumnIfNotExists(tx, "requests", "request_body_size", "INTEGER")
	addColumnIfNotExists(tx, "requests", "is_request_body_text", "BOOLEAN")
	addColumnIfNotExists(tx, "requests", "response_body_size", "INTEGER")
	addColumnIfNotExists(tx, "requests", "is_response_body_text", "BOOLEAN")

	if err := tx.Commit(); err != nil {
		log.Fatalf("Failed to commit schema migration: %v", err)
	}

	// Enable WAL mode for better concurrency
	_, err = db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		log.Printf("Failed to enable WAL mode: %v", err)
	}

	log.Println("Database initialized successfully.")
}

// addColumnIfNotExists checks if a column exists and adds it if not.
// It assumes it's called within a transaction.
func addColumnIfNotExists(tx *sql.Tx, tableName, columnName, columnType string) {
	query := fmt.Sprintf("PRAGMA table_info(%s);", tableName)
	rows, err := tx.Query(query)
	if err != nil {
		log.Fatalf("Failed to query table info for %s: %v", tableName, err)
	}
	defer rows.Close()

	columnExists := false
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			log.Fatalf("Failed to scan table info row: %v", err)
		}
		if name == columnName {
			columnExists = true
			break
		}
	}

	if !columnExists {
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", tableName, columnName, columnType)
		_, err := tx.Exec(alterSQL)
		if err != nil {
			log.Fatalf("Failed to add column %s to table %s: %v", columnName, tableName, err)
		}
		log.Printf("Added column %s to table %s.", columnName, tableName)
	}
}

func LogRequest(logEntry RequestLog) {
	// Populate size and text/binary info
	logEntry.RequestBodySize = len(logEntry.RequestBody)
	logEntry.IsRequestBodyText = isTextData(logEntry.RequestBody, getContentTypeFromHeaders(logEntry.RequestHeaders))
	logEntry.ResponseBodySize = len(logEntry.ResponseBody)
	logEntry.IsResponseBodyText = isTextData(logEntry.ResponseBody, getContentTypeFromHeaders(logEntry.ResponseHeaders))

	stmt, err := db.Prepare(`
	INSERT INTO requests(
		timestamp, method, url, request_headers, request_body, request_body_size, is_request_body_text,
		status_code, response_headers, response_body, response_body_size, is_response_body_text
	)
	VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		logEntry.RequestBodySize,
		logEntry.IsRequestBodyText,
		logEntry.StatusCode,
		logEntry.ResponseHeaders,
		logEntry.ResponseBody,
		logEntry.ResponseBodySize,
		logEntry.IsResponseBodyText,
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
