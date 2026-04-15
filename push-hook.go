app.OnRecordAfterCreateSuccess("inquiries").Add(func(e *core.RecordCreateEvent) error {

	inquiry := e.Record

	recipients, err := app.FindRecordsByFilter(
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

	// ✔️ Service Account JSON (wichtig!)
	serviceAccount := []byte(`{
		"type": "service_account",
		"project_id": "crm-push-4e3a2",
		...
	}`)

	for _, r := range recipients {

		deviceToken := r.GetString("device_token")

		data := map[string]string{
			"event":           "inquiry.created",
			"inquiryId":       inquiry.Id,
			"inquiryName":     inquiry.GetString("name"),
			"inquirySubject":  inquiry.GetString("subject"),
			"inquiryEmail":    inquiry.GetString("email"),
		}

		err := SendPush(
			serviceAccount,
			deviceToken,
			"Neue Anfrage im CRM",
			inquiry.GetString("name")+" · "+inquiry.GetString("subject"),
			data,
		)

		if err != nil {
			app.Logger().Error("push failed", "err", err)
		}
	}

	return nil
})