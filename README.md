# custom-pocketbase

Erweiterung von PocketBase mit FCM Push-Notifications und Mail-Versand direkt aus JS-Hooks.

## Konfiguration

**Firebase (Push)** — Umgebungsvariablen in `.env` oder Container:

```env
# Option A – kompletter Service-Account als JSON (empfohlen)
GOOGLE_SERVICE_ACCOUNT_JSON={"type":"service_account","project_id":...}

# Option B – Pfad zur JSON-Datei
GOOGLE_APPLICATION_CREDENTIALS=/app/secrets/firebase.json

# Option C – Einzelne Felder
GOOGLE_PROJECT_ID=mein-firebase-projekt
GOOGLE_CLIENT_EMAIL=firebase-adminsdk@mein-projekt.iam.gserviceaccount.com
GOOGLE_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\n..."

# Optional: Timeout für FCM-Requests (Standard: 10s)
PUSH_TIMEOUT_SECONDS=10
```

**Mail** — wird über Admin-UI → Settings → Mail Settings konfiguriert (SMTP). Absender-Name und -Adresse werden automatisch aus den App-Settings gelesen.

---

## JS-API

### `$push.send(title, body)`

Sendet eine Push-Notification an alle Superuser mit gesetztem `device_token`. Nicht-blockierend.

```js
onRecordAfterCreateSuccess((e) => {
   $push.send(
      "Neue Anfrage",
      `${e.record.get("name")} · ${e.record.get("subject")}`,
   );
}, "inquiries");
```

### `$mail.send(to, subject, html)`

Sendet eine HTML-E-Mail. Nicht-blockierend (interne Queue, 2 Worker).

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

## Fehlerverhalten (Push)

- **Ungültiger Token** (`NOT_FOUND`, `UNREGISTERED`): Token wird automatisch aus der DB gelöscht.
- **Transiente Fehler**: Bis zu 3 Versuche mit exponentiellem Backoff (1s, 2s).
- **Kein Superuser mit Token**: Notification wird still übersprungen.

## Device-Token setzen

```js
await pb.collection("_superusers").update(pb.authStore.record.id, {
   device_token: fcmToken,
});
```

Das Feld `device_token` muss in `_superusers` als Text-Feld angelegt sein.
