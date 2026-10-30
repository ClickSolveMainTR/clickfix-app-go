// Browser side of the ClickFix app protocol:
//   1. app → panel   {type: "clickfix.ready"}
//   2. panel → app   {type: "clickfix.context", context_token, scopes}   (again on refresh, ~every 100 s)
//   3. app → panel   {type: "clickfix.action", request_id, action, content | value | target_uuid}
//   4. panel → app   {type: "clickfix.action.result", request_id, success, error?}
// The token is only sent to this app's own server; never put it in a URL or storage.

const $ = (id) => document.getElementById(id);
const fmtMoney = (n, c) => new Intl.NumberFormat('tr-TR', { style: 'currency', currency: c }).format(n);
const fmtDate = (iso) => new Intl.DateTimeFormat('tr-TR', { dateStyle: 'medium' }).format(new Date(iso));
const STATUS = { processing: 'Hazırlanıyor', shipped: 'Kargoda', delivered: 'Teslim edildi', refunded: 'İade edildi' };

const { panel_origins: origins } = await (await fetch('/api/config')).json();
let ctx = null;
let lastToken = null;
const pending = new Map();

function post(message) {
  // A wrong targetOrigin is dropped by the browser, so posting to each allowed origin is safe.
  for (const origin of origins) window.parent.postMessage(message, origin);
}

function act(action, content) {
  if (!ctx?.scopes.includes(action)) return Promise.resolve({ success: false, error: 'scope_denied' });
  const request_id = crypto.randomUUID();
  return new Promise((resolve) => {
    pending.set(request_id, resolve);
    post({ type: 'clickfix.action', request_id, action, content });
  });
}

function summary(o) {
  const lines = [`Sipariş ${o.id}: ${STATUS[o.status] ?? o.status}`, `Tutar: ${fmtMoney(o.total, o.currency)}`, `Tarih: ${fmtDate(o.placed_at)}`];
  if (o.tracking) lines.push(`Kargo: ${o.carrier} · ${o.tracking}`);
  return lines.join('\n');
}

function render(data) {
  const list = $('orders');
  list.replaceChildren();
  $('who').textContent = data.email ? `${data.email}${data.demo ? ' (demo verisi)' : ''}` : 'Konuşmanın e-posta adresi yok.';
  if (data.reason === 'no_conversation') {
    $('status').textContent = 'Siparişleri görmek için uygulamayı bir konuşmanın kenar çubuğunda açın.';
    return;
  }
  $('status').textContent = data.orders.length ? '' : 'Bu müşteriye ait sipariş bulunamadı.';
  for (const o of data.orders) {
    const li = $('order').content.firstElementChild.cloneNode(true);
    li.querySelector('.id').textContent = o.id;
    li.querySelector('.badge').textContent = STATUS[o.status] ?? o.status;
    li.querySelector('.meta').textContent = `${fmtDate(o.placed_at)} · ${fmtMoney(o.total, o.currency)}`;
    li.querySelector('.items').textContent = o.items.join(', ');
    for (const [cls, action] of [['note', 'conversation.note'], ['reply', 'conversation.reply']]) {
      const btn = li.querySelector(`.${cls}`);
      btn.hidden = !ctx.scopes.includes(action);
      btn.addEventListener('click', async () => {
        btn.disabled = true;
        const result = await act(action, summary(o));
        btn.disabled = false;
        $('status').textContent = result.success ? 'Tamam.' : `Yapılamadı (${result.error ?? 'bilinmeyen hata'}).`;
      });
    }
    list.append(li);
  }
}

async function load() {
  $('status').textContent = 'Siparişler yükleniyor…';
  const res = await fetch('/api/orders', { headers: { Authorization: `Bearer ${ctx.token}` } });
  if (!res.ok) {
    $('status').textContent = res.status === 401 ? 'Bağlam doğrulanamadı. Paneli yenileyin.' : 'Siparişler yüklenemedi. Tekrar deneyin.';
    return;
  }
  render(await res.json());
}

window.addEventListener('message', (event) => {
  if (event.source !== window.parent || !origins.includes(event.origin)) return;
  const msg = event.data;
  if (msg?.type === 'clickfix.context') {
    const first = lastToken === null;
    ctx = { token: msg.context_token, scopes: msg.scopes ?? [] };
    lastToken = msg.context_token;
    // A refresh keeps the same conversation (the panel remounts the frame on a change).
    if (first) void load();
  } else if (msg?.type === 'clickfix.action.result') {
    pending.get(msg.request_id)?.({ success: msg.success, error: msg.error });
    pending.delete(msg.request_id);
  }
});

post({ type: 'clickfix.ready' });
