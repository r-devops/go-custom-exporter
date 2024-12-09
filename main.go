package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metric represents the structure of a metric to be exported.
type Metric struct {
	System    string
	Subsystem string
	Name      string
	Service   string
	Component string
	Value     float64
}

// GetArgs retrieves command line arguments for script execution.
func GetArgs() (string, string, time.Duration) {
	if len(os.Args) != 7 {
		UsageError()
	}

	if os.Args[1] != "-script" || os.Args[3] != "-port" || os.Args[5] != "-timeout" {
		UsageError()
	}

	timeout := StringToDuration(os.Args[6])
	return os.Args[2], os.Args[4], timeout
}

// UsageError displays usage instructions and exits.
func UsageError() {
	log.Fatal(`ERROR: Invalid arguments provided. Usage:
custom_exporter -script <script_path> -port <port> -timeout <seconds>
`)
}

// StringToDuration converts a string to time.Duration.
func StringToDuration(s string) time.Duration {
	value, err := strconv.Atoi(s)
	if err != nil {
		log.Fatalf("ERROR: Invalid timeout value: %v", err)
	}
	return time.Duration(value) * time.Second
}

// CheckCmdOutput validates the output of the custom script.
func CheckCmdOutput(fields []string) {
	if len(fields) != 6 {
		log.Fatal(`ERROR: Custom script output must have exactly six fields:
hostname, instance, metric_name, service_name, component_name, metric_value`)
	}
}

// ExecuteCommand runs the specified command and returns its output.
func ExecuteCommand(script string) ([]Metric, error) {
	cmd := exec.Command(script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	var metrics []Metric
	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ",")
		CheckCmdOutput(fields)

		value, err := strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid metric value: %v", err)
		}

		metrics = append(metrics, Metric{
			System:    strings.TrimSpace(fields[0]),
			Subsystem: strings.TrimSpace(fields[1]),
			Name:      strings.TrimSpace(fields[2]),
			Service:   strings.TrimSpace(fields[3]),
			Component: strings.TrimSpace(fields[4]),
			Value:     value,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading command output: %w", err)
	}

	return metrics, nil
}

// UpdateMetrics updates Prometheus metrics from the executed command.
func UpdateMetrics(script string, gauge *prometheus.GaugeVec, timeout time.Duration) {
	for {
		metrics, err := ExecuteCommand(script)
		if err != nil {
			log.Printf("Error executing command: %v", err)
			time.Sleep(5 * time.Second) // Retry after a delay on error
			continue
		}

        // Reset gauge values before updating
        gauge.Reset()

        // Update Prometheus metrics
        for _, metric := range metrics {
            gauge.With(prometheus.Labels{
                "system":    metric.System,
                "subsystem": metric.Subsystem,
                "metric":    metric.Name,
                "service":   metric.Service,
                "component": metric.Component,
            }).Set(metric.Value)
        }

        log.Println("Metrics updated successfully.")
        time.Sleep(timeout) // Use the timeout value for sleep duration
    }
}

// Main function to set up the HTTP server and start metrics collection.
func main() {
	script, portStr, timeout := GetArgs()
	port := fmt.Sprintf(":%s", portStr)

	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "custom_metrics",
			Help:      "Custom metrics from script execution",
			Namespace: "prom",
			Subsystem: "custom",
		},
        []string{"system", "subsystem", "metric", "service", "component"},
    )

	prometheus.MustRegister(gauge)
	http.Handle("/metrics", promhttp.Handler())

	go UpdateMetrics(script, gauge, timeout)

	log.Printf("Starting server on port %s...", port)
	if err := http.ListenAndServe(port, nil); err != nil {
	    log.Fatalf("Failed to start server: %v", err)
    }
}
