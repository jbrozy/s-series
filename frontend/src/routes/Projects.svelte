<script>
  import LoadError from '../components/LoadError.svelte';
  import { onMount } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import { go, href } from '../lib/router.svelte.js';
  import { toastError } from '../lib/state.svelte.js';
  import Modal from '../components/Modal.svelte';

  let projects = $state(null);
  let showNew = $state(false);
  let name = $state('');
  let description = $state('');
  let standard = $state('S1000D');
  let filter = $state('');
  let loadError = $state(null);

  const filtered = $derived(
    (projects || []).filter((p) => !filter || (p.name + ' ' + p.description).toLowerCase().includes(filter.toLowerCase()))
  );

  onMount(load);
  async function load() {
    loadError = null;
    try { projects = await api.get('/projects'); } catch (e) { loadError = e; projects = null; }
  }

  async function create(e) {
    e.preventDefault();
    try {
      const p = await api.post('/projects', { name, description, standard });
      showNew = false;
      go(`/p/${p.id}`);
    } catch (err) { toastError(err); }
  }
</script>

<div class="page">
  <div class="row head">
    <div class="grow">
      <h1>Projekte</h1>
      <p class="muted">Jedes Projekt hat eigene Business Rules, Uploads und Prüfergebnisse.</p>
    </div>
    <input class="search" aria-label="Projekte filtern" placeholder="Projekte filtern …" bind:value={filter} />
    <button class="primary" onclick={() => (showNew = true)}>+ Neues Projekt</button>
  </div>

  {#if loadError}
    <LoadError what="Die Projekte" error={loadError} onretry={load} />
  {:else if projects === null}
    <div class="empty" role="status">Projekte werden geladen …</div>
  {:else if projects.length === 0}
    <div class="card empty-state">
      <h2>Noch keine Projekte</h2>
      <p class="muted">Legen Sie ein Projekt an und laden Sie den Export Ihres CSDB-/Redaktionssystems hoch (ZIP, Ordner oder einzelne XML-Dateien und Grafiken). Ein HTML-Export geht ebenfalls.</p>
      <button class="primary" onclick={() => (showNew = true)}>Erstes Projekt anlegen</button>
    </div>
  {:else}
    <div class="register card">
      <div class="reg-head label-quiet" aria-hidden="true">
        <span>Projekt</span><span>Stand</span><span>Prüfstatus</span>
      </div>
      {#each filtered as p (p.id)}
        {@const state = p.snapshotCount === 0 ? 'none' : !p.latestRunStatus ? 'unchecked' : ['queued', 'running'].includes(p.latestRunStatus) ? 'running' : p.latestRunStatus === 'failed' ? 'failed' : p.openFindings > 0 ? 'open' : 'clean'}
        <a class="proj state-{state}" href={href(`/p/${p.id}`)}>
          <div class="who">
            <div class="row name">
              <h2 class="ellipsis">{p.name}</h2>
              <span class="badge outline">{p.standard}</span>
            </div>
            <p class="desc">{p.description || 'Keine Beschreibung'}</p>
          </div>
          <div class="when">
            {#if p.snapshotCount}
              <span>{p.snapshotCount} {p.snapshotCount === 1 ? 'Upload' : 'Uploads'}</span>
              <span class="muted">zuletzt {fmt.date(p.lastUpload)}</span>
            {:else}
              <span class="muted">Kein Upload</span>
            {/if}
          </div>
          <div class="status">
            {#if state === 'none'}
              <span class="badge outline">wartet auf ersten Upload</span>
            {:else if state === 'unchecked'}
              <span class="badge warning" title="Neuester Upload: {p.latestLabel}">Neuester Upload nicht geprüft</span>
            {:else if state === 'running'}
              <span class="badge info">Analyse läuft</span>
            {:else if state === 'failed'}
              <span class="badge error">Analyse fehlgeschlagen</span>
            {:else if state === 'open'}
              <span class="badge warning" title="Neuester Upload: {p.latestLabel}">{fmt.num(p.openFindings)} offene Befunde</span>
            {:else}
              <span class="badge ok">keine offenen Befunde</span>
            {/if}
            {#if p.latestRelease === 'approved'}<span class="badge ok">freigegeben</span>
            {:else if p.latestRelease === 'rejected'}<span class="badge error">abgelehnt</span>{/if}
            <span class="role">{labels.role[p.myRole] || p.myRole}</span>
          </div>
        </a>
      {:else}
        <p class="empty">Kein Projekt passt zum Filter „{filter}“.</p>
      {/each}
    </div>
  {/if}
</div>

{#if showNew}
  <Modal title="Neues Projekt" onclose={() => (showNew = false)}>
    <form class="stack" onsubmit={create}>
      <label><span class="lbl">Projektname</span><input bind:value={name} required placeholder="z. B. PW800 Engine Manual" /></label>
      <label><span class="lbl">Beschreibung</span><input bind:value={description} placeholder="optional" /></label>
      <label>
        <span class="lbl">Standard</span>
        <select bind:value={standard}>
          <option>S1000D</option><option>S2000M</option><option>S3000L</option><option>S4000P</option><option>S5000F</option><option>S6000T</option>
        </select>
        <span class="hint">Neue Projekte starten mit sinnvollen Standardregeln (Terminologie, Satzlänge, Wörterbuch), die Sie anpassen können.</span>
      </label>
      <div class="row"><span class="spacer"></span>
        <button type="button" onclick={() => (showNew = false)}>Abbrechen</button>
        <button class="primary" type="submit">Anlegen</button>
      </div>
    </form>
  </Modal>
{/if}

<style>
  .head { margin-bottom: var(--s-5); align-items: flex-end; flex-wrap: wrap; row-gap: var(--s-3); }
  .head p { margin-top: var(--s-1); }
  .search { width: 260px; }

  .register { overflow: hidden; }
  .reg-head, .proj {
    display: grid; grid-template-columns: minmax(0, 1fr) 190px 280px; gap: var(--s-5); align-items: center;
    padding: var(--s-3) var(--s-5) var(--s-3) calc(var(--s-5) - 4px);
  }
  .reg-head { background: var(--surface-2); border-bottom: 1px solid var(--ink-2); padding-top: var(--s-2); padding-bottom: var(--s-2); border-left: 4px solid transparent; }
  .proj { color: var(--ink); border-left: 4px solid var(--line-strong); border-top: 1px solid var(--line); }
  .proj:first-of-type { border-top: none; }
  .proj:hover { text-decoration: none; background: #fbf9f4; }
  .proj:hover h2 { color: var(--accent); text-decoration: underline; text-underline-offset: 3px; }
  .state-open, .state-unchecked { border-left-color: var(--warning); }
  .state-failed { border-left-color: var(--error); }
  .state-running { border-left-color: var(--info); }
  .state-clean { border-left-color: var(--ok); }
  .name { gap: var(--s-2); }
  .name h2 { font-size: 19px; }
  .desc { color: var(--muted); font-size: var(--t-sm); margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .when { display: flex; flex-direction: column; font-size: var(--t-sm); }
  .status { display: flex; align-items: center; gap: var(--s-2); flex-wrap: wrap; }
  .role { font-size: var(--t-sm); color: var(--muted); }

  .empty-state { padding: var(--s-7) var(--s-5); text-align: center; display: flex; flex-direction: column; gap: var(--s-3); align-items: center; border-style: dashed; border-color: var(--line-strong); }
  .empty-state p { max-width: 520px; }

  @media (max-width: 900px) {
    .reg-head { display: none; }
    .proj { grid-template-columns: minmax(0, 1fr); gap: var(--s-2); padding: var(--s-3) var(--s-4) var(--s-3) calc(var(--s-4) - 4px); }
    .when { flex-direction: row; gap: var(--s-2); }
    .search { width: 100%; }
    .head .grow { flex-basis: 100%; }
  }
</style>
