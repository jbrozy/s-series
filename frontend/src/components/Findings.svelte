<script>
  import { onMount, untrack } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import { route, go, href } from '../lib/router.svelte.js';
  import { toastError } from '../lib/state.svelte.js';
  import FileViewer from './FileViewer.svelte';
  import Modal from './Modal.svelte';

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
  });

  async function load() {
    if (!runId) return;
    loading = true;
    const p = new URLSearchParams({ limit, offset });
    if (severity) p.set('severity', severity);
    if (category) p.set('category', category);
    if (status) p.set('status', status);
    if (fileId) p.set('file', fileId);
    if (q) p.set('q', q);
    try {
      data = await api.get(`/runs/${runId}/findings?${p}`);
    } catch (e) {
      toastError(e);
    } finally {
      loading = false;
    }
  }

  function setFilter(k, v) {
    if (k === 'severity') severity = severity === v ? '' : v;
    if (k === 'category') category = v;
    if (k === 'status') status = v;
    if (k === 'file') fileId = v;
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

  function openViewer(f) {
    if (!f.fileId) return;
    viewer = { file: { id: f.fileId, path: f.file, dmc: f.dmc }, line: f.line };
  }

  const runOptions = $derived(runs.map((r) => ({ id: r.id, label: `${fmt.date(r.createdAt)} – ${r.status === 'done' ? (r.summary?.total ?? 0) + ' Befunde' : r.status}` })));
  const sevOrder = ['error', 'warning', 'info'];
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
    <p class="muted">{latest.message || 'In Warteschlange'} – die Ansicht aktualisiert sich automatisch.</p>
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
  <div class="toolbar card card-pad">
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
    {#if fileId}
      <div class="row filterinfo">
        <span class="badge accent">Datei: {fileLabel}</span>
        <button class="ghost small" onclick={() => setFilter('file', '')}>Filter entfernen</button>
      </div>
    {/if}
  </div>

  {#if !data}
    <div class="empty">Lade Befunde …</div>
  {:else if data.items.length === 0}
    <div class="card empty">
      {#if status === 'open' && !severity && !category && !q}
        Keine offenen Befunde – sehr gut.
      {:else}
        Keine Befunde für diese Filter.
      {/if}
    </div>
  {:else}
    <div class="list" class:loading>
      {#each data.items as f (f.id)}
        <article class="card finding {f.severity}" class:closed={f.status !== 'open'}>
          <div class="row top">
            <span class="badge {f.severity}">{labels.severity[f.severity]}</span>
            <span class="badge outline">{labels.category[f.category] || f.category}</span>
            <code class="code">{f.code}</code>
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
            <span class="spacer"></span>
            {#if f.context}
              <button class="ghost small" onclick={() => (expanded[f.id] = !expanded[f.id])}>{expanded[f.id] ? 'Kontext ausblenden' : 'Kontext'}</button>
            {/if}
            {#if f.category === 'images' && f.fileId}
              <a class="btn small" href={href(`/p/${project.id}/images`, { snap: snapshot.id })}>Zum Bild-Editor</a>
            {/if}
            {#if canEdit}
              {#if f.status === 'open'}
                <button class="small" onclick={() => setStatus(f, 'accepted')} title="Befund ist korrekt und wird korrigiert">Bestätigen</button>
                <button class="small" onclick={() => setStatus(f, 'fixed')}>Behoben</button>
                <button class="small" onclick={() => { dismissTarget = f; dismissNote = ''; }} title="Kein Fehler – bleibt auch in künftigen Analysen verworfen">Verwerfen</button>
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
      <span class="muted">{fmt.num(offset + 1)}–{fmt.num(Math.min(offset + limit, data.total))} von {fmt.num(data.total)}</span>
      <span class="spacer"></span>
      <button disabled={offset === 0} onclick={() => { offset = Math.max(0, offset - limit); load(); }}>Zurück</button>
      <button disabled={offset + limit >= data.total} onclick={() => { offset += limit; load(); }}>Weiter</button>
    </div>
  {/if}
{/if}

{#if viewer}
  <FileViewer snapshotId={snapshot.id} file={viewer.file} line={viewer.line} onclose={() => (viewer = null)} />
{/if}

{#if dismissTarget}
  <Modal title="Befund verwerfen" onclose={() => (dismissTarget = null)}>
    <div class="stack">
      <p>{dismissTarget.message}</p>
      <label><span class="lbl">Begründung (optional)</span><input bind:value={dismissNote} placeholder="z. B. Absichtlich so – Variante laut Kunde" /></label>
      <p class="hint">Verworfene Befunde werden in späteren Analysen dieses Projekts automatisch wieder als verworfen markiert.</p>
      <div class="row"><span class="spacer"></span>
        <button onclick={() => (dismissTarget = null)}>Abbrechen</button>
        <button class="primary" onclick={() => { setStatus(dismissTarget, 'dismissed', dismissNote); dismissTarget = null; }}>Verwerfen</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .toolbar { margin-bottom: 14px; padding: 12px 14px; }
  .state { padding: 28px 30px; display: flex; flex-direction: column; gap: 12px; max-width: 760px; }
  .sel { width: auto; min-width: 170px; }
  .search { min-width: 220px; }
  .filterinfo { margin-top: 10px; }
  .list { display: flex; flex-direction: column; gap: 8px; }
  .list.loading { opacity: .6; }
  .finding { padding: 12px 16px; border-left: 4px solid var(--line-strong); display: flex; flex-direction: column; gap: 6px; }
  .finding.error { border-left-color: var(--error); }
  .finding.warning { border-left-color: #e0a030; }
  .finding.info { border-left-color: #7aa7d8; }
  .finding.closed { opacity: .7; }
  .top { gap: 8px; }
  .code { color: var(--muted); font-size: 12px; }
  .msg { font-size: 14.5px; }
  .sugg { font-size: 13.5px; }
  .loc { font-size: 12.5px; gap: 8px; }
  .path { color: var(--muted); max-width: 420px; }
  .link { border: none; background: none; padding: 0; color: var(--accent); cursor: pointer; }
  .link:hover { text-decoration: underline; background: none !important; }
  .ctx { margin: 4px 0 0; padding: 10px 12px; background: var(--surface-2); border: 1px solid var(--line); border-radius: 6px; white-space: pre-wrap; font-family: var(--mono); font-size: 12.5px; }
  .pager { margin-top: 14px; }
</style>
