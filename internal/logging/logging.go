package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/syslog"
	"net/http"
	"os"
	"sync"
	"time"
)

// Logger handles multiple log destinations
type Logger struct {
	syslogWriter *syslog.Writer
	esConfig     *ElasticsearchConfig
	esClient     *http.Client
	localLogger  *log.Logger
	mu           sync.Mutex
}

// ElasticsearchConfig holds Elasticsearch configuration
type ElasticsearchConfig struct {
	Enabled  bool
	Endpoint string
	Index    string
	Username string
	Password string
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp   string                 `json:"timestamp"`
	Level       string                 `json:"level"`
	Message     string                 `json:"message"`
	Service     string                 `json:"service"`
	Hostname    string                 `json:"hostname"`
	Fields      map[string]interface{} `json:"fields,omitempty"`
	Source      string                 `json:"source,omitempty"`
	Method      string                 `json:"method,omitempty"`
	Path        string                 `json:"path,omitempty"`
	StatusCode  int                    `json:"status_code,omitempty"`
	Duration    float64                `json:"duration_ms,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	RemoteAddr  string                 `json:"remote_addr,omitempty"`
	Error       string                 `json:"error,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
}

// Config holds logging configuration
type Config struct {
	SyslogEnabled  bool
	SyslogServer   string // Format: "tcp://host:port" or "udp://host:port" or "unix:///path/to/socket"
	SyslogTag      string
	SyslogPriority string // DEBUG, INFO, WARNING, ERROR, CRITICAL
	Elasticsearch  ElasticsearchConfig
}

// NewLogger creates a new multi-destination logger
func NewLogger(cfg Config) (*Logger, error) {
	logger := &Logger{
		localLogger: log.New(os.Stdout, "", log.LstdFlags),
		esClient:    &http.Client{Timeout: 5 * time.Second},
	}

	// Setup syslog if enabled
	if cfg.SyslogEnabled {
		if err := logger.setupSyslog(cfg); err != nil {
			return nil, fmt.Errorf("failed to setup syslog: %w", err)
		}
	}

	// Setup Elasticsearch if enabled
	if cfg.Elasticsearch.Enabled {
		logger.esConfig = &cfg.Elasticsearch
		if err := logger.testElasticsearch(); err != nil {
			return nil, fmt.Errorf("failed to connect to Elasticsearch: %w", err)
		}
	}

	return logger, nil
}

// setupSyslog configures syslog connection
func (l *Logger) setupSyslog(cfg Config) error {
	priority := syslog.LOG_INFO | syslog.LOG_DAEMON

	// Parse priority
	switch cfg.SyslogPriority {
	case "DEBUG":
		priority = syslog.LOG_DEBUG | syslog.LOG_DAEMON
	case "INFO":
		priority = syslog.LOG_INFO | syslog.LOG_DAEMON
	case "WARNING":
		priority = syslog.LOG_WARNING | syslog.LOG_DAEMON
	case "ERROR":
		priority = syslog.LOG_ERR | syslog.LOG_DAEMON
	case "CRITICAL":
		priority = syslog.LOG_CRIT | syslog.LOG_DAEMON
	}

	tag := cfg.SyslogTag
	if tag == "" {
		tag = "ads-registry"
	}

	var err error
	if cfg.SyslogServer == "" || cfg.SyslogServer == "local" {
		// Local syslog
		l.syslogWriter, err = syslog.New(priority, tag)
	} else {
		// Remote syslog - parse protocol and address
		var network, addr string
		if len(cfg.SyslogServer) > 6 && cfg.SyslogServer[:6] == "tcp://" {
			network = "tcp"
			addr = cfg.SyslogServer[6:]
		} else if len(cfg.SyslogServer) > 6 && cfg.SyslogServer[:6] == "udp://" {
			network = "udp"
			addr = cfg.SyslogServer[6:]
		} else if len(cfg.SyslogServer) > 7 && cfg.SyslogServer[:7] == "unix://" {
			network = "unix"
			addr = cfg.SyslogServer[7:]
		} else {
			// Default to UDP
			network = "udp"
			addr = cfg.SyslogServer
		}

		l.syslogWriter, err = syslog.Dial(network, addr, priority, tag)
	}

	return err
}

// testElasticsearch verifies Elasticsearch connectivity
func (l *Logger) testElasticsearch() error {
	if l.esConfig == nil || !l.esConfig.Enabled {
		return nil
	}

	req, err := http.NewRequest("GET", l.esConfig.Endpoint, nil)
	if err != nil {
		return err
	}

	if l.esConfig.Username != "" {
		req.SetBasicAuth(l.esConfig.Username, l.esConfig.Password)
	}

	resp, err := l.esClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("elasticsearch returned status %d", resp.StatusCode)
	}

	return nil
}

