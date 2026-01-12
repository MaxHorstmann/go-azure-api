package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

type Server struct {
	subscriptionID string
	credential     *azidentity.DefaultAzureCredential
}

func NewServer() (*Server, error) {
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	if subscriptionID == "" {
		return nil, fmt.Errorf("AZURE_SUBSCRIPTION_ID environment variable is required")
	}

	// Create a default Azure credential (supports Azure CLI, Managed Identity, etc.)
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	return &Server{
		subscriptionID: subscriptionID,
		credential:     cred,
	}, nil
}

func (s *Server) handleResourceGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()

	// Create a resource groups client
	client, err := armresources.NewResourceGroupsClient(s.subscriptionID, s.credential, nil)
	if err != nil {
		log.Printf("Failed to create resource groups client: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// List all resource groups
	var resourceGroups []map[string]interface{}
	pager := client.NewListPager(nil)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			log.Printf("Failed to get resource groups: %v", err)
			http.Error(w, "Failed to retrieve resource groups", http.StatusInternalServerError)
			return
		}

		for _, rg := range page.Value {
			rgData := map[string]interface{}{
				"name":     *rg.Name,
				"location": *rg.Location,
				"id":       *rg.ID,
			}
			if rg.Tags != nil {
				rgData["tags"] = rg.Tags
			}
			resourceGroups = append(resourceGroups, rgData)
		}
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"subscription_id": s.subscriptionID,
		"count":           len(resourceGroups),
		"resource_groups": resourceGroups,
	}); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

func main() {
	server, err := NewServer()
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Setup routes
	http.HandleFunc("/resource-groups", server.handleResourceGroups)
	http.HandleFunc("/health", server.handleHealth)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Azure Resource Groups API",
			"endpoints": []string{
				"/resource-groups - GET all resource groups",
				"/health - Health check",
			},
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s...", port)
	log.Printf("Using subscription: %s", server.subscriptionID)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
