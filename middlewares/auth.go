package middlewares

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/samber/lo"

	"github.com/epifi/fi-mcp-lite/pkg"
)

var (
	loginRequiredJson = `{"status": "login_required","login_url": "%s","message": "Needs to login first by going to the login url.\nShow the login url as clickable link if client supports it. Otherwise display the URL for users to copy and paste into a browser. \nAsk users to come back and let you know once they are done with login in their browser"}`
	// Global storage for current request session ID
	currentSessionMutex sync.RWMutex
	currentSessionID string
)

type AuthMiddleware struct {
	sessionStore map[string]string
}

func NewAuthMiddleware() *AuthMiddleware {
	return &AuthMiddleware{
		sessionStore: make(map[string]string),
	}
}

func (m *AuthMiddleware) AuthMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Try to get sessionId from HTTP header first (our custom approach)
		sessionId := ""
		if headerSessionId := ctx.Value("http-session-id"); headerSessionId != nil {
			sessionId = headerSessionId.(string)
			log.Printf("[AUTH DEBUG] Using sessionID from HTTP header (context): '%s'", sessionId)
		} else if globalSessionId := GetCurrentSessionID(); globalSessionId != "" {
			sessionId = globalSessionId
			log.Printf("[AUTH DEBUG] Using sessionID from global store: '%s'", sessionId)
		} else {
			// Fallback to MCP context session ID
			sessionId = server.ClientSessionFromContext(ctx).SessionID()
			log.Printf("[AUTH DEBUG] Using sessionID from MCP context: '%s'", sessionId)
		}
		
		log.Printf("[AUTH DEBUG] Available sessions in store: %v", m.sessionStore)
		phoneNumber, ok := m.sessionStore[sessionId]
		if !ok {
			loginUrl := m.getLoginUrl(sessionId)
			return mcp.NewToolResultText(fmt.Sprintf(loginRequiredJson, loginUrl)), nil
		}
		if !lo.Contains(pkg.GetAllowedMobileNumbers(), phoneNumber) {
			return mcp.NewToolResultError("phone number is not allowed"), nil
		}
		ctx = context.WithValue(ctx, "phone_number", phoneNumber)
		toolName := req.Params.Name
		data, readErr := os.ReadFile("test_data_dir/" + phoneNumber + "/" + toolName + ".json")
		if readErr != nil {
			log.Println("error reading test data file", readErr)
			return mcp.NewToolResultError("error reading test data file"), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

// GetLoginUrl fetches dynamic login url for given sessionId
func (m *AuthMiddleware) getLoginUrl(sessionId string) string {
	return fmt.Sprintf("http://localhost:%s/mockWebPage?sessionId=%s", pkg.GetPort(), sessionId)
}

func (m *AuthMiddleware) AddSession(sessionId, phoneNumber string) {
	log.Printf("[AUTH DEBUG] Adding session to store: sessionId='%s', phoneNumber='%s'", sessionId, phoneNumber)
	m.sessionStore[sessionId] = phoneNumber
	log.Printf("[AUTH DEBUG] Session store now contains: %v", m.sessionStore)
}

// SetCurrentSessionID sets the current session ID from HTTP header
func SetCurrentSessionID(sessionId string) {
	currentSessionMutex.Lock()
	defer currentSessionMutex.Unlock()
	currentSessionID = sessionId
	log.Printf("[AUTH DEBUG] Set current session ID to: '%s'", sessionId)
}

// GetCurrentSessionID gets the current session ID
func GetCurrentSessionID() string {
	currentSessionMutex.RLock()
	defer currentSessionMutex.RUnlock()
	return currentSessionID
}
