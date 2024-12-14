package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// HandleWebSocket handles WebSocket connections and interacts with PTY
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Failed to set WebSocket upgrade: ", err)
		return
	}
	defer conn.Close()

	log.Println("Socket connected", conn.RemoteAddr())
	// serverDir := "." // Replace with the path where your code is hosted
	serverDir, err := os.Getwd()
	if err != nil {
		log.Println("Error getting current working directory:", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Error getting working directory"))
		return
	}
	workingDir := "working_dir"
	workingDirPath := filepath.Join(serverDir, workingDir)

	// Create the working directory if it does not exist
	if err := os.MkdirAll(workingDirPath, os.ModePerm); err != nil {
		log.Println("Failed to create directory:", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to create directory"))
		return
	}

	// Create a new file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("Error creating file watcher:", err)
		return
	}
	defer watcher.Close()

	// Add the working directory to the watcher
	err = watcher.Add(workingDirPath)
	if err != nil {
		log.Println("Error adding directory to watcher:", err)
		return
	}

	// Run bash with the working directory set
	cmd := exec.Command("bash")
	log.Println("working dir", workingDirPath)
	cmd.Dir = workingDirPath
	// cmd := exec.Command("bash", "-c", "cd "+workingDir+" && exec bash")

	// Create pipes for stdin and stdout
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		log.Println("Error creating StdinPipe:", err)
		return
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		log.Println("Error creating StdoutPipe:", err)
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		log.Println("Error creating StderrPipe:", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Println("Error starting command:", err)
		return
	}
	defer cmd.Wait()
	isRunning := true
	// Goroutine to handle PTY output and send it to the WebSocket client
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdoutPipe.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Println("Read error:", err)
				}
				isRunning = false
				break
			}

			if n > 0 {

				currentDir, err := getCurrentDirectory(stdinPipe, stdoutPipe)
				if err != nil {
					log.Println("Error getting current directory:", err)
					currentDir = "Unknown"
				}
				// Send terminal output to WebSocket client
				message := map[string]interface{}{
					"type":       "terminal:data",
					"data":       string(buf[:n]),
					"isRunning":  isRunning,
					"currentDir": currentDir,
				}
				responseJSON, err := json.Marshal(message)
				if err != nil {
					log.Println("Error encoding JSON message:", err)
					continue
				}
				if err := conn.WriteMessage(websocket.TextMessage, responseJSON); err != nil {
					log.Println("WebSocket write error:", err)
					break
				}
			}
		}
	}()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderrPipe.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Println("Read error:", err)
				}
				isRunning = false
				break
			}
			if n > 0 {
				currentDir, err := getCurrentDirectory(stdinPipe, stdoutPipe)
				if err != nil {
					log.Println("Error getting current directory:", err)
					currentDir = "Unknown"
				}
				errorMessage := map[string]interface{}{
					"type":       "terminal:error",
					"message":    string(buf[:n]),
					"isRunning":  isRunning,
					"currentDir": currentDir,
				}
				responseJSON, err := json.Marshal(errorMessage)
				if err != nil {
					log.Println("Error encoding error message:", err)
					continue
				}
				if err := conn.WriteMessage(websocket.TextMessage, responseJSON); err != nil {
					log.Println("WebSocket write error:", err)
					break
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				log.Println("Received file system event:", event)
				var eventType string
				switch event.Op {
				case fsnotify.Create:
					eventType = "created"
				case fsnotify.Remove:
					eventType = "removed"
				case fsnotify.Rename:
					eventType = "renamed"
				case fsnotify.Write:
					eventType = "modified"
				}
				message := map[string]interface{}{
					"type":  "file:event",
					"path":  event.Name,
					"event": eventType,
				}
				responseJSON, err := json.Marshal(message)
				if err != nil {
					log.Println("Error encoding JSON message:", err)
					continue
				}
				if err := conn.WriteMessage(websocket.TextMessage, responseJSON); err != nil {
					log.Println("WebSocket write error we got:", err)
					break
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("Error watching file system:", err)
			}
		}
	}()

	// Handle incoming WebSocket messages
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("WebSocket read error:", err)
			break
		}

		var event map[string]interface{}
		err = json.Unmarshal(msg, &event)
		if err != nil {
			log.Println("Error parsing JSON message:", err)
			continue
		}

		// Safe type assertion with default values
		eventType, ok := event["type"].(string)
		if !ok {
			log.Println("Invalid or missing 'type' in event message")
			continue
		}

		switch eventType {
		case "terminal:write":
			data, ok := event["data"].(string)
			if !ok {
				log.Println("Invalid or missing 'data' in terminal:write message")
				continue
			}
			_, err := stdinPipe.Write([]byte(data + "\n"))
			if err != nil {
				log.Println("Error writing to command stdin:", err)
			}
		case "server:stop":
			port, ok := event["port"].(string)
			if !ok {
				log.Println("Invalid or missing 'port' in server:stop message")
				continue
			}
			if err := StopServerOnPort(port); err != nil {
				conn.WriteMessage(websocket.TextMessage, []byte("Failed to stop server"))
			} else {
				conn.WriteMessage(websocket.TextMessage, []byte("Server stopped successfully"))
			}

		case "file:content":
			filePath, ok := event["path"].(string)
			if !ok {
				log.Println("Invalid or missing 'path' in file:content message")
				continue
			}
			fileContent, ok := event["content"].(string)
			if !ok {
				log.Println("Invalid or missing 'content' in file:content message")
				continue
			}
			fullPath := filepath.Join(workingDirPath, filePath)
			err = ioutil.WriteFile(fullPath, []byte(fileContent), 0644)
			if err != nil {
				log.Println("Error writing file content:", err)
				continue
			}

			// Acknowledge file write success
			ackMessage := map[string]interface{}{
				"type": "file:content:ack",
				"path": filePath,
			}
			responseJSON, err := json.Marshal(ackMessage)
			if err != nil {
				log.Println("Error encoding acknowledgment message:", err)
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, responseJSON); err != nil {
				log.Println("WebSocket write error:", err)
				break
			}

		default:
			log.Printf("Unknown event type: %s\n", eventType)
		}
	}
}

