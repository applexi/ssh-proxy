package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SessionLogger struct {
	file *os.File
	llm  LLMConfig
}

// Create a new [SessionLogger] that writes to logDir/session_name.log
//
// Where session_name is username + timestamp specific
func NewSessionLogger(logDir string, username string, llm LLMConfig) (*SessionLogger, error) {
	// Create directory if it does not exist at the config's log directory path
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, fmt.Errorf("Could not create log directory: %v", err)
	}
	// All files should be username + timestamp specific
	timestamp := time.Now().Format(time.RFC3339Nano)
	filename := fmt.Sprintf("%s_%s.log", username, timestamp)
	path := filepath.Join(logDir, filename)

	// Create a write only file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("Create session log file error: %v", err)
	}

	return &SessionLogger{file: file, llm: llm}, nil
}

// Writes all bytes from src to dst and also the session logger's file
func (logger *SessionLogger) ForwardInput(src io.Reader, dst io.Writer) error {
	writer := io.MultiWriter(dst, logger.file)
	_, err := io.Copy(writer, src)
	return err
}

// Closes the logger and the session log file
func (logger *SessionLogger) Close() error {
	if logger.file == nil {
		return nil
	}
	return logger.file.Close()
}

// Closes the logger and summarizes the session log file before closing it
func (logger *SessionLogger) Summarize() error {
	if err := logger.Close(); err != nil {
		return fmt.Errorf("Close session logger error: %w", err)
	}

	if logger.llm.APIKey == "" { return nil }

	logPath := logger.file.Name()
	rawInput, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("Session log could not be read: %v", err)
	}

	summary, err := GenerateSecuritySummary(logger.llm.APIKey, logger.llm.Model, rawInput)
	if err != nil {
		return fmt.Errorf("Error when generating security summary: %v", err)
	}

	summaryPath := strings.TrimSuffix(logPath, filepath.Ext(logPath)) + "_summary.txt"
	if err := os.WriteFile(summaryPath, []byte(summary), 0600); err != nil {
		return fmt.Errorf("Write session summary error: %v", err)
	}
	log.Printf("Session summary available at %s", summaryPath)
	return nil
}
