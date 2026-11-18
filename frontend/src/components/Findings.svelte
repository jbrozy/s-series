<script>
  import { onMount, untrack } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import { route, go, href } from '../lib/router.svelte.js';
  import { toastError, session } from '../lib/state.svelte.js';
  import FileViewer from './FileViewer.svelte';
  import Modal from './Modal.svelte';
  import FindingActivity from './FindingActivity.svelte';
  import LoadError from './LoadError.svelte';

  let { project, snapshot, previous = null, canEdit, onchange } = $props();
  const latest = untrack(() => snapshot.latestRun);
  let starting = $state(false);
  const defaultOptions = { lists: true, titles: true, references: true, consistency: true, rules: true, llm: false, images: true, llmMaxDocs: 25 };

  async function startAnalysis() {
    starting = true;
    try {
      await api.post(`/snapshots/${snapshot.id}/runs`, latest?.options || defaultOptions);
      onchange?.();
    } catch (e) {
      toastError(e);
      starting = false;
    }
  }

  let runs = $state([]);
  let runId = $state(untrack(() => (snapshot.latestRun?.status === 'done' ? snapshot.latestRun.id : '')));
  let data = $state(null);
  let loading = $state(false);
  let q = $state(route.query.q || '');
  let severity = $state(route.query.severity || '');
  let category = $state(route.query.category || '');
  let status = $state(route.query.status || 'open');
  let fileId = $state(route.query.file || '');
  let assignee = $state(route.query.assignee || '');
  let change = $state(route.query.change || '');
  let members = $state([]);
  let activityFor = $state(null);
  let dismissError = $state('');
  let loadError = $state(null);
  let resolved = $state(null);
  let offset = $state(0);
  const limit = 100;
  let expanded = $state({});
  let viewer = $state(null);
  let dismissTarget = $state(null);
  let dismissNote = $state('');
  let qTimer;

  onMount(async () => {
    runs = (await api.get(`/snapshots/${snapshot.id}/runs`).catch(() => [])).filter((r) => r.status === 'done');
    if (!runId && latest?.status === 'done' && runs[0]) runId = runs[0].id;
    load();
    if (canEdit) members = (await api.get(`/projects/${project.id}/members`).catch(() => [])).filter((m) => !m.disabled);
  });

  async function load() {
    if (!runId) return;
    loading = true;
    loadError = null;
    if (change === 'resolved') {
      try { resolved = await api.get(`/runs/${runId}/resolved`); } catch (e) { loadError = e; } finally { loading = false; }
      if (!data) data = await api.get(`/runs/${runId}/findings?limit=1`).catch(() => null);
      return;
    }
    const p = new URLSearchParams({ limit, offset });
    if (assignee) p.set('assignee', assignee);
    if (change) p.set('change', change);
    if (severity) p.set('severity', severity);
    if (category) p.set('category', category);
    if (status) p.set('status', status);
    if (fileId) p.set('file', fileId);
    if (q) p.set('q', q);
    try {
      data = await api.get(`/runs/${runId}/findings?${p}`);
    } catch (e) {
      loadError = e;
    } finally {
      loading = false;
    }
  }

  function setFilter(k, v) {
    if (k === 'severity') severity = severity === v ? '' : v;
    if (k === 'category') category = v;
    if (k === 'status') status = v;
    if (k === 'file') fileId = v;
    if (k === 'assignee') assignee = v;
    if (k === 'change') change = change === v ? '' : v;
    offset = 0;
    load();
  }

  function onSearch() {
    clearTimeout(qTimer);
    qTimer = setTimeout(() => { offset = 0; load(); }, 300);
  }

  async function setStatus(f, st, note = '') {
    try {
      const upd = await api.patch(`/findings/${f.id}`, { status: st, note });
      Object.assign(f, upd);
      if (status && status !== st) data.items = data.items.filter((x) => x.id !== f.id);
      onchange?.();
    } catch (e) { toastError(e); }
  }

  async function assign(f, uid) {
    try {
      const upd = await api.put(`/findings/${f.id}/assignee`, { userId: uid });
      Object.assign(f, upd);
      if (assignee && assignee !== (uid || 'none') && !(assignee === 'me' && uid === session.user.id)) data.items = data.items.filter((x) => x.id !== f.id);
    } catch (e) { toastError(e); }
  }

  function confirmDismiss() {
    if (!dismissNote.trim()) { dismissError = 'Bitte begründen, warum der Befund kein Fehler ist. Die Begründung erscheint im Protokoll und im Prüfbericht.'; return; }
    setStatus(dismissTarget, 'dismissed', dismissNote);
    dismissTarget = null;
  }

  function openViewer(f) {
    if (!f.fileId) return;
    viewer = { file: { id: f.fileId, path: f.file, dmc: f.dmc }, line: f.line };
  }

  const runOptions = $derived(runs.map((r) => ({ id: r.id, label: `${fmt.date(r.createdAt)}, ${r.status === 'done' ? (r.summary?.total ?? 0) + ' Befunde' : r.status}` })));
  const sevOrder = ['error', 'warning', 'info'];
  const changeFacet = $derived(data?.facets?.change || {});
  const hasPrevious = $derived(!!data?.previousRun);
  const fileLabel = $derived(fileId && data?.items?.[0] ? (data.items[0].dmc || data.items[0].file) : fileId ? 'gewählte Datei' : '');
