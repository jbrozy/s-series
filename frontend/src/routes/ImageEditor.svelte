<script>
  import { onMount, onDestroy } from 'svelte';
  import { api, postBlob, labels } from '../lib/api.js';
  import { route, go, href } from '../lib/router.svelte.js';
  import { toast, toastError } from '../lib/state.svelte.js';

  let { pid, iid } = $props();
  let project = $state(null);
  let image = $state(null);
  let boxes = $state([]);
  let selected = $state(null);
  let siblings = $state([]);
  let preview = $state('');
  let showPreview = $state(false);
  let busy = $state(false);
  let dirty = $state(false);
  let stage = $state();
  let drag = null;

  const snap = $derived(route.query.snap);
  const canEdit = $derived(project && project.myRole !== 'viewer');
  const sel = $derived(boxes.find((b) => b.id === selected));
  const idx = $derived(siblings.findIndex((s) => s.id === iid));

  onMount(async () => {
    try {
      [project, image] = await Promise.all([api.get(`/projects/${pid}`), api.get(`/images/${iid}`)]);
      boxes = image.detections.map((d) => ({ ...d }));
      siblings = (await api.get(`/snapshots/${image.snapshotId}/images`)).filter((i) => i.status !== 'unsupported');
      if (image.anonymized) await renderPreview(true);
    } catch (e) { toastError(e); }
  });
  onDestroy(() => preview && URL.revokeObjectURL(preview));

  function rel(e) {
    const r = stage.getBoundingClientRect();
    return [
      Math.round(((e.clientX - r.left) / r.width) * image.width),
      Math.round(((e.clientY - r.top) / r.height) * image.height)
    ];
  }

  function clamp(b) {
    const [x1, y1, x2, y2] = b;
    return [Math.max(0, Math.min(x1, x2)), Math.max(0, Math.min(y1, y2)), Math.min(image.width, Math.max(x1, x2)), Math.min(image.height, Math.max(y1, y2))];
  }

  function down(e, box, handle) {
    if (!canEdit || showPreview || !image.width) return;
    e.preventDefault();
    e.stopPropagation();
    const p = rel(e);
    if (box) {
      selected = box.id;
      drag = { kind: handle ? 'resize' : 'move', box, start: p, orig: [...box.box] };
    } else {
      const nb = { id: 'n' + Math.random().toString(36).slice(2, 9), label: 'hand', box: [p[0], p[1], p[0], p[1]], score: 1, source: 'manual', mode: 'silhouette' };
      boxes.push(nb);
      selected = nb.id;
      drag = { kind: 'new', box: boxes[boxes.length - 1], start: p };
    }
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up, { once: true });
  }

  function move(e) {
    if (!drag) return;
    const p = rel(e);
    const b = drag.box;
    if (drag.kind === 'new') b.box = clamp([drag.start[0], drag.start[1], p[0], p[1]]);
    else if (drag.kind === 'resize') b.box = clamp([drag.orig[0], drag.orig[1], p[0], p[1]]);
    else {
      const dx = p[0] - drag.start[0], dy = p[1] - drag.start[1];
      const w = drag.orig[2] - drag.orig[0], h = drag.orig[3] - drag.orig[1];
      const x1 = Math.max(0, Math.min(image.width - w, drag.orig[0] + dx));
      const y1 = Math.max(0, Math.min(image.height - h, drag.orig[1] + dy));
      b.box = [x1, y1, x1 + w, y1 + h];
    }
    b.source = 'manual';
    dirty = true;
  }

  function up() {
    window.removeEventListener('pointermove', move);
    if (drag?.kind === 'new') {
      const [x1, y1, x2, y2] = drag.box.box;
      if (x2 - x1 < 6 || y2 - y1 < 6) boxes = boxes.filter((b) => b.id !== drag.box.id);
    }
    drag = null;
  }

  function removeBox(id) {
    boxes = boxes.filter((b) => b.id !== id);
    selected = null;
    dirty = true;
  }

  function payload() {
    return { detections: boxes.map(({ id, label, box, score, source, mode }) => ({ id, label, box, score, source, mode })) };
  }

  async function renderPreview(saved = false) {
    try {
      const url = saved ? `/api/images/${iid}/anonymized?t=${Date.now()}` : await postBlob(`/images/${iid}/preview`, payload());
      if (preview && preview.startsWith('blob:')) URL.revokeObjectURL(preview);
      preview = url;
      showPreview = true;
    } catch (e) { toastError(e); }
  }

  async function save() {
    busy = true;
    try {
      image = await api.put(`/images/${iid}/detections`, payload());
      dirty = false;
      toast('Markierungen gespeichert');
    } catch (e) { toastError(e); } finally { busy = false; }
  }

  async function anonymize() {
    if (!boxes.length) return toastError(new Error('Bitte zuerst Bereiche markieren'));
    busy = true;
    try {
      image = await api.post(`/images/${iid}/anonymize`, payload());
      dirty = false;
      await renderPreview(true);
      toast('Anonymisierte Grafik gespeichert. Der Export verwendet sie anstelle des Originals.');
    } catch (e) { toastError(e); } finally { busy = false; }
  }

  async function markClean() {
    if (!confirm('Bestätigen, dass dieses Bild keine menschlichen Körperteile zeigt?')) return;
    busy = true;
    try {
      image = await api.post(`/images/${iid}/clean`);
      boxes = [];
      showPreview = false;
      toast('Als unbedenklich markiert');
      next();
    } catch (e) { toastError(e); } finally { busy = false; }
  }

  async function redetect() {
    busy = true;
    try {
      image = await api.post(`/images/${iid}/detect`);
      boxes = image.detections.map((d) => ({ ...d }));
      showPreview = false;
      toast(boxes.length ? `${boxes.length} Bereiche erkannt` : 'Keine Körperteile erkannt');
    } catch (e) { toastError(e); } finally { busy = false; }
  }

  function next() {
    const n = siblings.slice(idx + 1).find((s) => ['review', 'failed', 'pending'].includes(s.status)) || siblings[idx + 1];
    if (n) go(`/p/${pid}/images/${n.id}`, { snap });
  }

  function key(e) {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT') return;
    if ((e.key === 'Delete' || e.key === 'Backspace') && selected) removeBox(selected);
    if (e.key === 'p') showPreview ? (showPreview = false) : renderPreview();
  }

  const pct = (b) => {
    const [x1, y1, x2, y2] = b.box;
    return `left:${(x1 / image.width) * 100}%;top:${(y1 / image.height) * 100}%;width:${((x2 - x1) / image.width) * 100}%;height:${((y2 - y1) / image.height) * 100}%`;
  };
  const statusClass = { review: 'error', failed: 'error', anonymized: 'ok', clean: 'ok', pending: '', detecting: 'info', unsupported: 'warning' };
