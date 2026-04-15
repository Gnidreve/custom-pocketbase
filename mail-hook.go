package main

import (
	"log"
	"net/mail"
	"sync"

	"github.com/dop251/goja"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

type mailJob struct {
	to      string
	subject string
	html    string
}

type mailService struct {
	app   core.App
	queue chan mailJob
	wg    sync.WaitGroup
}

func newMailService(app core.App) *mailService {
	s := &mailService{app: app, queue: make(chan mailJob, 100)}
	for range 2 {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

func (s *mailService) worker() {
	defer s.wg.Done()
	for job := range s.queue {
		s.deliver(job)
	}
}

func (s *mailService) deliver(job mailJob) {
	msg := &mailer.Message{
		From: mail.Address{
			Address: s.app.Settings().Meta.SenderAddress,
			Name:    s.app.Settings().Meta.SenderName,
		},
		To:      []mail.Address{{Address: job.to}},
		Subject: job.subject,
		HTML:    job.html,
	}
	if err := s.app.NewMailClient().Send(msg); err != nil {
		log.Printf("[mail] send failed: to=%s err=%v", job.to, err)
		return
	}
	log.Printf("[mail] sent: to=%s subject=%q", job.to, job.subject)
}

func (s *mailService) Shutdown() {
	close(s.queue)
	s.wg.Wait()
}

func registerMailBindings(vm *goja.Runtime, ms *mailService) {
	mailObject := vm.NewObject()
	if err := mailObject.Set("send", func(to, subject, html string) {
		select {
		case ms.queue <- mailJob{to: to, subject: subject, html: html}:
			log.Printf("[mail] queued: to=%s subject=%q", to, subject)
		default:
			log.Printf("[mail] queue full, dropping mail to %s", to)
		}
	}); err != nil {
		panic(err)
	}
	vm.Set("$mail", mailObject)
}
