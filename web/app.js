const $ = (s) => document.querySelector(s),
  $$ = (s) => Array.from(document.querySelectorAll(s));
const esc = (s) =>
  String(s ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const icon = (name) => `<svg aria-hidden="true"><use href="#i-${name}"/></svg>`;
const api = async (url, opt) => {
  const r = await fetch(url, opt);
  let d;
  try {
    d = await r.json();
  } catch {
    d = {};
  }
  if (!r.ok) throw Error(d.error || "Не удалось выполнить запрос");
  return d;
};
function mountAmbient() {
  const root = $("#starfield");
  if (!root) return;
  const fragment = document.createDocumentFragment();
  for (let i = 0; i < 64; i++) {
    const star = document.createElement("i");
    star.className = i % 7 === 0 ? "falling-star comet" : "falling-star";
    star.style.setProperty("--x", `${(i * 29 + 7) % 101}%`);
    star.style.setProperty("--delay", `${-((i * 1.73) % 18)}s`);
    star.style.setProperty("--duration", `${13 + (i % 7) * 2.4}s`);
    star.style.setProperty("--size", `${i % 9 === 0 ? 4 : i % 5 === 0 ? 3 : 1 + (i % 3) * 0.55}px`);
    star.style.setProperty("--drift", `${-35 + ((i * 17) % 80)}px`);
    star.style.setProperty("--alpha", `${0.5 + (i % 4) * 0.14}`);
    fragment.appendChild(star);
  }
  root.appendChild(fragment);
}
mountAmbient();
const auth = {
  "Content-Type": "application/json",
  "X-Openwave-Token": window.APP_TOKEN,
};
let currentPage = "home",
  archives = [],
  audius = [],
  ccmixter = [],
  commons = [],
  freetouse = [],
  hitmo = [],
  zaycev = [],
  waveTracks = [],
  waveLoaded = false,
  shuffle = false,
  repeat = false,
  albumTracks = [],
  libraryTracks = [],
  favorites = [],
  queue = [],
  trackIndex = -1,
  searchVersion = 0,
  lastAlbum = null,
  showAllAudius = false,
  favSave = Promise.resolve(),
  queueIsWave = false,
  failedInRow = 0,
  waveHistory = (() => { try { return JSON.parse(localStorage.getItem("openwaveWaveHistory") || "[]"); } catch (_) { return []; } })();
function readLocal(key, fallback) {
  try {
    const value = JSON.parse(localStorage.getItem(key) || "null");
    return value == null ? fallback : value;
  } catch (_) { return fallback; }
}
function writeLocal(key, value) {
  try { localStorage.setItem(key, JSON.stringify(value)); } catch (_) {}
}
let searchHistory = readLocal("openwaveSearchHistory", []);
let listenHistory = readLocal("openwaveListenHistory", []);
if (!Array.isArray(searchHistory)) searchHistory = [];
if (!Array.isArray(listenHistory)) listenHistory = [];
const audio = $("#audio");
audio.volume = 0.8;
let audioContext = null, analyser = null, audioSource = null, equalizerFrame = 0;
const signalBars = $$(".signal-lines i");
function paintEqualizer() {
  if (!analyser || audio.paused) return;
  const data = new Uint8Array(analyser.frequencyBinCount);
  analyser.getByteFrequencyData(data);
  signalBars.forEach((bar, i) => {
    const sample = Math.min(data.length - 1, Math.floor(Math.pow(i / Math.max(1, signalBars.length - 1), 1.35) * (data.length - 1)));
    const energy = data[sample] / 255;
    bar.style.height = `${Math.max(8, Math.min(100, 8 + energy * 92))}%`;
  });
  equalizerFrame = requestAnimationFrame(paintEqualizer);
}
function startEqualizer() {
  try {
    if (!audioContext) {
      const AudioContext = window.AudioContext || window.webkitAudioContext;
      if (!AudioContext) return;
      audioContext = new AudioContext();
      analyser = audioContext.createAnalyser();
      analyser.fftSize = 128;
      analyser.smoothingTimeConstant = 0.76;
      audioSource = audioContext.createMediaElementSource(audio);
      audioSource.connect(analyser);
      analyser.connect(audioContext.destination);
    }
    if (audioContext.state === "suspended") audioContext.resume();
    cancelAnimationFrame(equalizerFrame);
    $(".signal-lines")?.classList.add("live");
    paintEqualizer();
  } catch (_) {
    // Audio playback must keep working even if Web Audio is unavailable.
  }
}
function stopEqualizer() {
  cancelAnimationFrame(equalizerFrame);
  $(".signal-lines")?.classList.remove("live");
  signalBars.forEach((bar) => { bar.style.height = "8%"; });
}
let mediaSessionReady = false;
function updateMediaSession(track, playing) {
  if (!track || !("mediaSession" in navigator) || !("MediaMetadata" in window)) return;
  try {
    navigator.mediaSession.metadata = new MediaMetadata({
      title: track.title || "OpenWave",
      artist: track.artist || "OpenWave",
      album: track.album || "OpenWave",
      artwork: track.cover ? [{ src: track.cover, sizes: "1000x1000", type: "image/jpeg" }] : [],
    });
    navigator.mediaSession.playbackState = playing ? "playing" : "paused";
    if (!mediaSessionReady) {
      const actions = {
        play: () => audio.play().catch(() => {}),
        pause: () => audio.pause(),
        nexttrack: () => nextTrack(),
        previoustrack: () => $("#prev").click(),
        seekbackward: () => { audio.currentTime = Math.max(0, audio.currentTime - 10); },
        seekforward: () => { audio.currentTime = Math.min(audio.duration || Infinity, audio.currentTime + 10); },
      };
      Object.entries(actions).forEach(([name, handler]) => {
        try { navigator.mediaSession.setActionHandler(name, handler); } catch (_) {}
      });
      mediaSessionReady = true;
    }
  } catch (_) {}
}
const identity = (t) =>
  (t.source || "archive") +
  "|" +
  t.id +
  "|" +
  (t.source === "audius" ? "" : t.source === "ccmixter" ? t.fileId : (t.file || ""));
const isFav = (t) => favorites.some((f) => identity(f) === identity(t));
const localVersion = (t) =>
  libraryTracks.find((x) => identity(x) === identity(t)) || t;
function tasteArtists() {
  let preferred = [];
  try { preferred = JSON.parse(localStorage.getItem("openwaveTaste") || "[]"); } catch (_) {}
  const liked = favorites.map((t) => t.artist).filter(Boolean);
  return [...new Set([...preferred, ...liked])].slice(0, 8);
}
function rememberSearch(query) {
  const q = String(query || "").trim();
  if (!q) return;
  searchHistory = [q, ...searchHistory.filter((x) => x.toLocaleLowerCase() !== q.toLocaleLowerCase())].slice(0, 30);
  writeLocal("openwaveSearchHistory", searchHistory);
}
function rememberListening(track) {
  if (!track || track.source === "radio") return;
  const item = { ...track, playedAt: Date.now() };
  listenHistory = [item, ...listenHistory.filter((x) => identity(x) !== identity(track))].slice(0, 40);
  writeLocal("openwaveListenHistory", listenHistory);
}
let suggestionTimer = 0;
let suggestionRequest = 0;
function suggestionItems(query = "", remote = []) {
  const q = String(query || "").trim().toLocaleLowerCase();
  const tracks = [
    ...remote,
    ...hitmo,
    ...zaycev,
    ...audius,
    ...favorites,
    ...listenHistory,
    ...waveTracks,
  ];
  const artists = [
    ...tasteArtists().map((artist) => ({ artist, title: "Исполнитель", kind: "artist" })),
    ...tracks.map((track) => ({ artist: track.artist, title: track.title, kind: "track", track })),
  ];
  const seen = new Set();
  return artists.filter((item) => {
    const value = String(item.artist || item.title || "").trim();
    if (!value || (q && !`${value} ${item.title || ""}`.toLocaleLowerCase().includes(q))) return false;
    const key = `${item.kind}|${value.toLocaleLowerCase()}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  }).slice(0, 8);
}
function hideSuggestions() {
  const root = $("#search-suggestions");
  if (!root) return;
  root.hidden = true;
  root.innerHTML = "";
}
function renderSuggestions(query = "", remote = []) {
  const root = $("#search-suggestions");
  if (!root) return;
  const items = suggestionItems(query, remote);
  if (!items.length) {
    hideSuggestions();
    return;
  }
  root.innerHTML = items.map((item) => {
    const value = item.kind === "artist" ? item.artist : item.title;
    const subtitle = item.kind === "artist"
      ? "Исполнитель"
      : `${item.artist || "Неизвестный исполнитель"} · трек`;
    const image = item.track?.cover || "/logo.png";
    return `<button class="suggestion" type="button" role="option" data-query="${esc(value)}">
      <img src="${esc(image)}" alt="" onerror="this.src='/logo.png'">
      <span><b>${esc(value)}</b><small>${esc(subtitle)}</small></span>
      <svg><use href="#i-search" /></svg>
    </button>`;
  }).join("");
  root.hidden = false;
  root.querySelectorAll(".suggestion").forEach((button) => {
    button.onclick = () => {
      const value = button.dataset.query || "";
      $("#global-query").value = value;
      hideSuggestions();
      page("search");
      $("#query").value = value;
      doSearch();
    };
  });
}
function scheduleSuggestions(value) {
  const query = String(value || "").trim();
  const seq = ++suggestionRequest;
  clearTimeout(suggestionTimer);
  renderSuggestions(query);
  if (query.length < 2) return;
  suggestionTimer = setTimeout(async () => {
    try {
      const data = await api("/api/hitmo/search?q=" + encodeURIComponent(query), {
        signal: AbortSignal.timeout(9000),
      });
      if (seq === suggestionRequest) renderSuggestions(query, data.tracks || []);
    } catch (_) {
      // Local artists, likes and history remain available if the catalog is asleep.
    }
  }, 260);
}
function renderHistory() {
  const root = $("#history-content");
  if (!root) return;
  const searchChips = searchHistory.length
    ? searchHistory.slice(0, 12).map((q) => `<button class="history-search" data-query="${esc(q)}">${esc(q)}</button>`).join("")
    : `<span class="history-empty">Поисковые запросы появятся здесь.</span>`;
  root.innerHTML = `<div class="history-heading"><div><div class="eyebrow muted">НЕ ТЕРЯЙ НАСТРОЕНИЕ</div><h2 id="history-title">История</h2></div>${listenHistory.length ? `<button class="history-clear" id="clear-history">Очистить</button>` : ""}</div>
    <div class="history-searches"><b>Последние поиски</b><div class="history-chips">${searchChips}</div></div>
    ${listenHistory.length ? `<div class="history-played"><b>Недавно слушал</b>${trackRows(listenHistory.slice(0, 12))}</div>` : `<div class="history-empty-card">${icon("history")}<strong>История прослушивания пуста</strong><span>Включи трек — он сохранится на этом телефоне.</span></div>`}`;
  root.querySelectorAll(".history-search").forEach((b) => b.onclick = () => {
    page("search");
    $("#query").value = b.dataset.query;
    doSearch();
  });
  root.querySelector("#clear-history")?.addEventListener("click", () => {
    listenHistory = [];
    searchHistory = [];
    writeLocal("openwaveListenHistory", []);
    writeLocal("openwaveSearchHistory", []);
    renderHistory();
    toast("История очищена");
  });
  const rows = root.querySelector(".history-played");
  if (rows) bindTracks(rows, listenHistory.slice(0, 12));
}
function closeOnboarding() {
  $("#onboarding").hidden = true;
  document.body.classList.remove("onboarding-open");
}
function saveOnboarding() {
  const typed = $("#onboarding-artists").value.split(",").map((x) => x.trim()).filter(Boolean);
  const picked = $$(".artist-pick.selected").map((x) => x.dataset.artist);
  const artists = [...new Set([...picked, ...typed])].slice(0, 8);
  try {
    localStorage.setItem("openwaveTaste", JSON.stringify(artists));
    localStorage.setItem("openwaveOnboarded", "1");
  } catch (_) {}
  closeOnboarding();
  toast(artists.length ? "Вкус сохранён — волна будет подстраиваться под тебя" : "Хорошо, начнём с актуальных хитов");
}
$$(".artist-pick").forEach((button) => button.onclick = () => {
  button.classList.toggle("selected");
  const values = $$(".artist-pick.selected").map((x) => x.dataset.artist);
  $("#onboarding-artists").value = values.join(", ");
});
$("#onboarding-start").onclick = saveOnboarding;
$("#onboarding-skip").onclick = () => {
  try { localStorage.setItem("openwaveOnboarded", "1"); } catch (_) {}
  closeOnboarding();
};
let hasOnboarded = false;
try { hasOnboarded = localStorage.getItem("openwaveOnboarded") === "1"; } catch (_) {}
if (!hasOnboarded) {
  setTimeout(() => { $("#onboarding").hidden = false; document.body.classList.add("onboarding-open"); }, 350);
}
let toastTimer;
function toast(text) {
  const el = $("#toast");
  el.textContent = text;
  el.classList.add("visible");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.remove("visible"), 4500);
}
function refreshCount() {
  $("#fav-count").textContent = favorites.length || "";
  let t = queue[trackIndex];
  $("#now-fav").classList.toggle("faved", !!t && isFav(t));
  $("#now-fav").setAttribute(
    "aria-label",
    t && isFav(t) ? "Убрать из избранного" : "Добавить в избранное",
  );
}
function persistFavorites() {
  const snapshot = JSON.stringify(favorites);
  writeLocal("openwaveFavorites", favorites);
  favSave = favSave
    .then(() =>
      api("/api/favorites", { method: "POST", headers: auth, body: snapshot }),
    )
    .catch(() => toast("Не удалось сохранить избранное на компьютере"));
}
function toggleFav(t) {
  if (!t) return;
  let idx = favorites.findIndex((f) => identity(f) === identity(t));
  if (idx >= 0) {
    favorites.splice(idx, 1);
    toast("Убрано из избранного");
  } else {
    let f = { ...t };
    if (f.local) {
      f.local = false;
      delete f.key;
    }
    favorites.unshift(f);
    toast("Добавлено в избранное");
  }
  writeLocal("openwaveFavorites", favorites);
  persistFavorites();
  refreshCount();
  if (currentPage === "favorites") { renderFavorites(); renderHistory(); }
  if (currentPage === "search") renderSearch();
  if (coverTrack && currentPage !== "cover") { const x=$("#cover-fav"); x.classList.toggle("faved",isFav(coverTrack)); }
  if (currentPage === "album" && lastAlbum) renderAlbum();
  if (currentPage === "library") renderLibrary();
  if (currentPage === "wave") renderWave();
  if (currentPage === "radio") renderRadio();
}
function page(name, load = true) {
  currentPage = name;
  $$(".view").forEach((x) => x.classList.add("hidden"));
  $("#" + name).classList.remove("hidden");
  $$(".nav").forEach((x) =>
    x.classList.toggle(
      "active",
      x.dataset.page === (name === "album" ? "search" : name),
    ),
  );
  $("main").scrollTop = 0;
  if (name === "library") loadLibrary(true);
  if (name === "favorites") { renderFavorites(); renderHistory(); }
  if (name === "radio") renderRadio();
  if (name === "wave" && !waveLoaded && load) loadWave();
  if (name === "search") setTimeout(() => $("#query").focus(), 30);
}
$$(".nav").forEach((b) => (b.onclick = () => page(b.dataset.page)));
$("#go-search").onclick = () => page("search");
$("#global-query").addEventListener("input", (e) => scheduleSuggestions(e.target.value));
$("#global-query").addEventListener("focus", (e) => renderSuggestions(e.target.value));
$("#global-search-form").onsubmit = (e) => {
  e.preventDefault();
  const q = $("#global-query").value.trim();
  if (!q) {
    page("search");
    $("#query").focus();
    return;
  }
  page("search");
  $("#query").value = q;
  hideSuggestions();
  doSearch();
};
document.addEventListener("click", (e) => {
  if (!e.target.closest("#global-search-form, #search-suggestions")) hideSuggestions();
});
$("#go-radio").onclick = () => page("radio");
$("#go-wave").onclick = async () => {
  page("wave", false);
  if (!waveLoaded) await loadWave();
  if (waveTracks.length) playTrack(waveTracks, 0);
};
$("#back").onclick = () => page("search");
$$(".genre").forEach(
  (b) =>
    (b.onclick = () => {
      page("search");
      $("#query").value = b.dataset.query;
      doSearch();
    }),
);
$("#search-form").onsubmit = (e) => {
  e.preventDefault();
  $("#global-query").value = $("#query").value;
  doSearch();
};
async function doSearch() {
  const q = $("#query").value.trim();
  if (!q) {
    toast("Введи название или имя исполнителя");
    return;
  }
  rememberSearch(q);
  $("#global-query").value = q;
  const seq = ++searchVersion;
  audius = [];
  ccmixter = [];
  commons = [];
  freetouse = [];
  archives = [];
  hitmo = []; zaycev = [];
  $("#soundcloud-search").href = "https://soundcloud.com/search/sounds?q=" + encodeURIComponent(q);
  $("#hitmo-search").href = "https://rus.hitmos.fm/search?q=" + encodeURIComponent(q);
  $("#zaycev-search").href = "https://zaycev.net/search.html?type=all&query_search=" + encodeURIComponent(q);
  let status = {
    audius: "loading",
    ccmixter: "loading",
    commons: "loading",
    freetouse: "loading",
    archive: "loading",
    hitmo: "loading", zaycev: "loading",
  };
  const root = $("#search-content");
  function paint() {
    if (seq !== searchVersion) return;
    renderSearch(status);
  }
  paint();
  const providers = [
    ["hitmo", "/api/hitmo/search?q="],
    ["zaycev", "/api/zaycev/search?q="],
    ["audius", "/api/audius/search?q="],
    ["ccmixter", "/api/cc/search?q="],
    ["commons", "/api/commons/search?q="],
    ["freetouse", "/api/free/search?q="],
    ["archive", "/api/search?q="],
  ];
  await Promise.all(
    providers.map(async ([name, url]) => {
      try {
        let d = await api(url + encodeURIComponent(q), {
          signal: AbortSignal.timeout(45000),
        });
        if (seq !== searchVersion) return;
        if (name === "hitmo") {
          hitmo = d.tracks || [];
          try { localStorage.setItem("openwaveSearch:" + q, JSON.stringify(hitmo.slice(0, 40))); } catch (_) {}
        }
        if (name === "zaycev") zaycev = d.tracks || [];
        if (name === "audius") audius = d.tracks || [];
        if (name === "ccmixter") ccmixter = d.tracks || [];
        if (name === "commons") commons = d.tracks || [];
        if (name === "freetouse") freetouse = d.tracks || [];
        if (name === "archive") archives = d.items || [];
        status[name] = "ok";
      } catch (e) {
        if (name === "hitmo") {
          try {
            const cached = JSON.parse(localStorage.getItem("openwaveSearch:" + q) || "[]");
            if (Array.isArray(cached) && cached.length) {
              hitmo = cached;
              status[name] = "cached";
              paint();
              return;
            }
          } catch (_) {}
        }
        status[name] = "error";
      }
      paint();
    }),
  );
}
function sourceLabel(t) {
  if (t.local) return "НА ПК";
  return ({hitmo:"HITMO", zaycev:"ZAYCEV.NET", audius:"AUDIUS", ccmixter:"CCMIXTER", commons:"COMMONS", freetouse:"FREE TO USE", radio:"RADIO RECORD"}[t.source] || "АРХИВ");
}
function trackRows(arr, local = false, limit = 0) {
  if (!arr.length)
    return '<div class="empty"><div class="empty-icon">' + icon("heart") + '</div><h2>Здесь пока пусто</h2><p>Добавь понравившиеся треки или сохрани их на компьютер.</p></div>';
  let shown = limit ? arr.slice(0, limit) : arr;
  return `<div class="table-label"><span>#</span><span>ТРЕК</span><span></span><span></span><span></span></div>${shown.map((t, i) => `<div class="track"><div class="track-num">${String(i + 1).padStart(2, "0")}</div><div class="track-main"><button class="track-art-button" data-cover-index="${i}" aria-label="Открыть обложку ${esc(t.title)}"><img class="track-art" src="${esc(t.cover || "/logo.png")}" alt="" loading="lazy" onerror="this.src='/logo.png'"></button><div class="track-title"><b title="${esc(t.title)}">${esc(t.title)}</b><button class="track-artist" data-artist="${esc(t.artist || "")}" title="Открыть песни ${esc(t.artist || "")}">${esc(t.artist || "Неизвестный исполнитель")}</button></div></div><button class="icon-button fav-row ${isFav(t) ? "faved" : ""}" data-index="${i}" aria-label="${isFav(t) ? "Убрать из" : "Добавить в"} избранное">${icon("heart")}</button><button class="icon-button download-row" data-index="${i}" ${local || t.source === "freetouse" || t.source === "radio" || t.source === "hitmo" || t.source === "zaycev" || (t.source === "audius" && !t.downloadable) ? "disabled" : ""} title="${local ? "Уже скачано" : (t.source === "hitmo" || t.source === "zaycev") ? "Скачивание через каталог недоступно" : t.source === "radio" ? "Прямой эфир нельзя скачать" : t.source === "freetouse" ? "Открой источник: скачивание в приложении недоступно" : t.source === "audius" && !t.downloadable ? "Автор не разрешил скачивание" : "Скачать"}" aria-label="Скачать ${esc(t.title)}">${icon("down")}</button><button class="icon-button play-row" data-index="${i}" aria-label="Слушать ${esc(t.title)}">${icon("play")}</button></div>`).join("")}`;
}
function bindTracks(root, arr) {
  root.querySelectorAll(".track-art-button").forEach((b) => b.onclick = () => openCover(arr[Number(b.dataset.coverIndex)]));
  root.querySelectorAll(".track-artist").forEach((b) => b.onclick = (e) => {
    e.stopPropagation();
    const artist = b.dataset.artist.trim();
    if (!artist) return;
    page("search");
    $("#query").value = artist;
    doSearch();
  });
  root
    .querySelectorAll(".play-row")
    .forEach(
      (b) => (b.onclick = () => playTrack(arr, Number(b.dataset.index))),
    );
  root
    .querySelectorAll(".fav-row")
    .forEach(
      (b) => (b.onclick = () => toggleFav(arr[Number(b.dataset.index)])),
    );
  root
    .querySelectorAll(".download-row:not(:disabled)")
    .forEach(
      (b) => (b.onclick = () => saveTrack(arr[Number(b.dataset.index)], b)),
    );
}
let lastStatus = {};
function renderSearch(status = lastStatus) {
  lastStatus = status;
  const root = $("#search-content");
  const priority = {hitmo: 0, zaycev: 1, audius: 2, ccmixter: 3, commons: 4, freetouse: 5};
  const direct = [...hitmo, ...zaycev, ...audius, ...ccmixter, ...commons, ...freetouse];
  const seen = new Set();
  const all = direct.filter(t => { const k = (t.title + "|" + t.artist).toLocaleLowerCase(); if (seen.has(k)) return false; seen.add(k); return true; }).sort((a,b) => (priority[a.source] ?? 9) - (priority[b.source] ?? 9));
  const loading = Object.values(status).some(x => x === "loading");
  const errors = Object.entries(status).filter(([,x]) => x === "error").map(([id]) => sourceLabel({source:id}));
  lastStatus = status;
  root.innerHTML = `<div class="search-summary"><b>${loading ? "Собираем музыку…" : all.length ? all.length + " треков найдено" : "Ничего не нашли"}</b><span>${loading ? "Hitmo показываем первым — остальные каталоги догружаются рядом" : errors.length ? "Не подключились: " + esc(errors.join(", ")) : "Hitmo · Zaycev.net · Audius и открытые каталоги"}</span></div>` +
    (all.length ? trackRows(all) : `<div class="empty"><div class="empty-icon">${icon("search")}</div><h2>${loading ? "Ищем подходящий звук" : "Попробуй другой запрос"}</h2><p>${loading ? "Результаты появятся здесь автоматически." : "Например: исполнитель, трек или жанр."}</p></div>`) +
    (archives.length ? `<section class="result-section"><div class="result-heading"><h2>Записи с обложкой</h2><span>${archives.length} вариантов</span></div><div class="results">${archives.slice(0, 12).map((x,i) => `<button class="card" data-index="${i}" aria-label="Открыть ${esc(x.title)}"><img src="${esc(x.cover)}" alt="" loading="lazy" onerror="this.src='/logo.png'"><b>${esc(x.title)}</b><span>${esc(x.artist)}</span></button>`).join("")}</div></section>` : "") +
    `<p class="note">Каталог отображается единой лентой; Hitmo всегда идёт первым.</p>`;
  bindTracks(root, all);
  root.querySelectorAll(".card").forEach(b => b.onclick = () => openAlbum(archives[Number(b.dataset.index)]));
}
let waveRequest = 0, waveBusy = false;
function shuffled(list) {
  let a = list.slice();
  for (let i=a.length-1; i>0; i--) { const j=Math.floor(Math.random()*(i+1)); [a[i],a[j]]=[a[j],a[i]]; }
  return a;
}
function rememberWave(t){
  if(!t) return;
  const key=identity(t);
  waveHistory=[key,...waveHistory.filter(x=>x!==key)].slice(0,120);
  try{localStorage.setItem("openwaveWaveHistory",JSON.stringify(waveHistory));}catch(_){}
}
function freshWave(list){
  const fresh=list.filter(t=>!waveHistory.includes(identity(t)));
  if(fresh.length>=Math.min(12,list.length)) return fresh;
  waveHistory=[]; try{localStorage.removeItem("openwaveWaveHistory");}catch(_){}
  return list;
}
function cachedWave(){
  try{
    const list=JSON.parse(localStorage.getItem("openwaveWaveCache")||"[]");
    return Array.isArray(list)?list:[];
  }catch(_){return [];}
}
async function loadWave() {
  const seq=++waveRequest, root=$("#wave-content"), genre=$("#wave-genre").value;
  waveBusy=true; waveLoaded=false; $("#start-wave").disabled=true;
  root.innerHTML='<div class="wave-loading"><div class="loading-bars"><i></i><i></i><i></i><i></i><i></i></div><div><b>Ловим волну Hitmo</b><span>Подбираем несколько направлений и убираем повторы…</span></div></div>';
  try {
    const seeds = tasteArtists();
    const seedQuery = seeds.length ? "&seed=" + encodeURIComponent(seeds.join(",")) : "";
    const d = await api("/api/hitmo/wave?genre=" + encodeURIComponent(genre) + seedQuery, {signal:AbortSignal.timeout(90000)});
    if(seq!==waveRequest)return;
    const tracks=d.tracks||[];
    try{localStorage.setItem("openwaveWaveCache",JSON.stringify(tracks.slice(0,80)));}catch(_){}
    waveTracks = shuffled(freshWave(tracks));
    window.waveUnavailable = [];
  } catch(e) {
    if(seq!==waveRequest)return;
    const cached=cachedWave();
    waveTracks=shuffled(freshWave(cached));
    window.waveUnavailable=cached.length?[]:["Hitmo"];
  }
  waveBusy=false; $("#start-wave").disabled=false; waveLoaded=true; renderWave();
}
function renderWave() {
  const root=$("#wave-content"), failures=window.waveUnavailable||[];
  if(!waveTracks.length){root.innerHTML=`<div class="empty"><h2>Волну не удалось поймать</h2><p>${failures.length?"Hitmo сейчас недоступен. ":"Для этого направления пока мало результатов. "}Попробуй обновить подборку или выбрать другой жанр.</p><button class="ghost" id="retry-wave">Повторить</button></div>`;$("#retry-wave").onclick=loadWave;return;}
  root.innerHTML=`<section class="result-section"><div class="result-heading"><div><div class="eyebrow muted">HITMO DISCOVERY / PERSONAL MIX</div><h2>Твоя подборка</h2></div><span>${waveTracks.length} треков · без повторов</span></div>${trackRows(waveTracks)}</section>`;
  bindTracks(root,waveTracks);
}
$("#wave-genre").onchange=loadWave;
$("#refresh-wave").onclick=loadWave;
$("#start-wave").onclick=async()=>{if(!waveLoaded||waveBusy)await loadWave();if(waveTracks.length)playTrack(waveTracks,0)};
$("#shuffle-wave").onclick=()=>{if(!waveTracks.length){toast("Сначала загрузи подборку");return}const current=queueIsWave?queue[trackIndex]:null;waveTracks=shuffled(waveTracks);if(current){const i=waveTracks.findIndex(t=>identity(t)===identity(current));if(i>=0){queue=waveTracks.slice();trackIndex=i}}renderWave();toast("Hitmo-волна перемешана")};
let radioFilter="all";
function renderRadio(){
 const q=$("#radio-query").value.trim().toLocaleLowerCase();
 const list=radioCatalog.filter(t=>(radioFilter!=="russian"||/russian|русск|шашлыки|гоп|нафталин|медляк/i.test(t.title)) && (!q||t.title.toLocaleLowerCase().includes(q)));
 $("#radio-count").textContent=list.length;
 const root=$("#radio-content");
 root.innerHTML=list.length ? `<div class="radio-grid">${list.map((t,i)=>`<article class="radio-card"><div class="radio-cover"><img src="${esc(t.cover)}" alt="" loading="lazy" onerror="this.src='/logo.png'"><span class="radio-live">● ЭФИР</span></div><div class="radio-card-copy"><div><h3>${esc(t.title)}</h3><span>Radio Record</span></div><button class="radio-play" data-index="${i}" aria-label="Включить ${esc(t.title)}">${icon("play")}</button></div><div class="radio-card-bottom"><button class="radio-fav ${isFav(t)?"faved":""}" data-index="${i}" aria-label="${isFav(t)?"Убрать из":"Добавить в"} избранное">${icon("heart")}</button><a href="${esc(t.permalink)}" target="_blank" rel="noopener">Станция ↗</a></div></article>`).join("")}</div>` : '<div class="empty"><h2>Станция не найдена</h2><p>Попробуй другое название или фильтр.</p></div>';
 root.querySelectorAll('.radio-play').forEach(b=>b.onclick=()=>playTrack(list,Number(b.dataset.index)));
 root.querySelectorAll('.radio-fav').forEach(b=>b.onclick=()=>toggleFav(list[Number(b.dataset.index)]));
}
$("#radio-query").addEventListener("input",renderRadio);
$$('.radio-filter').forEach(b=>b.onclick=()=>{radioFilter=b.dataset.filter;$$('.radio-filter').forEach(x=>{const on=x===b;x.classList.toggle('active',on);x.setAttribute('aria-pressed',on)});renderRadio()});
async function openAlbum(item) {
  page("album");
  const root = $("#album-content");
  root.innerHTML =
    '<div class="empty"><h2>Открываем запись…</h2><p>Получаем список аудиофайлов</p></div>';
  try {
    const d = await api("/api/tracks?id=" + encodeURIComponent(item.id), {
      signal: AbortSignal.timeout(45000),
    });
    albumTracks = d.tracks;
    lastAlbum = { ...d, title: d.title || item.title };
    renderAlbum();
  } catch (e) {
    lastAlbum = null;
    root.innerHTML = `<div class="empty"><h2>Не удалось открыть запись</h2><p>${esc(e.message)}</p></div>`;
  }
}
function renderAlbum() {
  const root = $("#album-content"),
    d = lastAlbum;
  if (!d) return;
  root.innerHTML = `<div class="album-head"><img class="album-cover" src="${esc(d.cover)}" alt="" onerror="this.src='/logo.png'"><div><div class="eyebrow subdued">ЗАПИСЬ INTERNET ARCHIVE</div><h1>${esc(d.title)}</h1><p>${esc(d.artist)} · ${albumTracks.length} аудиофайлов</p></div></div>${trackRows(albumTracks)}<p class="note">Наличие и условия использования определяются источником. Размер файла — до 300 МБ.</p>`;
  bindTracks(root, albumTracks);
}
async function saveTrack(t, button) {
  button.disabled = true;
  button.style.opacity = ".4";
  toast("Скачиваем «" + t.title + "»…");
  try {
    await api("/api/download", {
      method: "POST",
      headers: auth,
      body: JSON.stringify(t),
    });
    toast("Трек сохранён в «Музыка / OpenWave»");
    await loadLibrary(false);
  } catch (e) {
    button.disabled = false;
    toast(e.message);
  } finally {
    button.style.opacity = "";
  }
}
function renderFavorites() {
  const root = $("#favorites-content");
  const lead = favorites[0];
  const cover = lead?.cover || "/logo.png";
  root.innerHTML = `
    <section class="favorites-hero">
      <div class="favorites-hero-art"><img src="${esc(cover)}" alt="" onerror="this.src='/logo.png'"></div>
      <div class="favorites-hero-copy">
        <div class="eyebrow muted">ТВОЯ КОЛЛЕКЦИЯ</div>
        <h1>Мне нравится</h1>
        <p>${favorites.length ? `${favorites.length} ${favorites.length === 1 ? "трек" : favorites.length < 5 ? "трека" : "треков"} сохранено на этом устройстве` : "Сохраняй песни, чтобы собрать свою волну"}</p>
        <div class="favorites-hero-actions">
          <button class="primary" id="favorites-play" ${favorites.length ? "" : "disabled"}>${icon("play")} <span>Слушать</span></button>
          <button class="ghost" id="favorites-shuffle" ${favorites.length ? "" : "disabled"}>${icon("shuffle")} <span>Перемешать</span></button>
        </div>
      </div>
    </section>
    <div class="favorites-list-heading"><div><div class="eyebrow muted">СОХРАНЁННЫЕ ТРЕКИ</div><h2>Твои любимые</h2></div><span>${favorites.length || 0} треков</span></div>
    ${favorites.length ? trackRows(favorites) : '<div class="empty"><div class="empty-icon">' + icon("heart") + '</div><h2>Здесь пока пусто</h2><p>Нажми на сердечко у песни — она останется с тобой даже после перезапуска приложения.</p></div>'}`;
  bindTracks(root, favorites);
  root.querySelector("#favorites-play")?.addEventListener("click", () => playTrack(favorites, 0));
  root.querySelector("#favorites-shuffle")?.addEventListener("click", () => {
    if (!favorites.length) return;
    playTrack(favorites, Math.floor(Math.random() * favorites.length));
  });
}
function renderLibrary() {
  const root = $("#library-content");
  root.innerHTML = `<p class="folder">Папка на компьютере: ${esc(window.libraryFolder || "Музыка / OpenWave")}</p>${libraryTracks.length ? trackRows(libraryTracks, true) : '<div class="empty"><div class="empty-icon">' + icon("down") + '</div><h2>Скачанных треков пока нет</h2><p>Найди музыку и сохрани доступный трек на компьютер.</p></div>'}`;
  bindTracks(root, libraryTracks);
}
async function loadLibrary(render) {
  try {
    const d = await api("/api/library");
    libraryTracks = d.tracks;
    window.libraryFolder = d.folder;
    if (render && currentPage === "library") renderLibrary();
  } catch (e) {
    if (render)
      $("#library-content").innerHTML =
        `<p class="folder">${esc(e.message)}</p>`;
  }
}
let coverTrack = null, lyricsRequest = 0, activeLyricIndex = -1;
function closeCover(){ $("#cover-modal").hidden = true; document.body.classList.remove("cover-open"); }
function parseLrc(raw){
  return String(raw || "").split(/\r?\n/).flatMap(line => {
    const tags = [...line.matchAll(/\[(\d+):(\d+(?:\.\d+)?)\]/g)];
    const text = line.replace(/\[(\d+):(\d+(?:\.\d+)?)\]/g, "").trim();
    return tags.map(m => ({time: Number(m[1]) * 60 + Number(m[2]), text: text || "♪"}));
  }).sort((a,b) => a.time - b.time);
}
function renderLyrics(raw){
  const lines = parseLrc(raw);
  activeLyricIndex = -1;
  if(!lines.length) return {html: raw ? "<pre>"+esc(raw)+"</pre>" : '<div class="lyrics-empty">Текст для этого трека не найден.</div>', timed:false};
  return {timed:true, lines, html:'<div class="lyrics-timed">'+lines.map((x,i)=>`<button class="lyric-line" data-time="${x.time}" data-lyric-index="${i}">${esc(x.text)}</button>`).join("")+'</div>'};
}
function syncLyrics(seconds){
  if(!$("#cover-modal") || $("#cover-modal").hidden) return;
  const lines = [...$("#cover-lyrics").querySelectorAll(".lyric-line")];
  if(!lines.length) return;
  let next = -1;
  lines.forEach((line,i) => { if(Number(line.dataset.time) <= seconds + .05) next=i; });
  if(next === activeLyricIndex) return;
  activeLyricIndex = next;
  lines.forEach((line,i) => line.classList.toggle("active",i===next));
  const active = lines[next];
  if(active) active.scrollIntoView({behavior:"smooth",block:"center"});
}
async function openCover(t){
  if(!t)return;
  coverTrack=t;
  const img=$("#cover-image"); img.src=t.cover || "/logo.png"; img.alt=t.title || "Обложка";
  $("#cover-title").textContent=t.title || "Без названия";
  $("#cover-artist").textContent=t.artist || "Неизвестный исполнитель";
  const genius=$("#lyrics-genius");
  genius.href="https://genius.com/search?q="+encodeURIComponent((t.artist||"")+" "+(t.title||""));
  $("#lyrics-provider").textContent="СИНХРОНИЗИРОВАННЫЙ ТЕКСТ";
  const fav=$("#cover-fav"); fav.classList.toggle("faved",isFav(t)); fav.innerHTML=icon("heart") + (isFav(t)?" Убрать из избранного":" В избранное");
  $("#cover-lyrics").innerHTML='<div class="lyrics-loading">Ищем текст песни…</div>';
  $("#cover-modal").hidden=false; document.body.classList.add("cover-open");
  const seq=++lyricsRequest;
  try{
    const d=await api("/api/lyrics?artist="+encodeURIComponent(t.artist||"")+"&title="+encodeURIComponent(t.title||""),{signal:AbortSignal.timeout(15000)});
    if(seq!==lyricsRequest||coverTrack!==t)return;
    const text=d.synced || d.plain;
    genius.href=d.genius || genius.href;
    $("#lyrics-provider").textContent=d.synced ? "ТЕКСТ В ТАЙМИНГЕ · LRCLIB" : d.plain ? "ТЕКСТ · LRCLIB" : "ТЕКСТ НЕ НАЙДЕН";
    const rendered=renderLyrics(text);
    $("#cover-lyrics").innerHTML=rendered.html;
    if(rendered.timed) syncLyrics(audio.currentTime);
  }catch(_){ if(seq===lyricsRequest)$("#cover-lyrics").innerHTML='<div class="lyrics-empty">Сервис текста сейчас недоступен. Попробуй позже.</div>'; }
}
$("#cover-close").onclick=closeCover;
$("#cover-modal").querySelector("[data-cover-close]").onclick=closeCover;
$("#cover-play").onclick=()=>{if(coverTrack){const i=queue.findIndex(t=>identity(t)===identity(coverTrack));if(i>=0)playTrack(queue,i);else playTrack([coverTrack],0);closeCover();}};
$("#cover-fav").onclick=()=>{if(coverTrack){toggleFav(coverTrack);const fav=$("#cover-fav");fav.classList.toggle("faved",isFav(coverTrack));fav.innerHTML=icon("heart") + (isFav(coverTrack)?" Убрать из избранного":" В избранное");}};
$("#cover-lyrics-btn").onclick=()=>$("#cover-lyrics").scrollIntoView({behavior:"smooth",block:"nearest"});
$("#cover-lyrics").addEventListener("click", e => {
  const line=e.target.closest(".lyric-line");
  if(!line) return;
  audio.currentTime=Number(line.dataset.time)||0;
  audio.play().catch(()=>{});
  syncLyrics(audio.currentTime);
});
function syncMobilePlayer() {
  const t = queue[trackIndex];
  if (!t) return;
  const image = $("#mobile-player-image");
  image.src = t.cover || "/logo.png";
  image.alt = t.title || "Обложка";
  $("#mobile-player-title").textContent = t.title || "Без названия";
  $("#mobile-player-artist").textContent = t.artist || "Неизвестный исполнитель";
  $("#mobile-player-current").textContent = $("#current").textContent;
  $("#mobile-player-duration").textContent = $("#duration").textContent;
  $("#mobile-player-seek").value = $("#seek").value || 0;
  $("#mobile-player-fav").classList.toggle("faved", isFav(t));
  $("#mobile-player-play").innerHTML = audio.paused ? icon("play") : icon("pause");
  $("#mobile-player-play").setAttribute("aria-label", audio.paused ? "Воспроизвести" : "Пауза");
  $(".mobile-player-bg").style.backgroundImage = `linear-gradient(180deg, #050505 0%, #050505b8 45%, #050505 100%), url("${(t.cover || "/logo.png").replace(/"/g, "%22")}")`;
}
function openMobilePlayer() {
  if (!queue[trackIndex]) { toast("Сначала выбери трек"); return; }
  syncMobilePlayer();
  $("#mobile-player").hidden = false;
  document.body.classList.add("mobile-player-open");
}
function closeMobilePlayer() {
  $("#mobile-player").hidden = true;
  document.body.classList.remove("mobile-player-open");
}
$("#now-art").onclick=openMobilePlayer;
$(".now").addEventListener("click", (e) => {
  if (e.target.closest("button")) return;
  openMobilePlayer();
});
$("#mobile-player-close").onclick=closeMobilePlayer;
$("#mobile-player-play").onclick=()=>$("#play").click();
$("#mobile-player-prev").onclick=()=>$("#prev").click();
$("#mobile-player-next").onclick=()=>$("#next").click();
$("#mobile-player-shuffle").onclick=()=>$("#shuffle").click();
$("#mobile-player-repeat").onclick=()=>$("#repeat").click();
$("#mobile-player-fav").onclick=()=>toggleFav(queue[trackIndex]);
$("#mobile-player-lyrics").onclick=()=>{if(queue[trackIndex]){closeMobilePlayer();openCover(queue[trackIndex]);}};
$("#mobile-player-wave").onclick=()=>{closeMobilePlayer();page("wave");};
$("#mobile-player-artist").onclick=()=>{
  const artist=queue[trackIndex]?.artist?.trim();
  if(!artist)return;
  closeMobilePlayer();page("search");$("#query").value=artist;doSearch();
};
$("#mobile-player-seek").oninput=(e)=>{
  $("#seek").value=e.target.value;
  $("#seek").dispatchEvent(new Event("input"));
};
function playTrack(list, index, automatic = false) {
  if (index < 0 || index >= list.length) return;
  if (!automatic) {
    queueIsWave = list === waveTracks;
    failedInRow = 0;
  }
  queue = list.slice();
  trackIndex = index;
  const original = list[index], t = localVersion(original);
  if (queueIsWave) rememberWave(t);
  rememberListening(t);
  $("#now-title").textContent = t.title;
  $("#now-artist").textContent = t.artist;
  $("#wave-current").textContent = queueIsWave ? (t.artist + " — " + t.title) : "ГОТОВА К ПРОСЛУШИВАНИЮ";
  $("#now-art").innerHTML = `<img src="${esc(t.cover || "/logo.png")}" alt="" onerror="this.src='/logo.png'">`;
  updateMediaSession(t, false);
  syncMobilePlayer();
  refreshCount();
  const station = t.source === "radio" ? radioCatalog.find(x => x.id === t.id) : null;
  if (t.source === "radio" && !station) { toast("Станция больше недоступна"); return; }
  $("#seek").disabled = !!station;
  $("#current").textContent = station ? "LIVE" : "0:00";
  $("#duration").textContent = station ? "ЭФИР" : "0:00";
  audio.src = station ? station.stream : t.local
    ? "/api/local?key=" + encodeURIComponent(t.key)
    : t.source === "hitmo" ? "/api/hitmo/audio?id=" + encodeURIComponent(t.id)
    : t.source === "zaycev" ? "/api/zaycev/audio?id=" + encodeURIComponent(t.id)
    : t.source === "freetouse" ? "/api/free/audio?id=" + encodeURIComponent(t.id)
    : t.source === "audius" ? "/api/audius/audio?id=" + encodeURIComponent(t.id)
    : ["ccmixter", "commons"].includes(t.source) ? "/api/provider/audio?source=" + encodeURIComponent(t.source) + "&id=" + encodeURIComponent(t.id) + "&fileid=" + encodeURIComponent(t.fileId || "")
    : "/api/audio?id=" + encodeURIComponent(t.id) + "&file=" + encodeURIComponent(t.file);
  audio.play().catch(e => { if (e.name !== "AbortError") toast("Не удалось воспроизвести трек — источник может быть недоступен."); });
}
$("#play").onclick = () => {
  if (trackIndex < 0) {
    toast("Сначала выбери трек");
    return;
  }
  if (audio.paused)
    audio.play().catch(() => toast("Не удалось воспроизвести трек"));
  else audio.pause();
};
$("#prev").onclick = () => {
  if (queue[trackIndex]?.source !== "radio" && audio.currentTime > 3) { audio.currentTime = 0; return; }
  if (trackIndex > 0) playTrack(queue, trackIndex - 1, true);
  else if (queue[trackIndex]?.source !== "radio" && audio.currentTime > 0) audio.currentTime = 0;
};
$("#next").onclick = () => nextTrack();
$("#now-fav").onclick = () => toggleFav(queue[trackIndex]);
async function nextTrack() {
  if (!queue.length) return;
  let next =
    shuffle && queue.length > 1
      ? (trackIndex + 1 + Math.floor(Math.random() * (queue.length - 1))) %
        queue.length
      : trackIndex + 1;
  if (next < queue.length) { playTrack(queue, next, true); return; }
  if (queueIsWave) {
    const lastTrack=queue[trackIndex], lastIndex=trackIndex;
    toast("Подбираем продолжение волны…");
    await loadWave();
    if (queueIsWave && queue[trackIndex]===lastTrack && trackIndex===lastIndex && waveLoaded && waveTracks.length) playTrack(waveTracks,0,true);
  }
}
audio.onended = () => {
  if (repeat) {
    audio.currentTime = 0;
    audio.play().catch(() => {});
  } else nextTrack();
};
$("#shuffle").onclick = () => {
  shuffle = !shuffle;
  $("#shuffle").classList.toggle("on", shuffle);
  $("#shuffle").setAttribute("aria-pressed", shuffle);
};
$("#repeat").onclick = () => {
  repeat = !repeat;
  $("#repeat").classList.toggle("on", repeat);
  $("#repeat").setAttribute("aria-pressed", repeat);
};
audio.onplay = () => {
  if (window.scWidget) window.scWidget.pause();
  window.OpenWaveNative?.startAudio?.();
  startEqualizer();
  updateMediaSession(queue[trackIndex], true);
  $("#play").innerHTML = icon("pause");
  $("#play").setAttribute("aria-label", "Пауза");
  document.body.classList.add("is-playing");
  syncMobilePlayer();
};
audio.onpause = () => {
  window.OpenWaveNative?.stopAudio?.();
  stopEqualizer();
  $("#play").innerHTML = icon("play");
  $("#play").setAttribute("aria-label", "Воспроизвести");
  document.body.classList.remove("is-playing");
  updateMediaSession(queue[trackIndex], false);
  syncMobilePlayer();
};
audio.onplaying = () => {
  failedInRow = 0;
};
audio.onerror = () => {
  if (!audio.src) return;
  if (queueIsWave && failedInRow < 3 && trackIndex + 1 < queue.length) {
    failedInRow++;
    toast("Трек недоступен — включаем следующий");
    const failedTrack=queue[trackIndex];
    setTimeout(()=>{if(queueIsWave && queue[trackIndex]===failedTrack) nextTrack()},800);
  } else toast("Аудиофайл недоступен или не поддерживается плеером");
};
function fmt(n) {
  if (!isFinite(n)) return "0:00";
  return Math.floor(n / 60) + ":" + String(Math.floor(n % 60)).padStart(2, "0");
}
audio.ontimeupdate = () => {
  if(queue[trackIndex]?.source === "radio") return;
  $("#current").textContent = fmt(audio.currentTime);
  $("#duration").textContent = fmt(audio.duration);
  $("#seek").value =
    isFinite(audio.duration) && audio.duration
      ? Math.round((audio.currentTime / audio.duration) * 1000)
      : 0;
  syncLyrics(audio.currentTime);
  if (!$("#mobile-player").hidden) syncMobilePlayer();
  if (queue[trackIndex] && isFinite(audio.duration) && navigator.mediaSession?.setPositionState) {
    try {
      navigator.mediaSession.setPositionState({
        duration: audio.duration,
        playbackRate: audio.playbackRate || 1,
        position: Math.min(audio.currentTime, Math.max(0, audio.duration - 0.01)),
      });
    } catch (_) {}
  }
};
audio.onloadedmetadata = () => { if(queue[trackIndex]?.source !== "radio") $("#duration").textContent = fmt(audio.duration); };
$("#seek").oninput = (e) => {
  if (queue[trackIndex]?.source !== "radio" && isFinite(audio.duration))
    audio.currentTime = (e.target.value / 1000) * audio.duration;
};
let volumeChanged=false, volumeTimer, volumeSave=Promise.resolve();
$("#volume").oninput=(e)=>{volumeChanged=true;const v=Number(e.target.value);audio.volume=v/100;clearTimeout(volumeTimer);volumeTimer=setTimeout(()=>{volumeSave=volumeSave.then(()=>api("/api/settings",{method:"POST",headers:auth,body:JSON.stringify({volume:v})})).catch(()=>toast("Не удалось сохранить громкость"))},250)};
api("/api/settings").then(d=>{if(!volumeChanged && Number.isFinite(d.volume)){const v=Math.max(0,Math.min(100,d.volume));$("#volume").value=v;audio.volume=v/100}}).catch(()=>{});
document.addEventListener("keydown",e=>{ if(e.target.closest("input,textarea,select,button")||e.ctrlKey||e.altKey||e.metaKey)return; if(e.code==="Space"){e.preventDefault();$("#play").click()} else if(e.key==="ArrowRight"){e.preventDefault();if(queue[trackIndex]?.source!=="radio"&&Number.isFinite(audio.duration))audio.currentTime=Math.min(audio.duration,audio.currentTime+5)} else if(e.key==="ArrowLeft"){e.preventDefault();if(queue[trackIndex]?.source!=="radio")audio.currentTime=Math.max(0,audio.currentTime-5)} });
$("#quit").onclick = async () => {
  if (!confirm("Завершить работу OpenWave?")) return;
  if (window.NATIVE_WINDOW && window.nativeClose) {
    window.nativeClose();
    return;
  }
  try {
    await fetch("/api/quit", { method: "POST", headers: auth });
  } catch {}
  document.body.innerHTML =
    '<div style="padding:60px;font:16px Segoe UI;color:white">OpenWave завершил работу. Это окно можно закрыть.</div>';
  window.close();
};
if (window.NATIVE_WINDOW) {
  document.body.classList.add("native");
  $("#window-bar").hidden = false;
  $("#window-min").onclick = () => window.nativeMin?.();
  $("#window-max").onclick = () => window.nativeMax?.();
  $("#window-close").onclick = () => window.nativeClose?.();
  $("#window-drag").onmousedown = (e) => {
    if (e.button === 0) window.nativeDrag?.();
  };
  $("#window-drag").ondblclick = () => window.nativeMax?.();
}
function ping() {
  fetch("/api/ping").catch(() => {});
}
ping();
setInterval(ping, 30000);
api("/api/favorites")
  .then((d) => {
    const remote = d.tracks || [];
    const local = readLocal("openwaveFavorites", []);
    const merged = [...remote, ...(Array.isArray(local) ? local : [])];
    const seen = new Set();
    favorites = merged.filter((t) => {
      const key = identity(t);
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
    writeLocal("openwaveFavorites", favorites);
    refreshCount();
    if (currentPage === "favorites") { renderFavorites(); renderHistory(); }
  })
  .catch(() => {
    favorites = readLocal("openwaveFavorites", []);
    refreshCount();
    if (currentPage === "favorites") { renderFavorites(); renderHistory(); }
    toast(favorites.length ? "Избранное загружено с телефона" : "Не удалось загрузить избранное");
  });
loadLibrary(false);

// SoundCloud uses its own embedded player, not the OpenWave audio queue.
let scScriptRequested = false;
function connectSoundCloudWidget(){
 if (!window.SC || !$("#sc-player")) return;
 try {window.scWidget = SC.Widget($("#sc-player")); window.scWidget.bind(SC.Widget.Events.PLAY, () => audio.pause());} catch (_) {}
}
$("#sc-form").onsubmit = e => {
 e.preventDefault(); const value=$("#sc-url").value.trim(); let u;
 try { u = new URL(value); if(u.protocol!=="https:" || !["soundcloud.com","www.soundcloud.com"].includes(u.hostname) || u.username || u.password || u.pathname.split('/').filter(Boolean).length<2 || ["search","you","discover"].includes(u.pathname.split('/').filter(Boolean)[0])) throw Error(); } catch(_) { $("#sc-status").textContent="Вставь ссылку на трек или плейлист soundcloud.com"; return; }
 audio.pause(); if(window.scWidget){try{window.scWidget.pause()}catch(_){} window.scWidget=null;}
 const frame=$("#sc-player"); frame.hidden=false; frame.src="https://w.soundcloud.com/player/?url="+encodeURIComponent(u.href)+"&auto_play=false&show_user=true&show_artwork=true";
 $("#sc-status").textContent="Управляй воспроизведением в плеере SoundCloud. Доступность зависит от автора.";
 if(window.SC) connectSoundCloudWidget();
 else if(!scScriptRequested){scScriptRequested=true;let script=document.createElement('script');script.src="https://w.soundcloud.com/player/api.js";script.onload=connectSoundCloudWidget;script.onerror=()=>{$("#sc-status").textContent="Плеер открыт отдельно; автоматическая пауза OpenWave недоступна."};document.head.appendChild(script)}
};
