<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import cytoscape from 'cytoscape';
  import { api, labels } from '../lib/api.js';
  import { href } from '../lib/router.svelte.js';
  import { toastError } from '../lib/state.svelte.js';
  import FileViewer from './FileViewer.svelte';

  let { project, snapshot } = $props();

  const TYPES = {
    model: { label: 'Modell', color: '#15181c', shape: 'round-rectangle' },
    sns: { label: 'Systeme & Baugruppen', color: '#565c63', shape: 'round-rectangle' },
    dm: { label: 'Datenmodule', color: '#0f4a75', shape: 'ellipse' },
    pm: { label: 'Publikationsmodule', color: '#7a5a1e', shape: 'hexagon' },
    icn: { label: 'Grafiken', color: '#1d6b5a', shape: 'rectangle' },
    part: { label: 'Teile & Betriebsstoffe', color: '#56651c', shape: 'diamond' },
    acronym: { label: 'Abkürzungen', color: '#7a7f86', shape: 'tag' }
  };
  const EDGE = {
    part_of: { out: 'gehört zu', in: 'umfasst', color: '#c9c3b6', arrow: 'none' },
    references: { out: 'verweist auf', in: 'wird referenziert von', color: '#0f4a75', arrow: 'triangle' },
    contains: { out: 'enthält', in: 'ist enthalten in', color: '#7a5a1e', arrow: 'triangle' },
    uses_graphic: { out: 'verwendet Grafik', in: 'verwendet in', color: '#1d6b5a', arrow: 'triangle' },
    requires_part: { out: 'benötigt', in: 'benötigt in', color: '#56651c', arrow: 'triangle' },
    lists_part: { out: 'listet Teil', in: 'gelistet in', color: '#56651c', arrow: 'triangle' },
    defines: { out: 'definiert', in: 'definiert in', color: '#9a9488', arrow: 'triangle' }
  };

  let types = $state({ model: true, sns: true, dm: true, pm: true, icn: true, part: false, acronym: false });
  let layoutName = $state('structure');
  let data = $state(null);
  let loading = $state(false);
  let selected = $state(null); // node data
  let focus = $state(null); // { id, label }
  let depth = $state(1);
  let query = $state('');
  let highlight = $state(''); // insight key
  let viewer = $state(null);
  let container = $state();
  let cy;

  const counts = $derived(data?.counts || {});
  const stats = $derived(data?.stats || {});
  const nodeById = $derived(new Map((data?.nodes || []).map((n) => [String(n.id), n])));

  onMount(load);
  onDestroy(() => cy?.destroy());

  function propsOf(n) {
    return typeof n.props === 'string' ? JSON.parse(n.props) : n.props || {};
  }

  async function load() {
    loading = true;
    try {
      const t = Object.entries(types).filter(([, v]) => v).map(([k]) => k).join(',');
      const qs = new URLSearchParams({ types: t });
      if (focus) { qs.set('focus', focus.id); qs.set('depth', depth); }
      data = await api.get(`/snapshots/${snapshot.id}/graph?${qs}`);
      await tick();
      render();
    } catch (e) {
      toastError(e);
    } finally {
      loading = false;
    }
  }

  function shortLabel(n, p) {
    if (n.type === 'dm' && p.missing) return p.dmc;
    if (n.type === 'icn') return n.label.replace(/^ICN-/, '').slice(-22);
    return n.label.length > 42 ? n.label.slice(0, 40) + '…' : n.label;
  }

  function severity(n) {
    const f = n.findings || {};
    return f.error ? 'error' : f.warning ? 'warning' : f.info ? 'info' : '';
  }

  function render() {
    if (!container || !data) return;
    const elements = [];
    for (const n of data.nodes) {
      const p = propsOf(n);
      const t = TYPES[n.type] || TYPES.acronym;
      const size = n.type === 'model' ? 46 : n.type === 'sns' ? (p.level === 'assembly' ? 26 : 32) : 14 + Math.min(26, Math.log2(1 + n.degree) * 5);
      elements.push({
        data: {
          id: String(n.id), label: shortLabel(n, p), type: n.type, color: t.color, shape: t.shape, size,
          sev: severity(n), missing: !!p.missing, unused: !!p.unused
        }
      });
    }
    for (const e of data.edges) {
      const s = EDGE[e.type] || EDGE.part_of;
      elements.push({ data: { id: `e${e.source}-${e.target}-${e.type}`, source: String(e.source), target: String(e.target), type: e.type, color: s.color, arrow: s.arrow, weight: e.weight } });
    }
    cy?.destroy();
    cy = cytoscape({
      container,
      elements,
      minZoom: 0.05,
      maxZoom: 3,
      wheelSensitivity: 0.25,
      style: [
        { selector: 'node', style: {
          'background-color': 'data(color)', shape: 'data(shape)', width: 'data(size)', height: 'data(size)',
          label: 'data(label)', 'font-size': 10, 'font-family': 'IBM Plex Sans, system-ui, sans-serif', color: '#15181c',
          'text-valign': 'bottom', 'text-margin-y': 4, 'text-wrap': 'ellipsis', 'text-max-width': 150,
          'min-zoomed-font-size': 7, 'text-background-color': '#fffefb', 'text-background-opacity': 0.75, 'text-background-padding': 1,
          'border-width': 0
        } },
        { selector: 'node[type="sns"], node[type="model"]', style: { 'font-weight': 600, 'font-size': 11 } },
        { selector: 'node[sev="warning"]', style: { 'border-width': 3, 'border-color': '#b06d00' } },
        { selector: 'node[sev="error"]', style: { 'border-width': 3.5, 'border-color': '#a3161b' } },
        { selector: 'node[?missing]', style: { 'background-opacity': 0.12, 'border-width': 2, 'border-style': 'dashed', 'border-color': '#a3161b', color: '#7d1014' } },
        { selector: 'node[?unused]', style: { 'border-width': 2, 'border-style': 'dotted', 'border-color': '#7a7f86' } },
        { selector: 'edge', style: {
          width: 'mapData(weight, 1, 10, 1.2, 4)', 'line-color': 'data(color)', 'target-arrow-color': 'data(color)',
          'target-arrow-shape': 'data(arrow)', 'arrow-scale': 0.8, 'curve-style': 'bezier', opacity: 0.85
        } },
        { selector: 'edge[type="part_of"]', style: { width: 1, opacity: 0.7 } },
        { selector: 'node:selected', style: { 'border-width': 4, 'border-color': '#c25400', 'border-style': 'solid' } },
        { selector: '.faded', style: { opacity: 0.12, 'text-opacity': 0 } },
        { selector: '.hl', style: { 'border-width': 4, 'border-color': '#c25400' } }
      ]
    });
    runLayout();
    cy.on('tap', 'node', (e) => select(e.target.id(), false));
    cy.on('dbltap', 'node', (e) => focusOn(e.target.id()));
    cy.on('tap', (e) => { if (e.target === cy) clearSelection(); });
    if (selected && cy.getElementById(String(selected.id)).nonempty()) select(String(selected.id), true);
  }

  function runLayout() {
    if (!cy) return;
    const roots = cy.nodes('[type="model"]');
    const opts = layoutName === 'structure' && roots.nonempty()
      ? { name: 'breadthfirst', roots, directed: false, circle: true, spacingFactor: 1.05, animate: false, padding: 30, avoidOverlap: true }
      : { name: 'cose', animate: false, nodeRepulsion: () => 9000, idealEdgeLength: () => 70, padding: 30, randomize: true, numIter: 1500 };
    cy.layout(opts).run();
    fitView();
  }

  function fitView() {
    cy.fit(undefined, 30);
    if (cy.zoom() > 1.3) {
      cy.zoom(1.3);
      cy.center();
    }
  }

  function select(id, center) {
    const ele = cy?.getElementById(id);
    if (!ele || ele.empty()) return;
    highlight = '';
    cy.elements().unselect();
    ele.select();
    const hood = ele.closedNeighborhood();
    cy.elements().addClass('faded');
    hood.removeClass('faded');
    selected = nodeById.get(id);
    if (center) cy.animate({ center: { eles: ele }, zoom: Math.min(1.3, Math.max(cy.zoom(), 1)) }, { duration: 300 });
  }

  function clearSelection() {
    selected = null;
    highlight = '';
    cy?.elements().removeClass('faded hl').unselect();
  }

  async function focusOn(id) {
    const n = nodeById.get(String(id));
    if (!n) return;
    focus = { id: n.id, label: n.label };
    selected = n;
    await load();
    select(String(n.id), true);
  }

  async function resetFocus() {
    focus = null;
    await load();
  }

  function search(e) {
    e?.preventDefault();
    const q = query.trim().toLowerCase();
    if (!q || !data) return;
    const hit = data.nodes.find((n) => n.label.toLowerCase() === q)
      || data.nodes.find((n) => (n.label + ' ' + (propsOf(n).dmc || '') + ' ' + n.key).toLowerCase().includes(q));
    if (hit) select(String(hit.id), true);
    else toastError(new Error(`„${query}“ ist in der aktuellen Ansicht nicht enthalten`));
  }

  function showInsight(key) {
    if (!cy) return;
    if (highlight === key) return clearSelection();
    clearSelection();
    highlight = key;
    const ids = (stats[key] || []).map(String);
    const sel = cy.nodes().filter((n) => ids.includes(n.id()));
    cy.elements().addClass('faded');
    sel.removeClass('faded').addClass('hl');
    if (sel.nonempty()) cy.animate({ fit: { eles: sel, padding: 60 } }, { duration: 300 });
  }

  function toggleType(t) {
    types[t] = !types[t];
    load();
  }

  // neighbours of the selected node, grouped by relation
  const relations = $derived.by(() => {
    if (!selected || !data) return [];
    const id = selected.id;
    const groups = new Map();
    for (const e of data.edges) {
      let other, label;
      if (e.source === id) { other = e.target; label = EDGE[e.type]?.out || e.type; }
      else if (e.target === id) { other = e.source; label = EDGE[e.type]?.in || e.type; }
      else continue;
      const n = nodeById.get(String(other));
      if (!n) continue;
      if (!groups.has(label)) groups.set(label, []);
      groups.get(label).push(n);
    }
    return [...groups.entries()].map(([label, nodes]) => ({ label, nodes: nodes.sort((a, b) => a.label.localeCompare(b.label)) }));
  });

  const sp = $derived(selected ? propsOf(selected) : {});
  const insightList = $derived([
    ['missingDMs', 'fehlende Verweisziele', 'error'],
    ['missingICNs', 'fehlende Grafiken', 'error'],
    ['orphanDMs', `Datenmodule ${stats.orphanRule || ''}`, 'warning'],
    ['unusedICNs', 'ungenutzte Grafiken', 'info']
  ].filter(([k]) => (stats[k] || []).length));
