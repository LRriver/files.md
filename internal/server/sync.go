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

// FileInfo represents metadata about a file
type FileInfo struct {
	Path         string    `json:"path"`
	LastModified time.Time `json:"last_modified"`
	IsDirectory  bool      `json:"is_directory"`
	Content      string    `json:"content,omitempty"` // Only filled for sync requests
}

// SyncRequest represents the client's current state
type SyncRequest struct {
	Timestamps map[string]time.Time `json:"timestamps"` // Map of paths to last modified times
}

// SyncResponse is the structure returned when syncing
type SyncResponse struct {
	Files      []FileInfo           `json:"files"`      // Files with content that need syncing
	Timestamps map[string]time.Time `json:"timestamps"` // Current server timestamps
	ServerTime time.Time            `json:"server_time"`
}

// getDirectoryTimestamps recursively scans a directory and returns the latest modification time
// for each directory and file
func getDirectoryTimestamps(rootPath string) (map[string]time.Time, error) {
	timestamps := make(map[string]time.Time)

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

		// Skip non-markdown files unless they're directories
		if !info.IsDir() && !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
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

		// Add this file/directory's timestamp
		timestamps[relPath] = info.ModTime()

		// If it's a directory, we'll also track it for later updates based on content
		if info.IsDir() {
			return nil
		}

		// For files, update the parent directory's timestamp if needed
		dirPath := filepath.Dir(relPath)
		if dirPath == "." {
			dirPath = ""
		}

		if dirTime, exists := timestamps[dirPath]; !exists || info.ModTime().After(dirTime) {
			timestamps[dirPath] = info.ModTime()
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

// HandleGetTimestamps handles the endpoint for getting current timestamps
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
		Timestamps map[string]time.Time `json:"timestamps"`
		ServerTime time.Time            `json:"server_time"`
	}{
		Timestamps: timestamps,
		ServerTime: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding timestamp response: %v", err)
	}
}

// HandleSync processes a bulk sync request
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
	serverTimestamps, err := getDirectoryTimestamps(StorageDir)
	if err != nil {
		log.Printf("Error getting server timestamps: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get timestamps: %v", err), http.StatusInternalServerError)
		return
	}

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
			if clientTime, exists := request.Timestamps[fileName]; exists {
				clientTimestamp = clientTime
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
			if serverTime, exists := serverTimestamps[fileName]; exists {
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

			// Update the server timestamps for this file
			now := time.Now()
			serverTimestamps[fileName] = now

			// Also update parent directory timestamps
			dirPath := filepath.Dir(fileName)
			if dirPath == "." {
				dirPath = ""
			}
			serverTimestamps[dirPath] = now
		}
	}

	// Find files that need to be sent to the client
	filesToSync := make([]FileInfo, 0)

	// Resolve the symlink if needed
	realStorageDir, err := filepath.EvalSymlinks(StorageDir)
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
		clientTime, clientHasFile := request.Timestamps[relPath]
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
				LastModified: info.ModTime(),
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
		Timestamps: serverTimestamps,
		ServerTime: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding sync response: %v", err)
	}
}
