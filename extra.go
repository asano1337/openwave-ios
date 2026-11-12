package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

var musicTransport = func() http.RoundTripper {
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		t := base.Clone()
		// VPNs can replace the route while a keep-alive connection is still
		// alive. Fresh connections make search and playback recover cleanly.
		t.DisableKeepAlives = true
		return t
	}
	return http.DefaultTransport
}()

var musicClient = &http.Client{Transport: musicTransport, Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
	h := strings.ToLower(r.URL.Hostname())
	if len(via) > 5 || r.URL.Scheme != "https" || !(h == "rus.hitmos.fm" || h == "hitmos.fm" || strings.HasSuffix(h, ".hitmos.fm") || h == "zaycev.net" || strings.HasSuffix(h, ".zaycev.net") || h == "zaycev.weyland.zerocdn.com" || h == "zaycev.cdn.zerocdn.com") {
		return fmt.Errorf("invalid redirect")
	}
	return nil
}}

var musicStreamClient = &http.Client{Transport: musicTransport, Timeout: 20 * time.Minute, CheckRedirect: musicClient.CheckRedirect}
var deezerClient = &http.Client{Timeout: 8 * time.Second}

var lyricsClient = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
	if r.URL.Scheme != "https" || r.URL.Hostname() != "lrclib.net" || len(via) > 3 {
		return fmt.Errorf("invalid lyrics redirect")
	}
	return nil
}}

func pageGet(u string) ([]byte, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		req, e := http.NewRequest("GET", u, nil)
		if e != nil {
			return nil, e
		}
		req.Close = true
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenWave/7)")
		res, e := musicClient.Do(req)
		if e == nil && res.StatusCode == http.StatusOK {
			defer res.Body.Close()
			return io.ReadAll(io.LimitReader(res.Body, 6<<20))
		}
		if e != nil {
			last = e
		} else {
			last = fmt.Errorf("status %d", res.StatusCode)
			res.Body.Close()
		}
		time.Sleep(time.Duration(attempt+1) * 350 * time.Millisecond)
	}
	return nil, last
}

var hitmoOrigins = []string{"https://rus.hitmos.fm", "https://hitmos.fm"}

func hitmoPage(path string) ([]byte, error) {
	var last error
	for _, origin := range hitmoOrigins {
		b, e := pageGet(origin + path)
		if e == nil {
			return b, nil
		}
		last = e
	}
	return nil, last
}

var rsc = regexp.MustCompile(`self\.__next_f\.push\((\[1,"(?:\\.|[^"\\])*"\])\)`)
var hitmoTrack = regexp.MustCompile(`\{"id":[0-9]+,"title":"(?:\\.|[^"\\])*","artist":"(?:\\.|[^"\\])*","duration":[0-9]+,"play":"(?:\\.|[^"\\])*","download":"(?:\\.|[^"\\])*","image":"(?:\\.|[^"\\])*"`)

type hitmoRaw struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Play   string `json:"play"`
	Image  string `json:"image"`
}

func parseHitmo(b []byte) []hitmoRaw {
	out := []hitmoRaw{}
	seen := map[int]bool{}
	for _, m := range rsc.FindAllSubmatch(b, -1) {
		var a []json.RawMessage
		if json.Unmarshal(m[1], &a) != nil || len(a) < 2 {
			continue
		}
		var text string
		if json.Unmarshal(a[1], &text) != nil {
			continue
		}
		for _, v := range hitmoTrack.FindAllString(text, -1) {
			var t hitmoRaw
			if json.Unmarshal([]byte(v+"}"), &t) == nil && t.ID > 0 && !seen[t.ID] && validHitmoAudio(t.Play) {
				seen[t.ID] = true
				out = append(out, t)
			}
		}
	}
	return out
}
func validHitmoAudio(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	h := u.Hostname()
	return strings.HasSuffix(h, ".hitmos.fm") && (strings.HasPrefix(h, "pl") || strings.HasPrefix(h, "dw"))
}
func hitmoSearchTracks(q string) ([]Track, error) {
	b, e := hitmoPage("/search?q=" + url.QueryEscape(q))
	if e != nil {
		return nil, e
	}
	out := []Track{}
	for _, t := range parseHitmo(b) {
		out = append(out, Track{ID: strconv.Itoa(t.ID), Title: t.Title, Artist: t.Artist, Cover: t.Image, Source: "hitmo", Permalink: "https://rus.hitmos.fm/song/" + strconv.Itoa(t.ID)})
	}
	return out, nil
}

