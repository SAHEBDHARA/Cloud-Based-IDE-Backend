package handlers

import (
	"io/fs"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetFiles(c *gin.Context) {
	workingDir := "working_dir"
	fileTree, err := GenerateFileTree(workingDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, fileTree)
}

func GenerateFileTree(directory string) (map[string]interface{}, error) {
	tree := make(map[string]interface{})

	err := filepath.WalkDir(directory, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if filePath == directory { // skip the root directory
			return nil
		}

		relPath, err := filepath.Rel(directory, filePath)
		if err != nil {
			return err
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		currentTree := tree

		for _, part := range parts[:len(parts)-1] {
			if _, ok := currentTree[part]; !ok {
				currentTree[part] = make(map[string]interface{})
			}
			currentTree = currentTree[part].(map[string]interface{})
		}

		fileName := parts[len(parts)-1]
		if d.IsDir() {
			currentTree[fileName] = make(map[string]interface{})
		} else {
			currentTree[fileName] = nil
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return tree, nil
}

func GetFileContentHandler(c *gin.Context) {
	// Get the path from the query parameters
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Path is required"})
		return
	}

	// Resolve the path to prevent directory traversal attacks
	workingDir := "working_dir" // Adjust if your working directory is different
	fullPath := filepath.Join(workingDir, path)

	// Read the file content
	content, err := ioutil.ReadFile(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file: " + err.Error()})
		return
	}

	// Write the content back to the response
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, string(content))
}
