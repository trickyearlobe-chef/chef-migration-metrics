// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// maxFileContentSize is the maximum file size (1 MB) the content endpoint
// will serve. Files larger than this return 413.
const maxFileContentSize = 1 << 20

// fileEntry is one entry in a repository directory listing. Size is absent for
// a directory, which has none worth reporting.
type fileEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

// fileContentResponse is one file out of a repository clone.
//
// Encoding says how to read content: "text" for anything that is text, or
// "base64" for anything that is not. A caller that ignores it and treats a
// binary file as text gets mojibake rather than an error, so it is always
// sent rather than implied by the content.
type fileContentResponse struct {
	Path     string `json:"path"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
	Size     int    `json:"size"`
}

// handleGitRepoFileTree handles GET /api/v1/git-repos/:name/files?path=
// Returns a directory listing as a JSON array of entries.
func (r *Router) handleGitRepoFileTree(w http.ResponseWriter, req *http.Request, repoName string) {
	if !requireGET(w, req) {
		return
	}

	repoDir, ok := r.resolveRepoDir(w, repoName)
	if !ok {
		return
	}

	relPath := queryString(req, "path", ".")
	targetDir, ok := r.resolveSecurePath(w, repoDir, relPath)
	if !ok {
		return
	}

	info, err := os.Lstat(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			WriteNotFound(w, "Path not found.")
			return
		}
		WriteInternalError(w, "Failed to stat path.")
		return
	}

	if !info.IsDir() {
		WriteBadRequest(w, "Path is not a directory. Use the content endpoint for files.")
		return
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		r.logf("ERROR", "reading dir %s: %v", targetDir, err)
		WriteInternalError(w, "Failed to read directory.")
		return
	}

	result := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if name == ".git" {
			continue
		}
		fe := fileEntry{Name: name}
		if e.IsDir() {
			fe.Type = "dir"
		} else {
			fe.Type = "file"
			if fi, statErr := e.Info(); statErr == nil {
				fe.Size = fi.Size()
			}
		}
		result = append(result, fe)
	}

	WriteJSON(w, http.StatusOK, result)
}

// handleGitRepoFileContent handles GET /api/v1/git-repos/:name/files/content?path=
// Returns file content as JSON with encoding indicator.
func (r *Router) handleGitRepoFileContent(w http.ResponseWriter, req *http.Request, repoName string) {
	if !requireGET(w, req) {
		return
	}

	repoDir, ok := r.resolveRepoDir(w, repoName)
	if !ok {
		return
	}

	relPath := queryString(req, "path", "")
	if relPath == "" || relPath == "." {
		WriteBadRequest(w, "A file path is required.")
		return
	}

	targetFile, ok := r.resolveSecurePath(w, repoDir, relPath)
	if !ok {
		return
	}

	info, err := os.Lstat(targetFile)
	if err != nil {
		if os.IsNotExist(err) {
			WriteNotFound(w, "File not found.")
			return
		}
		WriteInternalError(w, "Failed to stat file.")
		return
	}

	if info.IsDir() {
		WriteBadRequest(w, "Path is a directory. Use the files endpoint for listings.")
		return
	}

	// Reject symlinks.
	if info.Mode()&os.ModeSymlink != 0 {
		WriteBadRequest(w, "Symlinks are not supported.")
		return
	}

	// Reject non-regular files.
	if !info.Mode().IsRegular() {
		WriteBadRequest(w, "Only regular files can be viewed.")
		return
	}

	if info.Size() > maxFileContentSize {
		WriteError(w, http.StatusRequestEntityTooLarge, "file_too_large",
			"File exceeds the 1 MB size limit.")
		return
	}

	f, err := os.Open(targetFile)
	if err != nil {
		r.logf("ERROR", "opening file %s: %v", targetFile, err)
		WriteInternalError(w, "Failed to open file.")
		return
	}
	defer f.Close()

	// Read with capped reader as safety belt.
	data, err := io.ReadAll(io.LimitReader(f, maxFileContentSize+1))
	if err != nil {
		r.logf("ERROR", "reading file %s: %v", targetFile, err)
		WriteInternalError(w, "Failed to read file.")
		return
	}

	if int64(len(data)) > maxFileContentSize {
		WriteError(w, http.StatusRequestEntityTooLarge, "file_too_large",
			"File exceeds the 1 MB size limit.")
		return
	}

	encoding := "text"
	var content string
	if isBinary(data) {
		encoding = "base64"
		content = base64.StdEncoding.EncodeToString(data)
	} else {
		content = string(data)
	}

	WriteJSON(w, http.StatusOK, fileContentResponse{
		Path:     relPath,
		Encoding: encoding,
		Content:  content,
		Size:     len(data),
	})
}

// resolveRepoDir validates the repo name and returns the absolute path to
// its local clone directory. Returns false if validation fails (response
// already written).
func (r *Router) resolveRepoDir(w http.ResponseWriter, repoName string) (string, bool) {
	if r.liveConfig().Storage.GitCookbookDir == "" {
		WriteError(w, http.StatusServiceUnavailable, "no_clone_dir",
			"Git clone directory is not configured.")
		return "", false
	}

	// Prevent path traversal via repo name.
	clean := filepath.Base(repoName)
	if clean == "." || clean == ".." || clean != repoName {
		WriteBadRequest(w, "Invalid repository name.")
		return "", false
	}

	repoDir := filepath.Join(r.liveConfig().Storage.GitCookbookDir, clean)
	if _, err := os.Stat(repoDir); err != nil {
		if os.IsNotExist(err) {
			WriteNotFound(w, "Repository clone not found. The repo may not have been cloned yet.")
			return "", false
		}
		WriteInternalError(w, "Failed to access repository directory.")
		return "", false
	}

	return repoDir, true
}

// resolveSecurePath validates and resolves a relative path within a repo
// directory, preventing path traversal and .git access. Returns false if
// validation fails (response already written).
func (r *Router) resolveSecurePath(w http.ResponseWriter, repoDir, relPath string) (string, bool) {
	// Reject any path containing a .git segment.
	for _, segment := range strings.Split(filepath.ToSlash(relPath), "/") {
		if segment == ".git" {
			WriteBadRequest(w, "Access to .git directory is not allowed.")
			return "", false
		}
	}

	// Clean and resolve the path.
	joined := filepath.Join(repoDir, filepath.Clean(relPath))

	// Ensure the resolved path is within the repo directory.
	rel, err := filepath.Rel(repoDir, joined)
	if err != nil || strings.HasPrefix(rel, "..") {
		WriteBadRequest(w, "Path escapes repository boundary.")
		return "", false
	}

	// Resolve symlinks and re-check containment.
	realRepo, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		WriteInternalError(w, "Failed to resolve repository path.")
		return "", false
	}

	realTarget, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if os.IsNotExist(err) {
			WriteNotFound(w, "Path not found.")
			return "", false
		}
		WriteInternalError(w, "Failed to resolve target path.")
		return "", false
	}

	// After resolving symlinks, verify target is still within the repo.
	finalRel, err := filepath.Rel(realRepo, realTarget)
	if err != nil || strings.HasPrefix(finalRel, "..") {
		WriteBadRequest(w, "Path escapes repository boundary.")
		return "", false
	}

	// Re-check .git segments after symlink resolution.
	for _, segment := range strings.Split(filepath.ToSlash(finalRel), "/") {
		if segment == ".git" {
			WriteBadRequest(w, "Access to .git directory is not allowed.")
			return "", false
		}
	}

	return joined, true
}

// isBinary checks if data appears to be binary by looking for null bytes
// in the first 512 bytes.
func isBinary(data []byte) bool {
	checkLen := 512
	if len(data) < checkLen {
		checkLen = len(data)
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
