package main

import "github.com/pocketbase/pocketbase/core"

func registerPushHooks(app core.App) {
	pushClient, err := newPushClientFromEnv()
	if err != nil {
		app.Logger().Warn("push notifications disabled", "err", err)
		return
	}

	app.OnRecordAfterCreateSuccess("inquiries").BindFunc(func(e *core.RecordEvent) error {
		recipients, err := e.App.FindRecordsByFilter(
			"_superusers",
			"device_token != ''",
			"",
			0,
			0,
		)
		if err != nil {
			return err
		}

		if len(recipients) == 0 {
			return nil
		}

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
				continue
			}

			if err := pushClient.Send(e.Context, deviceToken, title, body, data); err != nil {
				app.Logger().Error(
					"push delivery failed",
					"recipientId", recipient.Id,
					"deviceToken", deviceToken,
					"err", err,
				)
			}
		}

		return nil
	})
}
