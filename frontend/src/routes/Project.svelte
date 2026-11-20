<script>
  import { onMount, onDestroy } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import { route, go, href } from '../lib/router.svelte.js';
  import { toastError } from '../lib/state.svelte.js';
  import Overview from '../components/Overview.svelte';
  import Findings from '../components/Findings.svelte';
  import Images from '../components/Images.svelte';
  import Rules from '../components/Rules.svelte';
  import Knowledge from '../components/Knowledge.svelte';
  import Training from '../components/Training.svelte';
  import Members from '../components/Members.svelte';
  import Release from '../components/Release.svelte';
  import AuditLog from '../components/AuditLog.svelte';
  import { projectNav, projectSections } from '../lib/nav.svelte.js';
  import LoadError from '../components/LoadError.svelte';

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
    loadError = '';
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

  const sectionLabel = $derived(projectSections.find(([k]) => k === tab)?.[1] || 'Übersicht');

  // side navigation: project, counts and release state of the selected upload
  $effect(() => {
    if (!project) return;
    projectNav.project = { id: project.id, name: project.name, myRole: project.myRole };
    projectNav.snapshotId = route.query.snap || '';
    projectNav.counts = { findings: openFindings, images: imagesToReview };
  });
  $effect(() => {
    const id = snapshot?.id;
    const done = snapshot?.latestRun?.status === 'done';
    projectNav.release = '';
    if (!id || !done) return;
    api.get(`/snapshots/${id}/release`).then((r) => {
      projectNav.release = r.current && !r.stale ? r.current.decision : '';
    }).catch(() => {});
  });
  onDestroy(() => { projectNav.project = null; projectNav.counts = {}; projectNav.release = ''; });
</script>

<div class="page">
  {#if loadError}
    <LoadError what="Das Projekt" error={loadError} onretry={load} />
  {:else if !project}
    <div class="empty" role="status">Projekt wird geladen …</div>
  {:else}
    <header class="pagehead">
      <div class="titles">
        <nav class="crumbs" aria-label="Pfad">
          <ol>
            <li><a href={href('/')}>Projekte</a></li>
            {#if tab === 'overview'}<li aria-current="page">{project.name}</li>
            {:else}<li><a href={href(`/p/${pid}/overview`, { snap: route.query.snap })}>{project.name}</a></li><li aria-current="page">{sectionLabel}</li>{/if}
          </ol>
        </nav>
        <h1>{tab === 'overview' ? project.name : sectionLabel}</h1>
        <p class="sub">
          {#if tab === 'overview'}{project.description || `${project.standard} · ${labels.role[project.myRole]}`}{:else}{project.name}{/if}
        </p>
      </div>
      {#if snapshots.length}
        <label class="snapsel">
          <span class="lbl">Upload / Stand</span>
          <select value={sid} onchange={(e) => selectSnapshot(e.currentTarget.value)}>
            {#each snapshots as s}
              <option value={s.id}>{s.label}, {fmt.date(s.createdAt)}</option>
            {/each}
          </select>
        </label>
      {/if}
    </header>

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
      {#key snapshot.id}<Knowledge {project} {snapshot} />{/key}
    {:else if tab === 'training'}
      {#key snapshot.id}<Training {snapshot} {canEdit} />{/key}
    {:else if tab === 'members'}
      <Members {project} />
    {:else if tab === 'release'}
      {#key snapshot?.id}<Release {project} {snapshot} onchange={refreshSnapshots} />{/key}
    {:else if tab === 'audit' && canEdit}
      <AuditLog pid={project.id} />
    {/if}
  {/if}
</div>

<style>
  .snapsel { width: 340px; max-width: 100%; }
  @media (max-width: 640px) { .snapsel { width: 100%; } }
</style>
