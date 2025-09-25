package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LogEntry struct to hold the parsed log data. (Same as before)
type LogEntry struct {
	Timestamp string `json:"timestamp,omitempty"`
	Level     string `json:"level,omitempty"`
	Message   string `json:"message,omitempty"`
	Raw       string `json:"raw,omitempty"`
}

// LogRecord structure for PostgreSQL.
type LogRecord struct {
	Timestamp    time.Time       `json:"timestamp"`
	RemoteAddr   string          `json:"remote_addr"`
	RequestBody  string          `json:"request_body"`
	ResponseBody json.RawMessage `json:"response_body"` // Use RawMessage to save as JSONB
	StatusCode   int             `json:"status_code"`
	ErrorMsg     string          `json:"error_msg"`
}

var dbPool *pgxpool.Pool

// setupDatabase initializes and sets up the PostgreSQL connection pool.
func setupDatabase() {
	var err error
	
	// Read connection parameters from environment variables
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		os.Getenv("POSTGRES_USER"), // User
		os.Getenv("POSTGRES_PASSWORD"), // Password
		"db", // Hostname (the Docker Compose service name)
		5432, // Port
		os.Getenv("POSTGRES_DB_NAME"), // Database name
	)

	// Use context for database setup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}

	// Ping the database to ensure the connection is active
	err = dbPool.Ping(ctx)
	if err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Println("Successfully connected to PostgreSQL.")

	// Create table if it doesn't exist. Using JSONB for efficient JSON storage.
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS delogged (
		id SERIAL PRIMARY KEY,
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
		remote_addr TEXT,
		request_body TEXT,
		response_body JSONB,
		status_code INTEGER,
		error_msg TEXT
	);`

	_, err = dbPool.Exec(ctx, createTableSQL)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	log.Println("Database table 'delogged' ready.")
}

// recordLog inserts a new record into the PostgreSQL database.
func recordLog(record LogRecord) {
	// Use context for database operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	insertSQL := `
	INSERT INTO delogged (timestamp, remote_addr, request_body, response_body, status_code, error_msg) 
	VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := dbPool.Exec(ctx, insertSQL,
		record.Timestamp,
		record.RemoteAddr,
		record.RequestBody,
		record.ResponseBody,
		record.StatusCode,
		record.ErrorMsg,
	)
	if err != nil {
		log.Printf("Failed to insert log record into PostgreSQL: %v", err)
	}
}

// parseHandler handles the /api/parse endpoint.
func parseHandler(w http.ResponseWriter, r *http.Request) {
	record := LogRecord{
		Timestamp:  time.Now(),
		RemoteAddr: r.RemoteAddr,
		StatusCode: http.StatusOK,
	}
	
	// Use a named function for defer to ensure the correct record is captured
	defer func() {
		recordLog(record)
	}()

	log.Printf("Received request from %s for %s %s", r.RemoteAddr, r.Method, r.URL.Path)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		record.StatusCode = http.StatusMethodNotAllowed
		record.ErrorMsg = "Method not allowed"
		log.Printf("Rejected request from %s: Method %s not allowed", r.RemoteAddr, r.Method)
		return
	}

	// Read the request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Could not read request body", http.StatusInternalServerError)
		record.StatusCode = http.StatusInternalServerError
		record.ErrorMsg = "Could not read request body"
		log.Printf("Error reading request body from %s: %v", r.RemoteAddr, err)
		return
	}
	logText := string(body)
	record.RequestBody = logText

	log.Printf("Received log data of size %d bytes", len(logText))

	// Parsing Logic (Unchanged)
	lines := strings.Split(logText, "\n")
	logRegex := regexp.MustCompile(`^\[(.*?)\]\s+\[(.*?)\]\s+(.*)$`)
	var parsedData []LogEntry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		match := logRegex.FindStringSubmatch(line)
		if len(match) == 4 {
			parsedData = append(parsedData, LogEntry{ Timestamp: match[1], Level: match[2], Message: match[3]})
		} else {
			parsedData = append(parsedData, LogEntry{ Raw: line })
		}
	}

	// Marshal the JSON response to save it to the database record.
	responseBody, err := json.Marshal(parsedData)
	if err != nil {
		http.Error(w, "Error creating JSON response", http.StatusInternalServerError)
		record.StatusCode = http.StatusInternalServerError
		record.ErrorMsg = "Error creating JSON response"
		log.Printf("Error marshaling JSON response for %s: %v", r.RemoteAddr, err)
		return
	}
	record.ResponseBody = responseBody // Store the raw byte slice
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Write the JSON response to the client.
	_, err = w.Write(responseBody)
	if err != nil {
		log.Printf("Error writing JSON response for %s: %v", r.RemoteAddr, err)
	}

	log.Printf("Successfully parsed and sent JSON response for request from %s", r.RemoteAddr)
}

// main function to set up the server.
func main() {
	setupDatabase()
	
	log.Println("Starting Go log parser backend...")
	log.Println("Backend service available at port 8001.")

	http.HandleFunc("/api/parse", parseHandler)
	log.Fatal(http.ListenAndServe(":8001", nil))
}