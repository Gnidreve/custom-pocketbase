# custom-pocketbase

Erweiterung von PocketBase mit Firebase Cloud Messaging (FCM) Push-Notifications, die direkt aus JS-Hooks heraus aufgerufen werden können.

## Voraussetzungen

- Ein Firebase-Projekt mit aktiviertem Cloud Messaging
- Ein Service-Account mit der Rolle **Firebase Cloud Messaging API Admin**
- Mindestens ein Superuser mit gesetztem `device_token`-Feld in der `_superusers`-Collection

## Konfiguration

Umgebungsvariablen in `.env` (oder direkt im Container):

```env
# Projekt-ID aus der Firebase-Konsole
GOOGLE_PROJECT_ID=mein-firebase-projekt

# Option A – kompletter Service-Account als JSON (empfohlen)
GOOGLE_SERVICE_ACCOUNT_JSON={"type":"service_account","project_id":...}

# Option B – Pfad zu einer JSON-Datei
GOOGLE_APPLICATION_CREDENTIALS=/run/secrets/firebase.json

# Option C – Einzelne Felder
GOOGLE_CLIENT_EMAIL=firebase-adminsdk@mein-projekt.iam.gserviceaccount.com
GOOGLE_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\n..."

# Optional: Timeout für FCM-Requests (Standard: 10s)
PUSH_TIMEOUT_SECONDS=10
```

---

## Push-Notifications aus JS-Hooks senden

Das globale Objekt `$push` steht in allen PocketBase JS-Hooks zur Verfügung.

### `$push.send(title, body)`

Sendet eine Push-Notification an **alle Superuser**, die ein `device_token`-Feld gesetzt haben.

| Parameter | Typ    | Beschreibung           |
| --------- | ------ | ---------------------- |
| `title`   | string | Titel der Notification |
| `body`    | string | Text der Notification  |

Der Aufruf ist **nicht-blockierend** — die Notification wird im Hintergrund gesendet, der Hook kehrt sofort zurück.

---

## Beispiele

### Notification bei neuem Datensatz

```js
// pb_hooks/inquiries.pb.js
onRecordAfterCreateSuccess((e) => {
   $push.send(
      "Neue Anfrage",
      `${e.record.get("name")} · ${e.record.get("subject")}`,
   );
}, "inquiries");
```

### Notification bei Statusänderung

```js
// pb_hooks/orders.pb.js
onRecordAfterUpdateSuccess((e) => {
   const status = e.record.get("status");
   if (status === "shipped") {
      $push.send(
         "Bestellung versandt",
         `Bestellung #${e.record.get("order_number")} ist unterwegs`,
      );
   }
}, "orders");
```

### Notification bei neuem Benutzer

```js
// pb_hooks/users.pb.js
onRecordAfterCreateSuccess((e) => {
   $push.send(
      "Neuer Benutzer",
      `${e.record.get("email")} hat sich registriert`,
   );
}, "users");
```

---

## Verhalten bei Fehlern

- **Ungültiger/abgemeldeter Token** (`NOT_FOUND`, `UNREGISTERED`): Der Token wird automatisch aus der Datenbank gelöscht. Beim nächsten Login des Nutzers muss das Frontend den Token neu setzen.
- **Transiente Fehler** (Netzwerk, FCM 5xx): Es werden bis zu 3 Versuche mit exponentiellem Backoff unternommen (1s, dann 2s Pause).
- **Kein Superuser mit Token**: Die Notification wird still übersprungen.

## Device-Token setzen

Das Frontend muss nach dem FCM-Token-Request das Feld `device_token` beim eingeloggten Superuser aktualisieren:

```js
// Beispiel (PocketBase JS SDK)
await pb.collection("_superusers").update(pb.authStore.record.id, {
   device_token: fcmToken,
});
```

Das Feld `device_token` muss in der `_superusers`-Collection als Text-Feld angelegt sein.
