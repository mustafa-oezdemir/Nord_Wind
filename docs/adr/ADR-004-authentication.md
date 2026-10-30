# ADR-004: Kundenanmeldung und Sitzungen

- **Status:** Für NW-11 umgesetzt am 30. Oktober 2026; Passwort-Reset als Folgeschritt entworfen.
- **Geltung:** Go-API, PostgreSQL und Browser unter demselben öffentlichen Origin.

## Entscheidung

Kunden registrieren sich mit E-Mail und Passwort. Die API normalisiert die E-Mail, vergibt ausschließlich `customer` und speichert einen gesalzenen Argon2id-Hash (`m=19456`, `t=2`, `p=1`). Bestehende Kunden ohne Passwort-Hash bleiben erhalten, können sich aber bis zu einem geregelten Reset nicht anmelden. Adminrollen werden außerhalb der öffentlichen Registrierung vergeben.

Bei erfolgreichem Login erzeugt die API einen zufälligen 256-Bit-Session-Token und einen unabhängigen CSRF-Token. PostgreSQL speichert nur den SHA-256-Hash des Session-Tokens. Ein erneuter Login widerruft die vorherige Session und stellt neue Token aus. `auth_sessions` erzwingt zwölf Stunden absolute und 30 Minuten Inaktivitätsdauer. Ein Rollenwechsel macht die bisherige Session ungültig; eine Neuanmeldung ist nötig. Logout widerruft die Session in der Datenbank.

Das Browser-Cookie heißt in HTTPS-Umgebungen `__Host-nordwinds_session` und trägt `Secure`, `HttpOnly`, `SameSite=Strict` und `Path=/`; es besitzt kein `Domain` und ist ein Browser-Session-Cookie. Nur bei ausdrücklich gesetztem `APP_ENV=development` wird für lokales HTTP `nordwinds_session` ohne `Secure` verwendet. Nichtlokale Umgebungen verlangen `APP_PUBLIC_ORIGIN` mit HTTPS. Antworten mit Session- oder CSRF-Daten verwenden `Cache-Control: no-store`.

Schreibzugriffe mit Session prüfen `X-CSRF-Token` gegen den serverseitigen Sessionwert. Der Token wird nach Anmeldung und über `GET /api/v1/auth/session` im JSON ausgegeben, nicht im Cookie oder in einer URL. Die API prüft vorhandene `Origin`-/`Referer`-Header gegen `APP_PUBLIC_ORIGIN` und weist `Sec-Fetch-Site: cross-site` ab. Registrierung und Login akzeptieren ausschließlich JSON; Cross-Origin-Requests erhalten keine CORS-Freigabe. Künftige Kunden- und Admin-Schreibrouten müssen `RequireSession`, `RequireRole` und `RequireCSRF` verwenden.

Die Login-Grenze liegt bei fünf Versuchen je normalisierter E-Mail und 100 Versuchen je direkt verbundener Quelladresse in 15 Minuten; Zähler liegen gemeinsam in PostgreSQL. Die Schlüssel sind gehasht. Erfolgreicher Login setzt den E-Mail-Zähler zurück. Die Quelladresse stammt aus `RemoteAddr`, nicht aus ungeprüften Proxy-Headern; bei einem späteren Reverse-Proxy-Vertrauensmodell muss die Quellgrenze erneut bewertet werden. Abgelaufene Zähler und Sessions werden bei Login bereinigt. Unbekannte E-Mail und falsches Passwort liefern dieselbe 401-Antwort.

## Passwort-Reset, noch nicht implementiert

Ein späterer Reset-Endpunkt soll unabhängig von der Existenz eines Kontos dieselbe neutrale Antwort liefern und pro Konto/Quelle begrenzt werden. Ein zufälliger 256-Bit-Einmal-Token wird nur als Hash gespeichert, ist höchstens 15 Minuten gültig und wird ausschließlich über einen separat geprüften E-Mail-Kanal ausgeliefert. Der Bestätigungsaufruf verbraucht den Token atomar, speichert einen neuen Argon2id-Hash und widerruft alle Sessions des Kontos. Reset-Tokens und Passwörter dürfen weder in URLs von Drittressourcen noch in Logs oder Analytics gelangen. Der E-Mail-Dienst, Zustellbarkeit und Missbrauchsschutz sind vor Umsetzung zu entscheiden.

## Prüfung

`httptest` mit eigenem PostgreSQL prüft falsches Passwort, Session-Fixation und Rotation, CSRF, falschen Origin, Ablauf, Login-Grenze sowie 401/403. Ein Logtest prüft, dass Passwort, Session- und CSRF-Token sowie E-Mail nicht im Access-Log erscheinen. Migration `0004_identity` verweigert einen Down-Lauf, solange Passwort-Hashes oder Sessions vorhanden sind.
