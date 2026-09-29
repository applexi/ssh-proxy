package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type SessionLogger struct {
	file *os.File
}

// Create a new [SessionLogger] that writes to logDir/session_name.log
//
// Where session_name is username + timestamp specific
func NewSessionLogger(logDir string, username string) (*SessionLogger, error) {
	// Create directory if it does not exist at the config's log directory path
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, fmt.Errorf("Could not create log directory: %v", err)
	}
	// All files should be username + timestamp specific
	timestamp := time.Now().Format(time.RFC3339Nano)
	filename := fmt.Sprintf("%s_%s.log", username, timestamp)
	path := filepath.Join(logDir, filename)

	// Create a write only file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("Create session log file error: %v", err)
	}

	return &SessionLogger{file: file}, nil
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
