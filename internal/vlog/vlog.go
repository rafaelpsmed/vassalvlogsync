package vlog

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const savedataZipEntry = "savedata"

var savedataEntryNames = []string{savedataZipEntry, "savedata.xml"}

type savedataDoc struct {
	DateSaved string `xml:"dateSaved"`
}

func ReadDateSaved(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("abrir .vlog como zip: %w", err)
	}
	defer zr.Close()

	for _, entryName := range savedataEntryNames {
		for _, f := range zr.File {
			if !zipEntryMatches(f.Name, entryName) {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("ler entrada %q: %w", f.Name, err)
			}
			dateSaved, err := parseDateSaved(rc)
			rc.Close()
			if err != nil {
				return "", err
			}
			return dateSaved, nil
		}
	}

	return "", fmt.Errorf("entrada %q não encontrada no arquivo", savedataZipEntry)
}

func zipEntryMatches(name, entry string) bool {
	return name == entry || strings.HasSuffix(name, "/"+entry)
}

func parseDateSaved(r io.Reader) (string, error) {
	var doc savedataDoc
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return "", fmt.Errorf("parsear XML de savedata: %w", err)
	}
	if doc.DateSaved == "" {
		return "", fmt.Errorf("elemento <dateSaved> ausente ou vazio")
	}
	return doc.DateSaved, nil
}

func FormatDateSaved(raw string) string {
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	return time.UnixMilli(ms).Format(time.RFC3339)
}

func Basename(path string) string {
	return filepath.Base(path)
}

func ReadDateSavedWithRetry(path string, attempts int, delay time.Duration) (string, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(delay)
		}
		raw, err := ReadDateSaved(path)
		if err == nil {
			return raw, nil
		}
		lastErr = err
	}
	return "", lastErr
}
