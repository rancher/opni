package logger

import (
	"io"
	"sync"
)

var sharedPluginWriter = &pluginWriter{
	mu: &sync.Mutex{},
}

type pluginWriter struct {
	w  *io.Writer
	mu *sync.Mutex
}

func InitPluginWriter(agentId string) {
	sharedPluginWriter.mu.Lock()
	defer sharedPluginWriter.mu.Unlock()
	if sharedPluginWriter.w != nil {
		return
	}
	f := GetFileIfExists(agentId)
	if f == nil {
		f = WriteOnlyFile(agentId)
	}
	fileWriter := f.(io.Writer)
	sharedPluginWriter.w = &fileWriter
}

func (pw pluginWriter) Write(b []byte) (int, error) {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	if pw.w == nil {
		return 0, nil
	}

	n, err := (*pw.w).Write(b)
	if err != nil {
		return n, err
	}

	return n, nil
}
