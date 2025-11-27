package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Message struct {
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

func main() {
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/users", handleUsers)
	http.HandleFunc("/user/", handleUser)
	http.HandleFunc("/message", handleMessage)
	http.HandleFunc("/health", handleHealth)

	fmt.Println("Test HTTP server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Hello from test HTTP server!\nTime: %s\n", time.Now().Format(time.RFC3339))
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
		{ID: 3, Name: "Charlie"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(users)
}

func handleUser(w http.ResponseWriter, r *http.Request) {
	user := User{
		ID:   42,
		Name: "Test User",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(user)
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
	// Simulate some processing time
	time.Sleep(50 * time.Millisecond)

	msg := Message{
		Text:      r.URL.Query().Get("text"),
		Timestamp: time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(msg)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

