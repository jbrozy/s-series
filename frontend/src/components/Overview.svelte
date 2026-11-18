<script>
  import { api, upload, fmt, labels } from '../lib/api.js';
  import { go, href } from '../lib/router.svelte.js';
  import { toast, toastError } from '../lib/state.svelte.js';
  import Modal from './Modal.svelte';
  import FileViewer from './FileViewer.svelte';

  let { project, snapshots, snapshot, canEdit, onchange } = $props();

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
      toast(`Upload „${snap.label}“ mit ${snap.fileCount} Dateien gespeichert – Indexierung läuft`);
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
  let dmFilter = $state('');
  let viewFile = $state(null);
  $effect(() => {
    const id = snapshot?.id;
    const st = snapshot?.status;
    const run = snapshot?.latestRun?.status;
    if (!id || st !== 'ready') { dmFiles = null; return; }
    api.get(`/snapshots/${id}/files`).then((f) => (dmFiles = f)).catch(() => (dmFiles = []));
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
      <section class="card card-pad stack">
        <div class="row">
          <div class="grow">
            <h2>{snapshot.label}</h2>
            <p class="muted">Hochgeladen {fmt.date(snapshot.createdAt)} von {snapshot.uploadedBy} · {fmt.num(snapshot.fileCount)} Dateien · {fmt.bytes(snapshot.totalBytes)}</p>
          </div>
          {#if run?.status === 'done'}
            <span class="badge ok">Geprüft</span>
          {:else if snapshot.status === 'ready' && !run}
            <span class="badge warning">Noch nicht geprüft</span>
          {:else}
            <span class="badge {snapshot.status === 'failed' ? 'error' : 'info'}">{run && snapshot.status === 'ready' ? 'Analyse läuft' : statusText[snapshot.status] || snapshot.status}</span>
          {/if}
        </div>
        {#if snapshot.error}<div class="error-box">{snapshot.error}</div>{/if}

        <div class="stats">
          <div class="stat"><span class="n">{fmt.num(snapshot.kinds?.dm || 0)}</span><span>Datenmodule</span></div>
          <div class="stat"><span class="n">{fmt.num(snapshot.kinds?.icn || 0)}</span><span>Grafiken</span></div>
          <div class="stat"><span class="n">{fmt.num((snapshot.kinds?.pm || 0) + (snapshot.kinds?.brex || 0) + (snapshot.kinds?.dml || 0))}</span><span>PM / BREX / DML</span></div>
          {#if run?.status === 'done'}
            <a class="stat err" href={href(`/p/${project.id}/findings`, { snap: snapshot.id, severity: 'error' })}><span class="n">{fmt.num(sum.bySeverity?.error || 0)}</span><span>Fehler</span></a>
            <a class="stat warn" href={href(`/p/${project.id}/findings`, { snap: snapshot.id, severity: 'warning' })}><span class="n">{fmt.num(sum.bySeverity?.warning || 0)}</span><span>Warnungen</span></a>
            <a class="stat inf" href={href(`/p/${project.id}/findings`, { snap: snapshot.id, severity: 'info' })}><span class="n">{fmt.num(sum.bySeverity?.info || 0)}</span><span>Hinweise</span></a>
          {/if}
        </div>

        {#if run}
          <div class="runbox">
            <div class="row">
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
      </section>

      {#if dmFiles}
        <section class="card">
          <div class="row card-pad sect-head">
            <h3 class="grow">Dateien im Upload</h3>
            <input class="filter" aria-label="Dateien filtern" placeholder="DMC, Titel, Datei …" bind:value={dmFilter} />
          </div>
          <div class="tablewrap">
            <table>
              <thead><tr><th>Typ</th><th>DMC / Datei</th><th>Titel</th><th>Sprache</th><th class="right">Befunde</th></tr></thead>
              <tbody>
                {#each shownFiles.slice(0, 400) as f (f.id)}
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
            {#if shownFiles.length > 400}<p class="hint card-pad">{shownFiles.length - 400} weitere – Filter verwenden.</p>{/if}
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
      <section class="card card-pad stack">
        <h3>Projektexport hochladen</h3>
        <div
          class="drop" class:over={dragOver} role="button" tabindex="0"
          ondragover={(e) => { e.preventDefault(); dragOver = true; }}
          ondragleave={() => (dragOver = false)}
          ondrop={onDrop}
          onclick={() => fileInput.click()}
          onkeydown={(e) => e.key === 'Enter' && fileInput.click()}
        >
          <strong>Dateien oder Ordner hierher ziehen</strong>
          <span class="hint">ZIP-Export, Datenmodule (XML/SGML), Publikationsmodule, BREX und Grafiken (PNG, JPG, TIFF, SVG, CGM …)</span>
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
          <button class="primary" onclick={doUpload} disabled={uploading}>{uploading ? `Lade hoch … ${Math.round(progress * 100)} %` : 'Hochladen & prüfen'}</button>
        {/if}
      </section>
    {/if}

    {#if snapshots.length > 1}
      <section class="card card-pad stack">
        <h3>Frühere Uploads</h3>
        <ul class="history">
          {#each snapshots as s}
            <li class:current={s.id === snapshot?.id}>
              <a href={href(`/p/${project.id}/overview`, { snap: s.id })}>{s.label}</a>
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
  .layout { display: grid; grid-template-columns: 1fr 360px; gap: 18px; align-items: start; }
  .stats { display: grid; grid-template-columns: repeat(6, 1fr); gap: 10px; }
  .stat { border: 1px solid var(--line); border-radius: 8px; padding: 10px 12px; display: flex; flex-direction: column; color: var(--muted); font-size: 12.5px; }
  .stat .n { font-size: 22px; font-weight: 700; color: var(--ink); }
  a.stat:hover { text-decoration: none; border-color: var(--line-strong); }
  .stat.err .n { color: var(--error); }
  .stat.warn .n { color: var(--warning); }
  .stat.inf .n { color: var(--info); }
  .runbox { background: var(--surface-2); border: 1px solid var(--line); border-radius: 8px; padding: 12px 14px; display: flex; flex-direction: column; gap: 8px; }
  .notes { margin: 0; padding-left: 18px; color: var(--ink-2); font-size: 13px; }
  .drop {
    border: 2px dashed var(--line-strong); border-radius: 10px; padding: 26px 16px; text-align: center;
    display: flex; flex-direction: column; gap: 6px; cursor: pointer; background: var(--surface-2);
  }
  .drop.over, .drop:hover { border-color: var(--accent); background: var(--accent-soft); }
  .picked ul { margin: 6px 0 0; padding-left: 16px; font-size: 12px; }
  .check { gap: 8px; }
  .opt { display: flex; gap: 10px; align-items: flex-start; }
  .opt input { margin-top: 3px; }
  .sect-head { padding-bottom: 10px; }
  .filter { max-width: 280px; }
  .tablewrap { max-height: 560px; overflow: auto; border-top: 1px solid var(--line); }
  .link { border: none; background: none; padding: 0; color: var(--accent); cursor: pointer; text-align: left; white-space: normal; }
  .link:hover { text-decoration: underline; background: none !important; }
  .err { color: var(--error); }
  .history { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  .history li { display: flex; flex-direction: column; padding: 6px 8px; border-radius: 6px; }
  .history li.current { background: var(--accent-soft); }
  .empty-state { padding: 48px; text-align: center; display: flex; flex-direction: column; gap: 10px; }
  @media (max-width: 1100px) { .layout { grid-template-columns: 1fr; } .stats { grid-template-columns: repeat(3, 1fr); } }
</style>
