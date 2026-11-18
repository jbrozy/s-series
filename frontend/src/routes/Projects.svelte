<script>
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

  const filtered = $derived(
    (projects || []).filter((p) => !filter || (p.name + ' ' + p.description).toLowerCase().includes(filter.toLowerCase()))
  );

  onMount(load);
  async function load() {
    try { projects = await api.get('/projects'); } catch (e) { toastError(e); projects = []; }
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

  {#if projects === null}
    <div class="empty">Lade …</div>
  {:else if projects.length === 0}
    <div class="card empty-state">
      <h2>Noch keine Projekte</h2>
      <p class="muted">Legen Sie ein Projekt an und laden Sie den Export Ihres CSDB-/Redaktionssystems hoch (ZIP, Ordner oder einzelne XML-Dateien und Grafiken).</p>
      <button class="primary" onclick={() => (showNew = true)}>Erstes Projekt anlegen</button>
    </div>
  {:else}
    <div class="list">
      {#each filtered as p (p.id)}
        <a class="card proj" href={href(`/p/${p.id}`)}>
          <div class="row">
            <h2 class="grow ellipsis">{p.name}</h2>
            <span class="badge outline">{p.standard}</span>
          </div>
          <p class="desc muted">{p.description || 'Keine Beschreibung'}</p>
          <div class="meta row wrap">
            <span>{p.snapshotCount} {p.snapshotCount === 1 ? 'Upload' : 'Uploads'}</span>
            <span>·</span>
            <span>zuletzt {fmt.date(p.lastUpload)}</span>
            <span class="spacer"></span>
            {#if p.snapshotCount === 0}
              <span class="badge outline">noch kein Upload</span>
            {:else if !p.latestRunStatus}
              <span class="badge warning" title="Neuester Upload: {p.latestLabel}">Neuester Upload nicht geprüft</span>
            {:else if p.latestRunStatus === 'queued' || p.latestRunStatus === 'running'}
              <span class="badge info">Analyse läuft</span>
            {:else if p.latestRunStatus === 'failed'}
              <span class="badge error">Analyse fehlgeschlagen</span>
            {:else if p.openFindings > 0}
              <span class="badge warning" title="Neuester Upload: {p.latestLabel}">{fmt.num(p.openFindings)} offene Befunde</span>
            {:else}
              <span class="badge ok">keine offenen Befunde</span>
            {/if}
            <span class="badge">{labels.role[p.myRole] || p.myRole}</span>
          </div>
        </a>
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
  .head { margin-bottom: 22px; align-items: flex-end; }
  .search { max-width: 260px; }
  .list { display: grid; grid-template-columns: repeat(auto-fill, minmax(380px, 1fr)); gap: 16px; }
  .proj { padding: 18px 20px; color: var(--ink); display: flex; flex-direction: column; gap: 8px; transition: border-color .12s, transform .12s; }
  .proj:hover { text-decoration: none; border-color: var(--accent); }
  .desc { min-height: 20px; }
  .meta { font-size: 13px; color: var(--muted); gap: 8px; margin-top: 4px; }
  .empty-state { padding: 48px; text-align: center; display: flex; flex-direction: column; gap: 12px; align-items: center; }
  .empty-state p { max-width: 520px; }
</style>