// Log sends a log entry to all configured destinations
func (l *Logger) Log(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Set defaults
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.Service == "" {
		entry.Service = "ads-registry"
	}
	if entry.Hostname == "" {
		entry.Hostname, _ = os.Hostname()
	}

	// Format message for local logging
	msg := fmt.Sprintf("[%s] %s", entry.Level, entry.Message)
	if entry.Error != "" {
		msg += fmt.Sprintf(" error=%s", entry.Error)
	}

	// Log to stdout
	l.localLogger.Println(msg)

	// Log to syslog
	if l.syslogWriter != nil {
		l.logToSyslog(entry)
	}

	// Log to Elasticsearch
	if l.esConfig != nil && l.esConfig.Enabled {
		go l.logToElasticsearch(entry) // Async to not block
	}
}

// logToSyslog sends log to syslog
func (l *Logger) logToSyslog(entry LogEntry) {
	msg := entry.Message
	if entry.Error != "" {
		msg += fmt.Sprintf(" error=%s", entry.Error)
	}

	switch entry.Level {
	case "DEBUG":
		l.syslogWriter.Debug(msg)
	case "INFO":
		l.syslogWriter.Info(msg)
	case "WARNING":
		l.syslogWriter.Warning(msg)
	case "ERROR":
		l.syslogWriter.Err(msg)
	case "CRITICAL":
		l.syslogWriter.Crit(msg)
	default:
		l.syslogWriter.Info(msg)
	}
}

// logToElasticsearch sends log to Elasticsearch
func (l *Logger) logToElasticsearch(entry LogEntry) {
	if l.esConfig == nil || !l.esConfig.Enabled {
		return
	}

	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		l.localLogger.Printf("Failed to marshal log entry for Elasticsearch: %v", err)
		return
	}

	// Create index URL with date suffix
	index := l.esConfig.Index
	if index == "" {
		index = "ads-registry"
	}
	dateStr := time.Now().Format("2006.01.02")
	url := fmt.Sprintf("%s/%s-%s/_doc", l.esConfig.Endpoint, index, dateStr)

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		l.localLogger.Printf("Failed to create Elasticsearch request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	if l.esConfig.Username != "" {
		req.SetBasicAuth(l.esConfig.Username, l.esConfig.Password)
	}

	// Send request
	resp, err := l.esClient.Do(req)
	if err != nil {
		l.localLogger.Printf("Failed to send log to Elasticsearch: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		l.localLogger.Printf("Elasticsearch returned error %d: %s", resp.StatusCode, string(body))
	}
}

// Helper methods for common log levels
func (l *Logger) Debug(message string, fields ...map[string]interface{}) {
	entry := LogEntry{
		Level:   "DEBUG",
		Message: message,
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

func (l *Logger) Info(message string, fields ...map[string]interface{}) {
	entry := LogEntry{
		Level:   "INFO",
		Message: message,
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

func (l *Logger) Warning(message string, fields ...map[string]interface{}) {
	entry := LogEntry{
		Level:   "WARNING",
		Message: message,
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

func (l *Logger) Error(message string, err error, fields ...map[string]interface{}) {
	entry := LogEntry{
		Level:   "ERROR",
		Message: message,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

func (l *Logger) Critical(message string, err error, fields ...map[string]interface{}) {
	entry := LogEntry{
		Level:   "CRITICAL",
		Message: message,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

// LogHTTPRequest logs an HTTP request with details
func (l *Logger) LogHTTPRequest(r *http.Request, statusCode int, duration time.Duration, requestID string) {
	entry := LogEntry{
		Level:      "INFO",
		Message:    "HTTP Request",
		Method:     r.Method,
		Path:       r.URL.Path,
		StatusCode: statusCode,
		Duration:   float64(duration.Microseconds()) / 1000.0, // Convert to ms
		UserAgent:  r.UserAgent(),
		RemoteAddr: r.RemoteAddr,
		RequestID:  requestID,
	}
	l.Log(entry)
}

// Close closes all log connections
func (l *Logger) Close() error {
	if l.syslogWriter != nil {
		return l.syslogWriter.Close()
	}
	return nil
}

// Global logger instance
var globalLogger *Logger

// InitGlobalLogger initializes the global logger
func InitGlobalLogger(cfg Config) error {
	logger, err := NewLogger(cfg)
	if err != nil {
		return err
	}
	globalLogger = logger
	return nil
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() *Logger {
	if globalLogger == nil {
		// Fallback to basic logger
		globalLogger = &Logger{
			localLogger: log.New(os.Stdout, "", log.LstdFlags),
			esClient:    &http.Client{Timeout: 5 * time.Second},
		}
	}
	return globalLogger
}
