package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const firebaseMessagingScope = "https://www.googleapis.com/auth/firebase.messaging"
const defaultGoogleTokenURL = "https://oauth2.googleapis.com/token"
const defaultFCMTimeout = 10 * time.Second

type fcmClient struct {
	httpClient *http.Client
	projectID  string
	log        *slog.Logger
}

type fcmNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type fcmMessage struct {
	Message struct {
		Token        string           `json:"token"`
		Notification *fcmNotification `json:"notification,omitempty"`
	} `json:"message"`
}

type fcmErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func newFCMClient(log *slog.Logger) (*fcmClient, error) {
	ctx := context.Background()
	projectID := strings.TrimSpace(os.Getenv("GOOGLE_PROJECT_ID"))
	if projectID == "" {
		projectID = strings.TrimSpace(os.Getenv("FIREBASE_PROJECT_ID"))
	}
	if projectID == "" {
		return nil, fmt.Errorf("missing GOOGLE_PROJECT_ID or FIREBASE_PROJECT_ID")
	}

	credentials, err := loadGoogleCredentials(ctx)
	if err != nil {
		return nil, err
	}

	httpClient := oauth2.NewClient(ctx, credentials.TokenSource)
	httpClient.Timeout = loadPushTimeout()

	return &fcmClient{
		httpClient: httpClient,
		projectID:  projectID,
		log:        log,
	}, nil
}

func (c *fcmClient) ProjectID() string {
	return c.projectID
}

func (c *fcmClient) SendURL() string {
	return fmt.Sprintf(
		"https://fcm.googleapis.com/v1/projects/%s/messages:send",
		c.projectID,
	)
}

func loadGoogleCredentials(ctx context.Context) (*google.Credentials, error) {
	rawJSON := strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON"))
	if rawJSON != "" {
		return google.CredentialsFromJSON(ctx, []byte(rawJSON), firebaseMessagingScope)
	}

	credentialsPath := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	if credentialsPath != "" {
		credentialsJSON, err := os.ReadFile(credentialsPath)
		if err != nil {
			return nil, fmt.Errorf("read GOOGLE_APPLICATION_CREDENTIALS: %w", err)
		}

		return google.CredentialsFromJSON(ctx, credentialsJSON, firebaseMessagingScope)
	}

	serviceAccountJSON, err := buildServiceAccountJSONFromEnv()
	if err != nil {
		return nil, err
	}
	if len(serviceAccountJSON) > 0 {
		return google.CredentialsFromJSON(ctx, serviceAccountJSON, firebaseMessagingScope)
	}

	credentials, err := google.FindDefaultCredentials(ctx, firebaseMessagingScope)
	if err != nil {
		return nil, fmt.Errorf("google credentials not configured: %w", err)
	}

	return credentials, nil
}

func buildServiceAccountJSONFromEnv() ([]byte, error) {
	projectID := strings.TrimSpace(os.Getenv("GOOGLE_PROJECT_ID"))
	clientEmail := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_EMAIL"))
	privateKey := strings.TrimSpace(os.Getenv("GOOGLE_PRIVATE_KEY"))

	if projectID == "" && clientEmail == "" && privateKey == "" {
		return nil, nil
	}

	if projectID == "" || clientEmail == "" || privateKey == "" {
		return nil, fmt.Errorf("GOOGLE_PROJECT_ID, GOOGLE_CLIENT_EMAIL and GOOGLE_PRIVATE_KEY must all be set")
	}

	tokenURL := strings.TrimSpace(os.Getenv("GOOGLE_TOKEN_URL"))
	if tokenURL == "" {
		tokenURL = defaultGoogleTokenURL
	}

	payload := map[string]string{
		"type":         "service_account",
		"project_id":   projectID,
		"client_email": clientEmail,
		"private_key":  normalizePrivateKey(privateKey),
		"token_uri":    tokenURL,
	}

	return json.Marshal(payload)
}

func normalizePrivateKey(privateKey string) string {
	privateKey = strings.Trim(privateKey, "`")
	privateKey = strings.ReplaceAll(privateKey, `\n`, "\n")
	return privateKey
}

func loadPushTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("PUSH_TIMEOUT_SECONDS"))
	if raw == "" {
		return defaultFCMTimeout
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultFCMTimeout
	}

	return time.Duration(seconds) * time.Second
}

// Send delivers a push notification to a single device token.
// Returns (responseText, tokenInvalid, error).
// tokenInvalid is true when FCM reports the token is expired or unregistered —
// the caller should clear the token from the database to avoid future attempts.
func (c *fcmClient) Send(ctx context.Context, deviceToken, title, body string) (string, bool, error) {
	if strings.TrimSpace(deviceToken) == "" {
		return "", false, fmt.Errorf("missing device token")
	}

	msg := fcmMessage{}
	msg.Message.Token = deviceToken
	msg.Message.Notification = &fcmNotification{
		Title: title,
		Body:  body,
	}

	jsonBody, err := json.Marshal(msg)
	if err != nil {
		return "", false, fmt.Errorf("marshal fcm payload: %w", err)
	}

	url := c.SendURL()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", false, fmt.Errorf("create fcm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	c.log.Debug("FCM request", "project", c.projectID, "tokenSuffix", tokenSuffix(deviceToken), "url", url)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("send fcm request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	responseText := strings.TrimSpace(string(respBytes))

	c.log.Debug("FCM response", "project", c.projectID, "tokenSuffix", tokenSuffix(deviceToken), "status", resp.Status, "body", responseText)

	if resp.StatusCode >= 300 {
		tokenInvalid := isFCMTokenInvalid(resp.StatusCode, respBytes)
		return responseText, tokenInvalid, fmt.Errorf("fcm error %s: %s", resp.Status, responseText)
	}

	return responseText, false, nil
}

// isFCMTokenInvalid returns true when the FCM error response indicates the
// device token is permanently invalid and should be removed from the database.
func isFCMTokenInvalid(statusCode int, body []byte) bool {
	// Token errors always come back as 4xx; 5xx are transient server errors.
	if statusCode < 400 || statusCode >= 500 {
		return false
	}

	var fcmErr fcmErrorResponse
	if err := json.Unmarshal(body, &fcmErr); err != nil {
		return false
	}

	switch fcmErr.Error.Status {
	case "NOT_FOUND", "UNREGISTERED":
		return true
	case "INVALID_ARGUMENT":
		// Only flag as invalid when the message explicitly mentions the token.
		msg := strings.ToLower(fcmErr.Error.Message)
		return strings.Contains(msg, "token") || strings.Contains(msg, "registration")
	}

	return false
}
