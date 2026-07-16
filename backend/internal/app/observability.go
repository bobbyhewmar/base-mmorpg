package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type observer struct {
	mu       sync.Mutex
	families map[string]*metricFamily
}

type metricFamily struct {
	name       string
	help       string
	metricType string
	samples    map[string]*metricSample
}

type metricSample struct {
	labels map[string]string
	value  float64
}

func newObserver() *observer {
	return &observer{
		families: map[string]*metricFamily{},
	}
}

func (o *observer) incCounter(name string, help string, labels map[string]string, delta float64) {
	if o == nil {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	family := o.ensureMetricFamily(name, help, "counter")
	sample := family.ensureSample(labels)
	sample.value += delta
}

func (o *observer) setGauge(name string, help string, labels map[string]string, value float64) {
	if o == nil {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	family := o.ensureMetricFamily(name, help, "gauge")
	sample := family.ensureSample(labels)
	sample.value = value
}

func (o *observer) addGauge(name string, help string, labels map[string]string, delta float64) {
	if o == nil {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	family := o.ensureMetricFamily(name, help, "gauge")
	sample := family.ensureSample(labels)
	sample.value += delta
}

func (o *observer) maxGauge(name string, help string, labels map[string]string, value float64) {
	if o == nil {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	family := o.ensureMetricFamily(name, help, "gauge")
	sample := family.ensureSample(labels)
	if value > sample.value {
		sample.value = value
	}
}

func (o *observer) observeDurationSeconds(baseName string, help string, labels map[string]string, duration time.Duration) {
	seconds := duration.Seconds()
	o.incCounter(baseName+"_sum", help+" sum.", labels, seconds)
	o.incCounter(baseName+"_count", help+" count.", labels, 1)
}

func (o *observer) log(level string, event string, fields map[string]any) {
	if o == nil {
		return
	}

	payload := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"level":     level,
		"event":     event,
	}
	for key, value := range fields {
		payload[key] = value
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Println(string(bytes))
}

func (o *observer) renderPrometheus() string {
	if o == nil {
		return ""
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	familyNames := make([]string, 0, len(o.families))
	for name := range o.families {
		familyNames = append(familyNames, name)
	}
	sort.Strings(familyNames)

	var builder strings.Builder
	for _, familyName := range familyNames {
		family := o.families[familyName]
		builder.WriteString("# HELP ")
		builder.WriteString(family.name)
		builder.WriteByte(' ')
		builder.WriteString(escapePrometheusText(family.help))
		builder.WriteByte('\n')
		builder.WriteString("# TYPE ")
		builder.WriteString(family.name)
		builder.WriteByte(' ')
		builder.WriteString(family.metricType)
		builder.WriteByte('\n')

		sampleKeys := make([]string, 0, len(family.samples))
		for key := range family.samples {
			sampleKeys = append(sampleKeys, key)
		}
		sort.Strings(sampleKeys)
		for _, sampleKey := range sampleKeys {
			sample := family.samples[sampleKey]
			builder.WriteString(family.name)
			if len(sample.labels) > 0 {
				builder.WriteByte('{')
				labelNames := make([]string, 0, len(sample.labels))
				for labelName := range sample.labels {
					labelNames = append(labelNames, labelName)
				}
				sort.Strings(labelNames)
				for index, labelName := range labelNames {
					if index > 0 {
						builder.WriteByte(',')
					}
					builder.WriteString(labelName)
					builder.WriteString(`="`)
					builder.WriteString(escapePrometheusLabel(sample.labels[labelName]))
					builder.WriteByte('"')
				}
				builder.WriteByte('}')
			}
			builder.WriteByte(' ')
			builder.WriteString(strconv.FormatFloat(sample.value, 'f', -1, 64))
			builder.WriteByte('\n')
		}
	}

	return builder.String()
}

func (o *observer) ensureMetricFamily(name string, help string, metricType string) *metricFamily {
	family, exists := o.families[name]
	if exists {
		if family.help == "" && help != "" {
			family.help = help
		}
		return family
	}

	family = &metricFamily{
		name:       name,
		help:       help,
		metricType: metricType,
		samples:    map[string]*metricSample{},
	}
	o.families[name] = family
	return family
}

func (family *metricFamily) ensureSample(labels map[string]string) *metricSample {
	key := labelsKey(labels)
	if sample, exists := family.samples[key]; exists {
		return sample
	}

	clonedLabels := make(map[string]string, len(labels))
	for key, value := range labels {
		clonedLabels[key] = value
	}

	sample := &metricSample{
		labels: clonedLabels,
	}
	family.samples[key] = sample
	return sample
}

func labelsKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)

	var builder strings.Builder
	for _, name := range names {
		builder.WriteString(name)
		builder.WriteByte('=')
		builder.WriteString(labels[name])
		builder.WriteByte(';')
	}
	return builder.String()
}

func escapePrometheusText(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`).Replace(value)
}

func escapePrometheusLabel(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(value)
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (recorder *responseRecorder) WriteHeader(statusCode int) {
	recorder.statusCode = statusCode
	recorder.wroteHeader = true
	recorder.ResponseWriter.WriteHeader(statusCode)
}

func (recorder *responseRecorder) Write(payload []byte) (int, error) {
	if !recorder.wroteHeader {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(payload)
}

func (recorder *responseRecorder) ReadFrom(reader io.Reader) (int64, error) {
	if !recorder.wroteHeader {
		recorder.WriteHeader(http.StatusOK)
	}
	if delegatingReader, ok := recorder.ResponseWriter.(io.ReaderFrom); ok {
		return delegatingReader.ReadFrom(reader)
	}
	return io.Copy(recorder.ResponseWriter, reader)
}

func (recorder *responseRecorder) Flush() {
	if flusher, ok := recorder.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (recorder *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := recorder.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (recorder *responseRecorder) Push(target string, options *http.PushOptions) error {
	pusher, ok := recorder.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, options)
}

func (recorder *responseRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}
