<script>
  import SortTh from '../components/SortTh.svelte';
  import { createSorter } from '../lib/sort.svelte.js';
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

  const stateOf = (p) => p.snapshotCount === 0 ? 'none' : !p.latestRunStatus ? 'unchecked' : ['queued', 'running'].includes(p.latestRunStatus) ? 'running'
    : p.latestRunStatus === 'failed' ? 'failed' : p.openFindings > 0 ? 'open' : 'clean';
  const stateText = { none: 'kein Upload', unchecked: 'nicht geprüft', running: 'Analyse läuft', failed: 'Analyse fehlgeschlagen', open: 'Befunde offen', clean: 'ohne Befunde' };
  const stateBadge = { none: 'outline', unchecked: 'warning', running: 'info', failed: 'error', open: 'warning', clean: 'ok' };
  const stateRank = { failed: 0, open: 1, unchecked: 2, running: 3, clean: 4, none: 5 };
  const sorter = createSorter('name', {
    name: (p) => p.name, standard: (p) => p.standard, last: (p) => p.lastUpload ? new Date(p.lastUpload).getTime() : null,
    open: (p) => p.openFindings, state: (p) => stateRank[stateOf(p)], release: (p) => p.latestRelease || 'z', role: (p) => labels.role[p.myRole]
  }, 'asc', 'scheck.sort.projects');
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
  <header class="pagehead">
    <div class="titles">
      <h1>Projekte</h1>
      <p class="sub">Jedes Projekt hat eigene Uploads, Business Rules, Freigabekriterien und Mitglieder.</p>
    </div>
    <div class="row actions">
      <label class="search"><span class="sr-only">Projekte filtern</span>
        <input type="search" placeholder="Projekte filtern" bind:value={filter} /></label>
      <button class="primary" onclick={() => (showNew = true)}>Neues Projekt</button>
    </div>
  </header>

  {#if loadError}
    <LoadError what="Die Projekte" error={loadError} onretry={load} />
  {:else if projects === null}
    <div class="empty" role="status">Projekte werden geladen …</div>
  {:else if projects.length === 0}
    <section class="card card-pad stack empty-state">
      <h2>Noch keine Projekte</h2>
      <p class="muted">Legen Sie ein Projekt an und laden Sie danach den Export Ihres CSDB- oder Redaktionssystems hoch (ZIP, Ordner, einzelne XML-Dateien oder ein HTML-Export).</p>
      <div><button class="primary" onclick={() => (showNew = true)}>Erstes Projekt anlegen</button></div>
    </section>
  {:else}
    <section class="card">
      <div class="panel-head">
        <h2 class="grow">{filtered.length === projects.length ? `${projects.length} Projekte` : `${filtered.length} von ${projects.length} Projekten`}</h2>
      </div>
      <table>
        <thead>
          <tr>
            <SortTh {sorter} key="name">Projekt</SortTh>
            <SortTh {sorter} key="standard">Standard</SortTh>
            <SortTh {sorter} key="last">Letzter Upload</SortTh>
            <SortTh {sorter} key="open" align="right">Offene Befunde</SortTh>
            <SortTh {sorter} key="state">Prüfstatus</SortTh>
            <SortTh {sorter} key="release">Freigabe</SortTh>
            <SortTh {sorter} key="role">Ihre Rolle</SortTh>
          </tr>
        </thead>
        <tbody>
          {#each sorter.apply(filtered) as p (p.id)}
            {@const st = stateOf(p)}
            <tr>
              <td class="name">
                <a href={href(`/p/${p.id}`)}>{p.name}</a>
                {#if p.description}<span class="desc">{p.description}</span>{/if}
              </td>
              <td data-label="Standard">{p.standard}</td>
              <td class="nowrap" data-label="Letzter Upload">{#if p.snapshotCount}{fmt.date(p.lastUpload)}<span class="desc">{p.latestLabel}, {p.snapshotCount} {p.snapshotCount === 1 ? 'Upload' : 'Uploads'}</span>{:else}<span class="muted">kein Upload</span>{/if}</td>
              <td class="right" data-label="Offene Befunde">{st === 'open' || st === 'clean' ? fmt.num(p.openFindings) : ''}</td>
              <td data-label="Prüfstatus"><span class="badge {stateBadge[st]}">{stateText[st]}</span></td>
              <td data-label="Freigabe">{#if p.latestRelease === 'approved'}<span class="badge ok">freigegeben</span>{:else if p.latestRelease === 'rejected'}<span class="badge error">abgelehnt</span>{:else}<span class="muted">offen</span>{/if}</td>
              <td data-label="Ihre Rolle">{labels.role[p.myRole] || p.myRole}</td>
            </tr>
          {:else}
            <tr><td colspan="7" class="empty">Kein Projekt passt zum Filter „{filter}“.</td></tr>
          {/each}
        </tbody>
      </table>
    </section>
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
  .actions { flex-wrap: wrap; }
  .search { width: 260px; }
  .name a { font-weight: 600; }
  .desc { display: block; color: var(--muted); font-size: var(--t-sm); }
  .empty-state { max-width: 640px; }
  @media (max-width: 760px) {
    .actions, .search { width: 100%; }
    .actions button { flex: 1; }
    /* one project per block; sort buttons stay available as a bar */
    table, tbody, tr, td { display: block; }
    thead tr { display: flex; flex-wrap: wrap; gap: var(--s-1); padding: var(--s-2) var(--s-3); background: var(--surface-2); border-bottom: 1px solid var(--line); }
    thead :global(th) { display: block; border: none; padding: 0; background: none; }
    thead :global(th .sort) { min-height: 44px; padding: 0 var(--s-2); border: 1px solid var(--line-strong); border-radius: var(--r-1); background: var(--surface); }
    tbody tr { padding: var(--s-3) var(--s-4); border-bottom: 1px solid var(--line); }
    tbody td { border: none; padding: 2px 0; text-align: left; white-space: normal; }
    tbody td[data-label]::before { content: attr(data-label) ': '; color: var(--muted); font-size: var(--t-sm); }
    tbody td.name { margin-bottom: var(--s-1); }
    tbody td.name a { display: inline-block; min-height: 32px; }
  }
</style>
