<script>
  import { onMount, onDestroy } from 'svelte';
  import { api, fmt } from '../lib/api.js';
  import { route, go, href } from '../lib/router.svelte.js';
  import { toastError } from '../lib/state.svelte.js';
  import Overview from '../components/Overview.svelte';
  import Findings from '../components/Findings.svelte';
  import Images from '../components/Images.svelte';
  import Rules from '../components/Rules.svelte';
  import Knowledge from '../components/Knowledge.svelte';
  import Training from '../components/Training.svelte';
  import Members from '../components/Members.svelte';

  let { pid } = $props();
  let project = $state(null);
  let snapshots = $state([]);
  let loadError = $state('');
  let timer;

  const tab = $derived(route.parts[2] || 'overview');
  const sid = $derived(route.query.snap || snapshots[0]?.id || '');
  const snapshot = $derived(snapshots.find((s) => s.id === sid) || null);
  const canEdit = $derived(project && (project.myRole === 'owner' || project.myRole === 'editor'));
  const busy = $derived(
    snapshots.some((s) => ['uploaded', 'indexing'].includes(s.status) || ['queued', 'running'].includes(s.latestRun?.status))
  );
  const openFindings = $derived(
    snapshot?.latestRun?.status === 'done'
      ? Object.values(snapshot.latestRun.summary?.bySeverity || {}).reduce((a, b) => a + b, 0)
      : null
  );
  // most recent other upload with results (fallback while the current one is not analysed)
  const previous = $derived(snapshots.find((s) => s.id !== sid && s.latestRun?.status === 'done') || null);
  const imagesToReview = $derived((snapshot?.images?.review || 0) + (snapshot?.images?.failed || 0));

  onMount(load);
  onDestroy(() => clearTimeout(timer));

  async function load() {
    try {
      [project, snapshots] = await Promise.all([api.get(`/projects/${pid}`), api.get(`/projects/${pid}/snapshots`)]);
    } catch (e) {
      loadError = e.message;
    }
    schedule();
  }

  async function refreshSnapshots() {
    try { snapshots = await api.get(`/projects/${pid}/snapshots`); } catch { /* keep */ }
    schedule();
  }

  function schedule() {
    clearTimeout(timer);
    if (busy) timer = setTimeout(refreshSnapshots, 2000);
  }

  function selectSnapshot(id) {
    go(`/p/${pid}/${tab}`, { snap: id });
  }

  const tabs = [
    ['overview', 'Übersicht'],
    ['findings', 'Befunde'],
    ['images', 'Bilder'],
    ['rules', 'Business Rules'],
    ['knowledge', 'Wissensbasis'],
    ['training', 'Trainingsdaten'],
    ['members', 'Mitglieder']
  ];
</script>

<div class="page">
  {#if loadError}
    <div class="error-box">{loadError}</div>
  {:else if !project}
    <div class="empty">Lade Projekt …</div>
  {:else}
    <div class="crumbs"><a href={href('/')}>Projekte</a> / <span>{project.name}</span></div>
    <div class="row head">
      <div class="grow">
        <h1>{project.name}</h1>
        {#if project.description}<p class="muted">{project.description}</p>{/if}
      </div>
      {#if snapshots.length}
        <label class="snapsel">
          <span class="lbl">Upload / Stand</span>
          <select value={sid} aria-label="Upload auswählen" onchange={(e) => selectSnapshot(e.currentTarget.value)}>
            {#each snapshots as s}
              <option value={s.id}>{s.label} – {fmt.date(s.createdAt)}</option>
            {/each}
          </select>
        </label>
      {/if}
    </div>

    <nav class="tabs" aria-label="Projektbereiche">
      {#each tabs as [key, label]}
        <a href={href(`/p/${pid}/${key}`, { snap: route.query.snap })} class:active={tab === key}>
          {label}
          {#if key === 'findings' && openFindings !== null}<span class="badge count {openFindings ? 'warning' : 'ok'}">{fmt.num(openFindings)}</span>{/if}
          {#if key === 'images' && imagesToReview}<span class="badge count error">{imagesToReview}</span>{/if}
        </a>
      {/each}
    </nav>

    {#if tab === 'overview'}
      <Overview {project} {snapshots} {snapshot} {canEdit} onchange={refreshSnapshots} />
    {:else if !snapshot && ['findings', 'images', 'knowledge', 'training'].includes(tab)}
      <div class="card empty">Noch kein Upload vorhanden. Laden Sie zuerst einen Projektexport unter <a href={href(`/p/${pid}/overview`)}>Übersicht</a> hoch.</div>
    {:else if tab === 'findings'}
      {#key snapshot.id + (snapshot.latestRun?.id || '') + (snapshot.latestRun?.status || '')}
        <Findings {project} {snapshot} {previous} {canEdit} onchange={refreshSnapshots} />
      {/key}
    {:else if tab === 'images'}
      {#key snapshot.id}
        <Images {project} {snapshot} {canEdit} />
      {/key}
    {:else if tab === 'rules'}
      <Rules {project} {snapshot} {canEdit} />
    {:else if tab === 'knowledge'}
      {#key snapshot.id}<Knowledge {snapshot} />{/key}
    {:else if tab === 'training'}
      {#key snapshot.id}<Training {snapshot} {canEdit} />{/key}
    {:else if tab === 'members'}
      <Members {project} />
    {/if}
  {/if}
</div>

<style>
  .crumbs { font-size: 13px; color: var(--muted); margin-bottom: 6px; }
  .head { align-items: flex-end; margin-bottom: 16px; }
  .snapsel { min-width: 320px; }
  .tabs .badge { margin-left: 6px; }
</style>
