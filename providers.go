package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var providerClient = &http.Client{Timeout: 20 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
	h := strings.ToLower(r.URL.Hostname())
	if r.URL.Scheme != "https" || !(h == "ccmixter.org" || strings.HasSuffix(h, ".ccmixter.org") || h == "commons.wikimedia.org" || h == "upload.wikimedia.org") || len(via) > 8 {
		return fmt.Errorf("недопустимый сервер")
	}
	return nil
}}

func providerGet(u string, v any) error {
	req, e := http.NewRequest("GET", u, nil)
	if e != nil {
		return e
	}
	req.Header.Set("User-Agent", "OpenWave/3.0 (desktop music discovery; https://ccmixter.org)")
	resp, e := providerClient.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("источник: %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 15<<20)).Decode(v)
}

type ccFile struct {
	ID   int    `json:"file_id"`
	Name string `json:"file_name"`
	URL  string `json:"download_url"`
	Info struct {
		Media string `json:"media-type"`
	} `json:"file_format_info"`
}
type ccItem struct {
	ID      int      `json:"upload_id"`
	Title   string   `json:"upload_name"`
	User    string   `json:"user_name"`
	Real    string   `json:"user_real_name"`
	Page    string   `json:"file_page_url"`
	License string   `json:"license_name"`
	Files   []ccFile `json:"files"`
}

func ccLoad(p url.Values) ([]ccItem, error) {
	var d []ccItem
	p.Set("f", "json")
	e := providerGet("https://ccmixter.org/api/query?"+p.Encode(), &d)
	return d, e
}
func ccTrack(x ccItem, f ccFile) Track {
	artist := x.Real
	if artist == "" {
		artist = x.User
	}
	return Track{ID: strconv.Itoa(x.ID), FileID: strconv.Itoa(f.ID), File: f.Name, Title: x.Title, Artist: artist, Source: "ccmixter", Downloadable: true, Permalink: x.Page, License: x.License}
}
func validHost(raw, host string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() == host && u.User == nil
}
func ccSelect(x ccItem, fileID string) (Track, string, bool) {
	for _, f := range x.Files {
		if f.Info.Media == "audio" && safeFile(f.Name) && strings.EqualFold(filepath.Ext(f.Name), ".mp3") && validHost(f.URL, "ccmixter.org") && (fileID == "" || strconv.Itoa(f.ID) == fileID) {
			return ccTrack(x, f), f.URL, true
		}
	}
	return Track{}, "", false
}
func ccSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len([]rune(q)) > 100 {
		fail(w, "Некорректный запрос", 400)
		return
	}
	d, e := ccLoad(url.Values{"search": {q}, "search_type": {"any"}, "limit": {"30"}})
	if e != nil {
		fail(w, "ccMixter сейчас недоступен", 502)
		return
	}
	out := []Track{}
	for _, x := range d {
		if t, _, ok := ccSelect(x, ""); ok {
			out = append(out, t)
		}
	}
	send(w, map[string]any{"tracks": out})
}
func ccResolve(id, fileid string) (Track, string, error) {
	if _, e := strconv.Atoi(id); e != nil {
		return Track{}, "", fmt.Errorf("некорректный ID")
	}
	d, e := ccLoad(url.Values{"ids": {id}})
	if e != nil {
		return Track{}, "", e
	}
	for _, x := range d {
		if strconv.Itoa(x.ID) == id {
			if t, u, ok := ccSelect(x, fileid); ok {
				return t, u, nil
			}
		}
	}
	return Track{}, "", fmt.Errorf("файл не найден")
}

type commonsPage struct {
	PageID    int    `json:"pageid"`
	Title     string `json:"title"`
	Index     int    `json:"index"`
	ImageInfo []struct {
		URL         string `json:"url"`
		Description string `json:"descriptionurl"`
		Mime        string `json:"mime"`
		Metadata    map[string]struct {
			Value any `json:"value"`
		} `json:"extmetadata"`
	} `json:"imageinfo"`
}
type commonsResult struct {
	Query struct {
		Pages map[string]commonsPage `json:"pages"`
	} `json:"query"`
}

var tags = regexp.MustCompile(`<[^>]*>`)

