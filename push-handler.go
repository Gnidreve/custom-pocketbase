package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/google"
)

const (
	projectID = "crm-push-4e3a2"
	fcmURL    = "https://fcm.googleapis.com/v1/projects/" + projectID + "/messages:send"
)

type FCMMessage struct {
	Message struct {
		Token        string            `json:"token"`
		Notification map[string]string `json:"notification"`
		Data         map[string]string `json:"data"`
	} `json:"message"`
}

func SendPush(serviceAccountJSON []byte, deviceToken, title, body string, data map[string]string) error {
	ctx := context.Background()

	// ✔️ OAuth2 Client direkt aus Service Account
	client, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return err
	}

	msg := FCMMessage{}
	msg.Message.Token = deviceToken
	msg.Message.Notification = map[string]string{
		"title": title,
		"body":  body,
	}
	msg.Message.Data = data

	jsonBody, _ := json.Marshal(msg)

	req, _ := http.NewRequest("POST", fcmURL, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("fcm error: %s", resp.Status)
	}

	return nil
}