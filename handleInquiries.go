package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/pocketbase/pocketbase/core"
)

type inquiryPayload struct {
	Name    string `json:"name" form:"name"`
	Email   string `json:"email" form:"email"`
	Mail    string `json:"mail" form:"mail"`
	Phone   string `json:"phone" form:"phone"`
	Telefon string `json:"telefon" form:"telefon"`
	Message string `json:"message" form:"message"`
}

func registerInquiryRoutes(se *core.ServeEvent) {
	se.Router.POST("/newInquiry", handleNewInquiry)
}

func handleNewInquiry(e *core.RequestEvent) error {
	payload := new(inquiryPayload)
	if err := e.BindBody(payload); err != nil {
		return e.BadRequestError("An error occurred while loading the submitted data.", err)
	}

	name := strings.TrimSpace(payload.Name)
	email := strings.ToLower(strings.TrimSpace(firstNonEmpty(payload.Email, payload.Mail)))
	phone := strings.TrimSpace(firstNonEmpty(payload.Phone, payload.Telefon))
	message := strings.TrimSpace(payload.Message)

	if err := validation.ValidateStruct(payload,
		validation.Field(&name, validation.Required, validation.Length(1, 255)),
		validation.Field(&email, validation.Required, validation.Length(1, 255), is.EmailFormat),
		validation.Field(&phone, validation.Length(0, 255)),
		validation.Field(&message, validation.Required, validation.Length(1, 10000)),
	); err != nil {
		return e.BadRequestError("An error occurred while validating the submitted data.", err)
	}

	inquiriesCollection, err := e.App.FindCollectionByNameOrId("inquiries")
	if err != nil {
		return e.InternalServerError("Inquiries collection not found.", err)
	}

	record := core.NewRecord(inquiriesCollection)
	record.Set("name", name)
	record.Set("email", email)
	record.Set("telefon", phone)
	record.Set("message", message)

	customer, err := e.App.FindFirstRecordByData("customers", "email", email)
	if err == nil {
		record.Set("customer", customer.Id)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return e.InternalServerError("Failed to look up customer relation.", err)
	}

	if err := e.App.Save(record); err != nil {
		return e.InternalServerError("Failed to save inquiry.", err)
	}

	return e.JSON(http.StatusCreated, map[string]any{
		"success": true,
		"id":      record.Id,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}