// Hitmo sometimes serves a tiny preview image. Deezer's public catalog gives
// us a matching 1000px artwork URL without changing the audio source.
func deezerCover(artist, title string) string {
	var d struct {
		Data []struct {
			Title  string `json:"title"`
			Artist struct {
				Name string `json:"name"`
			} `json:"artist"`
			Album struct {
				CoverXL string `json:"cover_xl"`
			} `json:"album"`
		} `json:"data"`
	}
	req, e := http.NewRequest("GET", "https://api.deezer.com/search?"+url.Values{
		"q": {strings.TrimSpace(artist + " " + title)}, "limit": {"3"},
	}.Encode(), nil)
	if e != nil {
		return ""
	}
	req.Header.Set("User-Agent", "OpenWave/7 (cover artwork)")
	res, e := deezerClient.Do(req)
	if e != nil {
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&d) != nil {
		return ""
	}
	norm := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return ' '
		}, s)), " "))
	}
	wantA, wantT := norm(artist), norm(title)
	for _, x := range d.Data {
		a, t := norm(x.Artist.Name), norm(x.Title)
		if x.Album.CoverXL != "" &&
			(wantA == "" || strings.Contains(a, wantA) || strings.Contains(wantA, a)) &&
			(wantT == "" || strings.Contains(t, wantT) || strings.Contains(wantT, t)) {
			return x.Album.CoverXL
		}
	}
	if len(d.Data) > 0 {
		return d.Data[0].Album.CoverXL
	}
	return ""
}

func enrichHitmoCovers(items []Track) []Track {
	if len(items) == 0 {
		return items
	}
	type result struct {
		index int
		cover string
	}
	jobs := make(chan int)
	results := make(chan result)
	var wg sync.WaitGroup
	workers := 8
	if len(items) < workers {
		workers = len(items)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if cover := deezerCover(items[index].Artist, items[index].Title); cover != "" {
					results <- result{index, cover}
				}
			}
		}()
	}
	go func() {
		for i := range items {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	for r := range results {
		items[r.index].Cover = r.cover
	}
	return items
}

func hitSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 || len([]rune(q)) > 100 {
		fail(w, "Запрос должен содержать от 2 до 100 символов", 400)
		return
	}
	out, e := hitmoSearchTracks(q)
	if e != nil {
		fail(w, "Hitmo сейчас недоступен", 502)
		return
	}
	send(w, map[string]any{"tracks": enrichHitmoCovers(out)})
}

var hitmoChartLink = regexp.MustCompile("<a[^>]+title=\"([^\"]+)\"[^>]+class=\"card\" href=\"(/collection/[0-9]+)\"")
var hitmoChartJSON = regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)
var hitmoSongURL = regexp.MustCompile(`/song/([0-9]+)`)

type hitmoChartPage struct {
	Track []struct {
		URL string `json:"url"`
	} `json:"track"`
}