func clean(s string) string {
	s = html.UnescapeString(tags.ReplaceAllString(s, ""))
	s = strings.TrimSpace(s)
	if len([]rune(s)) > 150 {
		s = string([]rune(s)[:150])
	}
	return s
}
func commonsLoad(p url.Values) (commonsResult, error) {
	p.Set("action", "query")
	p.Set("format", "json")
	p.Set("prop", "imageinfo")
	p.Set("iiprop", "url|mime|extmetadata")
	var d commonsResult
	e := providerGet("https://commons.wikimedia.org/w/api.php?"+p.Encode(), &d)
	return d, e
}
func commonsTrack(x commonsPage) (Track, string, bool) {
	if len(x.ImageInfo) == 0 {
		return Track{}, "", false
	}
	i := x.ImageInfo[0]
	file := strings.TrimPrefix(x.Title, "File:")
	if !safeFile(file) || !validHost(i.URL, "upload.wikimedia.org") {
		return Track{}, "", false
	}
	artist := clean(asString(i.Metadata["Artist"].Value))
	if artist == "" {
		artist = "Wikimedia Commons"
	}
	return Track{ID: strconv.Itoa(x.PageID), File: file, Title: strings.TrimSuffix(file, filepath.Ext(file)), Artist: artist, Source: "commons", Downloadable: true, Permalink: i.Description, License: clean(asString(i.Metadata["LicenseShortName"].Value))}, i.URL, true
}
func commonsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len([]rune(q)) > 100 {
		fail(w, "Некорректный запрос", 400)
		return
	}
	d, e := commonsLoad(url.Values{"generator": {"search"}, "gsrsearch": {q + " filetype:audio"}, "gsrnamespace": {"6"}, "gsrlimit": {"25"}})
	if e != nil {
		log.Printf("commons: %v", e)
		fail(w, "Wikimedia Commons сейчас недоступен", 502)
		return
	}
	pages := []commonsPage{}
	for _, p := range d.Query.Pages {
		pages = append(pages, p)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Index < pages[j].Index })
	out := []Track{}
	for _, p := range pages {
		if t, _, ok := commonsTrack(p); ok {
			out = append(out, t)
		}
	}
	send(w, map[string]any{"tracks": out})
}
func commonsResolve(id string) (Track, string, error) {
	if _, e := strconv.Atoi(id); e != nil {
		return Track{}, "", fmt.Errorf("некорректный ID")
	}
	d, e := commonsLoad(url.Values{"pageids": {id}})
	if e != nil {
		return Track{}, "", e
	}
	for _, p := range d.Query.Pages {
		if strconv.Itoa(p.PageID) == id {
			if t, u, ok := commonsTrack(p); ok {
				return t, u, nil
			}
		}
	}
	return Track{}, "", fmt.Errorf("файл не найден")
}
func providerFetch(r *http.Request, source, id, fileid string) (*http.Response, Track, error) {
	var t Track
	var u string
	var e error
	if source == "ccmixter" {
		t, u, e = ccResolve(id, fileid)
	} else if source == "commons" {
		t, u, e = commonsResolve(id)
	} else {
		return nil, t, fmt.Errorf("неизвестный источник")
	}
	if e != nil {
		return nil, t, e
	}
	req, e := http.NewRequestWithContext(r.Context(), "GET", u, nil)
	if e != nil {
		return nil, t, e
	}
	req.Header.Set("User-Agent", "OpenWave/3.0 (desktop music discovery; https://ccmixter.org)")
	if source == "ccmixter" {
		req.Header.Set("User-Agent", "Mozilla/5.0 OpenWave/3.0")
		req.Header.Set("Referer", t.Permalink)
	}
	if s := r.Header.Get("Range"); s != "" {
		req.Header.Set("Range", s)
	}
	resp, e := providerClient.Do(req)
	return resp, t, e
}
func providerStream(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	res, _, e := providerFetch(r, source, r.URL.Query().Get("id"), r.URL.Query().Get("fileid"))
	if e != nil {
		fail(w, "Источник недоступен", 502)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		fail(w, "Аудиофайл недоступен", 502)
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
func wave(w http.ResponseWriter, r *http.Request) {
	p := url.Values{"time": {"week"}, "limit": {"50"}, "app_name": {"OpenWave"}}
	g := r.URL.Query().Get("genre")
	for _, allowed := range []string{"Electronic", "Rock", "Hip-Hop/Rap", "Pop", "Jazz"} {
		if g == allowed {
			p.Set("genre", g)
		}
	}
	var d struct {
		Data []audiusTrack `json:"data"`
	}
	if e := audiusJSON("https://api.audius.co/v1/tracks/trending?"+p.Encode(), &d); e != nil {
		fail(w, "Популярные треки Audius сейчас недоступны", 502)
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
