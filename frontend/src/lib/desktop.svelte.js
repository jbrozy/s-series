// Desktop version: the installed app serves this interface locally and forwards
// /api to the S-Check server chosen at sign-in. In the browser version none of
// this is active.

const meta = typeof document !== 'undefined' ? document.querySelector('meta[name="scheck-desktop"]') : null;

export const desktop = $state({
  on: !!meta,
  version: meta?.content || '',
  server: '',
  recent: [],
  locked: false,
  loaded: false,
  wantServer: false // sign-in page should start in the server field
});

export function serverHost(url = desktop.server) {
  try { return new URL(url).host; } catch { return url || ''; }
}

// Plain HTTP to another machine sends the password readable over the network.
export function insecure(url) {
  try {
    const u = new URL(url.includes('://') ? url : 'https://' + url);
    return u.protocol === 'http:' && !['localhost', '127.0.0.1', '[::1]'].includes(u.hostname);
  } catch { return false; }
}

export async function loadDesktop() {
  if (!desktop.on) return;
  try {
    const res = await fetch('/_desktop/config', { credentials: 'same-origin', cache: 'no-store' });
    if (res.ok) Object.assign(desktop, await res.json());
  } catch { /* the address can still be entered at sign-in */ }
  desktop.loaded = true;
}

// Checks the address and makes it the server for all further requests.
export async function connectServer(url) {
  let res;
  try {
    res = await fetch('/_desktop/server', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'scheck' },
      body: JSON.stringify({ url })
    });
  } catch {
    throw new Error('Die S-Check-Anwendung antwortet nicht. Bitte das Programm neu starten.');
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || 'Der Server konnte nicht geprüft werden.');
  Object.assign(desktop, data);
  return data;
}
