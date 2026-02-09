package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	//"strconv"
	"strings"
	"syscall"
	"time"
)

// MCP Protocol types
type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool definitions
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type ToolResult struct {
	Content []ContentItem `json:"content"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// GraphQL types
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type GraphQLResponse struct {
	Data   json.RawMessage   `json:"data"`
	Errors []GraphQLError    `json:"errors,omitempty"`
}

type GraphQLError struct {
	Message string `json:"message"`
}

// LitmusChaos configuration
type LitmusConfig struct {
	ChaoscenterEndpoint    string
	ProjectID              string
	AccessToken            string
	DefaultInfraID         string
	DefaultEnvironmentID   string
}

// Server struct
type LitmusChaosServer struct {
	config     *LitmusConfig
	httpClient *http.Client
}

// Initialize server
func NewLitmusChaosServer() *LitmusChaosServer {
	config := &LitmusConfig{
		ChaoscenterEndpoint:  getEnvOrDefault("CHAOS_CENTER_ENDPOINT", "http://localhost:8080"),
		ProjectID:            os.Getenv("LITMUS_PROJECT_ID"),
		AccessToken:          os.Getenv("LITMUS_ACCESS_TOKEN"),
		DefaultInfraID:       os.Getenv("DEFAULT_INFRA_ID"),
		DefaultEnvironmentID: getEnvOrDefault("DEFAULT_ENVIRONMENT_ID", "production"),
	}

	if config.ProjectID == "" {
		log.Fatal("LITMUS_PROJECT_ID environment variable is required")
	}

	return &LitmusChaosServer{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GraphQL request helper
func (s *LitmusChaosServer) graphqlRequest(ctx context.Context, query string, variables map[string]interface{}) (json.RawMessage, error) {
	if variables == nil {
		variables = make(map[string]interface{})
	}
	variables["projectID"] = s.config.ProjectID

	reqBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/query", s.config.ChaoscenterEndpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.config.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.config.AccessToken))
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var gqlResp GraphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		messages := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			messages[i] = e.Message
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(messages, ", "))
	}

	return gqlResp.Data, nil
}

// Tool definitions
func (s *LitmusChaosServer) getTools() []Tool {
	return []Tool{
		{
			Name:        "list_chaos_experiments",
			Description: "List all chaos experiments with optional filtering",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filter": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"experimentName": map[string]interface{}{"type": "string", "description": "Filter by experiment name"},
							"infraName":      map[string]interface{}{"type": "string", "description": "Filter by infrastructure name"},
							"infraId":        map[string]interface{}{"type": "string", "description": "Filter by infrastructure ID"},
							"status":         map[string]interface{}{"type": "string", "description": "Filter by experiment status"},
						},
					},
					"pagination": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"page":  map[string]interface{}{"type": "number", "minimum": 0},
							"limit": map[string]interface{}{"type": "number", "minimum": 1, "maximum": 100},
						},
					},
				},
			},
		},
		{
			Name:        "get_chaos_experiment",
			Description: "Get detailed information about a specific chaos experiment",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"experimentId": map[string]interface{}{"type": "string", "description": "Unique experiment identifier"},
					"includeRuns":  map[string]interface{}{"type": "boolean", "description": "Include recent experiment runs"},
				},
				"required": []string{"experimentId"},
			},
		},
		//{
			//Name:        "create_chaos_experiment",
			//Description: "Create a new chaos experiment using Chaos Studio",
			//InputSchema: map[string]interface{}{
				//"type": "object",
				//"properties": map[string]interface{}{
					//"name":        map[string]interface{}{"type": "string", "description": "Experiment name"},
					//"description": map[string]interface{}{"type": "string", "description": "Experiment description"},
					//"infraId":     map[string]interface{}{"type": "string", "description": "Infrastructure ID to run the experiment"},
					//"faults": map[string]interface{}{
						//"type": "array",
						//"items": map[string]interface{}{
							//"type": "object",
							//"properties": map[string]interface{}{
								//"name":       map[string]interface{}{"type": "string", "description": "Fault name (e.g., pod-delete, network-loss)"},
								//"weight":     map[string]interface{}{"type": "number", "minimum": 1, "maximum": 10, "description": "Fault weight for scoring"},
								//"targetApp":  map[string]interface{}{"type": "string", "description": "Target application selector"},
								//"duration":   map[string]interface{}{"type": "string", "description": "Fault duration (e.g., 60s, 5m)"},
								//"parameters": map[string]interface{}{"type": "object", "description": "Fault-specific parameters"},
							//},
							//"required": []string{"name", "targetApp"},
						//},
					//},
					//"schedule": map[string]interface{}{
						//"type": "object",
						//"properties": map[string]interface{}{
							//"cronExpression": map[string]interface{}{"type": "string", "description": "Cron expression for scheduling"},
						//},
					//},
					//"tags": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Experiment tags"},
				//},
				//"required": []string{"name", "infraId", "faults"},
			//},
		//},
		{
			Name:        "run_chaos_experiment",
			Description: "Execute a chaos experiment immediately",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"experimentId": map[string]interface{}{"type": "string", "description": "Experiment ID to run"},
				},
				"required": []string{"experimentId"},
			},
		},
		{
			Name:        "stop_chaos_experiment",
			Description: "Stop a running chaos experiment",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"experimentId":    map[string]interface{}{"type": "string", "description": "Experiment ID to stop"},
					"experimentRunId": map[string]interface{}{"type": "string", "description": "Specific run ID to stop (optional)"},
				},
				"required": []string{"experimentId"},
			},
		},
		{
			Name:        "list_experiment_runs",
			Description: "List experiment runs with detailed execution history",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"experimentId": map[string]interface{}{"type": "string", "description": "Filter by specific experiment"},
					"status": map[string]interface{}{
						"type": "string",
						"enum": []string{"Running", "Completed", "Failed", "Stopped", "Queued"},
						"description": "Filter by run status",
					},
					"limit": map[string]interface{}{"type": "number", "minimum": 1, "maximum": 50, "description": "Number of runs to return"},
				},
			},
		},
		{
			Name:        "get_experiment_run_details",
			Description: "Get detailed information about a specific experiment run",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"experimentRunId": map[string]interface{}{"type": "string", "description": "Experiment run ID"},
					"includeLogs":     map[string]interface{}{"type": "boolean", "description": "Include execution logs"},
				},
				"required": []string{"experimentRunId"},
			},
		},
		{
			Name:        "list_chaos_infrastructures",
			Description: "List all chaos infrastructures (formerly agents/delegates)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"environmentId": map[string]interface{}{"type": "string", "description": "Filter by environment"},
					"status": map[string]interface{}{
						"type": "string",
						"enum": []string{"Active", "Inactive"},
						"description": "Filter by infrastructure status",
					},
				},
			},
		},
		{
			Name:        "get_infrastructure_details",
			Description: "Get detailed information about a chaos infrastructure",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"infraId":         map[string]interface{}{"type": "string", "description": "Infrastructure ID"},
					"includeManifest": map[string]interface{}{"type": "boolean", "description": "Include installation manifest"},
				},
				"required": []string{"infraId"},
			},
		},
		{
			Name:        "list_environments",
			Description: "List all environments for organizing chaos infrastructures",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{
						"type": "string",
						"enum": []string{"PROD", "NON_PROD"},
						"description": "Filter by environment type",
					},
				},
			},
		},
		{
			Name:        "create_environment",
			Description: "Create a new environment for organizing chaos infrastructures",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string", "description": "Environment name"},
					"description": map[string]interface{}{"type": "string", "description": "Environment description"},
					"type": map[string]interface{}{
						"type": "string",
						"enum": []string{"PROD", "NON_PROD"},
						"description": "Environment type",
					},
					"tags": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Environment tags"},
				},
				"required": []string{"name", "type"},
			},
		},
		{
			Name:        "list_resilience_probes",
			Description: "List all resilience probes with plug-and-play architecture",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{
						"type": "string",
						"enum": []string{"httpProbe", "cmdProbe", "k8sProbe", "promProbe"},
						"description": "Filter by probe type",
					},
				},
			},
		},
		{
			Name:        "create_resilience_probe",
			Description: "Create a new resilience probe for steady-state validation",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string", "description": "Probe name"},
					"description": map[string]interface{}{"type": "string", "description": "Probe description"},
					"type": map[string]interface{}{
						"type": "string",
						"enum": []string{"httpProbe", "cmdProbe", "k8sProbe", "promProbe"},
						"description": "Probe type",
					},
					"properties": map[string]interface{}{
						"type":        "object",
						"description": "Probe-specific configuration properties",
						"properties": map[string]interface{}{
							"url":      map[string]interface{}{"type": "string", "description": "HTTP URL to probe (for httpProbe)"},
							"method":   map[string]interface{}{"type": "string", "enum": []string{"GET", "POST"}, "description": "HTTP method"},
							"command":  map[string]interface{}{"type": "string", "description": "Command to execute (for cmdProbe)"},
							"resource": map[string]interface{}{"type": "string", "description": "Kubernetes resource type (for k8sProbe)"},
							"endpoint": map[string]interface{}{"type": "string", "description": "Prometheus endpoint (for promProbe)"},
							"query":    map[string]interface{}{"type": "string", "description": "PromQL query"},
							"timeout":  map[string]interface{}{"type": "string", "description": "Probe timeout (e.g., 5s)"},
							"interval": map[string]interface{}{"type": "string", "description": "Probe interval (e.g., 2s)"},
						},
					},
					"tags": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Probe tags"},
				},
				"required": []string{"name", "type", "properties"},
			},
		},
		{
			Name:        "list_chaos_hubs",
			Description: "List all ChaosHubs (experiment repositories)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"hubType": map[string]interface{}{
						"type": "string",
						"enum": []string{"GIT", "REMOTE"},
						"description": "Filter by hub type",
					},
				},
			},
		},
		{
			Name:        "get_chaos_faults",
			Description: "Get available chaos faults from ChaosHub",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"hubId":    map[string]interface{}{"type": "string", "description": "ChaosHub ID"},
					"category": map[string]interface{}{"type": "string", "description": "Fault category (e.g., pod, node, network)"},
				},
				"required": []string{"hubId"},
			},
		},
		{
			Name:        "get_experiment_statistics",
			Description: "Get comprehensive experiment and infrastructure statistics",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"includeResiliencyScores": map[string]interface{}{"type": "boolean", "description": "Include resiliency score distribution"},
				},
			},
		},
		{
			Name:        "register_chaos_infrastructure",
			Description: "Register a new chaos infrastructure",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":           map[string]interface{}{"type": "string", "description": "Infrastructure name"},
					"description":    map[string]interface{}{"type": "string", "description": "Infrastructure description"},
					"environmentId":  map[string]interface{}{"type": "string", "description": "Environment ID"},
					"platformName":   map[string]interface{}{"type": "string", "description": "Platform name (e.g., GKE, EKS, AKS)"},
					"infraScope":     map[string]interface{}{"type": "string", "enum": []string{"namespace", "cluster"}, "description": "Infrastructure scope"},
					"infraNamespace": map[string]interface{}{"type": "string", "description": "Kubernetes namespace for infra components"},
					"tags":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Infrastructure tags"},
				},
				"required": []string{"name", "environmentId", "infraScope"},
			},
		},
	}
}

// Tool execution handlers
func (s *LitmusChaosServer) handleTool(ctx context.Context, toolName string, arguments json.RawMessage) (*ToolResult, error) {
	var args map[string]interface{}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, fmt.Errorf("failed to parse arguments: %w", err)
		}
	}

	switch toolName {
	case "list_chaos_experiments":
		return s.listChaosExperiments(ctx, args)
	case "get_chaos_experiment":
		return s.getChaosExperiment(ctx, args)
	//case "create_chaos_experiment":
		//return s.createChaosExperiment(ctx, args)
	case "run_chaos_experiment":
		return s.runChaosExperiment(ctx, args)
	case "stop_chaos_experiment":
		return s.stopChaosExperiment(ctx, args)
	case "list_experiment_runs":
		return s.listExperimentRuns(ctx, args)
	case "get_experiment_run_details":
		return s.getExperimentRunDetails(ctx, args)
	case "list_chaos_infrastructures":
		return s.listChaosInfrastructures(ctx, args)
	case "get_infrastructure_details":
		return s.getInfrastructureDetails(ctx, args)
	case "list_environments":
		return s.listEnvironments(ctx, args)
	case "create_environment":
		return s.createEnvironment(ctx, args)
	case "list_resilience_probes":
		return s.listResilienceProbes(ctx, args)
	case "create_resilience_probe":
		return s.createResilienceProbe(ctx, args)
	case "list_chaos_hubs":
		return s.listChaosHubs(ctx, args)
	case "get_chaos_faults":
		return s.getChaosFaults(ctx, args)
	case "get_experiment_statistics":
		return s.getExperimentStatistics(ctx, args)
	case "register_chaos_infrastructure":
		return s.registerChaosInfrastructure(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// MCP Protocol handlers
func (s *LitmusChaosServer) handleListTools() interface{} {
	return map[string]interface{}{
		"tools": s.getTools(),
	}
}

func (s *LitmusChaosServer) handleCallTool(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var callParams struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, fmt.Errorf("failed to parse call tool params: %w", err)
	}

	result, err := s.handleTool(ctx, callParams.Name, callParams.Arguments)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *LitmusChaosServer) handleInitialize() interface{} {
	return map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "litmuschaos-mcp-server",
			"version": "3.16.0",
		},
	}
}

// Main request handler
func (s *LitmusChaosServer) handleRequest(ctx context.Context, req *MCPRequest) *MCPResponse {
	resp := &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		resp.Result = s.handleInitialize()
	case "notifications/initialized", "initialized":
		// Notification - no response should be sent
		return nil
	case "tools/list":
		resp.Result = s.handleListTools()
	case "tools/call":
		result, err := s.handleCallTool(ctx, req.Params)
		if err != nil {
			resp.Error = &MCPError{
				Code:    -32603,
				Message: err.Error(),
			}
		} else {
			resp.Result = result
		}
	default:
		// If the method starts with "notifications/", it's a notification - ignore it
		if strings.HasPrefix(req.Method, "notifications/") {
			return nil
		}
		resp.Error = &MCPError{
			Code:    -32601,
			Message: fmt.Sprintf("Method not found: %s", req.Method),
		}
	}

	return resp
}

// Main server loop
func (s *LitmusChaosServer) run() error {
	scanner := bufio.NewScanner(os.Stdin)
	
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req MCPRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			log.Printf("Failed to parse request: %v", err)
			continue
		}

		ctx := context.Background()
		resp := s.handleRequest(ctx, &req)
		
		// Only send response for requests (messages with an id).
		// Notifications (no id) must never receive a response per JSON-RPC 2.0 spec.
		if resp != nil && req.ID != nil {
			respJSON, err := json.Marshal(resp)
			if err != nil {
				log.Printf("Failed to marshal response: %v", err)
				continue
			}
			
			fmt.Println(string(respJSON))
		}
	}

	return scanner.Err()
}

func main() {
	server := NewLitmusChaosServer()

	// Setup graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("Received shutdown signal, shutting down gracefully...")
		os.Exit(0)
	}()

	log.Printf("LitmusChaos MCP server v3.16.0 running on stdio")
	log.Printf("Connected to Chaos Center: %s", server.config.ChaoscenterEndpoint)
	log.Printf("Project ID: %s", server.config.ProjectID)

	if err := server.run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