func hitmoChartIDs() []string {
	b, e := hitmoPage("/top_charts")
	if e != nil {
		return nil
	}
	year := strconv.Itoa(time.Now().Year())
	out, seen := []string{}, map[string]bool{}
	for _, m := range hitmoChartLink.FindAllSubmatch(b, -1) {
		title, href := strings.ToLower(string(m[1])), string(m[2])
		wanted := strings.Contains(title, year) ||
			strings.Contains(title, "топ 30") || strings.Contains(title, "топ 40") ||
			strings.Contains(title, "топ 100") || strings.Contains(title, "еврохит") ||
			strings.Contains(title, "популярных хитов") || strings.Contains(title, "лучшие треки недели")
		for _, old := range []string{"2014", "2015", "2016", "2017", "2018", "2019", "2020", "2021", "2022", "2023", "2024"} {
			if strings.Contains(title, old) && old != year {
				wanted = false
			}
		}
		if wanted {
			id := strings.TrimPrefix(href, "/collection/")
			if !seen[id] {
				out = append(out, id)
				seen[id] = true
			}
		}
		if len(out) >= 6 {
			break
		}
	}
	return out
}

func hitmoChartSongIDs(collections []string) []string {
	out, seen := []string{}, map[string]bool{}
	for _, id := range collections {
		b, e := hitmoPage("/collection/" + id)
		if e != nil {
			continue
		}
		for _, match := range hitmoSongURL.FindAllSubmatch(b, -1) {
			if len(match) == 2 && !seen[string(match[1])] {
				out = append(out, string(match[1]))
				seen[string(match[1])] = true
			}
			if len(out) >= 60 {
				break
			}
		}
		if len(out) >= 60 {
			break
		}
	}
	return out
}

func hitmoChartTracks(ids []string) []Track {
	type result struct {
		index int
		track Track
		ok    bool
	}
	jobs := make(chan result)
	results := make(chan result)
	var wg sync.WaitGroup
	workers := 6
	if len(ids) < workers {
		workers = len(ids)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				t, ok := hitmoSong(job.track.ID)
				results <- result{job.index, t, ok}
			}
		}()
	}
	go func() {
		for i, id := range ids {
			jobs <- result{i, Track{ID: id}, true}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	found := make([]result, len(ids))
	for r := range results {
		found[r.index] = r
	}
	out := []Track{}
	for _, r := range found {
		if r.ok {
			out = append(out, r.track)
		}
	}
	return enrichHitmoCovers(out)
}

func hitmoSong(id string) (Track, bool) {
	b, e := hitmoPage("/song/" + id)
	if e != nil {
		return Track{}, false
	}
	for _, t := range parseHitmo(b) {
		if strconv.Itoa(t.ID) == id {
			return Track{ID: id, Title: t.Title, Artist: t.Artist, Cover: t.Image, Source: "hitmo", Permalink: "https://rus.hitmos.fm/song/" + id}, true
		}
	}
	return Track{}, false
}

// hitWave uses Hitmo's current chart collections first. Search results are
// only a fallback, so old generic queries cannot dominate the recommendation.
func hitWave(w http.ResponseWriter, r *http.Request) {
	genre := strings.TrimSpace(r.URL.Query().Get("genre"))
	seen := map[string]bool{}
	personal := []Track{}
	for _, seed := range strings.Split(r.URL.Query().Get("seed"), ",") {
		seed = strings.TrimSpace(seed)
		if seed == "" || len([]rune(seed)) > 80 {
			continue
		}
		tracks, e := hitmoSearchTracks(seed)
		if e != nil {
			continue
		}
		for _, t := range tracks {
			key := strings.ToLower(strings.TrimSpace(t.Title)) + "|" + strings.ToLower(strings.TrimSpace(t.Artist))
			if key == "|" || seen[key] {
				continue
			}
			seen[key] = true
			personal = append(personal, t)
			if len(personal) >= 36 {
				break
			}
		}
		if len(personal) >= 36 {
			break
		}
	}
	out := append(personal, hitmoChartTracks(hitmoChartSongIDs(hitmoChartIDs()))...)
	seen = map[string]bool{}
	unique := make([]Track, 0, len(out))
	for _, t := range out {
		key := strings.ToLower(strings.TrimSpace(t.Title)) + "|" + strings.ToLower(strings.TrimSpace(t.Artist))
		if key == "|" || seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, t)
		if len(unique) >= 60 {
			break
		}
	}
	out = unique
	if len(out) == 0 {
		queries := []string{"новинки " + strconv.Itoa(time.Now().Year()), "топ хиты " + strconv.Itoa(time.Now().Year())}
		if genre != "" {
			queries = append([]string{genre + " " + strconv.Itoa(time.Now().Year())}, queries...)
		}
		for _, q := range queries {
			tracks, e := hitmoSearchTracks(q)
			if e == nil {
				out = append(out, tracks...)
			}
		}
	}
	out = enrichHitmoCovers(out)
	if len(out) == 0 {
		fail(w, "Hitmo сейчас недоступен", 502)
		return
	}
	send(w, map[string]any{"tracks": out, "source": "hitmo", "genre": genre})
}

