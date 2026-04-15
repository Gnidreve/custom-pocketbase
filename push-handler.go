package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const firebaseMessagingScope = "https://www.googleapis.com/auth/firebase.messaging"
const defaultGoogleTokenURL = "https://oauth2.googleapis.com/token"

type fcmClient struct {
	httpClient *http.Client
	projectID  string
}

type fcmMessage struct {
	Message struct {
		Token        string            `json:"token"`
		Notification map[string]string `json:"notification,omitempty"`
	} `json:"message"`
}

func newPushClientFromEnv() (*fcmClient, error) {
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

	return &fcmClient{
		httpClient: oauth2.NewClient(ctx, credentials.TokenSource),
		projectID:  projectID,
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

func (c *fcmClient) Send(ctx context.Context, deviceToken, title, body string) (string, error) {
	if strings.TrimSpace(deviceToken) == "" {
		return "", fmt.Errorf("missing device token")
	}

	msg := fcmMessage{}
	msg.Message.Token = deviceToken
	msg.Message.Notification = map[string]string{
		"title": title,
		"body":  body,
	}

	jsonBody, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshal fcm payload: %w", err)
	}

	url := c.SendURL()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create fcm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	fmt.Printf("FCM request: project=%s tokenSuffix=%s url=%s payload=%s\n", c.projectID, tokenSuffix(deviceToken), url, string(jsonBody))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send fcm request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(resp.Body)
	responseText := strings.TrimSpace(string(responseBody))

	fmt.Printf("FCM response: project=%s tokenSuffix=%s status=%s body=%s\n", c.projectID, tokenSuffix(deviceToken), resp.Status, responseText)

	if resp.StatusCode >= 300 {
		return responseText, fmt.Errorf("fcm error: %s: %s", resp.Status, responseText)
	}

	return responseText, nil
}