</script>

<div class="toolbar card card-pad">
  <div class="row wrap">
    {#each Object.entries(TYPES) as [k, t]}
      <button class="chip" class:active={types[k]} aria-pressed={types[k]} onclick={() => toggleType(k)} disabled={loading}>
        <span class="dot" style="background:{t.color}"></span>{t.label} <strong>{counts[k] || 0}</strong>
      </button>
    {/each}
  </div>
  <div class="row wrap second">
    <div class="seg" role="group" aria-label="Darstellung">
      <button class:on={layoutName === 'structure'} onclick={() => { layoutName = 'structure'; runLayout(); }}>Struktur (Hierarchie)</button>
      <button class:on={layoutName === 'network'} onclick={() => { layoutName = 'network'; runLayout(); }}>Netz (Beziehungen)</button>
    </div>
    <form class="row grow gsearch" onsubmit={search}>
      <input class="grow" list="graph-nodes" bind:value={query} aria-label="Knoten suchen" placeholder="Knoten suchen: DMC, Titel, ICN, Teilenummer …" />
      <datalist id="graph-nodes">
        {#each (data?.nodes || []).slice(0, 1500) as n}<option value={n.label}></option>{/each}
      </datalist>
      <button type="submit">Suchen</button>
    </form>
    <button onclick={() => cy && fitView()}>Alles zeigen</button>
  </div>
  {#if focus}
    <div class="row wrap focusbar">
      <span class="badge accent">Umgebung von „{focus.label}“</span>
      <label class="row">Tiefe
        <select bind:value={depth} onchange={load} style="width:auto">
          <option value={1}>1</option><option value={2}>2</option><option value={3}>3</option>
        </select>
      </label>
      <button class="small" onclick={resetFocus}>Zur Gesamtansicht</button>
    </div>
  {/if}
  {#if insightList.length}
    <div class="row wrap insights">
      <span class="muted">Auffälligkeiten:</span>
      {#each insightList as [k, text, sev]}
        <button class="chip" class:active={highlight === k} aria-pressed={highlight === k} onclick={() => showInsight(k)}>
          <span class="badge {sev}">{stats[k].length}</span> {text}
        </button>
      {/each}
    </div>
  {/if}
</div>

<div class="layout">
  <div class="card canvas-card">
    {#if loading}<div class="overlay">Graph wird geladen …</div>{/if}
    {#if data?.truncated}<div class="trunc">Sehr großes Projekt: Es werden die wichtigsten 4000 Knoten gezeigt. Typen ausblenden oder einen Knoten fokussieren (Doppelklick).</div>{/if}
    <div class="cy" bind:this={container} role="application" aria-label="Wissensgraph des Projekts. Knoten über die Suche auswählen; Details erscheinen rechts."></div>
    <div class="legend">
      <span><i class="l-err"></i> Fehler</span>
      <span><i class="l-warn"></i> Warnungen</span>
      <span><i class="l-miss"></i> fehlt im Export</span>
      <span class="muted">Klick: Details · Doppelklick: Umgebung · Mausrad: Zoom</span>
    </div>
  </div>

  <aside class="card details">
    {#if !selected}
      <div class="intro">
        <h2 class="h3">Projektstruktur</h2>
        <p class="muted">Der Graph zeigt, wie das Projekt aufgebaut ist: die Produktgliederung (SNS) mit ihren Datenmodulen, Verweise zwischen Datenmodulen,
          Publikationsmodule, verwendete Grafiken, benötigte Teile und definierte Abkürzungen.</p>
        <p class="muted">Rot umrandete Datenmodule haben offene Fehler. Gestrichelte Knoten werden referenziert, fehlen aber im Export.</p>
        <p class="muted">Der KI-Review nutzt diese Beziehungen als Kontext, z. B. das Ausbau-Datenmodul derselben Baugruppe beim Prüfen des Einbaus.</p>
      </div>
    {:else}
      <div class="row head">
        <span class="badge" style="background:{TYPES[selected.type]?.color};color:#fff">{TYPES[selected.type]?.label.split(' ')[0]}</span>
        <button class="ghost small" onclick={clearSelection} aria-label="Auswahl aufheben">✕</button>
      </div>
      <h2 class="h3 title">{selected.label}</h2>

      {#if selected.type === 'dm'}
        {#if sp.missing}
          <div class="error-box">Wird referenziert, ist aber nicht im Export enthalten.</div>
        {/if}
        <dl>
          <dt>DMC</dt><dd class="mono">{sp.dmc}</dd>
          {#if sp.infoCode}<dt>Infocode</dt><dd>{sp.infoCode} ({sp.infoName})</dd>{/if}
          {#if sp.issue}<dt>Ausgabe</dt><dd>{sp.issue}{sp.lang ? ' · ' + sp.lang : ''}</dd>{/if}
          {#if sp.content}<dt>Inhaltstyp</dt><dd>{sp.content}</dd>{/if}
        </dl>
        {#if selected.findings}
          <a class="findings" href={href(`/p/${project.id}/findings`, { snap: snapshot.id, file: selected.fileId })}>
            {#each ['error', 'warning', 'info'] as s}{#if selected.findings[s]}<span class="badge {s}">{selected.findings[s]} {labels.severity[s]}</span>{/if}{/each}
            <span>Befunde ansehen ›</span>
          </a>
        {/if}
        <div class="row wrap">
          {#if selected.fileId}<button class="small" onclick={() => (viewer = { id: selected.fileId, dmc: sp.dmc, path: sp.path })}>Quelltext</button>{/if}
          <button class="small" onclick={() => focusOn(selected.id)}>Umgebung zeigen</button>
        </div>
      {:else if selected.type === 'icn'}
        {#if sp.missing}<div class="error-box">Wird verwendet, fehlt aber im Export.</div>{/if}
        {#if sp.unused}<div class="ok-box" style="background:var(--info-soft);color:var(--info)">Im Export enthalten, aber von keinem Datenmodul verwendet.</div>{/if}
        {#if selected.fileId}
          <img class="preview" alt={selected.label} src={`/api/snapshots/${snapshot.id}/files/${selected.fileId}/content`} onerror={(e) => (e.currentTarget.style.display = 'none')} />
        {/if}
        {#if sp.figures?.length}<dl><dt>Abbildung</dt>{#each sp.figures as f}<dd>{f}</dd>{/each}</dl>{/if}
        <div class="row wrap">
          <a class="btn small" href={href(`/p/${project.id}/images`, { snap: snapshot.id })}>Zur Bildprüfung</a>
          <button class="small" onclick={() => focusOn(selected.id)}>Umgebung zeigen</button>
        </div>
      {:else if selected.type === 'part'}
        <dl>
          {#if sp.partNumber}<dt>Teilenummer</dt><dd class="mono">{sp.partNumber}</dd>{/if}
          {#if sp.manufacturer}<dt>Hersteller (CAGE)</dt><dd class="mono">{sp.manufacturer}</dd>{/if}
          {#if sp.names?.length}<dt>Benennungen</dt>{#each sp.names as n}<dd>{n}</dd>{/each}{/if}
          {#if sp.roles?.length}<dt>Verwendung als</dt><dd>{sp.roles.join(', ')}</dd>{/if}
        </dl>
        {#if sp.names?.length > 1}<div class="error-box">Uneinheitliche Benennung derselben Teilenummer.</div>{/if}
      {:else if selected.type === 'acronym'}
        <dl><dt>Definitionen</dt>{#each sp.definitions || [] as d}<dd>{d}</dd>{/each}</dl>
        {#if sp.definitions?.length > 1}<div class="error-box">Abkürzung wird unterschiedlich definiert.</div>{/if}
      {:else if selected.type === 'sns' || selected.type === 'model'}
        <dl>
          <dt>Code</dt><dd class="mono">{sp.code}</dd>
          {#if sp.level}<dt>Ebene</dt><dd>{({ system: 'System', subsystem: 'Subsystem', assembly: 'Baugruppe' })[sp.level] || sp.level}</dd>{/if}
          {#if sp.name}<dt>Bezeichnung</dt><dd>{sp.name}</dd>{/if}
        </dl>
        <button class="small" onclick={() => focusOn(selected.id)}>Umgebung zeigen</button>
      {:else if selected.type === 'pm'}
        <dl><dt>PMC</dt><dd class="mono">{sp.pmc}</dd></dl>
        {#if sp.missing}<div class="error-box">Wird referenziert, ist aber nicht im Export enthalten.</div>{/if}
        <button class="small" onclick={() => focusOn(selected.id)}>Umgebung zeigen</button>
      {/if}

      {#if relations.length}
        <h3 class="rel-head">Beziehungen</h3>
        {#each relations as g}
          <div class="rel">
            <span class="muted">{g.label} ({g.nodes.length})</span>
            <ul>
              {#each g.nodes.slice(0, 30) as n}
                <li><button class="link" onclick={() => select(String(n.id), true)}><span class="dot" style="background:{TYPES[n.type]?.color}"></span>{n.label}</button></li>
              {/each}
              {#if g.nodes.length > 30}<li class="hint">… {g.nodes.length - 30} weitere</li>{/if}
            </ul>
          </div>
        {/each}
      {/if}
    {/if}
  </aside>
</div>

{#if viewer}
  <FileViewer snapshotId={snapshot.id} file={viewer} onclose={() => (viewer = null)} />
{/if}

<style>
  .toolbar { padding: 12px 14px; margin-bottom: 12px; display: flex; flex-direction: column; gap: 10px; }
  .chip .dot, .link .dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; flex: none; box-shadow: 0 0 0 1.5px var(--surface); }
  .chip:disabled { opacity: .6; }
  .chip:not(.active) .dot { opacity: .35; }
  .second { gap: 10px; }
  .seg { display: inline-flex; border: 1px solid var(--line-strong); border-radius: var(--r-1); overflow: hidden; }
  .seg button { border: none; border-radius: 0; }
  .seg button.on { background: var(--accent); color: #fff; }
  .focusbar, .insights { gap: 8px; font-size: 13.5px; }
  .layout { display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: 14px; align-items: start; }
  .canvas-card { position: relative; overflow: hidden; }
  .cy { height: 70vh; min-height: 480px; background: var(--surface); }
  .overlay { position: absolute; inset: 0; display: grid; place-items: center; background: rgba(255,255,255,.7); z-index: 2; color: var(--muted); }
  .trunc { padding: 8px 12px; background: var(--warning-soft); color: var(--warning); font-size: 13px; }
  .legend { display: flex; gap: 16px; flex-wrap: wrap; padding: 8px 12px; border-top: 1px solid var(--line); font-size: 12.5px; align-items: center; }
  .legend i { display: inline-block; width: 12px; height: 12px; border-radius: 50%; vertical-align: -2px; margin-right: 4px; background: #fff; }
  .l-err { border: 3px solid var(--error); }
  .l-warn { border: 3px solid #b06d00; }
  .l-miss { border: 2px dashed var(--error); }
  .details { padding: 16px 18px; max-height: calc(70vh + 40px); overflow: auto; display: flex; flex-direction: column; gap: 10px; }
  .intro { display: flex; flex-direction: column; gap: 10px; }
  .head { justify-content: space-between; }
  .title { overflow-wrap: anywhere; }
  dl { margin: 0; display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; font-size: 13.5px; }
  dt { color: var(--muted); }
  dd { margin: 0; overflow-wrap: anywhere; }
  dl dd + dd { grid-column: 2; }
  .findings { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; font-size: 13px; }
  .preview { max-width: 100%; max-height: 180px; object-fit: contain; border: 1px solid var(--line); border-radius: var(--r-1); background: var(--surface-sunken); }
  .rel-head { font-family: var(--font-head); font-weight: 600; font-size: var(--t-sm); color: var(--ink-2); margin-top: var(--s-2); }
  .rel ul { list-style: none; margin: 4px 0 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .link { border: none; background: none; padding: 2px 0; color: var(--accent); cursor: pointer; text-align: left; white-space: normal; display: inline-flex; gap: 6px; align-items: center; }
  .link:hover { text-decoration: underline; background: none !important; }
  @media (max-width: 640px) { .gsearch { flex: 1 1 100%; flex-wrap: wrap; } .gsearch input { flex: 1 1 100%; } }
  @media (max-width: 1100px) { .layout { grid-template-columns: minmax(0, 1fr); } .details { max-height: none; } }
</style>
