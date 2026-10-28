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
