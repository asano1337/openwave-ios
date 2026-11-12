package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var freeID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var freeClient = &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
	if r.URL.Scheme != "https" || r.URL.Hostname() != "api.freetouse.com" || len(via) > 3 {
		return fmt.Errorf("invalid redirect")
	}
	return nil
}}

type freeTrack struct {
	ID         string              `json:"id"`
	Title      string              `json:"title"`
	Premium    bool                `json:"is_premium"`
	Artists    [][]json.RawMessage `json:"artists"`
	Thumbnails map[string]string   `json:"thumbnails"`
	Files      map[string]string   `json:"files"`
}

func freeGet(u string) ([]freeTrack, error) {
	req, e := http.NewRequest("GET", u, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "OpenWave/4.0 (personal music discovery)")
	r, e := freeClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("catalog unavailable")
	}
	var d struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if e = json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&d); e != nil || !d.OK {
		return nil, fmt.Errorf("invalid catalog response")
	}
	var tracks []freeTrack
	if e = json.Unmarshal(d.Data, &tracks); e != nil {
		var t freeTrack
		if e = json.Unmarshal(d.Data, &t); e != nil {
			return nil, e
		}
		tracks = []freeTrack{t}
	}
	return tracks, nil
}
func freeItem(t freeTrack) (Track, bool) {
	if t.Premium || !freeID.MatchString(t.ID) || strings.TrimSpace(t.Title) == "" {
		return Track{}, false
	}
	u, e := url.Parse(t.Files["mp3"])
	if e != nil || u.Scheme != "https" || u.Hostname() != "eu.data.freetouse.com" || u.User != nil {
		return Track{}, false
	}
	var names []string
	for _, pair := range t.Artists {
		if len(pair) > 1 {
			var a struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(pair[1], &a) == nil && a.Name != "" {
				names = append(names, a.Name)
			}
		}
	}
	artist := strings.Join(names, ", ")
	if artist == "" {
		artist = "Free To Use"
	}
	return Track{ID: t.ID, Title: t.Title, Artist: artist, Cover: t.Thumbnails["md"], Source: "freetouse", Permalink: "https://freetouse.com/music", License: "Free To Use — личное некоммерческое использование с указанием автора и ссылки"}, true
}
func freeSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	mode := r.URL.Query().Get("mode")
	if len([]rune(q)) > 100 {
		fail(w, "Слишком длинный запрос", 400)
		return
	}
	params := url.Values{"limit": {"75"}}
	endpoint := "all"
	if q != "" {
		endpoint = "search"
		params.Set("query", q)
	} else if mode == "wave" {
		params.Set("order", "random")
	} else {
		fail(w, "Введите запрос", 400)
		return
	}
	tracks, e := freeGet("https://api.freetouse.com/v3/music/tracks/" + endpoint + "?" + params.Encode())
	if e != nil {
		fail(w, "Free To Use сейчас недоступен", 502)
		return
	}
	out := []Track{}
	for _, x := range tracks {
		if t, ok := freeItem(x); ok {
			out = append(out, t)
		}
	}
	send(w, map[string]any{"tracks": out})
}
func freeAudio(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !freeID.MatchString(id) {
		fail(w, "Некорректный ID", 400)
		return
	}
	tracks, e := freeGet("https://api.freetouse.com/v3/music/tracks/" + id)
	if e != nil || len(tracks) != 1 {
		fail(w, "Трек недоступен", 502)
		return
	}
	if _, ok := freeItem(tracks[0]); !ok {
		fail(w, "Трек недоступен", 403)
		return
	}
	http.Redirect(w, r, tracks[0].Files["mp3"], http.StatusTemporaryRedirect)
}
