package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// HAR represents the top-level HAR structure
type HAR struct {
	Log HARLog `json:"log"`
}

// HARLog represents the log object in HAR format
type HARLog struct {
	Version string     `json:"version"`
	Creator HARCreator `json:"creator"`
	Entries []HAREntry `json:"entries"`
	Pages   []HARPage  `json:"pages,omitempty"`
	Comment string     `json:"comment,omitempty"`
}

// HARCreator represents the creator of the HAR file
type HARCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Comment string `json:"comment,omitempty"`
}

// HAREntry represents a single request/response pair
type HAREntry struct {
	Pageref         string      `json:"pageref,omitempty"`
	StartedDateTime time.Time   `json:"startedDateTime"`
	Time            int64       `json:"time"` // Time in milliseconds
	Request         HARRequest  `json:"request"`
	Response        HARResponse `json:"response"`
	Cache           interface{} `json:"cache"` // Empty object
	Timings         HARTimings  `json:"timings"`
	ServerIPAddress string      `json:"serverIPAddress,omitempty"`
	Connection      string      `json:"connection,omitempty"`
	Comment         string      `json:"comment,omitempty"`
}

// HARRequest represents the request part of an entry
type HARRequest struct {
	Method      string             `json:"method"`
	URL         string             `json:"url"`
	HTTPVersion string             `json:"httpVersion"`
	Cookies     []HARCookie        `json:"cookies"`
	Headers     []HARNameValuePair `json:"headers"`
	QueryString []HARNameValuePair `json:"queryString"`
	PostData    *HARPostData       `json:"postData,omitempty"`
	HeadersSize int64              `json:"headersSize"`
	BodySize    int64              `json:"bodySize"`
	Comment     string             `json:"comment,omitempty"`
}

// HARResponse represents the response part of an entry
type HARResponse struct {
	Status      int                `json:"status"`
	StatusText  string             `json:"statusText"`
	HTTPVersion string             `json:"httpVersion"`
	Cookies     []HARCookie        `json:"cookies"`
	Headers     []HARNameValuePair `json:"headers"`
	Content     HARContent         `json:"content"`
	RedirectURL string             `json:"redirectURL"`
	HeadersSize int64              `json:"headersSize"`
	BodySize    int64              `json:"bodySize"`
	Comment     string             `json:"comment,omitempty"`
}

// HARNameValuePair represents a key-value pair for headers, query strings, etc.
type HARNameValuePair struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
}

// HARCookie represents a cookie
type HARCookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Path     string    `json:"path,omitempty"`
	Domain   string    `json:"domain,omitempty"`
	Expires  time.Time `json:"expires,omitempty"`
	HTTPOnly bool      `json:"httpOnly,omitempty"`
	Secure   bool      `json:"secure,omitempty"`
	Comment  string    `json:"comment,omitempty"`
}

// HARPostData represents the request body
type HARPostData struct {
	MimeType string             `json:"mimeType"`
	Text     string             `json:"text,omitempty"`
	Params   []HARPostDataParam `json:"params,omitempty"`
	Comment  string             `json:"comment,omitempty"`
}

// HARPostDataParam represents a parameter in post data
type HARPostDataParam struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	FileName    string `json:"fileName,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Comment     string `json:"comment,omitempty"`
}

// HARContent represents the response content
type HARContent struct {
	Size        int64  `json:"size"`
	Compression int64  `json:"compression,omitempty"`
	MimeType    string `json:"mimeType"`
	Text        string `json:"text,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	Comment     string `json:"comment,omitempty"`
}

