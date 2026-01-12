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
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

type Server struct {
	subscriptionID string
	credential     *azidentity.DefaultAzureCredential
}

func NewServer() (*Server, error) {
	// Create a default Azure credential (supports Azure CLI, Managed Identity, etc.)
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	if subscriptionID == "" {
		// If not specified, get the default subscription
		log.Println("AZURE_SUBSCRIPTION_ID not set, fetching default subscription...")
		subscriptionID, err = getDefaultSubscription(cred)
		if err != nil {
			return nil, fmt.Errorf("failed to get default subscription: %w", err)
		}
		log.Printf("Using default subscription: %s", subscriptionID)
	}

	return &Server{
		subscriptionID: subscriptionID,
		credential:     cred,
	}, nil
}

func getDefaultSubscription(cred *azidentity.DefaultAzureCredential) (string, error) {
	ctx := context.Background()
	client, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	// Get the first available subscription (typically the default)
	pager := client.NewListPager(nil)
	if pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list subscriptions: %w", err)
		}

		if len(page.Value) > 0 {
			// Find the default subscription (IsDefault == true) or use the first one
			for _, sub := range page.Value {
				if sub.State != nil && *sub.State == armsubscriptions.SubscriptionStateEnabled {
					if sub.SubscriptionID != nil {
						return *sub.SubscriptionID, nil
					}
				}
			}
			// Fallback to first subscription if no enabled one found
			if page.Value[0].SubscriptionID != nil {
				return *page.Value[0].SubscriptionID, nil
			}
		}
	}

	return "", fmt.Errorf("no subscriptions found")
}

func (s *Server) handleResourceGroup(w http.ResponseWriter, r *http.Request, name string) {
	ctx := context.Background()
	client, err := armresources.NewResourceGroupsClient(s.subscriptionID, s.credential, nil)
	if err != nil {
		log.Printf("Failed to create resource groups client: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get specific resource group
		resp, err := client.Get(ctx, name, nil)
		if err != nil {
			log.Printf("Failed to get resource group: %v", err)
			http.Error(w, "Resource group not found", http.StatusNotFound)
			return
		}

		rgData := map[string]interface{}{
			"name":     *resp.Name,
			"location": *resp.Location,
			"id":       *resp.ID,
		}
		if resp.Tags != nil {
			rgData["tags"] = resp.Tags
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rgData)

	case http.MethodPut:
		// Create or update resource group
		var req struct {
			Location string            `json:"location"`
			Tags     map[string]string `json:"tags,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Location == "" {
			http.Error(w, "location is required", http.StatusBadRequest)
			return
		}

		// Convert tags to Azure format
		var tags map[string]*string
		if len(req.Tags) > 0 {
			tags = make(map[string]*string)
			for k, v := range req.Tags {
				val := v
				tags[k] = &val
			}
		}

		params := armresources.ResourceGroup{
			Location: &req.Location,
			Tags:     tags,
		}

		resp, err := client.CreateOrUpdate(ctx, name, params, nil)
		if err != nil {
			log.Printf("Failed to create/update resource group: %v", err)
			http.Error(w, "Failed to create resource group", http.StatusInternalServerError)
			return
		}

		rgData := map[string]interface{}{
			"name":     *resp.Name,
			"location": *resp.Location,
			"id":       *resp.ID,
		}
		if resp.Tags != nil {
			rgData["tags"] = resp.Tags
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rgData)

	case http.MethodDelete:
		// Delete resource group
		poller, err := client.BeginDelete(ctx, name, nil)
		if err != nil {
			log.Printf("Failed to delete resource group: %v", err)
			http.Error(w, "Failed to delete resource group", http.StatusInternalServerError)
			return
		}

		// Wait for deletion to complete (optional - you could return immediately)
		_, err = poller.PollUntilDone(ctx, nil)
		if err != nil {
			log.Printf("Failed to complete deletion: %v", err)
			http.Error(w, "Failed to complete deletion", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleResourceGroups(w http.ResponseWriter, r *http.Request) {
	// Check if this is a request for a specific resource group
	path := r.URL.Path
	if len(path) > len("/resource-groups/") && path[:len("/resource-groups/")] == "/resource-groups/" {
		// Extract resource group name from path
		rgName := path[len("/resource-groups/"):]
		s.handleResourceGroup(w, r, rgName)
		return
	}

	// Handle list all resource groups
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
	http.HandleFunc("/resource-groups/", server.handleResourceGroups)
	http.HandleFunc("/health", server.handleHealth)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Azure Resource Groups API",
			"endpoints": []string{
				"GET /resource-groups - List all resource groups",
				"GET /resource-groups/{name} - Get specific resource group",
				"PUT /resource-groups/{name} - Create or update resource group",
				"DELETE /resource-groups/{name} - Delete resource group",
				"GET /health - Health check",
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
