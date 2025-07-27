package main

import (
    "context"
    "fmt"
    "html/template"
    "log"
    "net/http"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"

    "github.com/epifi/fi-mcp-lite/middlewares"
    "github.com/epifi/fi-mcp-lite/pkg"
)

var authMiddleware *middlewares.AuthMiddleware

// Session middleware to extract session ID from HTTP header and inject into context
func sessionMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract session ID from Mcp-Session-Id header
        sessionId := r.Header.Get("Mcp-Session-Id")
        if sessionId != "" {
            // Add session ID to request context
            ctx := context.WithValue(r.Context(), "http-session-id", sessionId)
            r = r.WithContext(ctx)
            // Also store in global variable as backup
            middlewares.SetCurrentSessionID(sessionId)
            log.Printf("[SESSION MIDDLEWARE] Extracted session ID from header: '%s'", sessionId)
        } else {
            log.Printf("[SESSION MIDDLEWARE] No session ID found in header")
        }
        next.ServeHTTP(w, r)
    })
}

// CORS middleware function
func corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        
        // Allow requests from localhost on various ports for development
        allowedOrigins := []string{
            "https://project-artha-frontend.vercel.app",
            "http://127.0.0.1:3000",
        }
        
        // Check if origin is allowed
        originAllowed := false
        for _, allowedOrigin := range allowedOrigins {
            if origin == allowedOrigin {
                originAllowed = true
                break
            }
        }
        
        // Set CORS headers
        if originAllowed {
            w.Header().Set("Access-Control-Allow-Origin", origin)
        } else {
            w.Header().Set("Access-Control-Allow-Origin", "https://project-artha-backup.vercel.app")
        }
        
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Mcp-Session-Id")
        w.Header().Set("Access-Control-Allow-Credentials", "true")
        w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

        // Handle preflight OPTIONS request
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }

        // Continue to next handler
        next.ServeHTTP(w, r)
    })
}

func main() {
    authMiddleware = middlewares.NewAuthMiddleware()
    s := server.NewMCPServer(
        "Hackathon MCP",
        "0.1.0",
        // Notifies clients when new tools gets added or any changes in tools
        server.WithInstructions("A financial portfolio management MCP server that provides secure access to users' financial data through Fi Money, a financial hub for all things money. This MCP server enables users to:\n- Access comprehensive net worth analysis with asset/liability breakdowns\n- Retrieve detailed transaction histories for mutual funds and Employee Provident Fund accounts\n- View credit reports with scores, loan details, and account histories, this also contains user's date of birth that can be used for calculating their age\n\nIf the person asks, you can tell about Fi Money that it is money management platform that offers below services in partnership with regulated entities:\n\nAVAILABLE SERVICES:\n- Digital savings account with zero Forex cards\n- Invest in Indian Mutual funds, US Stocks (partnership with licensed brokers), Smart and Fixed Deposits.\n- Instant Personal Loans \n- Faster UPI and Bank Transfers payments\n- Credit score monitoring and reports\n\nIMPORTANT LIMITATIONS:\n- This MCP server retrieves only actual user data via Net worth tracker and based on consent provided by the user  and does not generate hypothetical or estimated financial information\n- In this version of the MCP server, user's historical bank transactions, historical stocks transaction data, salary (unless categorically declared) is not present. Don't assume these data points for any kind of analysis.\n\nCRITICAL INSTRUCTIONS FOR FINANCIAL DATA:\n\n1. DATA BOUNDARIES: Only provide information that exists in the user's Fi Money Net worth tracker. Never estimate, extrapolate, or generate hypothetical financial data.\n\n2. SPENDING ANALYSIS: If user asks about spending patterns, categories, or analysis tell the user we currently don't offer that data through the MCP:\n   - For detailed spending insights, direct them to: \"For comprehensive spending analysis and categorization, please use the Fi Money mobile app which provides detailed spending insights and budgeting tools.\"\n\n3. MISSING DATA HANDLING: If requested data is not available:\n   - Clearly state what data is missing\n   - Explain how user can connect additional accounts in Fi Money app\n   - Never fill gaps with estimated or generic information\n"),
        server.WithToolCapabilities(true),
        server.WithResourceCapabilities(true, true),
        server.WithLogging(),
        server.WithToolHandlerMiddleware(authMiddleware.AuthMiddleware),
    )

    // Register tools from pkg.ToolList
    for _, tool := range pkg.ToolList {
        s.AddTool(mcp.NewTool(tool.Name, mcp.WithDescription(tool.Description)), dummyHandler)
    }

    // Configure streamable HTTP server with proper endpoints
    httpMux := http.NewServeMux()
    httpMux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
    
    // Create streamable server with session middleware
    streamableServer := server.NewStreamableHTTPServer(s,
        server.WithEndpointPath("/stream"),
    )
    // Apply session middleware to the streamable server
    httpMux.Handle("/mcp/", sessionMiddleware(streamableServer))
    httpMux.HandleFunc("/mockWebPage", webPageHandler)
    httpMux.HandleFunc("/login", loginHandler)
    httpMux.HandleFunc("/health", healthCheckHandler)
    
    port := pkg.GetPort()
    log.Println("starting server on port:", port)
    
    // Wrap the entire httpMux with CORS middleware
    corsHandler := corsMiddleware(httpMux)
    
    if servErr := http.ListenAndServe(fmt.Sprintf(":%s", port), corsHandler); servErr != nil {
        log.Fatalln("error starting server", servErr)
    }
}

// Rest of your functions remain exactly the same...
func dummyHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    return mcp.NewToolResultText("dummy handler"), nil
}

func webPageHandler(w http.ResponseWriter, r *http.Request) {
    sessionId := r.URL.Query().Get("sessionId")
    if sessionId == "" {
        http.Error(w, "sessionId is required", http.StatusBadRequest)
        return
    }

    tmpl, err := template.ParseFiles("static/login.html")
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    data := struct {
        SessionId            string
        AllowedMobileNumbers []string
    }{
        SessionId:            sessionId,
        AllowedMobileNumbers: pkg.GetAllowedMobileNumbers(),
    }

    err = tmpl.Execute(w, data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    sessionId := r.FormValue("sessionId")
    phoneNumber := r.FormValue("phoneNumber")

    log.Printf("Received login request: sessionId=%s, phoneNumber=%s\n", sessionId, phoneNumber)

    if sessionId == "" || phoneNumber == "" {
        http.Error(w, "sessionId and phoneNumber are required", http.StatusBadRequest)
        return
    }

    authMiddleware.AddSession(sessionId, phoneNumber)

    tmpl, err := template.ParseFiles("static/login_successful.html")
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Pass data to template
    data := struct {
        SessionId   string
        PhoneNumber string
    }{
        SessionId:   sessionId,
        PhoneNumber: phoneNumber,
    }

    err = tmpl.Execute(w, data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, `{"status":"healthy","message":"MCP Server is running","timestamp":"%s"}`, 
        fmt.Sprintf("%d", 1735143134)) // Simple timestamp
}
