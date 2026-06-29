package logutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type RotatingWriter struct {
	mu        sync.Mutex
	dir       string
	prefix    string
	file      *os.File
	currentSz int64
	maxSize   int64
	maxFiles  int
	fileDate  string
}

func NewRotatingWriter(dir, prefix string, maxSizeMB, maxFiles int) (*RotatingWriter, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	rw := &RotatingWriter{
		dir:      dir,
		prefix:   prefix,
		maxSize:  int64(maxSizeMB) * 1024 * 1024,
		maxFiles: maxFiles,
	}
	if err := rw.rotate(); err != nil {
		return nil, err
	}
	return rw, nil
}

func (rw *RotatingWriter) rotate() error {
	if rw.file != nil {
		rw.file.Sync()
		rw.file.Close()
	}

	rw.fileDate = time.Now().Format("2006-01-02")
	fileName := filepath.Join(rw.dir, fmt.Sprintf("%s_%s.log", rw.prefix, rw.fileDate))

	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	info, _ := f.Stat()
	if info != nil {
		rw.currentSz = info.Size()
	}

	rw.file = f
	rw.cleanupOldFiles()
	return nil
}

func (rw *RotatingWriter) cleanupOldFiles() {
	pattern := filepath.Join(rw.dir, rw.prefix+"*.log")
	matches, _ := filepath.Glob(pattern)
	if len(matches) <= rw.maxFiles {
		return
	}

	type fInfo struct {
		path string
		mod  time.Time
	}
	var files []fInfo
	for _, m := range matches {
		if filepath.Base(m) == filepath.Base(rw.file.Name()) {
			continue
		}
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		files = append(files, fInfo{path: m, mod: info.ModTime()})
	}

	for i := 0; i < len(files)-rw.maxFiles+1; i++ {
		oldest := 0
		for j := 1; j < len(files); j++ {
			if files[j].mod.Before(files[oldest].mod) {
				oldest = j
			}
		}
		os.Remove(files[oldest].path)
		files = append(files[:oldest], files[oldest+1:]...)
	}
}

func (rw *RotatingWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if today != rw.fileDate {
		rw.rotate()
	}

	if rw.currentSz+int64(len(p)) > rw.maxSize {
		rw.rotate()
	}

	n, err := rw.file.Write(p)
	rw.currentSz += int64(n)
	return n, err
}

func (rw *RotatingWriter) Sync() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.file != nil {
		return rw.file.Sync()
	}
	return nil
}

func (rw *RotatingWriter) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.file != nil {
		return rw.file.Close()
	}
	return nil
}

func (rw *RotatingWriter) WriteSync() zapcore.WriteSyncer {
	return zapcore.AddSync(rw)
}

func NewLogger(dir, prefix string, level zapcore.Level) (*zap.Logger, *RotatingWriter) {
	rw, err := NewRotatingWriter(dir, prefix, 10, 7)
	if err != nil {
		panic(fmt.Sprintf("failed to create log writer: %v", err))
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder := zapcore.NewJSONEncoder(encoderCfg)

	core := zapcore.NewTee(
		zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level),
		zapcore.NewCore(encoder, rw.WriteSync(), level),
	)

	logger := zap.New(core, zap.AddCaller())
	return logger, rw
}
