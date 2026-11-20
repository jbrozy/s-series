<script>
  import SortTh from './SortTh.svelte';
  import { createSorter } from '../lib/sort.svelte.js';
  import LoadError from './LoadError.svelte';
  import { api, upload, fmt, labels } from '../lib/api.js';
  import { go, href, route } from '../lib/router.svelte.js';
  import { toast, toastError } from '../lib/state.svelte.js';
  import Modal from './Modal.svelte';
  import FileViewer from './FileViewer.svelte';

  let { project, snapshots, snapshot, canEdit, onchange } = $props();
  const fileSorter = createSorter('dmc', { kind: (f) => f.kind, dmc: (f) => f.dmc || f.path, title: (f) => f.title, lang: (f) => f.lang, findings: (f) => f.findings || 0 }, 'asc', 'scheck.sort.files');

  // ---- upload
  let files = $state([]);
  let label = $state('');
  let autoAnalyze = $state(true);
  let progress = $state(0);
  let uploading = $state(false);
  let dragOver = $state(false);
  let fileInput = $state();
  let folderInput = $state();
  const totalSize = $derived(files.reduce((a, f) => a + f.file.size, 0));

  const defaultOptions = { lists: true, titles: true, references: true, consistency: true, rules: true, llm: false, images: true, llmMaxDocs: 25 };
  let options = $state({ ...defaultOptions });
  let showRunDialog = $state(false);

  const checkGroups = [
    ['lists', 'Listen & Duplikate', 'Doppelte Listeneinträge, Schritte, Vorbedingungen, Tabellenzeilen'],
    ['titles', 'Überschriften & Schreibung', 'Tippfehler, doppelte/uneinheitliche Titel, techName/infoName'],
    ['references', 'Verweise', 'internalRef, dmRef, ICN-Grafiken, ungenutzte Grafiken'],
    ['consistency', 'Konsistenz', 'Abkürzungen, Teilenummern-Benennung, Warnhinweise, leere Absätze'],
    ['rules', 'Business Rules', 'Projektspezifische Regeln inkl. importierter BREX-Regeln'],
    ['images', 'Bildprüfung', 'Menschliche Körperteile in Abbildungen erkennen'],
    ['llm', 'KI-Review', 'Inhaltliche Prüfung durch das konfigurierte Sprachmodell (langsamer)']
  ];

  async function addEntries(items) {
    const out = [];
    const walk = async (entry, path) => {
      if (entry.isFile) {
        const file = await new Promise((res, rej) => entry.file(res, rej));
        out.push({ file, path: path + file.name });
      } else if (entry.isDirectory) {
        const reader = entry.createReader();
        let batch;
        do {
          batch = await new Promise((res, rej) => reader.readEntries(res, rej));
          for (const e of batch) await walk(e, path + entry.name + '/');
        } while (batch.length);
      }
    };
    for (const it of items) {
      const entry = it.webkitGetAsEntry?.();
      if (entry) await walk(entry, '');
      else if (it.getAsFile?.()) out.push({ file: it.getAsFile(), path: it.getAsFile().name });
    }
    files = [...files, ...out];
  }

  function onDrop(e) {
    e.preventDefault();
    dragOver = false;
    addEntries([...e.dataTransfer.items]);
  }

  function onPick(e) {
    const picked = [...e.currentTarget.files].map((f) => ({ file: f, path: f.webkitRelativePath || f.name }));
    files = [...files, ...picked];
    e.currentTarget.value = '';
  }

  async function doUpload() {
    if (!files.length) return;
    uploading = true;
    progress = 0;
    const fd = new FormData();
    fd.append('label', label);
    fd.append('autoAnalyze', String(autoAnalyze));
    fd.append('options', JSON.stringify(options));
    for (const f of files) fd.append('files', f.file, f.path);
    try {
      const snap = await upload(`/projects/${project.id}/snapshots`, fd, (p) => (progress = p));
      toast(`Upload „${snap.label}“ mit ${snap.fileCount} Dateien gespeichert. Indexierung läuft.`);
      files = [];
      label = '';
      go(`/p/${project.id}/overview`, { snap: snap.id });
      onchange();
    } catch (e) {
      toastError(e);
    } finally {
      uploading = false;
    }
  }

  // ---- runs
  async function startRun() {
    try {
      await api.post(`/snapshots/${snapshot.id}/runs`, options);
      showRunDialog = false;
      toast('Analyse gestartet');
      onchange();
    } catch (e) { toastError(e); }
  }

  async function deleteSnapshot(s) {
    if (!confirm(`Upload „${s.label}“ mit allen Prüfergebnissen löschen?`)) return;
    try {
      await api.del(`/snapshots/${s.id}`);
      go(`/p/${project.id}/overview`);
      onchange();
    } catch (e) { toastError(e); }
  }

  // ---- data modules
  let dmFiles = $state(null);
  let filesError = $state(null);
  let release = $state(null);
  $effect(() => {
    const id = snapshot?.id;
    const run = snapshot?.latestRun?.status;
    release = null;
    if (!id || run !== 'done') return;
    api.get(`/snapshots/${id}/release`).then((r) => (release = r)).catch(() => (release = null));
  });
  const relText = { approved: 'Freigegeben', rejected: 'Abgelehnt' };
  const gateText = { passed: 'Freigabekriterien erfüllt', failed: 'Freigabekriterien nicht erfüllt', pending: 'Freigabekriterien noch nicht bewertbar' };
  let filesReload = $state(0);
  let dmFilter = $state('');
  let viewFile = $state(null);
  $effect(() => {
    const id = snapshot?.id;
    const st = snapshot?.status;
    const run = snapshot?.latestRun?.status;
    if (!id || st !== 'ready') { dmFiles = null; return; }
    void filesReload;
    filesError = null;
    api.get(`/snapshots/${id}/files`).then((f) => {
      dmFiles = f;
      // opened from the search: show the requested file right away
      const want = route.query.view;
      if (want) { const hit = f.find((x) => x.id === want); if (hit) viewFile = hit; }
    }).catch((e) => { filesError = e; dmFiles = null; });
    void run;
  });
  const shownFiles = $derived(
    (dmFiles || []).filter((f) => {
      if (!dmFilter) return true;
      const q = dmFilter.toLowerCase();
      return (f.dmc + ' ' + f.title + ' ' + f.path).toLowerCase().includes(q);
    })
  );

  const run = $derived(snapshot?.latestRun);
  const sum = $derived(run?.summary || {});
  const statusText = { uploading: 'Wird hochgeladen', uploaded: 'In Warteschlange', indexing: 'Wird eingelesen', ready: 'Eingelesen', failed: 'Fehler beim Einlesen' };
