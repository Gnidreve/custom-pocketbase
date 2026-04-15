# Plan: Mail-Service als JS-Binding ($mail.send)

## Context

Analog zu `$push.send()` soll ein `$mail.send()` als globales Objekt in PocketBase JS-Hooks verfügbar sein. Das Frontend kennt Empfängerdaten (To, Subject, HTML-Body), aber nie Absenderdaten — die werden zur Laufzeit aus den App-Settings gelesen. Verarbeitung asynchron über eine Queue (Channel + Worker-Goroutinen), damit der Hook nicht blockiert.

---

## Neue Datei: `mail-hook.go`

### Datenstrukturen

```go
type mailJob struct {
    to      string
    subject string
    html    string
}

type mailService struct {
    app    core.App
    queue  chan mailJob
    wg     sync.WaitGroup
}
```

### Queue & Worker

- Buffered Channel: `make(chan mailJob, 100)` — 100 Jobs Puffer
- 2 Worker-Goroutinen lesen dauerhaft aus dem Channel
- Nicht-blockierendes Enqueue via `select { case queue <- job: default: log drop }`
- `Shutdown()`: `close(queue)` → Worker-Goroutinen beenden sich sauber → `wg.Wait()`

```go
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
    for job := range s.queue { // endet wenn channel geschlossen wird
        s.deliver(job)
    }
}
```

### `deliver()` — Absender aus App-Settings

```go
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
```

### JS-Binding: `$mail.send(to, subject, html)`

```go
func registerMailBindings(vm *goja.Runtime, ms *mailService) {
    mailObject := vm.NewObject()
    mailObject.Set("send", func(to, subject, html string) {
        select {
        case ms.queue <- mailJob{to: to, subject: subject, html: html}:
            log.Printf("[mail] queued: to=%s subject=%q", to, subject)
        default:
            log.Printf("[mail] queue full, dropping mail to %s", to)
        }
    })
    vm.Set("$mail", mailObject)
}
```

---

## Änderungen in `main.go`

1. `newMailService(app)` aufrufen
2. `registerMailBindings(vm, mailService)` in `OnInit` ergänzen
3. `mailService.Shutdown()` in `OnTerminate` ergänzen

---

## Kritische Dateien

- **neu**: `mail-hook.go` — mailJob, mailService, worker, deliver, registerMailBindings
- **ändern**: `main.go` — Service erstellen, Bindings registrieren, Shutdown

---

## JS-Verwendung (Beispiel)

```js
onRecordAfterCreateSuccess((e) => {
   $mail.send(
      e.record.get("email"),
      "Neue Anfrage eingegangen",
      `<p>Hallo, deine Anfrage wurde empfangen.</p>`,
   );
}, "inquiries");
```

---

## Verifikation

1. `go build ./...` — kompiliert fehlerfrei
2. Hook triggern → Log zeigt `[mail] queued` und `[mail] sent`
3. E-Mail kommt beim Empfänger an
4. `docker stop` → Container wartet auf laufende Mail-Jobs
