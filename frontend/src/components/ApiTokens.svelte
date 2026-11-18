<script>
  // Personal API tokens for scripts and CI pipelines, plus the scheck-ci client.
  import { onMount } from 'svelte';
  import { api, fmt } from '../lib/api.js';
  import { toast, toastError } from '../lib/state.svelte.js';
  import Modal from './Modal.svelte';
  import LoadError from './LoadError.svelte';

  let tokens = $state(null);
  let tools = $state([]);
  let error = $state(null);
  let name = $state('');
  let days = $state('90');
  let formError = $state('');
  let busy = $state(false);
  let created = $state(null); // { token, info }
  let revokeTarget = $state(null);
  let copied = $state(false);

  onMount(load);
  async function load() {
    error = null;
    try {
      [tokens, tools] = await Promise.all([api.get('/tokens'), api.get('/tools').catch(() => [])]);
    } catch (e) { error = e; }
  }

  async function create(e) {
    e.preventDefault();
    if (!name.trim()) { formError = 'Bitte einen Namen angeben, z. B. „CSDB-Export Pipeline“.'; return; }
    busy = true;
    formError = '';
    try {
      created = await api.post('/tokens', { name, days: Number(days) });
      name = '';
      copied = false;
      load();
    } catch (err) { formError = err.message; } finally { busy = false; }
  }

  async function revoke() {
    try {
      await api.del(`/tokens/${revokeTarget.id}`);
      toast(`Token „${revokeTarget.name}“ widerrufen`);
      revokeTarget = null;
      load();
    } catch (e) { toastError(e); }
  }

  async function copy() {
    try { await navigator.clipboard.writeText(created.token); copied = true; } catch { copied = false; }
  }

  const tokenState = (t) => (t.revokedAt ? 'widerrufen' : t.expiresAt && new Date(t.expiresAt) < new Date() ? 'abgelaufen' : 'aktiv');
  const origin = typeof location !== 'undefined' ? location.origin : '';
  const fmtSize = (n) => `${(n / 1048576).toFixed(1).replace('.', ',')} MB`;
</script>

<section class="card card-pad stack">
  <h2>API-Tokens</h2>
  <p class="hint">Mit einem Token laden Skripte und CI-Pipelines Exporte hoch und lesen Ergebnisse und Freigabekriterien, mit Ihren Rechten in den Projekten. Konten, Tokens und die Administration sind mit Tokens nicht erreichbar.</p>

  <form class="row wrap newtoken" onsubmit={create} novalidate>
    <label class="grow"><span class="lbl">Name</span><input bind:value={name} maxlength="80" placeholder="z. B. CSDB-Export Pipeline" aria-invalid={!!formError} /></label>
    <label class="valid"><span class="lbl">Gültig</span>
      <select bind:value={days}>
        <option value="30">30 Tage</option><option value="90">90 Tage</option><option value="365">1 Jahr</option><option value="0">unbegrenzt</option>
      </select>
    </label>
    <button class="primary" type="submit" disabled={busy}>{busy ? 'Erstellt …' : 'Token erstellen'}</button>
  </form>
  {#if formError}<p class="error-box" role="alert">{formError}</p>{/if}

  {#if error}
    <LoadError what="Ihre Tokens" {error} onretry={load} />
  {:else if !tokens}
    <p class="muted" role="status">Tokens werden geladen …</p>
  {:else if tokens.length === 0}
    <p class="muted">Noch keine Tokens. Erstellen Sie eines pro Pipeline, damit Sie es einzeln widerrufen können.</p>
  {:else}
    <table>
      <thead><tr><th>Name</th><th class="opt">Kennung</th><th>Zuletzt benutzt</th><th class="opt">Läuft ab</th><th>Status</th><th><span class="sr-only">Aktionen</span></th></tr></thead>
      <tbody>
        {#each tokens as t (t.id)}
          <tr class:off={tokenState(t) !== 'aktiv'}>
            <td>{t.name}</td>
            <td class="opt"><code>{t.prefix}…</code></td>
            <td class="nowrap">{t.lastUsedAt ? fmt.date(t.lastUsedAt) : 'noch nie'}</td>
            <td class="nowrap opt">{t.expiresAt ? fmt.date(t.expiresAt) : 'nie'}</td>
            <td>{tokenState(t)}</td>
            <td class="right">{#if tokenState(t) === 'aktiv'}<button class="small danger" onclick={() => (revokeTarget = t)}>Widerrufen</button>{/if}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</section>

<section class="card card-pad stack">
  <h2>Prüfung in der Pipeline: scheck-ci</h2>
  <p class="hint">Das Werkzeug lädt einen Export hoch, wartet auf die Analyse und meldet das Ergebnis der Freigabekriterien als Rückgabewert: 0 erfüllt, 1 nicht erfüllt, 2 Fehler. Es braucht nur Netzzugriff auf diesen Server.</p>
  {#if tools.length}
    <div class="row wrap">
      {#each tools as t}<a class="btn" href={`/api/tools/${t.name}`} download>{t.name.includes('windows') ? 'Für Windows' : 'Für Linux'} ({fmtSize(t.size)})</a>{/each}
    </div>
  {:else}
    <p class="muted">Auf diesem Server liegen keine vorgebauten Programme. Im Docker-Image sind sie enthalten; lokal entsteht es mit <code>go build ./cmd/scheck-ci</code>.</p>
  {/if}
  <pre class="usage">export SCHECK_URL={origin}
export SCHECK_TOKEN=sck_…
export SCHECK_PROJECT=&lt;Projekt-ID aus der Adresszeile&gt;
scheck-ci -label "Issue 004" -csv befunde.csv export.zip</pre>
</section>

{#if created}
  <Modal title="Token erstellt" onclose={() => (created = null)}>
    <div class="stack">
      <p>Kopieren Sie das Token jetzt. Es wird nur dieses eine Mal angezeigt; gespeichert ist nur eine Prüfsumme.</p>
      <code class="secret">{created.token}</code>
      <div class="row">
        <button class="primary" onclick={copy}>{copied ? 'Kopiert' : 'In die Zwischenablage'}</button>
        <span class="spacer"></span>
        <button onclick={() => (created = null)}>Fertig</button>
      </div>
    </div>
  </Modal>
{/if}

{#if revokeTarget}
  <Modal title="Token widerrufen" onclose={() => (revokeTarget = null)}>
    <div class="stack">
      <p>„{revokeTarget.name}“ funktioniert danach sofort nicht mehr. Pipelines, die es benutzen, schlagen fehl, bis sie ein neues Token bekommen.</p>
      <div class="row"><span class="spacer"></span>
        <button onclick={() => (revokeTarget = null)}>Abbrechen</button>
        <button class="danger" onclick={revoke}>Widerrufen</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .newtoken { align-items: flex-end; }
  .valid { width: 160px; }
  tr.off td { color: var(--muted); }
  .usage { margin: 0; padding: var(--s-3); background: var(--header); color: #e9e6de; border-radius: var(--r-1); font-family: var(--mono); font-size: 12.5px; white-space: pre-wrap; overflow-wrap: anywhere; }
  .secret { display: block; padding: var(--s-3); background: var(--surface-2); border: 1px solid var(--line-strong); border-radius: var(--r-1); font-size: var(--t-md); overflow-wrap: anywhere; user-select: all; }
  @media (max-width: 760px) { table { display: table; } .opt { display: none; } td { white-space: normal; } }
  @media (max-width: 640px) { .valid { width: 100%; } .newtoken button { width: 100%; justify-content: center; } }
</style>