func getCurrentDirectory(stdinPipe io.Writer, stdoutPipe io.Reader) (string, error) {
	// Define the working directory manually
	workingDir := "/c/Users/saheb/OneDrive/Desktop/Myproject/IDE/server/working_dir" // Adjust this path to your specific working_dir path

	// Write the 'pwd' command to bash stdin
	_, err := stdinPipe.Write([]byte("pwd\n"))
	if err != nil {
		return "", err
	}

	// Read the output from stdout
	buf := make([]byte, 1024)
	n, err := stdoutPipe.Read(buf)
	if err != nil {
		return "", err
	}

	// Convert the output to a string and trim any newline characters
	fullPath := string(buf[:n])
	fullPath = filepath.ToSlash(fullPath) // Ensure uniform path separators

	// Find the position of 'working_dir' in the full path
	workingDir = filepath.ToSlash(workingDir) // Ensure uniform path separators
	pos := strings.Index(fullPath, workingDir)

	// If 'working_dir' is found in the path, extract the part after it
	if pos != -1 {
		relativePath := fullPath[pos+len(workingDir):]
		return strings.TrimSpace(relativePath), nil
	}

	// If 'working_dir' is not found, return the full path as-is
	return strings.TrimSpace(fullPath), nil
}

// StopServerOnPort stops a server running on a given port
func StopServerOnPort(port string) error {
	// Find the process ID (PID) listening on the specified port
	findCmd := exec.Command("lsof", "-i", fmt.Sprintf(":%s", port), "-t")
	var out bytes.Buffer
	findCmd.Stdout = &out
	if err := findCmd.Run(); err != nil {
		log.Printf("Error finding process on port %s: %v\n", port, err)
		return err
	}

	// Get the PID from the output
	pid := strings.TrimSpace(out.String())
	if pid == "" {
		log.Printf("No process found on port %s\n", port)
		return fmt.Errorf("no process found on port %s", port)
	}

	// Kill the process by PID
	killCmd := exec.Command("kill", "-9", pid)
	if err := killCmd.Run(); err != nil {
		log.Printf("Error killing process %s: %v\n", pid, err)
		return err
	}

	log.Printf("Server on port %s stopped successfully\n", port)
	return nil
}