</script>

<svelte:window onkeydown={key} />

<div class="page">
  {#if !image}
    <div class="empty" role="status">Bild wird geladen …</div>
  {:else}
    <nav class="crumbs" aria-label="Pfad"><ol>
      <li><a href={href('/')}>Projekte</a></li><li><a href={href(`/p/${pid}`)}>{project?.name}</a></li>
      <li><a href={href(`/p/${pid}/images`, { snap })}>Bilder</a></li><li aria-current="page" class="mono">{image.icn}</li>
    </ol></nav>
    <div class="row head">
      <h1 class="grow title">{image.icn}</h1>
      <span class="badge {statusClass[image.status]}">{labels.imageStatus[image.status]}</span>
      {#if idx > 0}<a class="btn small" href={href(`/p/${pid}/images/${siblings[idx - 1].id}`, { snap })}>‹ Vorheriges</a>{/if}
      {#if idx >= 0 && idx < siblings.length - 1}<a class="btn small" href={href(`/p/${pid}/images/${siblings[idx + 1].id}`, { snap })}>Nächstes ›</a>{/if}
    </div>

    <div class="layout">
      <div class="card canvas-card">
        <div class="row viewbar">
          <div class="seg">
            <button class:on={!showPreview} onclick={() => (showPreview = false)}>Original & Markierungen</button>
            <button class:on={showPreview} onclick={() => renderPreview()}>Vorschau anonymisiert</button>
          </div>
          <span class="hint">{canEdit && !showPreview ? 'Rahmen aufziehen, um einen Bereich zu markieren · Entf löscht · P = Vorschau' : ''}</span>
        </div>
        <div class="stagewrap">
          <div class="stage" bind:this={stage} style={image.width ? `aspect-ratio:${image.width}/${image.height};width:min(100%, calc(72vh * ${image.width / image.height}))` : 'width:100%'}
            onpointerdown={(e) => down(e, null)} role="application" aria-label="Bildbereich">
            {#if showPreview && preview}
              <img src={preview} alt="Vorschau" draggable="false" />
            {:else}
              <img src={`/api/images/${iid}/original`} alt={image.icn} draggable="false"
                onload={(e) => { if (!image.width) { image.width = e.currentTarget.naturalWidth; image.height = e.currentTarget.naturalHeight; } }} />
              {#each boxes as b (b.id)}
                <div class="box {b.source}" class:sel={b.id === selected} style={pct(b)}
                  onpointerdown={(e) => down(e, b)} role="button" tabindex="-1">
                  <span class="tag">{labels.bodyPart[b.label] || b.label}{b.source === 'auto' ? ` ${Math.round(b.score * 100)} %` : ''}</span>
                  {#if canEdit}<span class="handle" onpointerdown={(e) => down(e, b, true)} role="presentation"></span>{/if}
                </div>
              {/each}
            {/if}
          </div>
        </div>
      </div>

      <aside class="stack">
        <section class="card card-pad stack">
          <h2 class="h3">Markierte Bereiche ({boxes.length})</h2>
          {#if !boxes.length}
            <p class="hint">Keine Körperteile markiert. Ziehen Sie mit der Maus einen Rahmen um Hände, Gesichter oder andere Körperteile.</p>
          {/if}
          <ul class="boxes">
            {#each boxes as b (b.id)}
              <li class:sel={b.id === selected}>
                <button class="ghost pick" onclick={() => (selected = b.id)}>
                  <strong>{labels.bodyPart[b.label] || b.label}</strong>
                  <span class="hint">{b.source === 'auto' ? `erkannt ${Math.round(b.score * 100)} %` : 'manuell'} · {labels.mode[b.mode]}</span>
                </button>
                {#if canEdit}<button class="ghost small" onclick={() => removeBox(b.id)} title="Entfernen">✕</button>{/if}
              </li>
            {/each}
          </ul>
          {#if sel && canEdit}
            <div class="editsel stack">
              <label><span class="lbl">Körperteil</span>
                <select bind:value={sel.label} onchange={() => { dirty = true; if (['face', 'head'].includes(sel.label)) sel.mode = 'pixelate'; }}>
                  {#each Object.entries(labels.bodyPart) as [k, v]}<option value={k}>{v}</option>{/each}
                </select>
              </label>
              <label><span class="lbl">Darstellung</span>
                <select bind:value={sel.mode} onchange={() => (dirty = true)}>
                  {#each Object.entries(labels.mode) as [k, v]}<option value={k}>{v}</option>{/each}
                </select>
              </label>
              <p class="hint">„Silhouette“ ersetzt die Hand durch eine neutrale Fläche mit Kontur. Die Form bleibt erkennbar, Hautfarbe, Struktur und Merkmale verschwinden.</p>
            </div>
          {/if}
        </section>

        {#if canEdit}
          <section class="card card-pad stack">
            <button class="primary" onclick={anonymize} disabled={busy || !boxes.length}>Anonymisieren & speichern</button>
            <div class="row">
              <button class="grow" onclick={save} disabled={busy || !dirty}>Nur Markierungen speichern</button>
            </div>
            <button onclick={redetect} disabled={busy}>Automatisch neu erkennen</button>
            <button onclick={markClean} disabled={busy}>Keine Körperteile, freigeben</button>
            {#if image.anonymized}
              <a class="btn" href={`/api/images/${iid}/anonymized?download=1`}>Anonymisierte Grafik herunterladen</a>
            {/if}
          </section>
        {/if}

        <section class="card card-pad stack small-info">
          <div><span class="lbl">Datei</span><span class="mono">{image.path}</span></div>
          <div><span class="lbl">Größe</span>{image.width} × {image.height} px</div>
          {#if image.detector}<div><span class="lbl">Erkennung</span>{image.detector}</div>{/if}
          {#if image.usedIn?.length}<div><span class="lbl">Verwendet in</span>{#each image.usedIn as d}<div class="mono">{d}</div>{/each}</div>{/if}
          {#if image.error}<div class="error-box">{image.error}</div>{/if}
        </section>
      </aside>
    </div>
  {/if}
</div>

<style>
  .crumbs { font-size: 13px; color: var(--muted); margin-bottom: 6px; }
  .head { margin-bottom: 14px; }
  .title { font-size: 17px; }
  .layout { display: grid; grid-template-columns: minmax(0, 1fr) 330px; gap: 16px; align-items: start; }
  .canvas-card { overflow: hidden; }
  .viewbar { padding: 10px 12px; border-bottom: 1px solid var(--line); gap: 14px; }
  .seg { display: inline-flex; border: 1px solid var(--line-strong); border-radius: var(--r-1); overflow: hidden; }
  .seg button { border: none; border-radius: 0; }
  .seg button.on { background: var(--accent-btn); color: var(--accent-ink); }
  .stagewrap { padding: 14px; background: var(--surface-sunken) repeating-conic-gradient(var(--bg) 0 25%, var(--surface-sunken) 0 50%) 0 0 / 18px 18px; display: grid; place-items: center; }
  .stage { position: relative; user-select: none; touch-action: none; cursor: crosshair; }
  .stage img { width: 100%; height: 100%; display: block; pointer-events: none; }
  .box { position: absolute; border: 2px solid #b42318; background: rgba(180, 35, 24, .12); cursor: move; }
  .box.manual { border-color: #8a4b00; background: rgba(138, 75, 0, .12); }
  .box.sel { outline: 2px solid #fff; box-shadow: 0 0 0 4px rgba(11, 92, 173, .85); }
  .tag { position: absolute; left: -2px; top: -22px; background: #b42318; color: #fff; font-size: var(--t-xs); padding: 1px 6px; border-radius: var(--r-1) var(--r-1) 0 0; white-space: nowrap; }
  .box.manual .tag { background: #8a4b00; }
  .handle { position: absolute; right: -6px; bottom: -6px; width: 12px; height: 12px; background: #fff; border: 2px solid var(--accent); border-radius: 2px; cursor: nwse-resize; }
  .boxes { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
  .boxes li { display: flex; align-items: center; border-radius: var(--r-1); }
  .boxes li.sel { background: var(--accent-soft); }
  .pick { flex: 1; flex-direction: column; align-items: flex-start; gap: 0; }
  .editsel { border-top: 1px solid var(--line); padding-top: 12px; }
  .small-info { font-size: 13px; }
  .small-info .lbl { display: block; font-size: 12px; font-weight: 600; color: var(--muted); }
  @media (max-width: 1000px) { .layout { grid-template-columns: minmax(0, 1fr); } }
</style>
