# NordWinds-Oberfläche

Angular-Storefront für den deutschen Markt: Startseite, Produktliste mit Suche, Sortierung und Kategorien sowie Produktdetail mit Variantenauswahl. Die geschützte Verwaltungsseite liegt unter `/admin`. Alle Daten kommen aus der Go-API (`/api/v1`, Vertrag in [`docs/api/openapi.yaml`](../docs/api/openapi.yaml)). Bestellungen sind noch nicht möglich.

## Lokale Prüfung

```bash
npm ci
npm run format:check
npm run lint
npm run typecheck
npm test -- --watch=false
npm run build
```

`npm start` startet den Entwicklungsserver und leitet `/api` über `proxy.conf.json` an den lokalen Compose-Stack (`http://127.0.0.1:8080`) weiter. Der Docker-Build verwendet das gesperrte `package-lock.json` und erwartet die Ausgabe unter `dist/nordwinds/browser/`.

## Aufbau

- `src/app/core`: API-Konfiguration (`API_BASE_URL`), HTTP-Interceptor mit Zeitlimit und sicherer Wiederholung, Fehlerübersetzung und Seitentitel.
- `src/app/layouts/storefront-layout`: Kopfbereich, mobiles Menü (natives `<dialog>`), Fußzeile und Fokusführung.
- `src/app/features/catalog`: API-Zugriff mit Antwortprüfung, Modelle, Kategorien-Store, Produktliste, Produktdetail (`/produkte/:slug` mit Galerie und Variantenauswahl) und Katalog-Bausteine.
- `src/app/features/home`: Startseite.
- `src/app/features/admin`: Admin-Anmeldung, Kategoriebaum, Produkt-/Variantenpflege und Audit-Ansicht. Schreibzugriffe senden den Session-CSRF-Token nur im Header; Berechtigungen werden in der Go-API erzwungen.
- `src/app/shared`: Geldformatierung (`de-DE`, EUR) sowie Leer-, Fehler-, 404- und Seitenbausteine.
- `src/styles`: Design-Tokens und Bootstrap-Sass-Konfiguration. Komponenten binden die Tokens mit `@use 'tokens' as t;` ein.
- `src/testing`: Testdaten nach dem OpenAPI-Vertrag; nur für Specs.

## Regeln

- Preise sind ganzzahlige Cent-Beträge aus der API und werden nur zur Anzeige formatiert. Die Oberfläche berechnet keine Preise.
- Texte der Oberfläche sind deutsch; Bezeichner im Code sind englisch.
- Kein Bootstrap-JavaScript und keine direkte DOM-Manipulation für Dialoge oder Menüs.
- Nur Systemschriften und keine externen Ressourcen, damit keine Drittanbieter-Anfragen entstehen.