</script>

{#if !latest}
  <section class="card state">
    <h2>Dieser Upload wurde noch nicht geprüft</h2>
    <p class="muted">„{snapshot.label}“ vom {fmt.date(snapshot.createdAt)} ist eingelesen, aber noch nicht analysiert.</p>
    <div class="row wrap">
      {#if canEdit}
        <button class="primary" onclick={startAnalysis} disabled={starting}>{starting ? 'Wird gestartet …' : 'Analyse jetzt starten'}</button>
      {:else}
        <p class="hint">Ein Redakteur oder Eigentümer kann die Analyse starten.</p>
      {/if}
      {#if previous}
        <a class="btn" href={href(`/p/${project.id}/findings`, { snap: previous.id })}>Befunde von „{previous.label}“ ansehen</a>
      {/if}
    </div>
  </section>
{:else if latest.status === 'queued' || latest.status === 'running'}
  <section class="card state" aria-live="polite">
    <h2>Analyse läuft …</h2>
    <div class="progress" role="progressbar" aria-valuenow={latest.progress} aria-valuemin="0" aria-valuemax="100"><div style="width:{Math.max(3, latest.progress)}%"></div></div>
    <p class="muted">{latest.message || 'In Warteschlange'}. Die Ansicht aktualisiert sich automatisch.</p>
    {#if previous}
      <a class="btn" href={href(`/p/${project.id}/findings`, { snap: previous.id })}>Solange: Befunde von „{previous.label}“ ansehen</a>
    {/if}
  </section>
{:else if latest.status === 'failed'}
  <section class="card state">
    <h2>Die Analyse ist fehlgeschlagen</h2>
    {#if latest.error}<div class="error-box">{latest.error}</div>{/if}
    {#if canEdit}<div class="row"><button class="primary" onclick={startAnalysis} disabled={starting}>Erneut starten</button></div>{/if}
  </section>
{:else if !runId}
  <div class="card empty">Lade …</div>
{:else}
  <div class="toolbar">
    <div class="row wrap">
      {#each sevOrder as s}
        <button class="chip {severity === s ? 'active' : ''}" aria-pressed={severity === s} onclick={() => setFilter('severity', s)}>
          <span class="badge {s}">{labels.severity[s]}</span>
          <span>{fmt.num(data?.facets?.severity?.[s] || 0)}</span>
        </button>
      {/each}
      <select class="sel" aria-label="Kategorie filtern" value={category} onchange={(e) => setFilter('category', e.currentTarget.value)}>
        <option value="">Alle Kategorien</option>
        {#each Object.entries(labels.category) as [k, v]}
          <option value={k}>{v} ({data?.facets?.category?.[k] || 0})</option>
        {/each}
      </select>
      <select class="sel" aria-label="Status filtern" value={status} onchange={(e) => setFilter('status', e.currentTarget.value)}>
        <option value="">Alle Status</option>
        {#each Object.entries(labels.status) as [k, v]}
          <option value={k}>{v} ({data?.facets?.status?.[k] || 0})</option>
        {/each}
      </select>
      <input class="grow search" aria-label="Befunde durchsuchen" placeholder="Suchen in Meldung, DMC, Kontext …" bind:value={q} oninput={onSearch} />
      {#if runOptions.length > 1}
        <select class="sel" aria-label="Analyselauf wählen" bind:value={runId} onchange={() => { offset = 0; load(); }}>
          {#each runOptions as r}<option value={r.id}>{r.label}</option>{/each}
        </select>
      {/if}
      <a class="btn" href={`/api/runs/${runId}/findings.csv`}>CSV-Export</a>
    </div>
    <div class="row wrap second">
      {#if hasPrevious}
        <span class="label-quiet">Gegenüber dem vorherigen Upload:</span>
        <button class="chip {change === 'new' ? 'active' : ''}" aria-pressed={change === 'new'} onclick={() => setFilter('change', 'new')}>Neu <strong>{fmt.num(changeFacet.new || 0)}</strong></button>
        <button class="chip {change === 'persisting' ? 'active' : ''}" aria-pressed={change === 'persisting'} onclick={() => setFilter('change', 'persisting')}>Unverändert <strong>{fmt.num(changeFacet.persisting || 0)}</strong></button>
        <button class="chip {change === 'resolved' ? 'active' : ''}" aria-pressed={change === 'resolved'} onclick={() => setFilter('change', 'resolved')}>Nicht mehr vorhanden <strong>{fmt.num(changeFacet.resolved || 0)}</strong></button>
      {/if}
      <span class="spacer"></span>
      <select class="sel" aria-label="Nach Zuständigen filtern" value={assignee} onchange={(e) => setFilter('assignee', e.currentTarget.value)}>
        <option value="">Alle Zuständigen</option>
        <option value="me">Mir zugewiesen ({data?.facets?.assignee?.me || 0})</option>
        <option value="none">Nicht zugewiesen ({data?.facets?.assignee?.none || 0})</option>
        {#each members as m}<option value={m.userId}>{m.name}</option>{/each}
      </select>
    </div>
    {#if fileId}
      <div class="row filterinfo">
        <span class="badge accent">Datei: {fileLabel}</span>
        <button class="ghost small" onclick={() => setFilter('file', '')}>Filter entfernen</button>
      </div>
    {/if}
  </div>

  {#if loadError}
    <LoadError what="Die Befunde" error={loadError} onretry={load} />
  {:else if change === 'resolved'}
    {#if !resolved}
      <p class="empty" role="status">Behobene Befunde werden geladen …</p>
    {:else if resolved.items.length === 0}
      <p class="card empty">Kein Befund aus „{resolved.previousLabel}“ ist weggefallen. Alles, was dort gemeldet war, tritt hier wieder auf.</p>
    {:else}
      <p class="hint resolved-hint">{resolved.items.length} Befunde aus „{resolved.previousLabel}“ treten in diesem Upload nicht mehr auf. Meist wurden sie korrigiert; geänderter Inhalt kann einen Befund ebenfalls verschwinden lassen.</p>
      <div class="list card">
        {#each resolved.items as f (f.id)}
          <article class="finding gone">
            <div class="row top">
              <span class="badge ok">nicht mehr vorhanden</span>
              <span class="badge outline">{labels.severity[f.severity]}</span>
              <code class="code">{f.code}</code>
            </div>
            <p class="msg">{f.message}</p>
            <p class="loc mono muted">{f.dmc || f.file}{f.line ? ` : Zeile ${f.line} (im vorherigen Upload)` : ''}</p>
          </article>
        {/each}
      </div>
    {/if}
  {:else if !data}
    <div class="empty" role="status">Befunde werden geladen …</div>
  {:else if data.items.length === 0}
    <div class="card empty">
      {#if status === 'open' && !severity && !category && !q}
        Keine offenen Befunde in diesem Upload. Erledigte Befunde finden Sie über den Statusfilter.
      {:else}
        Keine Befunde für diese Filter.
      {/if}
    </div>
  {:else}
    <div class="list card" class:loading>
      {#each data.items as f (f.id)}
        <article class="finding {f.severity}" class:closed={f.status !== 'open'}>
          <div class="row top">
            <span class="badge {f.severity}">{labels.severity[f.severity]}</span>
            <span class="badge outline">{labels.category[f.category] || f.category}</span>
            <code class="code">{f.code}</code>
            {#if f.change === 'new'}<span class="badge accent" title="Im vorherigen Upload nicht gemeldet">neu</span>{/if}
            <span class="spacer"></span>
            {#if f.status !== 'open'}<span class="badge">{labels.status[f.status]}{f.statusNote ? ': ' + f.statusNote : ''}</span>{/if}
          </div>
          <p class="msg">{f.message}</p>
          {#if f.suggestion}
            <p class="sugg"><span class="muted">Vorschlag:</span> {f.suggestion}</p>
          {/if}
          <div class="row wrap loc">
            {#if f.fileId}
              <button class="link mono" onclick={() => openViewer(f)} title="Quelltext an dieser Stelle anzeigen">{f.dmc || f.file}{f.line ? ` : Zeile ${f.line}` : ''}</button>
            {:else if f.file}
              <span class="mono muted">{f.file}</span>
            {/if}
            {#if f.path}<span class="mono path ellipsis" title={f.path}>{f.path}</span>{/if}
          </div>
          <div class="row wrap acts">
            {#if f.context}
              <button class="small" aria-expanded={!!expanded[f.id]} onclick={() => (expanded[f.id] = !expanded[f.id])}>{expanded[f.id] ? 'Kontext ausblenden' : 'Kontext'}</button>
            {/if}
            {#if f.category === 'images' && f.fileId}
              <a class="btn small" href={href(`/p/${project.id}/images`, { snap: snapshot.id })}>Zum Bild-Editor</a>
            {/if}
            <button class="small" onclick={() => (activityFor = f)}>Verlauf{f.comments ? ` (${f.comments})` : ''}</button>
            {#if canEdit}
              <select class="assign" aria-label="Zuständig für diesen Befund" value={f.assigneeId} onchange={(e) => assign(f, e.currentTarget.value)}>
                <option value="">Niemand zuständig</option>
                {#each members as m}<option value={m.userId}>{m.name}</option>{/each}
              </select>
            {:else if f.assigneeName}
              <span class="who">Zuständig: {f.assigneeName}</span>
            {/if}
            <span class="spacer"></span>
            {#if canEdit}
              {#if f.status === 'open'}
                <button class="small" onclick={() => setStatus(f, 'accepted')} title="Befund ist korrekt und wird korrigiert">Bestätigen</button>
                <button class="small" onclick={() => setStatus(f, 'fixed')}>Behoben</button>
                <button class="small" onclick={() => { dismissTarget = f; dismissNote = ''; dismissError = ''; }} title="Kein Fehler: bleibt auch in künftigen Analysen verworfen">Verwerfen</button>
              {:else}
                <button class="small" onclick={() => setStatus(f, 'open')}>Wieder öffnen</button>
              {/if}
            {/if}
          </div>
          {#if expanded[f.id]}<pre class="ctx">{f.context}</pre>{/if}
        </article>
      {/each}
    </div>
    <div class="row pager">
      <span class="muted">{fmt.num(offset + 1)} bis {fmt.num(Math.min(offset + limit, data.total))} von {fmt.num(data.total)}</span>
      <span class="spacer"></span>
      <button disabled={offset === 0} onclick={() => { offset = Math.max(0, offset - limit); load(); }}>Zurück</button>
      <button disabled={offset + limit >= data.total} onclick={() => { offset += limit; load(); }}>Weiter</button>
    </div>
  {/if}
{/if}

{#if viewer}
  <FileViewer snapshotId={snapshot.id} file={viewer.file} line={viewer.line} onclose={() => (viewer = null)} />
{/if}

{#if activityFor}
  <FindingActivity finding={activityFor} {canEdit} onclose={() => (activityFor = null)} oncount={(n) => (activityFor.comments = n)} />
{/if}

{#if dismissTarget}
  <Modal title="Befund verwerfen" onclose={() => (dismissTarget = null)}>
    <div class="stack">
      <p>{dismissTarget.message}</p>
      <label><span class="lbl">Begründung</span><input bind:value={dismissNote} aria-invalid={!!dismissError} aria-describedby={dismissError ? 'dismiss-err' : 'dismiss-hint'}
        placeholder="z. B. Absichtlich so, Variante laut Kundenvorgabe CR-118" onkeydown={(e) => e.key === 'Enter' && confirmDismiss()} /></label>
      {#if dismissError}<p class="error-box" id="dismiss-err" role="alert">{dismissError}</p>{/if}
      <p class="hint" id="dismiss-hint">Verworfene Befunde bleiben in späteren Analysen dieses Projekts verworfen. Die Begründung steht im Protokoll und im Prüfbericht.</p>
      <div class="row"><span class="spacer"></span>
        <button onclick={() => (dismissTarget = null)}>Abbrechen</button>
        <button class="primary" onclick={confirmDismiss}>Verwerfen</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .toolbar { margin-bottom: var(--s-4); padding: var(--s-3) 0; border-top: 1px solid var(--ink-2); border-bottom: 1px solid var(--line-strong); }
  .toolbar .row { gap: var(--s-2); }
  .state { padding: var(--s-5) var(--s-6); display: flex; flex-direction: column; gap: var(--s-3); max-width: 760px; border-top: 3px solid var(--rule); }
  .sel { width: auto; min-width: 170px; }
  .search { min-width: 220px; }
  .filterinfo { margin-top: var(--s-3); }
  .second { margin-top: var(--s-3); }
  .chip strong { font-weight: 600; }
  .assign { width: auto; min-height: 28px; padding: 2px var(--s-2); font-size: var(--t-sm); max-width: 220px; }
  .who { font-size: var(--t-sm); color: var(--ink-2); }
  .finding.gone { border-left-color: var(--ok); }
  .resolved-hint { margin-bottom: var(--s-3); }
  /* one record sheet, entries divided by hairlines */
  .list { display: flex; flex-direction: column; }
  .list.loading { opacity: .6; }
  .finding {
    padding: var(--s-3) var(--s-5) var(--s-3) calc(var(--s-5) - 4px);
    border-left: 4px solid var(--line-strong); border-top: 1px solid var(--line);
    display: flex; flex-direction: column; gap: var(--s-2);
  }
  .finding:first-child { border-top: none; }
  .finding.error { border-left-color: var(--error); }
  .finding.warning { border-left-color: var(--warning); }
  .finding.info { border-left-color: var(--info); }
  .finding.closed { background: var(--surface-2); }
  .finding.closed .msg { color: var(--ink-2); }
  .top { gap: var(--s-2); }
  .code { color: var(--muted); font-size: var(--t-xs); }
  .msg { font-size: 15px; }
  .sugg { font-size: var(--t-sm); }
  .loc { font-size: 12.5px; gap: var(--s-2); }
  .acts { gap: var(--s-2); }
  .path { color: var(--muted); max-width: 420px; }
  .link { border: none; background: none; padding: 0; min-height: 0; font-weight: 400; color: var(--accent); cursor: pointer; }
  .link:hover { text-decoration: underline; background: none !important; }
  .ctx { margin: var(--s-1) 0 0; padding: var(--s-2) var(--s-3); background: var(--surface-2); border: 1px solid var(--line); border-radius: var(--r-1); white-space: pre-wrap; font-family: var(--mono); font-size: 12.5px; overflow-wrap: anywhere; }
  .pager { margin-top: var(--s-4); }
  @media (max-width: 640px) {
    .sel, .search, .assign { min-width: 0; flex: 1 1 100%; max-width: none; }
    .assign { min-height: 44px; }
    .finding { padding: var(--s-3) var(--s-3) var(--s-3) calc(var(--s-3) - 4px); }
  }
</style>