</script>

<div class="layout">
  <div class="main stack">
    {#if snapshot}
      <section class="card summary" aria-labelledby="h-snap">
        <div class="panel-head">
          <div class="grow">
            <h2 id="h-snap">Upload „{snapshot.label}“</h2>
            <p class="muted meta">Hochgeladen {fmt.date(snapshot.createdAt)} von {snapshot.uploadedBy} · {fmt.num(snapshot.fileCount)} Dateien · {fmt.bytes(snapshot.totalBytes)}</p>
          </div>
          {#if run?.status === 'done'}
            <span class="badge ok">Geprüft</span>
          {:else if snapshot.status === 'ready' && !run}
            <span class="badge warning">Noch nicht geprüft</span>
          {:else}
            <span class="badge {snapshot.status === 'failed' ? 'error' : 'info'}">{run && snapshot.status === 'ready' ? 'Analyse läuft' : statusText[snapshot.status] || snapshot.status}</span>
          {/if}
        </div>
        <div class="card-pad stack">
        {#if snapshot.error}<div class="error-box">{snapshot.error}</div>{/if}

        <div class="plate">
          <dl class="group">
            <div class="cell"><dt>Datenmodule</dt><dd>{fmt.num(snapshot.kinds?.dm || 0)}</dd></div>
            <div class="cell"><dt>Grafiken</dt><dd>{fmt.num(snapshot.kinds?.icn || 0)}</dd></div>
            <div class="cell"><dt>PM / BREX / DML</dt><dd>{fmt.num((snapshot.kinds?.pm || 0) + (snapshot.kinds?.brex || 0) + (snapshot.kinds?.dml || 0))}</dd></div>
          </dl>
          {#if run?.status === 'done'}
            <div class="group sev" aria-label="Befunde nach Schweregrad">
              <a class="cell err" href={href(`/p/${project.id}/findings`, { snap: snapshot.id, severity: 'error' })}><span class="k">Fehler</span><span class="v">{fmt.num(sum.bySeverity?.error || 0)}</span></a>
              <a class="cell warn" href={href(`/p/${project.id}/findings`, { snap: snapshot.id, severity: 'warning' })}><span class="k">Warnungen</span><span class="v">{fmt.num(sum.bySeverity?.warning || 0)}</span></a>
              <a class="cell inf" href={href(`/p/${project.id}/findings`, { snap: snapshot.id, severity: 'info' })}><span class="k">Hinweise</span><span class="v">{fmt.num(sum.bySeverity?.info || 0)}</span></a>
            </div>
          {/if}
        </div>

        {#if run}
          <div class="runbox">
            <div class="row wrap">
              <strong>Letzte Analyse</strong>
              <span class="muted">{fmt.date(run.createdAt)}</span>
              <span class="spacer"></span>
              {#if run.status === 'done'}<span class="badge ok">abgeschlossen</span>
              {:else if run.status === 'failed'}<span class="badge error">fehlgeschlagen</span>
              {:else}<span class="badge info">{run.status === 'queued' ? 'wartet' : 'läuft'}</span>{/if}
            </div>
            {#if run.status === 'queued' || run.status === 'running'}
              <div class="progress"><div style="width:{Math.max(3, run.progress)}%"></div></div>
              <p class="hint">{run.message}</p>
            {/if}
            {#if run.error}<div class="error-box">{run.error}</div>{/if}
            {#if sum.notes?.length}
              <ul class="notes">{#each sum.notes as n}<li>{n}</li>{/each}</ul>
            {/if}
          </div>
        {/if}

        {#if release}
          <div class="releaseline r-{release.current && !release.stale ? release.current.decision : 'none'}">
            {#if release.current && !release.stale}
              <strong>{relText[release.current.decision]}</strong>
              <span>von {release.current.userName}, {fmt.date(release.current.decidedAt)}</span>
            {:else}
              <strong>{release.current ? 'Neu zu entscheiden' : 'Noch nicht freigegeben'}</strong>
              <span>{gateText[release.gate.state]}</span>
            {/if}
            <span class="spacer"></span>
            <a href={href(`/p/${project.id}/release`, { snap: snapshot.id })}>Zur Freigabe</a>
            <a href={href(`/p/${project.id}/report`, { snap: snapshot.id })}>Prüfbericht</a>
          </div>
        {/if}

        <div class="row wrap">
          {#if run?.status === 'done'}
            <a class="btn primary" href={href(`/p/${project.id}/findings`, { snap: snapshot.id })}>Befunde ansehen</a>
          {/if}
          {#if canEdit}
            <button class:primary={!run && snapshot.status === 'ready'} onclick={() => (showRunDialog = true)} disabled={run && ['queued', 'running'].includes(run.status)}>{run ? 'Neue Analyse …' : 'Analyse starten …'}</button>
          {/if}
          <a class="btn" href={`/api/snapshots/${snapshot.id}/export`} title="Alle Dateien, anonymisierte Bilder ersetzen die Originale, inkl. Befundliste (CSV)">Export (ZIP)</a>
          <span class="spacer"></span>
          {#if canEdit}<button class="danger" onclick={() => deleteSnapshot(snapshot)}>Upload löschen</button>{/if}
        </div>
        </div>
      </section>

      {#if filesError}
        <LoadError what="Die Dateien des Uploads" error={filesError} onretry={() => filesReload++} />
      {:else if dmFiles}
        <section class="card">
          <div class="panel-head">
            <h2 class="grow">Dateien im Upload</h2>
            <input class="filter" aria-label="Dateien filtern" placeholder="DMC, Titel, Datei …" bind:value={dmFilter} />
          </div>
          <div class="tablewrap">
            <table>
              <thead><tr><SortTh sorter={fileSorter} key="kind">Typ</SortTh><SortTh sorter={fileSorter} key="dmc">DMC / Datei</SortTh><SortTh sorter={fileSorter} key="title">Titel</SortTh><SortTh sorter={fileSorter} key="lang">Sprache</SortTh><SortTh sorter={fileSorter} key="findings" align="right">Befunde</SortTh></tr></thead>
              <tbody>
                {#each fileSorter.apply(shownFiles).slice(0, 400) as f (f.id)}
                  <tr>
                    <td><span class="badge outline">{labels.kind[f.kind] || f.kind}</span></td>
                    <td>
                      {#if f.kind !== 'icn' && f.kind !== 'other'}
                        <button class="link mono" onclick={() => (viewFile = f)}>{f.dmc || f.path}</button>
                      {:else}
                        <a class="mono" href={`/api/snapshots/${snapshot.id}/files/${f.id}/content`} target="_blank" rel="noopener">{f.path}</a>
                      {/if}
                      {#if f.parseError}<div class="hint err">{f.parseError}</div>{/if}
                    </td>
                    <td>{f.title}</td>
                    <td class="muted">{f.lang}</td>
                    <td class="right">
                      {#if f.findings}
                        <a href={href(`/p/${project.id}/findings`, { snap: snapshot.id, file: f.id })}><span class="badge warning">{f.findings}</span></a>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
            {#if shownFiles.length > 400}<p class="hint card-pad">{shownFiles.length - 400} weitere Dateien. Grenzen Sie die Liste mit dem Filter ein.</p>{/if}
          </div>
        </section>
      {/if}
    {:else}
      <section class="card empty-state">
        <h2>Noch kein Upload</h2>
        <p class="muted">Laden Sie rechts den Export Ihres Projekts hoch. Die Dateien werden indexiert und automatisch geprüft.</p>
      </section>
    {/if}
  </div>

  <aside class="stack">
    {#if canEdit}
      <section class="card" aria-labelledby="h-up">
        <div class="panel-head"><h2 id="h-up">Neuen Export hochladen</h2></div>
        <div class="card-pad stack">
        <div
          class="drop" class:over={dragOver} role="button" tabindex="0"
          ondragover={(e) => { e.preventDefault(); dragOver = true; }}
          ondragleave={() => (dragOver = false)}
          ondrop={onDrop}
          onclick={() => fileInput.click()}
          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); fileInput.click(); } }}
        >
          <strong>Dateien oder Ordner hierher ziehen</strong>
          <span class="hint">ZIP-Export, Datenmodule (XML/SGML), HTML-Export (Seiten + Bilder), Publikationsmodule, BREX und Grafiken (PNG, JPG, TIFF, SVG, CGM …)</span>
        </div>
        <input type="file" multiple bind:this={fileInput} onchange={onPick} hidden />
        <input type="file" webkitdirectory multiple bind:this={folderInput} onchange={onPick} hidden />
        <div class="row">
          <button class="small" onclick={() => fileInput.click()}>Dateien wählen</button>
          <button class="small" onclick={() => folderInput.click()}>Ordner wählen</button>
        </div>
        {#if files.length}
          <div class="picked">
            <div class="row"><strong class="grow">{files.length} Dateien · {fmt.bytes(totalSize)}</strong><button class="ghost small" onclick={() => (files = [])}>leeren</button></div>
            <ul>{#each files.slice(0, 6) as f}<li class="mono ellipsis">{f.path}</li>{/each}</ul>
            {#if files.length > 6}<span class="hint">… und {files.length - 6} weitere</span>{/if}
          </div>
          <label><span class="lbl">Bezeichnung</span><input bind:value={label} placeholder="z. B. Export KW 47 / Issue 003" /></label>
          <label class="row check"><input type="checkbox" bind:checked={autoAnalyze} /> Direkt nach dem Upload prüfen</label>
          {#if uploading}
            <div class="progress"><div style="width:{progress * 100}%"></div></div>
          {/if}
          <button class="primary" onclick={doUpload} disabled={uploading}>{uploading ? `Lade hoch … ${Math.round(progress * 100)} %` : 'Hochladen und prüfen'}</button>
        {/if}
        </div>
      </section>
    {/if}

    {#if snapshots.length > 1}
      <section class="card" aria-labelledby="h-hist">
        <div class="panel-head"><h2 id="h-hist">Alle Uploads</h2></div>
        <ul class="history">
          {#each snapshots as s}
            <li>
              {#if s.id === snapshot?.id}
                <span class="cur" aria-current="true">{s.label}</span><span class="badge accent">angezeigt</span>
              {:else}
                <a href={href(`/p/${project.id}/overview`, { snap: s.id })}>{s.label}</a>
              {/if}
              <span class="hint">{fmt.date(s.createdAt)}</span>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  </aside>
</div>

{#if showRunDialog}
  <Modal title="Analyse starten" onclose={() => (showRunDialog = false)}>
    <div class="stack">
      {#each checkGroups as [key, title, desc]}
        <label class="opt">
          <input type="checkbox" bind:checked={options[key]} />
          <span><strong>{title}</strong><br /><span class="hint">{desc}</span></span>
        </label>
      {/each}
      {#if options.llm}
        <label><span class="lbl">Max. Datenmodule für KI-Review</span><input type="number" min="1" max="500" bind:value={options.llmMaxDocs} /></label>
      {/if}
      <div class="row"><span class="spacer"></span>
        <button onclick={() => (showRunDialog = false)}>Abbrechen</button>
        <button class="primary" onclick={startRun}>Starten</button>
      </div>
    </div>
  </Modal>
{/if}

{#if viewFile}
  <FileViewer snapshotId={snapshot.id} file={viewFile} onclose={() => (viewFile = null)} />
{/if}

<style>
  .layout { display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: var(--s-4); align-items: start; }
  .main, aside { gap: var(--s-4); }
  .summary h2 { font-size: var(--t-lg); }
  .meta { font-size: var(--t-sm); }

  /* key figures: label above value, status by text label */
  .plate { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 1px; background: var(--line); border: 1px solid var(--line); border-radius: var(--r-1); overflow: hidden; }
  .group { display: contents; margin: 0; }
  .cell { display: flex; flex-direction: column; gap: 2px; padding: var(--s-2) var(--s-3); background: var(--surface); min-width: 0; color: var(--ink); text-decoration: none; }
  .cell dt, .cell .k { font-size: var(--t-sm); color: var(--muted); }
  .cell dd, .cell .v { margin: 0; font-size: 22px; font-weight: 600; line-height: 1.2; }
  a.cell:hover { background: var(--surface-hover); }
  a.cell:hover .k { text-decoration: underline; color: var(--accent); }
  .sev .err .v { color: var(--error); } .sev .warn .v { color: var(--warning); } .sev .inf .v { color: var(--info); }

  .runbox { border: 1px solid var(--line); border-radius: var(--r-1); background: var(--surface-2); padding: var(--s-3); display: flex; flex-direction: column; gap: var(--s-2); }
  .runbox .muted { font-size: var(--t-sm); }
  .releaseline { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--s-2) var(--s-3); padding: var(--s-2) var(--s-3);
    border: 1px solid var(--line-strong); border-radius: var(--r-1); background: var(--surface); }
  .releaseline strong { font-size: var(--t-md); }
  .releaseline span:not(.spacer) { color: var(--ink-2); font-size: var(--t-sm); }
  .releaseline a { font-size: var(--t-sm); text-decoration: underline; text-underline-offset: 2px; }
  .r-approved { border-color: var(--ok); background: var(--ok-soft); }
  .r-rejected { border-color: var(--error); background: var(--error-soft); }
  .notes { margin: 0; padding-left: var(--s-4); color: var(--ink-2); font-size: var(--t-sm); overflow-wrap: anywhere; }

  .drop {
    border: 2px dashed var(--line-strong); border-radius: var(--r-1); padding: var(--s-5) var(--s-4); text-align: center;
    display: flex; flex-direction: column; gap: var(--s-2); cursor: pointer; background: var(--surface);
  }
  .drop.over, .drop:hover { border-color: var(--accent); border-style: solid; background: var(--accent-soft); }
  .picked ul { margin: var(--s-2) 0 0; padding-left: var(--s-4); font-size: var(--t-xs); }
  .check { gap: var(--s-2); }
  .opt { display: flex; gap: var(--s-3); align-items: flex-start; }
  .opt input { margin-top: 3px; }
  .filter { width: 280px; max-width: 100%; }
  .tablewrap { max-height: 560px; overflow: auto; border-top: 1px solid var(--line); }
  .link { border: none; background: none; padding: 0; min-height: 0; color: var(--accent); cursor: pointer; text-align: left; white-space: normal; font-weight: 400; }
  .link:hover { text-decoration: underline; background: none !important; }
  .err { color: var(--error); }
  .history { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
  .history li { display: flex; flex-wrap: wrap; align-items: center; gap: 2px var(--s-2); padding: var(--s-2) var(--s-5); border-bottom: 1px solid var(--line); }
  .history li:last-child { border-bottom: none; }
  .history .cur { font-weight: 600; }
  .history .hint { flex-basis: 100%; font-size: var(--t-xs); }
  .empty-state { padding: var(--s-7) var(--s-5); text-align: center; display: flex; flex-direction: column; gap: var(--s-3); border-style: dashed; border-color: var(--line-strong); }
  @media (max-width: 1100px) { .layout { grid-template-columns: minmax(0, 1fr); } }
  .runbox .muted { white-space: nowrap; }
  @media (max-width: 640px) {
    .tablewrap :global(th:nth-child(1)), .tablewrap td:nth-child(1), .tablewrap :global(th:nth-child(4)), .tablewrap td:nth-child(4) { display: none; }
    .group { flex-basis: 100%; }
    .group + .group { border-left: none; border-top: 1px solid var(--ink-2); }
    .cell dd, .cell .v { font-size: 22px; }
  }
</style>
