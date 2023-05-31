package filereader

import (
	"bufio"
	"os"
)

type FileReader struct {
	file *os.File
}

func NewFileReader(path string) (*FileReader, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &FileReader{
		file: file,
	}, nil
}

func (f *FileReader) Scan() *bufio.Scanner {
	return bufio.NewScanner(f.file)
}

func (f *FileReader) Close() error {
	return f.file.Close()
}
