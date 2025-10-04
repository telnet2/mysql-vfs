package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
)

type WebhookResponse struct {
	Veto    bool   `json:"veto,omitempty"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

func main() {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Webhook received request!")

		response := WebhookResponse{
			Veto:    true,
			Message: "Test veto",
			Code:    "TEST",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Call webhook
	payload := map[string]interface{}{
		"event_type": "file.create.authorization.started",
		"payload":    map[string]string{"test": "data"},
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(server.URL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var webhookResp WebhookResponse
	json.NewDecoder(resp.Body).Decode(&webhookResp)

	fmt.Printf("Response: Veto=%v, Message=%s, Code=%s\n", webhookResp.Veto, webhookResp.Message, webhookResp.Code)
}