// HARTimings represents timing information
type HARTimings struct {
	Blocked int64  `json:"blocked,omitempty"`
	DNS     int64  `json:"dns,omitempty"`
	Connect int64  `json:"connect,omitempty"`
	Send    int64  `json:"send"`
	Wait    int64  `json:"wait"`
	Receive int64  `json:"receive"`
	SSL     int64  `json:"ssl,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// HARPage represents a page in the HAR
type HARPage struct {
	StartedDateTime time.Time      `json:"startedDateTime"`
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	PageTimings     HARPageTimings `json:"pageTimings"`
	Comment         string         `json:"comment,omitempty"`
}

// HARPageTimings represents timing for a page
type HARPageTimings struct {
	OnContentLoad int64  `json:"onContentLoad,omitempty"`
	OnLoad        int64  `json:"onLoad,omitempty"`
	Comment       string `json:"comment,omitempty"`
}

// exportRequestsToHAR exports requests to HAR format
func exportRequestsToHAR(requests []RequestLog) (*HAR, error) {
	har := &HAR{
		Log: HARLog{
			Version: "1.2",
			Creator: HARCreator{
				Name:    "dGateway",
				Version: "1.0",
			},
			Entries: make([]HAREntry, len(requests)),
		},
	}

	// Create a single page for all entries
	pageID := uuid.New().String()
	har.Log.Pages = []HARPage{
		{
			StartedDateTime: time.Now(),
			ID:              pageID,
			Title:           "dGateway Export",
			PageTimings:     HARPageTimings{},
		},
	}

	for i, req := range requests {
		// Parse request headers
		var reqHeaders http.Header
		if err := json.Unmarshal([]byte(req.RequestHeaders), &reqHeaders); err != nil {
			return nil, fmt.Errorf("failed to parse request headers for request %d: %v", req.ID, err)
		}

		// Parse response headers
		var respHeaders http.Header
		if err := json.Unmarshal([]byte(req.ResponseHeaders), &respHeaders); err != nil {
			return nil, fmt.Errorf("failed to parse response headers for request %d: %v", req.ID, err)
		}

		// Convert request headers to HAR format
		var harReqHeaders []HARNameValuePair
		for name, values := range reqHeaders {
			for _, value := range values {
				harReqHeaders = append(harReqHeaders, HARNameValuePair{
					Name:  name,
					Value: value,
				})
			}
		}

		// Convert response headers to HAR format
		var harRespHeaders []HARNameValuePair
		for name, values := range respHeaders {
			for _, value := range values {
				harRespHeaders = append(harRespHeaders, HARNameValuePair{
					Name:  name,
					Value: value,
				})
			}
		}

		// Convert query parameters
		var queryString []HARNameValuePair
		if parsedURL, err := url.Parse(req.URL); err == nil {
			queryParams := parsedURL.Query()
			for name, values := range queryParams {
				for _, value := range values {
					queryString = append(queryString, HARNameValuePair{
						Name:  name,
						Value: value,
					})
				}
			}
		}

		// Prepare request post data if exists
		var postData *HARPostData
		if len(req.RequestBody) > 0 {
			mimeType := "application/octet-stream"
			if contentType := reqHeaders.Get("Content-Type"); contentType != "" {
				mimeType = contentType
			}

			postData = &HARPostData{
				MimeType: mimeType,
				Text:     string(req.RequestBody),
			}
		}

		// Prepare response content
		mimeType := "application/octet-stream"
		if contentType := respHeaders.Get("Content-Type"); contentType != "" {
			mimeType = contentType
		}

		content := HARContent{
			Size:     int64(len(req.ResponseBody)),
			MimeType: mimeType,
			Text:     string(req.ResponseBody),
		}

		// Create HAR entry
		entry := HAREntry{
			Pageref:         pageID,
			StartedDateTime: req.Timestamp,
			Time:            0, // We don't have timing information
			Request: HARRequest{
				Method:      req.Method,
				URL:         req.URL,
				HTTPVersion: "HTTP/1.1",
				Cookies:     []HARCookie{}, // We don't track cookies
				Headers:     harReqHeaders,
				QueryString: queryString,
				PostData:    postData,
				HeadersSize: int64(len(req.RequestHeaders)),
				BodySize:    int64(len(req.RequestBody)),
			},
			Response: HARResponse{
				Status:      req.StatusCode,
				StatusText:  http.StatusText(req.StatusCode),
				HTTPVersion: "HTTP/1.1",
				Cookies:     []HARCookie{}, // We don't track cookies
				Headers:     harRespHeaders,
				Content:     content,
				RedirectURL: "",
				HeadersSize: int64(len(req.ResponseHeaders)),
				BodySize:    int64(len(req.ResponseBody)),
			},
			Cache: interface{}(struct{}{}), // Empty cache object
			Timings: HARTimings{
				Send:    0,
				Wait:    0,
				Receive: 0,
			},
		}

		har.Log.Entries[i] = entry
	}

	return har, nil
}
