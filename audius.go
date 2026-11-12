package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var audiusClient = &http.Client{Timeout: 20 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
	host := strings.ToLower(r.URL.Hostname())
	allowed := false
	for _, suffix := range []string{"audius.co", "open-audio-validator.com", "theblueprint.xyz", "figment.io", "monophonic.digital", "r2.cloudflarestorage.com"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			allowed = true
		}
	}
	if r.URL.Scheme != "https" || !allowed || len(via) > 8 {
		return fmt.Errorf("неподдерживаемый сервер Audius")
	}
	return nil
}}

type audiusTrack struct {
	ID                 string          `json:"id"`
	Title              string          `json:"title"`
	Permalink          string          `json:"permalink"`
	IsDownloadable     bool            `json:"is_downloadable"`
	IsStreamGated      bool            `json:"is_stream_gated"`
	IsDownloadGated    bool            `json:"is_download_gated"`
	DownloadConditions json.RawMessage `json:"download_conditions"`
	OrigFilename       string          `json:"orig_filename"`
	User               struct {
		Name string `json:"name"`
	} `json:"user"`
	Artwork map[string]any `json:"artwork"`
}

func permitted(t audiusTrack) bool {
	return t.IsDownloadable && !t.IsDownloadGated && (len(t.DownloadConditions) == 0 || string(t.DownloadConditions) == "null")
}
func audiusJSON(u string, d any) error {
	r, e := audiusClient.Get(u)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("Audius: %d", r.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(r.Body, 15<<20)).Decode(d)
}
func audiusItem(t audiusTrack) Track {
	file := t.OrigFilename
	if !safeFile(file) {
		file = "track.mp3"
	}
	artist := t.User.Name
	if artist == "" {
		artist = "Audius"
	}
	return Track{ID: t.ID, File: file, Title: t.Title, Artist: artist, Cover: asString(t.Artwork["1000x1000"]), Source: "audius", Downloadable: permitted(t), Permalink: "https://audius.co" + t.Permalink}
}
func audiusSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len([]rune(q)) > 100 {
		fail(w, "Введите запрос до 100 символов", 400)
		return
	}
	p := url.Values{"query": {q}, "limit": {"40"}, "app_name": {"OpenWave"}}
	var d struct {
		Data []audiusTrack `json:"data"`
	}
	if e := audiusJSON("https://api.audius.co/v1/tracks/search?"+p.Encode(), &d); e != nil {
		fail(w, "Audius сейчас недоступен", 502)
		return
	}
	out := []Track{}
	for _, t := range d.Data {
		if validID.MatchString(t.ID) && !t.IsStreamGated {
			out = append(out, audiusItem(t))
		}
	}
	send(w, map[string]any{"tracks": out})
}
func audiusFetch(r *http.Request, id string, download bool) (*http.Response, error) {
	if !validID.MatchString(id) {
		return nil, fmt.Errorf("bad id")
	}
	path := "stream"
	if download {
		path = "download"
	}
	u := "https://api.audius.co/v1/tracks/" + url.PathEscape(id) + "/" + path + "?app_name=OpenWave"
	req, e := http.NewRequestWithContext(r.Context(), "GET", u, nil)
	if e != nil {
		return nil, e
	}
	if v := r.Header.Get("Range"); v != "" {
		req.Header.Set("Range", v)
	}
	return audiusClient.Do(req)
}
func audiusStream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validID.MatchString(id) {
		fail(w, "Некорректный трек", 400)
		return
	}
	res, e := audiusFetch(r, id, false)
	if e != nil {
		fail(w, "Не удалось подключиться к Audius", 502)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		fail(w, "Трек Audius недоступен", res.StatusCode)
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
func audiusDownload(r *http.Request, id string) (*http.Response, Track, error) {
	var d struct {
		Data audiusTrack `json:"data"`
	}
	if !validID.MatchString(id) {
		return nil, Track{}, fmt.Errorf("Некорректный трек")
	}
	if e := audiusJSON("https://api.audius.co/v1/tracks/"+url.PathEscape(id), &d); e != nil {
		return nil, Track{}, e
	}
	if !permitted(d.Data) {
		return nil, Track{}, fmt.Errorf("Автор не разрешил бесплатное скачивание")
	}
	resp, e := audiusFetch(r, id, true)
	return resp, audiusItem(d.Data), e
}
