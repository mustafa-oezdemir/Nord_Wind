# NordWinds — Staging- und Release-Runbook

## Lokale Staging-Vorschau

Die Befehle im Repository-Stamm erzeugen ausschließlich synthetische Daten. `.env.staging` wird nicht überschrieben; bestehende Staging-Daten bleiben bei `down` erhalten. `backups-staging/` enthält personenbezogene Test-Snapshots und gehört nicht in Git oder in ein Image.

```powershell
pwsh -NoProfile -File scripts/create-local-staging-env.ps1
docker compose --env-file .env.staging -f compose.yaml -f compose.staging.yaml config --quiet
docker compose --env-file .env.staging -f compose.yaml -f compose.staging.yaml up -d --build
Get-Content -Raw -Encoding UTF8 backend/seeds/demo_catalog.sql |
  docker compose --env-file .env.staging -f compose.yaml -f compose.staging.yaml exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -X -v ON_ERROR_STOP=1'
pwsh -NoProfile -File scripts/staging-check.ps1 -SyntheticFlow
pwsh -NoProfile -File scripts/staging-preflight.ps1
docker compose --env-file .env.staging -f compose.yaml -f compose.staging.yaml down
```

Die interne Caddy-CA dient nur `staging.localhost`; der Test prüft ihr Zertifikat ausdrücklich, ohne die CA dem Windows-Truststore hinzuzufügen. Das Skript schließt sein synthetisches Kundenkonto. Die separate Sicherung wird vor dem Migrations-Dry-Run wiederhergestellt; `migrate up` und `status` laufen auf der Kopie. Ein fehlgeschlagener Dry-Run lässt die Kopie zur Untersuchung bestehen. Eine produktive Datenbank wird nie als Dry-Run-Ziel verwendet.

## Release-Tag und Remote-Freigabe

1. Alle Änderungen durch einen gemergten PR nach `main` bringen. `quality`, `images` und `secrets` müssen für den exakten Commit grün sein. Branch-/Ruleset-Schutz muss PR und Checks erzwingen; die GitHub-Umgebung `staging` muss erforderliche Reviewer haben. Der aktuelle GitHub-Abrechnungsblocker ist vorher zu beheben.
2. Erst dann `git fetch origin main --tags` ausführen, den 40-stelligen SHA des geprüften Commits notieren und einen **annotierten** SemVer-Tag wie `v1.2.3` auf genau diesem Commit erstellen. Tag und Push sind ein bewusster Release-Schritt, nicht Teil des lokalen Staging-Tests.
3. `Release controls` per `workflow_dispatch` mit Tag und SHA starten. Der Workflow prüft die PR-/CI-/Security-Bedingungen erneut, wartet auf die Environment-Freigabe und führt einen isolierten Staging-Smoke, Kundenfluss sowie Backup-/Migrations-Dry-Run aus. Er stellt keine Anwendung bereit.
4. Für eine echte Staging- oder Produktionsbereitstellung Zielhost, öffentliche Domain, TLS-Aussteller, Netzwerkgrenzen und unveränderliche Image-Digests ergänzen. Die lokale `tls internal`-Konfiguration darf nicht als öffentliches TLS eingesetzt werden.

## Produktions-Checkliste und Rollback

Vor der separaten Deploy-Zustimmung müssen ein verantwortlicher Betreiber und juristische/steuerliche Abnahme vorliegen; echte Preis-, Steuer-, Versand-, Grundpreis-, GPSR- und Widerrufsdaten sind im Shop zu prüfen. BFSG/Screenreader-Audit, Datenschutz- und Aufbewahrungsregeln, echtes Payment-Sandbox-E2E mit Bestellbestätigung und Reconciliation müssen bestanden sein. Live-Provider-Schlüssel dürfen nur als Laufzeitsecret übergeben werden. Datenbankrolle, Network-Policy, HTTPS, zentrale Logs, Metriken, Alarmzustellung und ein externes verschlüsseltes Backup mit Restore-Probe sind nachzuweisen.

Für den konkreten Release: vor Migration und Traffic-Wechsel Sicherung erstellen und SHA-256 prüfen; Migration auf einer Rücksicherung trockentesten; bisherigen App-Image-Digest und Datenbankstand notieren; danach Migration und gepinnte Images ausrollen. Health, kritische Seiten, Registrierung, Checkout, 5xx-/Latenzwerte und Zahlungsabgleich beobachten. Bei einem Fehler zuerst Traffic/Anwendungsimage auf den zuvor geprüften Digest zurücknehmen, sofern das Schema kompatibel ist. `migrate down` ist kein automatisches Rollback. Eine Datenbankrücksicherung darf nur mit Schreibpause, Prüfung neuer Bestellungen/Zahlungen seit dem Backup und expliziter Betriebsentscheidung erfolgen. Die einzelnen Schritte mit Zeit, SHA, Verantwortlichen und Ergebnissen protokollieren.

Diese Liste ist eine Vorlage; für NordWinds fehlen die [NW-26-Freigaben](../docs/progress/NW-26.md) und ein tatsächlicher Produktions-Deploy ist gesperrt.
