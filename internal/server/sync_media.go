package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"zakirullin/stuffbot/internal/fs"
)

var syncMediasRequest struct {
	Timestamp     int64  `json:"timestamp"`
	FilenamesHash string `json:"filenamesHash"`
}

type media struct {
	Path         string `json:"path"`
	LastModified int64  `json:"lastModified"`
	Data         string `json:"data"`
}

func SyncMedias(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&syncMediasRequest); err != nil {
		log.Printf("Error parsing syncMediasRequest JSON: %v", err)
		http.Error(w, "Invalid syncMediasRequest JSON", http.StatusBadRequest)
		return
	}

	// TODO ../.. Attacks (fixed with fs.FS?)
	logSync(fmt.Sprintf("Media sync syncMediasRequest for folder: '%s', last sync: %d", fs.DirMedia, syncMediasRequest.Timestamp))

	userFS, err := fs.NewUserFS(userID(r))
	if err != nil {
		log.Printf("Error creating media FS: %v", err)
		http.Error(w, "Error creating media FS", http.StatusInternalServerError)
		return
	}

	// Find media files newer than client's timestamp
	ctimes, err := userFS.Ctimes(fs.DirMedia)
	if err != nil {
		log.Printf("Error getting ctimes for media files: %v", err)
		http.Error(w, "Error getting media file times", http.StatusInternalServerError)
		return
	}

	mediaFiles := make([]media, 0)
	latestTimestamp := int64(0)
	for filename, modTime := range ctimes {
		// TODO theoretically it is possible to miss some files if there were created in the same second.
		if modTime <= syncMediasRequest.Timestamp {
			continue
		}
		if modTime > latestTimestamp {
			latestTimestamp = modTime
		}

		mediaFiles = append(mediaFiles, media{
			Path:         filename,
			LastModified: modTime,
		})
	}

	response := struct {
		Files     []media `json:"files"`
		Timestamp int64   `json:"timestamp"`
	}{
		Files:     mediaFiles,
		Timestamp: latestTimestamp,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding media sync response: %v", err)
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
}

// SyncMedia syncs a single media file by path.
func SyncMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var clientMedia media
	if err := json.NewDecoder(r.Body).Decode(&clientMedia); err != nil {
		log.Printf("Error parsing syncMedia Request JSON: %v", err)
		http.Error(w, "Invalid syncMedia Request JSON", http.StatusBadRequest)
		return
	}

	userFS, err := fs.NewUserFS(userID(r))
	if err != nil {
		log.Printf("Error creating user FS: %v", err)
		http.Error(w, "Error creating user FS", http.StatusInternalServerError)
		return
	}

	exists, err := userFS.Exists(fs.DirMedia, clientMedia.Path)
	if err != nil {
		log.Printf("Error checking if media exists: %v", err)
		http.Error(w, "Error checking media existence", http.StatusInternalServerError)
		return
	}

	shouldWriteToServer := clientMedia.Data != "" && !exists
	if shouldWriteToServer {
		content, err := base64.StdEncoding.DecodeString(clientMedia.Data)
		if err != nil {
			http.Error(w, "Invalid base64 data", http.StatusBadRequest)
			return
		}

		err = userFS.Write(fs.DirMedia, clientMedia.Path, string(content))
		if err != nil {
			http.Error(w, "Invalid base64 data", http.StatusBadRequest)
			return
		}

		logSync(fmt.Sprintf("Media created: %s", clientMedia.Path))
		return
	}

	http.ServeFile(w, r, filepath.Join(StorageDir, fs.DirMedia, clientMedia.Path))
}
