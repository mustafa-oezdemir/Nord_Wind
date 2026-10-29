# NordWinds-Oberfläche

Dieses Angular-Projekt bildet die technische Basis der deutschsprachigen Shop-Oberfläche. Derzeit zeigt es nur eine klar gekennzeichnete Startseite; Katalog und Checkout folgen in späteren Schritten.

## Lokale Prüfung

```bash
npm ci
npm run build
npm test -- --watch=false
```

Für die Entwicklung startet `npm start` den Angular-Entwicklungsserver. Der Docker-Build verwendet das gesperrte `package-lock.json` und erwartet die Ausgabe unter `dist/nordwinds/browser/`.
