// Minimal hash router: #/path/segments?query
export const route = $state({ path: '/', parts: [], query: {} });

function parse() {
  const raw = decodeURIComponent(location.hash.slice(1)) || '/';
  const [p, q] = raw.split('?');
  route.path = p || '/';
  route.parts = route.path.split('/').filter(Boolean);
  route.query = Object.fromEntries(new URLSearchParams(q || ''));
}

window.addEventListener('hashchange', parse);
parse();

export function go(path, query) {
  let h = '#' + path;
  if (query) {
    const qs = new URLSearchParams(Object.entries(query).filter(([, v]) => v !== undefined && v !== null && v !== '')).toString();
    if (qs) h += '?' + qs;
  }
  if (location.hash !== h) location.hash = h;
}

export function href(path, query) {
  let h = '#' + path;
  if (query) {
    const qs = new URLSearchParams(Object.entries(query).filter(([, v]) => v !== undefined && v !== null && v !== '')).toString();
    if (qs) h += '?' + qs;
  }
  return h;
}
