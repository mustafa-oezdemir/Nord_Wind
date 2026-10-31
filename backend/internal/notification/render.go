package notification

import (
	"bytes"
	"embed"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

//go:embed templates/layout.html templates/layout.txt
var templateFiles embed.FS

// html/template maskiert alle Werte kontextabhängig; Kundendaten können so weder HTML
// noch Skripte noch fremde Link-Schemata einschleusen.
var (
	htmlLayout = htmltemplate.Must(htmltemplate.ParseFS(templateFiles, "templates/layout.html"))
	textLayout = texttemplate.Must(texttemplate.ParseFS(templateFiles, "templates/layout.txt"))
)

// Render erzeugt Betreff, Text- und HTML-Fassung aus einem View.
func Render(view View) (subject, text, html string, err error) {
	var htmlBody, textBody bytes.Buffer
	if err := htmlLayout.Execute(&htmlBody, view); err != nil {
		return "", "", "", err
	}
	if err := textLayout.Execute(&textBody, view); err != nil {
		return "", "", "", err
	}
	return view.Subject, collapseBlankLines(textBody.String()), htmlBody.String(), nil
}

// collapseBlankLines hält die Textfassung kompakt: höchstens eine Leerzeile am Stück.
func collapseBlankLines(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			if blank || len(result) == 0 {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n")) + "\n"
}
