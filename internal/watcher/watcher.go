package watcher

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rafael/vassal-vlog-sync/internal/vlog"
)

type Handler func(path, dateSaved string)

type Watcher struct {
	dir      string
	onChange Handler
	lastSeen sync.Map
	ignored  sync.Map // dateSaved values to skip (from server download)
	mu       sync.Mutex
}

func New(dir string, onChange Handler) *Watcher {
	return &Watcher{dir: dir, onChange: onChange}
}

func (w *Watcher) IgnoreDateSaved(dateSaved string) {
	w.ignored.Store(dateSaved, true)
}

func (w *Watcher) ClearIgnore(dateSaved string) {
	w.ignored.Delete(dateSaved)
}

func (w *Watcher) ShouldIgnore(dateSaved string) bool {
	_, ok := w.ignored.Load(dateSaved)
	return ok
}

func (w *Watcher) Run() error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fw.Close()

	go w.loop(fw)

	if err := fw.Add(w.dir); err != nil {
		return err
	}
	log.Printf("observando %s — aguardando alterações em arquivos .vlog", w.dir)
	<-make(chan struct{})
	return nil
}

func (w *Watcher) loop(fw *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-fw.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-fw.Errors:
			if !ok {
				return
			}
			log.Println("erro do watcher:", err)
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	if !strings.EqualFold(filepath.Ext(event.Name), ".vlog") {
		return
	}
	if !event.Has(fsnotify.Write | fsnotify.Create) {
		return
	}

	raw, err := vlog.ReadDateSavedWithRetry(event.Name, 5, 150*time.Millisecond)
	if err != nil {
		log.Printf("%s: não foi possível ler <dateSaved>: %v", vlog.Basename(event.Name), err)
		return
	}
	if w.ShouldIgnore(raw) {
		return
	}
	if prev, ok := w.lastSeen.Load(event.Name); ok && prev == raw {
		return
	}
	w.lastSeen.Store(event.Name, raw)
	if w.onChange != nil {
		w.onChange(event.Name, raw)
	}
}
