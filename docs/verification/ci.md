# CI und Pull-Request-Kontrollen

Der Workflow [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) läuft bei Pull Requests nach `main`, Pushes auf `main` und manuellem Start. Der einzelne Job `quality` prüft in dieser Reihenfolge Format, Angular-Lint und `go vet`, Go- und Angular-Tests, Typecheck und Build, npm-/Go-Abhängigkeiten, Compose-Konfiguration und beide Docker-Images. Ein fehlgeschlagener Befehl beendet den Job mit rotem Status.

## Pull Request prüfen

1. Einen Pull Request gegen `main` öffnen und im Bereich **Checks** den GitHub-Actions-Job **quality** abwarten.
2. Bei Rot den fehlgeschlagenen Schritt und sein Protokoll öffnen, den Fehler im Branch beheben und erneut pushen.
3. Erst nach grünem **quality** und den weiteren für das Repository vorgesehenen Sicherheitsprüfungen zusammenführen.

Der Workflow verwendet `pull_request` statt `pull_request_target`, ein `GITHUB_TOKEN` mit ausschließlich `contents: read`, Checkout ohne gespeicherte Zugangsdaten und keine Repository-Secrets. Die automatischen Node-/Go-Caches sind abgeschaltet; Docker-Images werden nur lokal im Runner gebaut und nicht veröffentlicht. Für die reine Compose-Syntaxprüfung wird ein ausdrücklich nicht geheimer Dummy-Wert statt einer echten Datenbankanmeldung eingesetzt. Fork-PRs erhalten dadurch keine privilegierten Zugangsdaten.

## Statuscheck für `main` verpflichtend machen

Unter **Settings → Branches** oder **Settings → Rules → Rulesets** eine Regel für `main` anlegen: Pull Request vor Merge verlangen, **Require status checks to pass before merging** aktivieren und den nach dem ersten erfolgreichen Lauf sichtbaren Check **quality** aus GitHub Actions auswählen. Nach Bedarf weitere Sicherheitschecks auswählen, veraltete Branches vor dem Merge aktualisieren lassen sowie Force-Push und Umgehungen deaktivieren. Einen absichtlich fehlgeschlagenen Test in einem separaten PR prüfen: `quality` muss rot werden und der Merge muss abgewiesen werden; danach den Test korrigieren.

**Aktuelle Kontogrenze (31. Oktober 2026):** Das Repository ist privat. Die GitHub-API antwortet beim Lesen der `main`-Branch-Protection weiterhin mit HTTP 403 und der Meldung, dass hierfür GitHub Pro oder ein öffentliches Repository erforderlich ist. Deshalb konnte `quality` bislang **nicht** als verpflichtender Merge-Check aktiviert werden. Ein fehlschlagender Test macht den PR-Check rot; das technische Merge-Verbot ist erst nach Aktivierung der Schutzregel nachweisbar. Die [GitHub-Dokumentation zu geschützten Branches](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches) beschreibt die verfügbaren Pläne und Einstellungen.

**Aktueller CI-Blocker (31. Oktober 2026):** Auch die [NW-24-CI](https://github.com/mustafa-oezdemir/Nord_Wind/actions/runs/41183424949) und [Security-Prüfung](https://github.com/mustafa-oezdemir/Nord_Wind/actions/runs/41183425021) endeten vor dem ersten Schritt. Die GitHub-Check-Annotation nennt fehlgeschlagene Kontozahlungen oder ein zu niedriges Ausgabenlimit. Sobald die Kontoeinstellung geklärt ist, beide Jobs erneut ausführen und erst dann ihren tatsächlichen Prüferfolg bewerten. Der neue [Release-Kontrollworkflow](../../.github/workflows/release-controls.yml) verweigert einen Kandidaten ohne gemergten PR, grüne Checks und erforderliche Staging-Reviewer. Eine Environment-Regel ist derzeit nicht eingerichtet; der Workflow ist kein Produktions-Deploy.
