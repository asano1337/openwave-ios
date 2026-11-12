package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var favMu sync.Mutex

func favorites(w http.ResponseWriter, r *http.Request) {
	favMu.Lock()
	defer favMu.Unlock()
	path := filepath.Join(dir, "_favorites.json")
	if r.Method == "GET" {
		out := []Track{}
		b, e := os.ReadFile(path)
		if e == nil {
			json.Unmarshal(b, &out)
		}
		send(w, map[string]any{"tracks": out})
		return
	}
	if r.Method != "POST" || r.Header.Get("X-Openwave-Token") != token {
		fail(w, "Доступ запрещён", 403)
		return
	}
	var items []Track
	if json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&items) != nil || len(items) > 5000 {
		fail(w, "Не удалось сохранить избранное", 400)
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		fail(w, "Не удалось сохранить избранное", 500)
		return
	}
	b, _ := json.Marshal(items)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		fail(w, "Не удалось сохранить избранное", 500)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		fail(w, "Не удалось сохранить избранное", 500)
		return
	}
	send(w, map[string]any{"ok": true})
}
