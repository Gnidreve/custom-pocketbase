package main

import "github.com/pocketbase/pocketbase/core"

func registerPushHooks(app core.App) {
	pushClient, err := newPushClientFromEnv()
	if err != nil {
		app.Logger().Warn("push notifications disabled", "err", err)
		return
	}

	app.Logger().Info("push notifications enabled", "projectId", pushClient.ProjectID())

	app.OnRecordAfterCreateSuccess("inquiries").BindFunc(func(e *core.RecordEvent) error {
		app.Logger().Info(
			"inquiry create hook triggered",
			"recordId", e.Record.Id,
			"collection", e.Record.Collection().Name,
		)

		recipients, err := e.App.FindRecordsByFilter(
			"_superusers",
			"device_token != ''",
			"",
			0,
			0,
		)
		if err != nil {
			app.Logger().Error("failed to load push recipients", "err", err)
			return err
		}

		if len(recipients) == 0 {
			app.Logger().Warn("no push recipients found")
			return nil
		}

		app.Logger().Info("push recipients loaded", "count", len(recipients))

		inquiry := e.Record
		title := "Neue Anfrage im CRM"
		body := inquiry.GetString("name") + " · " + inquiry.GetString("subject")
		data := map[string]string{
			"event":          "inquiry.created",
			"inquiryId":      inquiry.Id,
			"inquiryName":    inquiry.GetString("name"),
			"inquirySubject": inquiry.GetString("subject"),
			"inquiryEmail":   inquiry.GetString("email"),
		}

		for _, recipient := range recipients {
			deviceToken := recipient.GetString("device_token")
			if deviceToken == "" {
				app.Logger().Warn("recipient has empty device token", "recipientId", recipient.Id)
				continue
			}

			responseBody, err := pushClient.Send(e.Context, deviceToken, title, body, data)
			if err != nil {
				app.Logger().Error(
					"push delivery failed",
					"recipientId", recipient.Id,
					"deviceTokenSuffix", tokenSuffix(deviceToken),
					"response", responseBody,
					"err", err,
				)
				continue
			}

			app.Logger().Info(
				"push delivered",
				"recipientId", recipient.Id,
				"deviceTokenSuffix", tokenSuffix(deviceToken),
				"response", responseBody,
			)
		}

		return nil
	})
}

func tokenSuffix(token string) string {
	if len(token) <= 8 {
		return token
	}

	return token[len(token)-8:]
}
