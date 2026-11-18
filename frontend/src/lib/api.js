import { session } from './state.svelte.js';
import { go } from './router.svelte.js';

export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

async function request(method, url, body) {
  const opts = { method, credentials: 'same-origin', headers: { 'X-Requested-With': 'scheck' } };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const res = await fetch('/api' + url, opts);
  const ct = res.headers.get('content-type') || '';
  const data = ct.includes('json') ? await res.json().catch(() => null) : await res.text();
  if (res.status === 401 && !url.startsWith('/auth/')) {
    session.user = null;
    go('/login');
  }
  if (!res.ok) throw new ApiError(res.status, (data && data.error) || res.statusText || 'Fehler');
  return data;
}

export const api = {
  get: (u) => request('GET', u),
  post: (u, b = {}) => request('POST', u, b),
  put: (u, b = {}) => request('PUT', u, b),
  patch: (u, b = {}) => request('PATCH', u, b),
  del: (u) => request('DELETE', u)
};

/** POST JSON and return an object URL of the binary response (image preview). */
export async function postBlob(url, body) {
  const res = await fetch('/api' + url, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'X-Requested-With': 'scheck', 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  if (!res.ok) {
    const d = await res.json().catch(() => ({}));
    throw new ApiError(res.status, d.error || res.statusText);
  }
  return URL.createObjectURL(await res.blob());
}

/** Multipart upload with progress callback (0..1). */
export function upload(url, formData, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/api' + url);
    xhr.setRequestHeader('X-Requested-With', 'scheck');
    xhr.upload.onprogress = (e) => e.lengthComputable && onProgress?.(e.loaded / e.total);
    xhr.onload = () => {
      let data = null;
      try { data = JSON.parse(xhr.responseText); } catch { /* ignore */ }
      if (xhr.status >= 200 && xhr.status < 300) resolve(data);
      else reject(new ApiError(xhr.status, data?.error || 'Upload fehlgeschlagen'));
    };
    xhr.onerror = () => reject(new ApiError(0, 'Netzwerkfehler beim Upload'));
    xhr.send(formData);
  });
}

export const fmt = {
  date(d) {
    if (!d) return '–';
    return new Date(d).toLocaleString('de-DE', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' });
  },
  bytes(n) {
    if (!n) return '0 B';
    const u = ['B', 'KB', 'MB', 'GB'];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return `${n.toFixed(i ? 1 : 0)} ${u[i]}`;
  },
  num: (n) => (n ?? 0).toLocaleString('de-DE')
};

export const labels = {
  severity: { error: 'Fehler', warning: 'Warnung', info: 'Hinweis' },
  category: {
    lists: 'Listen & Duplikate',
    titles: 'Überschriften & Schreibung',
    references: 'Verweise',
    consistency: 'Konsistenz',
    rules: 'Business Rules',
    llm: 'KI-Review',
    images: 'Bilder'
  },
  status: { open: 'Offen', accepted: 'Bestätigt', dismissed: 'Verworfen', fixed: 'Behoben' },
  imageStatus: {
    pending: 'Ausstehend', detecting: 'Wird geprüft', review: 'Körperteile erkannt', clean: 'Unbedenklich',
    anonymized: 'Anonymisiert', unsupported: 'Manuell prüfen', failed: 'Fehler'
  },
  bodyPart: {
    hand: 'Hand', finger: 'Finger', face: 'Gesicht', head: 'Kopf', arm: 'Arm', leg: 'Bein',
    foot: 'Fuß', torso: 'Oberkörper', person: 'Person', skin: 'Hautbereich'
  },
  mode: { silhouette: 'Silhouette (Form bleibt)', outline: 'Strichzeichnung', pixelate: 'Verpixeln', blur: 'Weichzeichnen', solid: 'Abdecken', none: 'Nicht verändern' },
  kind: { dm: 'Datenmodul', pm: 'Publikationsmodul', dml: 'DM-Liste', brex: 'BREX', icn: 'Grafik (ICN)', xml: 'XML', other: 'Sonstige' },
  role: { owner: 'Eigentümer', editor: 'Redakteur', viewer: 'Leser' },
  ruleType: {
    xpath_forbidden: 'XPath – verboten',
    xpath_required: 'XPath – erforderlich',
    xpath_values: 'XPath – zulässige Werte',
    forbidden_terms: 'Unzulässige Begriffe',
    title_case: 'Schreibweise Überschriften',
    sentence_length: 'Satzlänge',
    dictionary: 'Projektwörterbuch'
  }
};
