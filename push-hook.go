package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/pocketbase/pocketbase/core"
)

const maxSendRetries = 3

type pushService struct {
	app    core.App
	client *fcmClient
	wg     sync.WaitGroup
}

func newPushService(app core.App) (*pushService, error) {
	client, err := newFCMClient(app.Logger())
	if err != nil {
		return nil, err
	}

	return &pushService{
		app:    app,
		client: client,
	}, nil
}

// Shutdown blocks until all in-flight push goroutines have finished.
// Call this before the process exits to avoid dropping notifications.
func (s *pushService) Shutdown() {
	s.wg.Wait()
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

	log.Printf("[push] dispatch requested: title=%q projectId=%s", title, s.client.ProjectID())

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
		log.Printf("[push] skipped: no superusers with device_token found")
		return nil
	}

	log.Printf("[push] sending to %d superuser(s)", len(recipients))

	var failed int

	for _, recipient := range recipients {
		if recipient.GetString("device_token") == "" {
			continue
		}

		if err := s.sendWithRetry(ctx, recipient, title, body); err != nil {
			failed++
			log.Printf("[push] delivery failed: recipientId=%s tokenSuffix=%s err=%v",
				recipient.Id, tokenSuffix(recipient.GetString("device_token")), err)
			continue
		}

		log.Printf("[push] delivered: recipientId=%s tokenSuffix=%s",
			recipient.Id, tokenSuffix(recipient.GetString("device_token")))
	}

	if failed > 0 {
		return fmt.Errorf("push delivery failed for %d recipient(s)", failed)
	}

	return nil
}

// sendWithRetry attempts to deliver a push notification up to maxSendRetries times.
// Transient errors (5xx, network) are retried with exponential backoff.
// Permanent token errors (token expired/unregistered) cause the token to be
// cleared from the database immediately without retrying.
func (s *pushService) sendWithRetry(ctx context.Context, recipient *core.Record, title, body string) error {
	deviceToken := recipient.GetString("device_token")
	var lastErr error

	for attempt := range maxSendRetries {
		_, tokenInvalid, err := s.client.Send(ctx, deviceToken, title, body)
		if err == nil {
			return nil
		}

		lastErr = err

		if tokenInvalid {
			s.app.Logger().Warn(
				"FCM token invalid, clearing from database",
				"recipientId", recipient.Id,
				"deviceTokenSuffix", tokenSuffix(deviceToken),
			)
			recipient.Set("device_token", "")
			if saveErr := s.app.Save(recipient); saveErr != nil {
				s.app.Logger().Error(
					"failed to clear invalid device token",
					"recipientId", recipient.Id,
					"err", saveErr,
				)
			}
			return err
		}

		if attempt < maxSendRetries-1 {
			wait := time.Duration(1<<attempt) * time.Second // 1s, 2s
			s.app.Logger().Warn(
				"transient push error, retrying",
				"recipientId", recipient.Id,
				"attempt", attempt+1,
				"retryIn", wait,
				"err", err,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
	}

	return lastErr
}

func registerPushBindings(vm *goja.Runtime, push *pushService) {
	pushObject := vm.NewObject()

	if err := pushObject.Set("send", func(title string, body string) {
		log.Printf("[push] send called: title=%q", title)

		push.wg.Add(1)
		go func() {
			defer push.wg.Done()
			if err := push.SendToAllSuperusers(context.Background(), title, body); err != nil {
				log.Printf("[push] send failed: %v", err)
				return
			}
			log.Printf("[push] send completed successfully")
		}()
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
