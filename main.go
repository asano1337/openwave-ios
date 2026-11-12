package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

//go:embed web/*
var assets embed.FS
var client = &http.Client{Timeout: 20 * time.Minute, CheckRedirect: func(r *http.Request, v []*http.Request) error {
	h := r.URL.Hostname()
	if h != "archive.org" && !strings.HasSuffix(h, ".archive.org") {
		return fmt.Errorf("redirect outside archive")
	}
	if len(v) > 8 {
		return fmt.Errorf("too many redirects")
	}
	return nil
}}
var validID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,150}$`)
var last atomic.Int64
var token string
var dir string

type Track struct {
	ID           string `json:"id"`
	File         string `json:"file"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	Cover        string `json:"cover"`
	Local        bool   `json:"local"`
	Key          string `json:"key,omitempty"`
	Source       string `json:"source,omitempty"`
	Downloadable bool   `json:"downloadable,omitempty"`
	Permalink    string `json:"permalink,omitempty"`
	FileID       string `json:"fileId,omitempty"`
	License      string `json:"license,omitempty"`
}

func send(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, s string, code int) {
	w.WriteHeader(code)
	send(w, map[string]string{"error": s})
}
func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		if len(x) > 0 {
			return asString(x[0])
		}
	}
	return ""
}
func getJSON(u string, v any) error {
	r, e := client.Get(u)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("archive status %d", r.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(r.Body, 15<<20)).Decode(v)
}
func safeFile(s string) bool {
	ext := strings.ToLower(filepath.Ext(s))
	return len(s) > 0 && len(s) < 500 && !strings.ContainsAny(s, "\\\x00") && !strings.HasPrefix(s, "/") && !strings.Contains(s, "../") && (ext == ".mp3" || ext == ".ogg" || ext == ".flac" || ext == ".m4a" || ext == ".wav")
}
func dlURL(id, file string) string {
	parts := strings.Split(file, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "https://archive.org/download/" + url.PathEscape(id) + "/" + strings.Join(parts, "/")
}
func search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len([]rune(q)) > 100 {
		fail(w, "Введите запрос до 100 символов", 400)
		return
	}
	q = strings.NewReplacer("\"", " ", "\\", " ").Replace(q)
	p := url.Values{"q": {`collection:opensource_audio AND mediatype:audio AND ("` + q + `")`}, "fl[]": {"identifier", "title", "creator", "downloads"}, "sort[]": {"downloads desc"}, "rows": {"36"}, "output": {"json"}}
	var data struct {
		Response struct {
			Docs []map[string]any `json:"docs"`
			Num  int              `json:"numFound"`
		} `json:"response"`
	}
	if e := getJSON("https://archive.org/advancedsearch.php?"+p.Encode(), &data); e != nil {
		fail(w, "Не удалось связаться с Internet Archive. Проверьте интернет и повторите.", 502)
		return
	}
	type Item struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Artist string `json:"artist"`
		Cover  string `json:"cover"`
	}
	out := []Item{}
	for _, d := range data.Response.Docs {
		id := asString(d["identifier"])
		if !validID.MatchString(id) {
			continue
		}
		artist := asString(d["creator"])
		if artist == "" {
			artist = "Internet Archive"
		}
		title := asString(d["title"])
		if title == "" {
			title = id
		}
		out = append(out, Item{id, title, artist, "https://archive.org/services/img/" + url.PathEscape(id)})
	}
	send(w, map[string]any{"items": out, "total": data.Response.Num})
}
func tracks(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validID.MatchString(id) {
		fail(w, "Некорректный идентификатор", 400)
		return
	}
	var data struct {
		IsDark   bool             `json:"is_dark"`
		Metadata map[string]any   `json:"metadata"`
		Files    []map[string]any `json:"files"`
	}
	if e := getJSON("https://archive.org/metadata/"+url.PathEscape(id), &data); e != nil {
		fail(w, "Не удалось открыть запись архива", 502)
		return
	}
	if data.IsDark || asString(data.Metadata["access-restricted-item"]) == "true" {
		fail(w, "Запись ограничена архивом", 403)
		return
	}
	album := asString(data.Metadata["title"])
	artist := asString(data.Metadata["creator"])
	if artist == "" {
		artist = "Internet Archive"
	}
	cover := "https://archive.org/services/img/" + url.PathEscape(id)
	out := []Track{}
	for _, f := range data.Files {
		name := asString(f["name"])
		if !safeFile(name) || asString(f["private"]) == "true" {
			continue
		}
		title := asString(f["title"])
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
		}
		out = append(out, Track{ID: id, File: name, Title: title, Artist: artist, Album: album, Cover: cover})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].File < out[j].File })
	send(w, map[string]any{"tracks": out, "title": album, "artist": artist, "cover": cover, "source": "https://archive.org/details/" + url.PathEscape(id), "license": asString(data.Metadata["licenseurl"])})
}
func remote(r *http.Request, id, file string) (*http.Response, error) {
	req, e := http.NewRequestWithContext(r.Context(), "GET", dlURL(id, file), nil)
	if e != nil {
		return nil, e
	}
	if x := r.Header.Get("Range"); x != "" {
		req.Header.Set("Range", x)
	}
	return client.Do(req)
}
func stream(w http.ResponseWriter, r *http.Request) {
	id, file := r.URL.Query().Get("id"), r.URL.Query().Get("file")
	if !validID.MatchString(id) || !safeFile(file) {
		fail(w, "Некорректный файл", 400)
		return
	}
	res, e := remote(r, id, file)
	if e != nil {
		fail(w, "Не удалось загрузить аудио", 502)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		fail(w, "Файл недоступен в архиве", res.StatusCode)
		return
	}
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if x := res.Header.Get(h); x != "" {
			w.Header().Set(h, x)
		}
	}
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}
func download(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.Header.Get("X-Openwave-Token") != token {
		fail(w, "Доступ запрещён", 403)
		return
	}
	var t Track
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&t) != nil || !validID.MatchString(t.ID) || (t.Source != "audius" && t.Source != "ccmixter" && t.Source != "commons" && !safeFile(t.File)) {
		fail(w, "Некорректный файл", 400)
		return
	}
	var resp *http.Response
	var e error
	if t.Source == "audius" {
		var trusted Track
		resp, trusted, e = audiusDownload(r, t.ID)
		if e == nil {
			t = trusted
		}
	} else if t.Source == "ccmixter" || t.Source == "commons" {
		var trusted Track
		resp, trusted, e = providerFetch(r, t.Source, t.ID, t.FileID)
		if e == nil {
			t = trusted
		}
	} else {
		resp, e = remote(r, t.ID, t.File)
	}
	if e != nil {
		fail(w, "Скачивание недоступно: "+e.Error(), 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fail(w, "Файл недоступен для скачивания", 502)
		return
	}
	const max = 300 << 20
	if resp.ContentLength > max {
		fail(w, "Файл больше 300 МБ", 400)
		return
	}
	os.MkdirAll(dir, 0700)
	hash := make([]byte, 8)
	rand.Read(hash)
	key := hex.EncodeToString(hash)
	label := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, t.Title))
	label = strings.TrimRight(label, ". ")
	if label == "" {
		label = "Трек"
	}
	rr := []rune(label)
	if len(rr) > 80 {
		label = string(rr[:80])
	}
	filename := key + " - " + label + strings.ToLower(filepath.Ext(t.File))
	tmp := filepath.Join(dir, filename+".part")
	f, e := os.Create(tmp)
	if e != nil {
		fail(w, "Не удалось сохранить файл", 500)
		return
	}
	n, e := io.Copy(f, io.LimitReader(resp.Body, max+1))
	f.Close()
	if e != nil || n > max {
		os.Remove(tmp)
		fail(w, "Скачивание не завершено или файл больше 300 МБ", 500)
		return
	}
	if e = os.Rename(tmp, filepath.Join(dir, filename)); e != nil {
		os.Remove(tmp)
		fail(w, "Не удалось сохранить файл", 500)
		return
	}
	t.Local = true
	t.Key = filename
	b, _ := json.Marshal(t)
	if e = os.WriteFile(filepath.Join(dir, key+".json"), b, 0600); e != nil {
		os.Remove(filepath.Join(dir, filename))
		fail(w, "Не удалось сохранить информацию о треке", 500)
		return
	}
	send(w, map[string]any{"ok": true, "track": t})
}
func library(w http.ResponseWriter, r *http.Request) {
	os.MkdirAll(dir, 0700)
	entries, _ := os.ReadDir(dir)
	out := []Track{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var t Track
		if json.Unmarshal(b, &t) == nil && filepath.Base(t.Key) == t.Key {
			if _, e := os.Stat(filepath.Join(dir, t.Key)); e == nil {
				out = append(out, t)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	send(w, map[string]any{"tracks": out, "folder": dir})
}
func local(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if filepath.Base(key) != key || (!regexp.MustCompile(`^[a-f0-9]{16} - `).MatchString(key) || !safeFile(key)) {
		http.Error(w, "not found", 404)
		return
	}
	http.ServeFile(w, r, filepath.Join(dir, key))
}
func openWindow(address string) {
	if runtime.GOOS == "windows" {
		paths := []string{filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft/Edge/Application/msedge.exe"), filepath.Join(os.Getenv("ProgramFiles"), "Microsoft/Edge/Application/msedge.exe"), filepath.Join(os.Getenv("LocalAppData"), "Microsoft/Edge/Application/msedge.exe")}
		profile := filepath.Join(os.Getenv("LocalAppData"), "OpenWave", "browser")
		for _, p := range paths {
			if _, e := os.Stat(p); e == nil {
				exec.Command(p, "--app="+address, "--user-data-dir="+profile, "--no-first-run").Start()
				return
			}
		}
		exec.Command("rundll32", "url.dll,FileProtocolHandler", address).Start()
	} else {
		exec.Command("xdg-open", address).Start()
	}
}
func main() {
	randBytes := make([]byte, 24)
	rand.Read(randBytes)
	token = hex.EncodeToString(randBytes)
	home, _ := os.UserHomeDir()
	dir = filepath.Join(home, "Music", "OpenWave")
	if runtime.GOOS == "windows" && os.Getenv("USERPROFILE") != "" {
		dir = filepath.Join(os.Getenv("USERPROFILE"), "Music", "OpenWave")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", search)
	mux.HandleFunc("/api/hitmo/search", hitSearch)
	mux.HandleFunc("/api/hitmo/wave", hitWave)
	mux.HandleFunc("/api/hitmo/audio", hitAudio)
	mux.HandleFunc("/api/lyrics", lyrics)
	mux.HandleFunc("/api/zaycev/search", zaySearch)
	mux.HandleFunc("/api/zaycev/audio", zayAudio)
	mux.HandleFunc("/api/cc/search", ccSearch)
	mux.HandleFunc("/api/commons/search", commonsSearch)
	mux.HandleFunc("/api/provider/audio", providerStream)
	mux.HandleFunc("/api/wave", wave)
	mux.HandleFunc("/api/free/search", freeSearch)
	mux.HandleFunc("/api/free/audio", freeAudio)
	mux.HandleFunc("/api/audius/search", audiusSearch)
	mux.HandleFunc("/api/audius/audio", audiusStream)
	mux.HandleFunc("/api/favorites", favorites)
	mux.HandleFunc("/api/settings", settings)
	mux.HandleFunc("/api/tracks", tracks)
	mux.HandleFunc("/api/audio", stream)
	mux.HandleFunc("/api/download", download)
	mux.HandleFunc("/api/library", library)
	mux.HandleFunc("/api/local", local)
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) { last.Store(time.Now().Unix()); w.WriteHeader(204) })
	var srv *http.Server
	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("X-Openwave-Token") != token {
			fail(w, "Доступ запрещён", 403)
			return
		}
		w.WriteHeader(204)
		go func() { time.Sleep(200 * time.Millisecond); srv.Shutdown(context.Background()) }()
	})
	web, _ := fs.Sub(assets, "web")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			data, _ := fs.ReadFile(web, "index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, strings.ReplaceAll(string(data), "__TOKEN__", token))
			return
		}
		http.FileServer(http.FS(web)).ServeHTTP(w, r)
	})
	bindAddress := os.Getenv("OPENWAVE_BIND")
	if bindAddress == "" {
		bindAddress = "127.0.0.1:0"
	}
	listener, e := net.Listen("tcp", bindAddress)
	if e != nil {
		log.Fatal(e)
	}
	srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	last.Store(time.Now().Unix())
	go func() {
		for range time.Tick(30 * time.Second) {
			if time.Now().Unix()-last.Load() > 180 {
				srv.Shutdown(context.Background())
				return
			}
		}
	}()
	addressHost, addressPort, _ := net.SplitHostPort(listener.Addr().String())
	if addressHost == "0.0.0.0" || addressHost == "::" {
		addressHost = firstLANAddress()
	}
	address := "http://" + net.JoinHostPort(addressHost, addressPort)
	if os.Getenv("OPENWAVE_PRINT") != "" {
		fmt.Println(address)
	}
	if os.Getenv("OPENWAVE_NO_OPEN") == "1" {
		if e = srv.Serve(listener); e != nil && e != http.ErrServerClosed {
			log.Fatal(e)
		}
		return
	}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Print(err)
		}
	}()
	runApp(address, srv)
}

// firstLANAddress returns a usable local-network address for a phone or tablet.
// Desktop builds still bind to loopback by default; this is only used when
// OPENWAVE_BIND explicitly asks the server to listen on the LAN.
func firstLANAddress() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.To4().String()
			}
		}
	}
	return "127.0.0.1"
}
