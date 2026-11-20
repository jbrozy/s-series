<script>
  // Personal settings. Every change applies at once and is stored for the account.
  import { prefs, savePrefs } from '../lib/prefs.svelte.js';
  import { href, go } from '../lib/router.svelte.js';
  import { toastError, session } from '../lib/state.svelte.js';
  import { api } from '../lib/api.js';
  import { desktop } from '../lib/desktop.svelte.js';

  async function switchServer() {
    desktop.wantServer = true;
    await api.post('/auth/logout').catch(() => {});
    session.user = null;
    go('/login', { server: '1' });
  }

  let status = $state('');
  let timer;

  async function set(partial, text) {
    try {
      await savePrefs(partial);
      status = `${text} gespeichert.`;
      clearTimeout(timer);
      timer = setTimeout(() => (status = ''), 4000);
    } catch (e) { toastError(e); }
  }
</script>

<div class="page narrow">
  <header class="pagehead">
    <div class="titles">
      <nav class="crumbs" aria-label="Pfad"><ol><li><a href={href('/')}>Projekte</a></li><li aria-current="page">Einstellungen</li></ol></nav>
      <h1>Einstellungen</h1>
      <p class="sub">Gilt für Ihr Konto auf allen Arbeitsplätzen. Änderungen wirken sofort.</p>
    </div>
  </header>

  <p class="sr-only" role="status">{status}</p>
  {#if status}<p class="ok-box saved" aria-hidden="true">{status}</p>{/if}

  <section class="card" aria-labelledby="h-look">
    <div class="panel-head"><h2 id="h-look">Darstellung</h2></div>
    <div class="card-pad stack">
      <fieldset class="stack">
        <legend class="lbl">Design</legend>
        <label class="opt"><input type="radio" name="theme" value="system" checked={prefs.theme === 'system'} onchange={() => set({ theme: 'system' }, 'Design')} aria-labelledby="opt1-l" aria-describedby="opt1-d" />
          <span><strong id="opt1-l">Wie im Betriebssystem</strong><span class="hint block" id="opt1-d">Folgt der Einstellung von Windows bzw. des Browsers.</span></span></label>
        <label class="opt"><input type="radio" name="theme" value="light" checked={prefs.theme === 'light'} onchange={() => set({ theme: 'light' }, 'Design')} aria-labelledby="opt2-l" aria-describedby="opt2-d" />
          <span><strong id="opt2-l">Hell</strong><span class="hint block" id="opt2-d">Empfohlen für helle Räume und zur Beurteilung von Grafiken.</span></span></label>
        <label class="opt"><input type="radio" name="theme" value="dark" checked={prefs.theme === 'dark'} onchange={() => set({ theme: 'dark' }, 'Design')} aria-labelledby="opt3-l" aria-describedby="opt3-d" />
          <span><strong id="opt3-l">Dunkel</strong><span class="hint block" id="opt3-d">Weniger Leuchtdichte bei gedämpftem Raumlicht.</span></span></label>
      </fieldset>
      <fieldset class="stack">
        <legend class="lbl">Dichte von Tabellen und Listen</legend>
        <label class="opt"><input type="radio" name="density" value="normal" checked={prefs.density === 'normal'} onchange={() => set({ density: 'normal' }, 'Dichte')} aria-labelledby="opt4-l" />
          <span><strong id="opt4-l">Normal</strong></span></label>
        <label class="opt"><input type="radio" name="density" value="compact" checked={prefs.density === 'compact'} onchange={() => set({ density: 'compact' }, 'Dichte')} aria-labelledby="opt5-l" aria-describedby="opt5-d" />
          <span><strong id="opt5-l">Kompakt</strong><span class="hint block" id="opt5-d">Mehr Zeilen auf einen Blick, kleinere Abstände.</span></span></label>
      </fieldset>
      <p class="hint">Schrift größer stellen: Browser-Zoom mit <kbd>Strg</kbd> und <kbd>+</kbd>, zurück mit <kbd>Strg</kbd> und <kbd>0</kbd>. Die Oberfläche bleibt bis 200&nbsp;% bedienbar.</p>
    </div>
  </section>

  <section class="card" aria-labelledby="h-work">
    <div class="panel-head"><h2 id="h-work">Arbeitsweise</h2></div>
    <div class="card-pad stack">
      <fieldset class="stack">
        <legend class="lbl">Startseite nach der Anmeldung</legend>
        <label class="opt"><input type="radio" name="start" value="projects" checked={prefs.startPage === 'projects'} onchange={() => set({ startPage: 'projects' }, 'Startseite')} aria-labelledby="opt6-l" />
          <span><strong id="opt6-l">Projektliste</strong></span></label>
        <label class="opt"><input type="radio" name="start" value="last" checked={prefs.startPage === 'last'} onchange={() => set({ startPage: 'last' }, 'Startseite')} aria-labelledby="opt7-l" />
          <span><strong id="opt7-l">Zuletzt geöffnetes Projekt</strong></span></label>
      </fieldset>
      <label class="opt"><input type="checkbox" checked={prefs.shortcuts} onchange={(e) => set({ shortcuts: e.currentTarget.checked }, 'Tastenkürzel')} aria-labelledby="opt8-l" aria-describedby="opt8-d" />
        <span><strong id="opt8-l">Einzeltasten-Kürzel verwenden</strong><span class="hint block" id="opt8-d">Zum Beispiel <kbd>/</kbd> für die Suche oder <kbd>j</kbd>/<kbd>k</kbd> in der Befundliste. Ausschalten, wenn Sie Spracheingabe nutzen oder versehentlich Tasten auslösen. <kbd>Strg</kbd>+<kbd>K</kbd>, <kbd>F1</kbd> und <kbd>Esc</kbd> bleiben aktiv.</span></span></label>
      <label class="opt"><input type="checkbox" checked={prefs.navHidden} onchange={(e) => set({ navHidden: e.currentTarget.checked }, 'Navigation')} aria-labelledby="opt9-l" aria-describedby="opt9-d" />
        <span><strong id="opt9-l">Seitenleiste ausblenden</strong><span class="hint block" id="opt9-d">Mehr Platz für Tabellen. Einblenden jederzeit über „Navigation“ oben links.</span></span></label>
    </div>
  </section>

  {#if desktop.on}
    <section class="card" aria-labelledby="h-conn">
      <div class="panel-head"><h2 id="h-conn">Verbindung</h2></div>
      <div class="card-pad stack">
        <dl class="kv">
          <dt>Server</dt><dd class="mono">{desktop.server}</dd>
          <dt>Programmversion</dt><dd>S-Check Desktop {desktop.version}</dd>
        </dl>
        <p class="hint">Projekte, Uploads und Befunde liegen ausschließlich auf dem Server. Auf diesem Rechner speichert S-Check nur die Serveradresse und die Fenstereinstellungen.</p>
        {#if desktop.locked}
          <p class="hint">Die Serveradresse ist von Ihrer IT vorgegeben.</p>
        {:else}
          <div><button onclick={switchServer}>Abmelden und Server wechseln</button></div>
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
  .narrow { max-width: 820px; display: flex; flex-direction: column; gap: var(--s-4); }
  .pagehead { margin-bottom: 0; }
  .opt { display: flex; gap: var(--s-3); align-items: flex-start; }
  .opt input { margin-top: 3px; flex: none; }
  .block { display: block; }
  .saved { align-self: flex-start; }
  .kv { display: grid; grid-template-columns: max-content 1fr; gap: var(--s-1) var(--s-4); margin: 0; }
  .kv dt { color: var(--muted); }
  .kv dd { margin: 0; overflow-wrap: anywhere; }
</style>
