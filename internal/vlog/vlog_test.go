package vlog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDateSavedFromTestFile(t *testing.T) {
	path := filepath.Join("..", "..", "teste", "savedGame")
	if _, err := os.Stat(path); err != nil {
		t.Skip("arquivo de teste não disponível")
	}
	// savedGame in teste/ may be a raw vlog copy; try reading if zip
	raw, err := ReadDateSaved(path)
	if err != nil {
		t.Skipf("arquivo de teste não é .vlog zip válido: %v", err)
	}
	if raw == "" {
		t.Fatal("dateSaved vazio")
	}
}
