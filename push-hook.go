package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/pocketbase/pocketbase/core"
)

type pushService struct {
	app    core.App
	client *fcmClient
}

func newPushService(app core.App) (*pushService, error) {
	client, err := newPushClientFromEnv()
	if err != nil {
		return nil, err
	}

	return &pushService{
		app:    app,
		client: client,
	}, nil
}

func (s *pushService) SendToAllSuperusers(ctx context.Context, title, body string) error {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)

	if title == "" {
		return fmt.Errorf("push title must not be empty")
	}

	if body == "" {
		return fmt.Errorf("push body must not be empty")
	}

	recipients, err := s.app.FindRecordsByFilter(
		"_superusers",
		"device_token != ''",
		"",
		0,
		0,
	)
	if err != nil {
		return fmt.Errorf("load push recipients: %w", err)
	}

	if len(recipients) == 0 {
		s.app.Logger().Warn("push skipped because no superusers with device_token were found")
		return nil
	}

	s.app.Logger().Info("sending push to superusers", "count", len(recipients), "title", title)

	var failed int

	for _, recipient := range recipients {
		deviceToken := recipient.GetString("device_token")
		if deviceToken == "" {
			continue
		}

		responseBody, err := s.client.Send(ctx, deviceToken, title, body)
		if err != nil {
			failed++
			s.app.Logger().Error(
				"push delivery failed",
				"recipientId", recipient.Id,
				"deviceTokenSuffix", tokenSuffix(deviceToken),
				"response", responseBody,
				"err", err,
			)
			continue
		}

		s.app.Logger().Info(
			"push delivered",
			"recipientId", recipient.Id,
			"deviceTokenSuffix", tokenSuffix(deviceToken),
			"response", responseBody,
		)
	}

	if failed > 0 {
		return fmt.Errorf("push delivery failed for %d recipient(s)", failed)
	}

	return nil
}

func registerPushBindings(vm *goja.Runtime, push *pushService) {
	pushObject := vm.NewObject()

	if err := pushObject.Set("send", func(title string, body string) {
		if err := push.SendToAllSuperusers(context.Background(), title, body); err != nil {
			push.app.Logger().Error("push.send failed", "err", err)
		}
	}); err != nil {
		panic(err)
	}

	vm.Set("$push", pushObject)
}

func tokenSuffix(token string) string {
	if len(token) <= 8 {
		return token
	}

	return token[len(token)-8:]
}
