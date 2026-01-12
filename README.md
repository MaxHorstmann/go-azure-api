# go-azure-api
Experimental REST API to query Azure resources, written in Go

## Overview

This is a simple REST API that calls Azure Management (ARM) endpoints to retrieve information about Azure resources.

## Features

- List all resource groups in an Azure subscription
- Uses Azure SDK for Go v2
- Supports multiple authentication methods via `DefaultAzureCredential`

## Prerequisites

- Go 1.21 or later
- Azure subscription
- Azure CLI installed and authenticated, OR service principal credentials

## Setup

1. Install dependencies:
```bash
go mod download
```

2. Set up authentication:
   - **Option 1 (Recommended for local development):** Sign in with Azure CLI:
     ```bash
     az login
     ```
   
   - **Option 2:** Set up service principal environment variables (see `.env.example`)

3. Set your Azure subscription ID:
```bash
export AZURE_SUBSCRIPTION_ID="your-subscription-id-here"
```

You can find your subscription ID by running:
```bash
az account show --query id -o tsv
```

## Running the API

```bash
go run main.go
```

The server will start on port 8080 by default.

## API Endpoints

### GET /
Returns API information and available endpoints.

```bash
curl http://localhost:8080/
```

### GET /resource-groups
Returns all resource groups in the configured subscription.

```bash
curl http://localhost:8080/resource-groups
```

Example response:
```json
{
  "subscription_id": "12345678-1234-1234-1234-123456789abc",
  "count": 2,
  "resource_groups": [
    {
      "id": "/subscriptions/12345678-1234-1234-1234-123456789abc/resourceGroups/my-rg",
      "location": "eastus",
      "name": "my-rg",
      "tags": {
        "environment": "dev"
      }
    }
  ]
}
```

### GET /health
Health check endpoint.

```bash
curl http://localhost:8080/health
```

## Environment Variables

- `AZURE_SUBSCRIPTION_ID` (required): Your Azure subscription ID
- `PORT` (optional): Server port, defaults to 8080
- `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` (optional): For service principal authentication

## Building

```bash
go build -o go-azure-api
./go-azure-api
```

## Authentication Methods

The application uses `DefaultAzureCredential` which attempts authentication in the following order:
1. Environment variables (service principal)
2. Workload identity (for Azure Kubernetes Service)
3. Managed identity (for Azure VMs, App Service, etc.)
4. Azure CLI credentials
5. Azure PowerShell credentials

For local development, using Azure CLI (`az login`) is the easiest method.