func lyrics(w http.ResponseWriter, r *http.Request) {
	artist := strings.TrimSpace(r.URL.Query().Get("artist"))
	title := strings.TrimSpace(r.URL.Query().Get("title"))
	geniusURL := "https://genius.com/search?q=" + url.QueryEscape(strings.TrimSpace(artist+" "+title))
	if len([]rune(artist)) > 160 || len([]rune(title)) > 160 || title == "" {
		fail(w, "Некорректное название трека", 400)
		return
	}
	req, e := http.NewRequestWithContext(r.Context(), "GET", "https://lrclib.net/api/get?"+url.Values{
		"artist_name": {artist},
		"track_name":  {title},
	}.Encode(), nil)
	if e != nil {
		fail(w, "Текст песни недоступен", 502)
		return
	}
	req.Header.Set("User-Agent", "OpenWave/6 (lyrics panel)")
	res, e := lyricsClient.Do(req)
	if e != nil {
		fail(w, "Не удалось получить текст песни", 502)
		return
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		send(w, map[string]any{"found": false, "genius": geniusURL})
		return
	}
	if res.StatusCode != http.StatusOK {
		fail(w, "Текст песни недоступен", 502)
		return
	}
	var d struct {
		PlainLyrics  string `json:"plainLyrics"`
		SyncedLyrics string `json:"syncedLyrics"`
		Instrumental bool   `json:"instrumental"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&d) != nil {
		fail(w, "Не удалось прочитать текст песни", 502)
		return
	}
	send(w, map[string]any{
		"found": d.PlainLyrics != "" || d.SyncedLyrics != "",
		"plain": d.PlainLyrics, "synced": d.SyncedLyrics,
		"instrumental": d.Instrumental, "provider": "LRCLIB",
		"genius": geniusURL,
	})
}
func hitAudio(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if len(id) > 12 {
		fail(w, "Некорректный трек", 400)
		return
	}
	n, e := strconv.Atoi(id)
	if e != nil || n <= 0 {
		fail(w, "Некорректный трек", 400)
		return
	}
	b, e := hitmoPage("/song/" + id)
	if e != nil {
		fail(w, "Hitmo недоступен", 502)
		return
	}
	var audio string
	for _, t := range parseHitmo(b) {
		if t.ID == n {
			audio = t.Play
			break
		}
	}
	if audio == "" {
		fail(w, "Трек недоступен", 404)
		return
	}
	req, e := http.NewRequestWithContext(r.Context(), "GET", audio, nil)
	if e != nil {
		fail(w, "Трек недоступен", 502)
		return
	}
	req.Close = true
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://rus.hitmos.fm/")
	if v := r.Header.Get("Range"); v != "" {
		req.Header.Set("Range", v)
	}
	res, e := musicStreamClient.Do(req)
	if e != nil {
		fail(w, "Ошибка загрузки аудио", 502)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		fail(w, "Источник не отдал аудио", 502)
		return
	}
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}

var nextData = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)
var zLink = regexp.MustCompile(`href="(/pages/[0-9]+/([0-9]+)\.shtml)"`)

func zaySearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 || len([]rune(q)) > 100 {
		fail(w, "Запрос должен содержать от 2 до 100 символов", 400)
		return
	}
	b, e := pageGet("https://zaycev.net/search.html?type=all&query_search=" + url.QueryEscape(q))
	if e != nil {
		fail(w, "Zaycev.net сейчас недоступен", 502)
		return
	}
	m := nextData.FindSubmatch(b)
	if len(m) < 2 {
		fail(w, "Zaycev.net изменил формат ответа", 502)
		return
	}
	var d struct {
		Props struct {
			State struct {
				Playlist struct {
					Info map[string]struct {
						Track  string `json:"track"`
						Artist string `json:"artistName"`
						Image  string `json:"imageJpg"`
					} `json:"info"`
					Ids map[string][]int `json:"trackIds"`
				} `json:"playlist"`
			} `json:"initialReduxState"`
		} `json:"props"`
	}
	if json.Unmarshal(m[1], &d) != nil {
		fail(w, "Не удалось прочитать Zaycev.net", 502)
		return
	}
	paths := map[string]string{}
	for _, v := range zLink.FindAllSubmatch(b, -1) {
		paths[string(v[2])] = "https://zaycev.net" + string(v[1])
	}
	out := []Track{}
	for k, ids := range d.Props.State.Playlist.Ids {
		if !strings.HasPrefix(k, "search_") {
			continue
		}
		for _, n := range ids {
			id := strconv.Itoa(n)
			t, ok := d.Props.State.Playlist.Info[id]
			p := paths[id]
			if ok && p != "" {
				out = append(out, Track{ID: id, Title: t.Track, Artist: t.Artist, Cover: t.Image, Source: "zaycev", Permalink: p})
			}
		}
		break
	}
	send(w, map[string]any{"tracks": out})
}

// Zaycev.net's own web player resolves a fresh playback URL for each track.
func zayAudio(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	n, e := strconv.Atoi(id)
	if e != nil || n <= 0 || len(id) > 12 {
		fail(w, "Некорректный трек", 400)
		return
	}
	body := strings.NewReader(fmt.Sprintf(`{"trackIds":[%d],"subscription":null}`, n))
	req, e := http.NewRequestWithContext(r.Context(), "POST", "https://zaycev.net/api/external/track/filezmeta", body)
	if e != nil {
		fail(w, "Ошибка запроса", 502)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://zaycev.net")
	req.Header.Set("Referer", "https://zaycev.net/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	res, e := musicClient.Do(req)
	if e != nil {
		fail(w, "Zaycev.net недоступен", 502)
		return
	}
	var meta struct {
		Tracks []struct {
			ID        int    `json:"id"`
			Streaming string `json:"streaming"`
		} `json:"tracks"`
	}
	err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&meta)
	res.Body.Close()
	if res.StatusCode != 200 || err != nil {
		fail(w, "Не удалось получить трек", 502)
		return
	}
	var hash string
	for _, v := range meta.Tracks {
		if v.ID == n {
			hash = v.Streaming
			break
		}
	}
	if !regexp.MustCompile(`^[a-fA-F0-9]{8,128}$`).MatchString(hash) {
		fail(w, "Трек недоступен для воспроизведения", 404)
		return
	}
	b, e := pageGet("https://zaycev.net/api/external/track/play/" + hash)
	if e != nil {
		fail(w, "Не удалось получить поток", 502)
		return
	}
	var data struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(b, &data) != nil || !validHost(data.URL, "dl.zaycev.net") {
		fail(w, "Недопустимый адрес потока", 502)
		return
	}
	req, e = http.NewRequestWithContext(r.Context(), "GET", data.URL, nil)
	if e != nil {
		fail(w, "Поток недоступен", 502)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://zaycev.net/")
	if v := r.Header.Get("Range"); v != "" {
		req.Header.Set("Range", v)
	}
	res, e = musicStreamClient.Do(req)
	if e != nil {
		fail(w, "Ошибка загрузки потока", 502)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		fail(w, "Аудио недоступно", 502)
		return
	}
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}
