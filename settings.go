package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var settingsMu sync.Mutex

type playerSettings struct {
	Volume int `json:"volume"`
}

func settings(w http.ResponseWriter, r *http.Request) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	path := filepath.Join(dir, "_settings.json")
	if r.Method == "GET" {
		x := playerSettings{Volume: 80}
		if b, e := os.ReadFile(path); e == nil {
			var tmp playerSettings
			if json.Unmarshal(b, &tmp) == nil && tmp.Volume >= 0 && tmp.Volume <= 100 {
				x = tmp
			}
		}
		send(w, x)
		return
	}
	if r.Method != "POST" || r.Header.Get("X-Openwave-Token") != token {
		fail(w, "Доступ запрещён", 403)
		return
	}
	var v playerSettings
	if json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&v) != nil || v.Volume < 0 || v.Volume > 100 {
		fail(w, "Некорректные настройки", 400)
		return
	}
	if os.MkdirAll(dir, 0700) != nil {
		fail(w, "Не удалось сохранить настройки", 500)
		return
	}
	data, _ := json.Marshal(v)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0600) != nil {
		fail(w, "Не удалось сохранить настройки", 500)
		return
	}
	if os.Rename(tmp, path) != nil {
		os.Remove(tmp)
		fail(w, "Не удалось сохранить настройки", 500)
		return
	}
	send(w, map[string]bool{"ok": true})
}
