package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Configuration
const (
	StorageDir = "/app/mystorage"           // Base directory for markdown storage
	AuthToken  = "your-really-secret-token" // Replace with your actual token or get from environment
)

// FileInfo represents metadata about a file with Unix timestamp
type FileInfo struct {
	Path         string `json:"path"`
	LastModified int64  `json:"last_modified"` // Unix timestamp
	IsDirectory  bool   `json:"is_directory"`
	Content      string `json:"content,omitempty"` // Only filled for sync requests
}

// SyncRequest represents the client's current state with Unix timestamps
type SyncRequest struct {
	Timestamps map[string]int64 `json:"timestamps"` // Map of paths to last modified times in Unix format
}

// SyncResponse is the structure returned when syncing with Unix timestamps
type SyncResponse struct {
	Files      []FileInfo       `json:"files"`       // Files with content that need syncing
	Timestamps map[string]int64 `json:"timestamps"`  // Current server timestamps in Unix format
	ServerTime int64            `json:"server_time"` // Current server time in Unix format
}

// getDirectoryTimestamps recursively scans a directory and returns the latest modification time
// for directories only (including root directory) as Unix timestamps
func getDirectoryTimestamps(rootPath string) (map[string]int64, error) {
	timestamps := make(map[string]int64)
	timeObjects := make(map[string]time.Time) // Used for comparing times

	// Resolve the symlink if it exists
	realPath, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		log.Printf("Warning: Could not resolve symlink: %v. Using original path.", err)
		realPath = rootPath
	} else {
		log.Printf("Resolved symlink: %s -> %s", rootPath, realPath)
	}

	// Walk through the directory
	err = filepath.Walk(realPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Get the relative path
		relPath, err := filepath.Rel(realPath, path)
		if err != nil {
			return nil
		}

		// Use empty string for the root directory
		if relPath == "." {
			relPath = ""
		}

		// For directories, track their timestamp
		if info.IsDir() {
			modTime := info.ModTime()
			timeObjects[relPath] = modTime
			timestamps[relPath] = modTime.Unix()
			return nil
		}

		// Skip non-markdown files for file processing
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		// For files, only update the parent directory's timestamp if needed
		dirPath := filepath.Dir(relPath)
		if dirPath == "." {
			dirPath = ""
		}

		modTime := info.ModTime()
		if dirTime, exists := timeObjects[dirPath]; !exists || modTime.After(dirTime) {
			timeObjects[dirPath] = modTime
			timestamps[dirPath] = modTime.Unix()
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return timestamps, nil
}

// validateAuthToken checks if the request has a valid auth token
func validateAuthToken(r *http.Request) bool {
	token := r.Header.Get("Authorization")

	// Check for "Bearer " prefix and remove if present
	if strings.HasPrefix(token, "Bearer ") {
		token = token[7:]
	}

	return token == AuthToken
}

// AuthMiddleware wraps a handler and adds token authentication
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateAuthToken(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// Timestamps handles the endpoint for getting current timestamps
func Timestamps(w http.ResponseWriter, r *http.Request) {
	// Auth check is now handled by middleware
	// Get the latest timestamps for all directories and files
	timestamps, err := getDirectoryTimestamps(StorageDir)
	if err != nil {
		log.Printf("Error getting timestamps: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get timestamps: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the timestamps
	response := struct {
		Timestamps map[string]int64 `json:"timestamps"`
		ServerTime int64            `json:"server_time"`
	}{
		Timestamps: timestamps,
		ServerTime: time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding timestamp response: %v", err)
	}
}

// Sync processes a bulk sync request
func Sync(w http.ResponseWriter, r *http.Request) {
	// Auth check is now handled by middleware

	// Only allow POST method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the multipart form with max 32MB memory
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if err != http.ErrNotMultipart {
			log.Printf("Error parsing multipart form: %v", err)
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			return
		}
	}

	// Parse the timestamps from the form
	var request SyncRequest
	timestampsJSON := r.FormValue("timestamps")
	if timestampsJSON == "" {
		// If there's no timestamps field, try to parse the whole body as JSON
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			log.Printf("Error decoding request body: %v", err)
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
	} else {
		// Parse the timestamps JSON
		if err := json.Unmarshal([]byte(timestampsJSON), &request); err != nil {
			log.Printf("Error parsing timestamps JSON: %v", err)
			http.Error(w, "Invalid timestamps JSON", http.StatusBadRequest)
			return
		}
	}

	// Get current server timestamps
	serverTimestampsUnix, err := getDirectoryTimestamps(StorageDir)
	if err != nil {
		log.Printf("Error getting server timestamps: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get timestamps: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert Unix timestamps back to time.Time for internal processing
	serverTimestampsTime := make(map[string]time.Time)

	// Only directory timestamps are in the map, but we need to check file timestamps
	// when processing uploads, so we'll build a more complete map by scanning
	dirTimestamps := make(map[string]time.Time)

	// First convert the directory timestamps we have
	for path, unixTime := range serverTimestampsUnix {
		timeObj := time.Unix(unixTime, 0)
		serverTimestampsTime[path] = timeObj
		dirTimestamps[path] = timeObj
	}

	// Now scan for individual files to get their timestamps
	realStorageDir, _ := filepath.EvalSymlinks(StorageDir)
	filepath.Walk(realStorageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Only process markdown files
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		relPath, err := filepath.Rel(realStorageDir, path)
		if err != nil {
			return nil
		}

		// Store the file's timestamp for conflict detection
		serverTimestampsTime[relPath] = info.ModTime()

		return nil
	})

	// Process any uploaded files if this is a multipart form
	if r.MultipartForm != nil {
		for fileName, fileHeaders := range r.MultipartForm.File {
			// Skip the timestamps field
			if fileName == "timestamps" {
				continue
			}

			if len(fileHeaders) == 0 {
				continue
			}

			// Process the first file (ignore any duplicates)
			file, err := fileHeaders[0].Open()
			if err != nil {
				log.Printf("Error opening uploaded file %s: %v", fileName, err)
				continue
			}

			// Read the file content
			content, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				log.Printf("Error reading uploaded file %s: %v", fileName, err)
				continue
			}

			// Get the client's base timestamp for this file
			clientTimestamp := time.Time{}
			if clientUnixTime, exists := request.Timestamps[fileName]; exists {
				clientTimestamp = time.Unix(clientUnixTime, 0)
			}

			// Check if we need to handle a conflict
			needsMerge := false
			existingContent := ""

			// Resolve the symlink path
			realStorageDir, err := filepath.EvalSymlinks(StorageDir)
			if err != nil {
				realStorageDir = StorageDir
			}

			// Check if the file exists on the server and has been modified
			localPath := filepath.Join(realStorageDir, fileName)
			if serverTime, exists := serverTimestampsTime[fileName]; exists {
				if !clientTimestamp.IsZero() && serverTime.After(clientTimestamp) {
					// File exists and has been modified since the client's version
					needsMerge = true

					// Read the existing content
					existingBytes, err := os.ReadFile(localPath)
					if err == nil {
						existingContent = string(existingBytes)
					}
				}
			}

			// Apply merge strategy if needed
			finalContent := content
			if needsMerge {
				// Simple merge: append client content after server content with a conflict marker
				mergedContent := fmt.Sprintf("%s\n\n==== CONFLICT (Server changes above, Client changes below) ====\n\n%s",
					existingContent, string(content))
				finalContent = []byte(mergedContent)
			}

			// Ensure the directory exists
			dir := filepath.Dir(localPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Printf("Error creating directory for %s: %v", fileName, err)
				continue
			}

			// Write the file
			if err := os.WriteFile(localPath, finalContent, 0644); err != nil {
				log.Printf("Error writing file %s: %v", fileName, err)
				continue
			}

			// Update the file timestamp in our internal map
			now := time.Now()
			serverTimestampsTime[fileName] = now

			// Only update the directory timestamp in the response map
			dirPath := filepath.Dir(fileName)
			if dirPath == "." {
				dirPath = ""
			}
			serverTimestampsTime[dirPath] = now
			serverTimestampsUnix[dirPath] = now.Unix()
		}
	}

	// Find files that need to be sent to the client
	filesToSync := make([]FileInfo, 0)

	// Resolve the symlink if needed
	realStorageDir, err = filepath.EvalSymlinks(StorageDir)
	if err != nil {
		log.Printf("Warning: Could not resolve symlink: %v. Using original path.", err)
		realStorageDir = StorageDir
	}

	// Walk through all server files
	err = filepath.Walk(realStorageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Skip directories and non-markdown files
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		// Get the relative path
		relPath, err := filepath.Rel(realStorageDir, path)
		if err != nil {
			return nil
		}

		// Check if the client needs this file
		clientUnixTime, clientHasFile := request.Timestamps[relPath]
		clientTime := time.Time{}
		if clientHasFile {
			clientTime = time.Unix(clientUnixTime, 0)
		}

		if !clientHasFile || info.ModTime().After(clientTime) {
			// Client doesn't have the file or has an older version
			// Read the file content
			content, err := os.ReadFile(path)
			if err != nil {
				log.Printf("Error reading file %s: %v", relPath, err)
				return nil
			}

			// Add the file to the response
			filesToSync = append(filesToSync, FileInfo{
				Path:         relPath,
				LastModified: info.ModTime().Unix(),
				IsDirectory:  false,
				Content:      string(content),
			})
		}

		return nil
	})

	if err != nil {
		log.Printf("Error scanning files: %v", err)
		http.Error(w, fmt.Sprintf("Error scanning files: %v", err), http.StatusInternalServerError)
		return
	}

	// Build and send the response
	response := SyncResponse{
		Files:      filesToSync,
		Timestamps: serverTimestampsUnix,
		ServerTime: time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding sync response: %v", err)
	}
}
