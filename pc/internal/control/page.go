package control

const pairHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Pair a phone - phoneset</title>
<link rel="icon" href="/logo.png?t=__TOKEN__">
<style>
  :root { color-scheme: light dark; --bg:#f4f6fb; --card:#fff; --fg:#14161c; --muted:#5d6472; --accent:#3b6bff; --ok:#1f9d4a; --line:#e3e7f0; }
  @media (prefers-color-scheme: dark) { :root { --bg:#0f1218; --card:#181c25; --fg:#eef1f7; --muted:#9aa3b5; --line:#262c38; --ok:#35d070; } }
  * { box-sizing: border-box; }
  body { margin:0; min-height:100vh; display:grid; place-items:center; background:var(--bg); color:var(--fg);
         font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif; padding:24px; }
  main { width:min(420px,100%); background:var(--card); border:1px solid var(--line); border-radius:20px; padding:28px; text-align:center; }
  img.logo { width:56px; height:56px; border-radius:14px; }
  h1 { font-size:22px; margin:12px 0 4px; }
  p { margin:0; color:var(--muted); }
  .qr { margin:22px auto 14px; width:240px; height:240px; background:#fff; border-radius:12px; padding:8px; }
  .qr img { width:100%; height:100%; display:block; }
  .code { font:600 34px/1.2 ui-monospace,"SF Mono",Consolas,monospace; letter-spacing:.12em; color:var(--accent); }
  .small { font-size:13px; margin-top:6px; }
  .done { display:none; color:var(--ok); font-weight:600; font-size:20px; margin:30px 0; }
  .ok main .wait { display:none; } .ok main .done { display:block; }
  button { margin-top:18px; font:inherit; padding:10px 18px; border-radius:12px; border:1px solid var(--line); background:transparent; color:var(--fg); cursor:pointer; }
</style>
</head>
<body>
<main>
  <img class="logo" src="/logo.png?t=__TOKEN__" alt="">
  <h1>Pair a phone</h1>
  <div class="wait">
    <p>Open phoneset on your phone, tap <b>+</b>, then scan this code or type the digits.</p>
    <div class="qr"><img id="qr" src="/qr.png?t=__TOKEN__" alt="QR code"></div>
    <div class="code" id="code">······</div>
    <p class="small" id="left"></p>
    <button id="again">New code</button>
  </div>
  <div class="done" id="done"></div>
  <p class="small">Your phone and this PC must be on the same Wi-Fi.</p>
</main>
<script>
const T = "__TOKEN__";
const q = (p, o) => fetch(p + (p.includes("?") ? "&" : "?") + "t=" + T, o).then(r => r.json());
const opened = Date.now() - 2000;
let lastCode = "";
function tick() {
  q("/api/status").then(s => {
    // The offer is used up and a phone connected after this page opened: that phone just paired.
    if (!s.pairing && s.phone && Date.parse(s.phone.since) > opened) {
      document.getElementById("done").textContent = "✓ " + s.phone.name + " is paired";
      document.body.classList.add("ok");
      return;
    }
    const p = s.pairing;
    document.getElementById("code").textContent = p ? p.code : "expired";
    if (p && p.code !== lastCode) { lastCode = p.code; document.getElementById("qr").src = "/qr.png?t=" + T + "&c=" + encodeURIComponent(p.code); }
    const secs = p ? Math.max(0, Math.round((new Date(p.expiresAt) - Date.now()) / 1000)) : 0;
    document.getElementById("left").textContent = p ? "Valid for " + Math.floor(secs / 60) + ":" + String(secs % 60).padStart(2, "0") : "Press New code to start again.";
  }).catch(() => { document.getElementById("left").textContent = "phoneset isn't running any more."; });
}
document.getElementById("again").onclick = () => q("/api/pair", { method: "POST" }).then(tick);
setInterval(tick, 1000); tick();
</script>
</body>
</html>`

// videoHTML shows the camera of the connected phone and what the PC measures of it. It reads /video.frames and
// decodes the pictures in the browser: a JPEG with createImageBitmap, h264 with WebCodecs (the Annex B frames are
// turned into the length-prefixed form the decoder takes), and turns each picture upright as the record says.
// (No backticks in the script: this is a Go raw string.)
const videoHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Phone camera - phoneset</title>
<link rel="icon" href="/logo.png?t=__TOKEN__">
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body { margin:0; height:100vh; display:flex; flex-direction:column; background:#0b0d12; color:#eef1f7;
         font:14px/1.4 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif; }
  #view { flex:1; min-height:0; }
  #view canvas { width:100%; height:100%; object-fit:contain; display:block; }
  #info { padding:8px 14px; color:#9aa3b5; font-variant-numeric:tabular-nums; text-align:center; }
  #wait { position:absolute; inset:0; display:grid; place-items:center; pointer-events:none; color:#9aa3b5; text-align:center; padding:0 24px; }
</style>
</head>
<body>
<div id="view"><canvas id="c" width="16" height="9"></canvas></div>
<div id="wait">Waiting for the phone's camera…</div>
<div id="info">&nbsp;</div>
<script>
  const info = document.getElementById("info"), wait = document.getElementById("wait");
  const canvas = document.getElementById("c"), ctx = canvas.getContext("2d");
  let problem = "", shown = 0;

  // Draws a decoded picture upright: the quarter turns clockwise, then the mirror (docs/protocol.md, 8.3).
  function draw(img, w, h, rot, mirror) {
    const cw = rot % 2 ? h : w, ch = rot % 2 ? w : h;
    if (canvas.width !== cw || canvas.height !== ch) { canvas.width = cw; canvas.height = ch; }
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.translate(cw / 2, ch / 2);
    if (mirror) ctx.scale(-1, 1);
    ctx.rotate(rot * Math.PI / 2);
    ctx.drawImage(img, -w / 2, -h / 2, w, h);
    shown++; wait.style.display = "none";
  }

  // ---- h264: Annex B access units in, decoded pictures out
  let decoder = null, config = "", needKey = true, metas = new Map();
  function nals(d) { // [start, end] of every NAL unit of an Annex B buffer
    const out = []; let at = -1;
    for (let i = 0; i + 2 < d.length; i++) {
      if (d[i] === 0 && d[i + 1] === 0 && d[i + 2] === 1) {
        if (at >= 0) out.push([at, i - (d[i - 1] === 0 ? 1 : 0)]);
        at = i + 3; i += 2;
      }
    }
    if (at >= 0) out.push([at, d.length]);
    return out;
  }
  function openDecoder(sps, pps) {
    const hex = (n) => n.toString(16).padStart(2, "0");
    const codec = "avc1." + hex(sps[1]) + hex(sps[2]) + hex(sps[3]);
    const key = codec + ":" + sps.length + ":" + pps.length;
    if (decoder && config === key) return true;
    if (decoder) { try { decoder.close(); } catch {} }
    const desc = new Uint8Array(11 + sps.length + pps.length);
    desc.set([1, sps[1], sps[2], sps[3], 0xFF, 0xE1, sps.length >> 8, sps.length & 255]);
    desc.set(sps, 8); desc.set([1, pps.length >> 8, pps.length & 255], 8 + sps.length); desc.set(pps, 11 + sps.length);
    metas = new Map(); needKey = true; config = key;
    decoder = new VideoDecoder({
      output: (f) => { const m = metas.get(f.timestamp); metas.delete(f.timestamp); if (m) draw(f, m.w, m.h, m.rot, m.mirror); f.close(); },
      error: (e) => { problem = "The decoder stopped: " + e.message; decoder = null; config = ""; }
    });
    decoder.configure({ codec, description: desc, optimizeForLatency: true });
    return true;
  }
  function h264(r) {
    if (typeof VideoDecoder === "undefined") { problem = "This browser can't decode h264 here. Use Chrome, Edge or Safari 16.4 or newer."; return; }
    const d = r.data, list = nals(d);
    if (r.key) {
      let sps = null, pps = null;
      for (const [a, b] of list) { const t = d[a] & 31; if (t === 7) sps = d.subarray(a, b); else if (t === 8) pps = d.subarray(a, b); }
      if (!sps || !pps) return;
      openDecoder(sps, pps);
    }
    if (!decoder || (needKey && !r.key)) return;
    if (decoder.decodeQueueSize > 6) { needKey = true; if (!r.key) return; } // fell behind: wait for the next key frame
    let size = 0; const keep = [];
    for (const [a, b] of list) { const t = d[a] & 31; if (t === 7 || t === 8 || t === 9) continue; keep.push([a, b]); size += 4 + b - a; }
    const out = new Uint8Array(size); let o = 0;
    for (const [a, b] of keep) { const n = b - a; out[o++] = n >>> 24; out[o++] = n >>> 16 & 255; out[o++] = n >>> 8 & 255; out[o++] = n & 255; out.set(d.subarray(a, b), o); o += n; }
    needKey = false;
    metas.set(r.seq, { w: r.w, h: r.h, rot: r.rot, mirror: r.mirror });
    try { decoder.decode(new EncodedVideoChunk({ type: r.key ? "key" : "delta", timestamp: r.seq, data: out })); }
    catch (e) { problem = "The decoder refused a frame: " + e.message; needKey = true; }
  }

  // ---- jpeg: the newest picture wins while one is being decoded
  let busy = false, next = null;
  async function jpeg(r) {
    if (busy) { next = r; return; }
    busy = true;
    try {
      const bmp = await createImageBitmap(new Blob([r.data], { type: "image/jpeg" }));
      draw(bmp, bmp.width, bmp.height, r.rot, r.mirror); bmp.close();
    } catch { problem = "A picture could not be decoded."; }
    busy = false;
    if (next) { const n = next; next = null; jpeg(n); }
  }

  // ---- the stream of records (docs/protocol.md, section 9)
  async function run() {
    for (;;) {
      try {
        const res = await fetch("/video.frames?t=__TOKEN__", { cache: "no-store" });
        const rd = res.body.getReader();
        let chunks = [], have = 0, head = null;
        const take = (n) => {
          const out = new Uint8Array(n); let o = 0;
          while (o < n) { const c = chunks[0], k = Math.min(c.length, n - o); out.set(c.subarray(0, k), o); o += k; if (k === c.length) chunks.shift(); else chunks[0] = c.subarray(k); }
          have -= n; return out;
        };
        for (;;) {
          const { value, done } = await rd.read();
          if (done) break;
          chunks.push(value); have += value.length;
          for (;;) {
            if (!head) {
              if (have < 18) break;
              const b = take(18), v = new DataView(b.buffer);
              head = { seq: v.getUint32(0), codec: b[8], rot: (b[9] >> 1) & 3, key: !!(b[9] & 1), mirror: !!(b[9] & 8), w: v.getUint16(10), h: v.getUint16(12), len: v.getUint32(14) };
            }
            if (have < head.len) break;
            head.data = take(head.len);
            if (head.codec === 2) h264(head); else if (head.codec === 1) jpeg(head);
            head = null;
          }
        }
      } catch {}
      await new Promise((r) => setTimeout(r, 1000)); // the agent or the camera went away: try again
    }
  }
  run();

  async function poll() {
    try {
      const r = await fetch("/api/status?t=__TOKEN__", { cache: "no-store" });
      const v = (await r.json()).video;
      if (!v.on) { ctx.clearRect(0, 0, canvas.width, canvas.height); wait.style.display = "grid"; wait.textContent = "Waiting for the phone's camera…"; }
      else if (problem && !shown) { wait.style.display = "grid"; wait.textContent = problem; }
      info.textContent = v.on
        ? (v.codec || "").toUpperCase() + "  ·  " + (v.width ? v.width + "×" + v.height : "…") + "  ·  " + v.fps.toFixed(1) + " fps  ·  " + v.kBps.toFixed(0) + " kB/s  ·  " + (v.camera || "") + " camera" + (problem ? "  ·  " + problem : "")
        : "camera off";
      shown = 0;
    } catch { info.textContent = "phoneset isn't answering"; }
  }
  poll(); setInterval(poll, 1000);
</script>
</body>
</html>
`
