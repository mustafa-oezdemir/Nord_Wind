package server

import (
	"math"
	"os"
	"runtime"
	rtmetrics "runtime/metrics"
	"strconv"
	"strings"
	"time"
)

// promWriter schreibt das Prometheus-Textformat 0.0.4: je Familie zuerst HELP und
// TYPE, danach alle Werte dieser Familie zusammenhängend.
type promWriter struct{ b *strings.Builder }

func (w promWriter) family(name, kind, help string) {
	w.b.WriteString("# HELP " + name + " " + escapeHelp(help) + "\n")
	w.b.WriteString("# TYPE " + name + " " + kind + "\n")
}

func (w promWriter) sample(name, labelSet string, value float64) {
	w.b.WriteString(name)
	if labelSet != "" {
		w.b.WriteString("{" + labelSet + "}")
	}
	w.b.WriteString(" " + formatFloat(value) + "\n")
}

// labels erwartet Paare aus Name und Wert. Werte werden nach dem Textformat maskiert.
func labels(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for index := 0; index+1 < len(pairs); index += 2 {
		parts = append(parts, pairs[index]+`="`+escapeLabel(pairs[index+1])+`"`)
	}
	return strings.Join(parts, ",")
}

func escapeLabel(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(value)
}

func escapeHelp(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`).Replace(value)
}

func formatFloat(value float64) string {
	switch {
	case math.IsInf(value, 1):
		return "+Inf"
	case math.IsInf(value, -1):
		return "-Inf"
	case math.IsNaN(value):
		return "NaN"
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

var runtimeSamples = []struct{ source, name, kind, help string }{
	{"/sched/goroutines:goroutines", "go_goroutines", "gauge", "Laufende Goroutinen."},
	{"/sched/gomaxprocs:threads", "go_sched_gomaxprocs_threads", "gauge", "GOMAXPROCS: gleichzeitig nutzbare Prozessorkerne."},
	{"/memory/classes/heap/objects:bytes", "go_memstats_heap_alloc_bytes", "gauge", "Belegter Heap-Speicher lebender und noch nicht freigegebener Objekte."},
	{"/memory/classes/total:bytes", "go_memstats_sys_bytes", "gauge", "Vom Go-Laufzeitsystem beim Betriebssystem belegter Speicher."},
	{"/gc/heap/goal:bytes", "go_gc_heap_goal_bytes", "gauge", "Heap-Größe, bei der die nächste Speicherbereinigung beginnt."},
	{"/gc/cycles/total:gc-cycles", "go_gc_cycles_total", "counter", "Abgeschlossene Speicherbereinigungen."},
}

// writeRuntime ergänzt Go-Laufzeit- und Prozesswerte. Die Prozesswerte stammen aus
// /proc und fehlen außerhalb von Linux (z. B. bei lokalen Tests unter Windows).
func writeRuntime(w promWriter, started time.Time) {
	samples := make([]rtmetrics.Sample, len(runtimeSamples))
	for index, sample := range runtimeSamples {
		samples[index].Name = sample.source
	}
	rtmetrics.Read(samples)
	for index, sample := range samples {
		var value float64
		switch sample.Value.Kind() {
		case rtmetrics.KindUint64:
			value = float64(sample.Value.Uint64())
		case rtmetrics.KindFloat64:
			value = sample.Value.Float64()
		default:
			continue
		}
		w.family(runtimeSamples[index].name, runtimeSamples[index].kind, runtimeSamples[index].help)
		w.sample(runtimeSamples[index].name, "", value)
	}
	w.family("go_info", "gauge", "Go-Version der API.")
	w.sample("go_info", labels("version", runtime.Version()), 1)
	w.family("process_start_time_seconds", "gauge", "Startzeit der API als Unix-Zeit.")
	w.sample("process_start_time_seconds", "", float64(started.UnixNano())/1e9)

	if statm, err := os.ReadFile("/proc/self/statm"); err == nil {
		if fields := strings.Fields(string(statm)); len(fields) > 1 {
			if pages, err := strconv.ParseFloat(fields[1], 64); err == nil {
				w.family("process_resident_memory_bytes", "gauge", "Belegter Arbeitsspeicher (RSS) der API.")
				w.sample("process_resident_memory_bytes", "", pages*float64(os.Getpagesize()))
			}
		}
	}
	if stat, err := os.ReadFile("/proc/self/stat"); err == nil {
		// Nach dem Prozessnamen in Klammern folgen die Felder ab Nummer 3; utime und stime sind 14 und 15.
		if end := strings.LastIndexByte(string(stat), ')'); end > 0 {
			if fields := strings.Fields(string(stat)[end+1:]); len(fields) > 12 {
				user, userErr := strconv.ParseFloat(fields[11], 64)
				system, systemErr := strconv.ParseFloat(fields[12], 64)
				if userErr == nil && systemErr == nil {
					w.family("process_cpu_seconds_total", "counter", "Verbrauchte CPU-Zeit der API (Benutzer und System).")
					w.sample("process_cpu_seconds_total", "", (user+system)/100)
				}
			}
		}
	}
	if entries, err := os.ReadDir("/proc/self/fd"); err == nil {
		w.family("process_open_fds", "gauge", "Offene Dateideskriptoren der API.")
		w.sample("process_open_fds", "", float64(len(entries)))
	}
	if limits, err := os.ReadFile("/proc/self/limits"); err == nil {
		for _, line := range strings.Split(string(limits), "\n") {
			if strings.HasPrefix(line, "Max open files") {
				if fields := strings.Fields(strings.TrimPrefix(line, "Max open files")); len(fields) > 0 {
					if soft, err := strconv.ParseFloat(fields[0], 64); err == nil {
						w.family("process_max_fds", "gauge", "Obergrenze offener Dateideskriptoren.")
						w.sample("process_max_fds", "", soft)
					}
				}
			}
		}
	}
}
