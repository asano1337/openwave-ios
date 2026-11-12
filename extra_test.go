package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestParseHitmo(t *testing.T) {
	v := `{"id":123,"title":"Тест","artist":"Артист","duration":120,"play":"https://pl1.hitmos.fm/a.mp3","download":"https://dw1.hitmos.fm/a.mp3","image":"https://static-c1.hitmos.fm/a.webp"}`
	payload, _ := json.Marshal([]any{1, v})
	html := []byte(fmt.Sprintf(`<script>self.__next_f.push(%s)</script>`, payload))
	got := parseHitmo(html)
	if len(got) != 1 || got[0].ID != 123 || got[0].Title != "Тест" {
		t.Fatalf("incorrect parse: %+v", got)
	}
}
func TestHitmoAudioHost(t *testing.T) {
	for _, v := range []struct {
		u  string
		ok bool
	}{{"https://pl1.hitmos.fm/test.mp3", true}, {"http://pl1.hitmos.fm/test.mp3", false}, {"https://pl1.hitmos.fm.evil.org/a", false}, {"https://evil.org/test.mp3", false}, {"https://evil@pl1.hitmos.fm/test.mp3", false}} {
		if validHitmoAudio(v.u) != v.ok {
			t.Errorf("unexpected decision for %s", v.u)
		}
	}
}
