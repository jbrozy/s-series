// Personal settings (ISO 9241-110 Steuerbarkeit, ArbStättV Anhang 6.5):
// stored on the server per account, cached locally so the theme is right
// before the first request returns.
import { api } from './api.js';

const CACHE = 'scheck.prefs';
const defaults = { theme: 'system', density: 'normal', startPage: 'projects', shortcuts: true, navHidden: false, lastProject: '' };

function cached() {
  try { return { ...defaults, ...JSON.parse(localStorage.getItem(CACHE) || '{}') }; } catch { return { ...defaults }; }
}

export const prefs = $state(cached());

const media = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: dark)') : null;

export function applyPrefs() {
  const dark = prefs.theme === 'dark' || (prefs.theme === 'system' && media?.matches);
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  document.documentElement.dataset.density = prefs.density;
  try { localStorage.setItem(CACHE, JSON.stringify(prefs)); } catch { /* storage unavailable: settings still apply for this visit */ }
}
media?.addEventListener?.('change', () => prefs.theme === 'system' && applyPrefs());

/** Take over the settings delivered with the user record. */
export function loadPrefs(p) {
  Object.assign(prefs, defaults, p || {});
  applyPrefs();
}

/** Change one or more settings; applies at once, then stores on the server. */
export async function savePrefs(partial) {
  Object.assign(prefs, partial);
  applyPrefs();
  const stored = await api.put('/auth/me/preferences', partial);
  Object.assign(prefs, stored);
  applyPrefs();
  return stored;
}
