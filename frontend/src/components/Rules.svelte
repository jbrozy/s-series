<script>
  import { route } from '../lib/router.svelte.js';
  import SortTh from './SortTh.svelte';
  import { createSorter } from '../lib/sort.svelte.js';
  import LoadError from './LoadError.svelte';
  import { onMount } from 'svelte';
  import { api, labels } from '../lib/api.js';
  import { toast, toastError } from '../lib/state.svelte.js';
  import Modal from './Modal.svelte';

  let { project, snapshot, canEdit } = $props();
  const sevRank = { error: 0, warning: 1, info: 2 };
  const sorter = createSorter('name', { enabled: (r) => (r.enabled ? 0 : 1), name: (r) => r.name, type: (r) => labels.ruleType[r.type] || r.type, severity: (r) => sevRank[r.severity] }, 'asc', 'scheck.sort.rules');
  let rules = $state(null);
  let templates = $state([]);
  let edit = $state(null); // { id?, name, type, severity, enabled, cfg }
  let error = $state('');
  let filter = $state('all');

  const shown = $derived((rules || []).filter((r) => filter === 'all' || (filter === 'brex' ? r.source === 'brex' : r.source !== 'brex')));
  const hasBrex = $derived((snapshot?.kinds?.brex || 0) > 0);

  onMount(load);
  let loadError = $state(null);
  async function load() {
    loadError = null;
    try {
      [rules, { templates }] = await Promise.all([api.get(`/projects/${project.id}/rules`), api.get('/rules/templates')]);
      // opened from the search: edit the requested rule
      const want = route.query.edit && rules.find((r) => r.id === route.query.edit);
      if (want && canEdit) openEdit(want);
    } catch (e) { loadError = e; }
  }

  const csv = (a) => (a || []).join(', ');
  const fromCsv = (s) => s.split(',').map((x) => x.trim()).filter(Boolean);

  function toForm(cfg, type) {
    const c = JSON.parse(JSON.stringify(cfg || {}));
    c.appliesTo = c.appliesTo || {};
    c._infoCodes = csv(c.appliesTo.infoCodes);
    c._contents = csv(c.appliesTo.contents);
    c._dmcPattern = c.appliesTo.dmcPattern || '';
    if (type === 'xpath_values') { c._allowed = csv(c.allowed); c._ranges = csv(c.ranges); }
    if (type === 'dictionary') c._words = (c.words || []).join('\n');
    if (type === 'title_case') { c._elements = csv(c.elements); c._exceptions = csv(c.exceptions); }
    if (type === 'forbidden_terms') c.terms = c.terms?.length ? c.terms : [{ term: '', replacement: '', note: '' }];
    return c;
  }

  function fromForm(c, type) {
    const appliesTo = {};
    if (c._infoCodes) appliesTo.infoCodes = fromCsv(c._infoCodes);
    if (c._contents) appliesTo.contents = fromCsv(c._contents);
    if (c._dmcPattern) appliesTo.dmcPattern = c._dmcPattern;
    switch (type) {
      case 'xpath_forbidden': case 'xpath_required':
        return { xpath: c.xpath || '', context: c.context || undefined, message: c.message || '', appliesTo };
      case 'xpath_values':
        return { xpath: c.xpath || '', context: c.context || undefined, message: c.message || '', allowed: fromCsv(c._allowed || ''),
          pattern: c.pattern || undefined, ranges: fromCsv(c._ranges || ''), appliesTo };
      case 'forbidden_terms':
        return { terms: c.terms.filter((t) => t.term.trim()), scope: c.scope || 'all', caseSensitive: !!c.caseSensitive, partialWord: !!c.partialWord, appliesTo };
      case 'title_case':
        return { style: c.style || 'sentence', elements: fromCsv(c._elements || ''), exceptions: fromCsv(c._exceptions || ''), appliesTo };
      case 'sentence_length':
        return { procedural: +c.procedural || 20, descriptive: +c.descriptive || 25, appliesTo };
      case 'dictionary':
        return { words: (c._words || '').split(/\n|,/).map((w) => w.trim()).filter(Boolean) };
    }
    return {};
  }

  function openEdit(r) {
    error = '';
    edit = { id: r.id, name: r.name, type: r.type, severity: r.severity, enabled: r.enabled, source: r.source, cfg: toForm(r.config, r.type) };
  }

  function openNew(type) {
    const tpl = templates.find((t) => t.type === type);
    error = '';
    edit = { name: '', type, severity: 'warning', enabled: true, cfg: toForm(tpl && ['dictionary'].includes(type) ? {} : tpl?.config, type) };
  }

  async function saveRule() {
    error = '';
    const body = { name: edit.name, type: edit.type, severity: edit.severity, enabled: edit.enabled, config: fromForm(edit.cfg, edit.type) };
    try {
      if (edit.id) await api.put(`/rules/${edit.id}`, body);
      else await api.post(`/projects/${project.id}/rules`, body);
      edit = null;
      toast('Regel gespeichert');
      load();
    } catch (e) { error = e.message; }
  }

  async function toggle(r) {
    try {
      await api.put(`/rules/${r.id}`, { ...r, enabled: !r.enabled });
      r.enabled = !r.enabled;
    } catch (e) { toastError(e); }
  }

  async function remove(r) {
    if (!confirm(`Regel „${r.name}“ löschen?`)) return;
    try { await api.del(`/rules/${r.id}`); load(); } catch (e) { toastError(e); }
  }

  // ---- take over rules from a file or another project
  let transfer = $state(null); // { mode, projects, from, file, fileName, error, busy }
  async function openTransfer() {
    transfer = { mode: 'project', projects: null, from: '', file: null, fileName: '', error: '', busy: false };
    try {
      transfer.projects = (await api.get('/projects')).filter((p) => p.id !== project.id);
    } catch (e) { transfer.projects = []; transfer.error = e.message; }
  }
  async function pickFile(e) {
    const f = e.currentTarget.files?.[0];
    transfer.error = '';
    if (!f) return;
    try {
      transfer.file = JSON.parse(await f.text());
      transfer.fileName = f.name;
    } catch { transfer.file = null; transfer.error = `„${f.name}“ ist keine lesbare JSON-Datei.`; }
  }
  async function runTransfer(e) {
    e.preventDefault();
    const body = transfer.mode === 'project' ? { fromProject: transfer.from } : { file: transfer.file };
    if (transfer.mode === 'project' && !transfer.from) { transfer.error = 'Bitte ein Quellprojekt wählen.'; return; }
    if (transfer.mode === 'file' && !transfer.file) { transfer.error = transfer.error || 'Bitte eine exportierte Regeldatei wählen.'; return; }
    transfer.busy = true;
    try {
      const r = await api.post(`/projects/${project.id}/rules/import`, body);
      const parts = [`${r.imported} Regeln übernommen`];
      if (r.skipped) parts.push(`${r.skipped} gab es schon`);
      if (r.invalid?.length) parts.push(`${r.invalid.length} ungültig`);
      toast(parts.join(', ') + '.');
      transfer = null;
      load();
    } catch (err) { transfer.error = err.message; transfer.busy = false; }
  }

  async function importBrex() {
    try {
      const res = await api.post(`/projects/${project.id}/rules/import-brex`, { snapshotId: snapshot.id });
      toast(`${res.imported} Regeln aus BREX importiert (vorherige BREX-Regeln ersetzt)`);
      load();
    } catch (e) { toastError(e); }
  }

  function summary(r) {
    const c = r.config || {};
    switch (r.type) {
      case 'forbidden_terms': return (c.terms || []).map((t) => t.term).join(', ');
      case 'dictionary': return `${(c.words || []).length} Begriffe`;
      case 'sentence_length': return `max. ${c.procedural || 20} / ${c.descriptive || 25} Wörter`;
      case 'title_case': return c.style;
      default: return c.xpath;
    }
  }
</script>

<div class="row wrap bar">
  <button class="chip {filter === 'all' ? 'active' : ''}" aria-pressed={filter === 'all'} onclick={() => (filter = 'all')}>Alle</button>
  <button class="chip {filter === 'manual' ? 'active' : ''}" aria-pressed={filter === 'manual'} onclick={() => (filter = 'manual')}>Eigene Regeln</button>
  <button class="chip {filter === 'brex' ? 'active' : ''}" aria-pressed={filter === 'brex'} onclick={() => (filter = 'brex')}>Aus BREX</button>
  <span class="spacer"></span>
  <a class="btn" href={`/api/projects/${project.id}/rules/export`} title="Alle Regeln dieses Projekts als Datei, z. B. für andere Projekte oder zur Ablage">Regeln exportieren</a>
  {#if canEdit}
    <button onclick={openTransfer}>Regeln übernehmen …</button>
    {#if hasBrex}<button onclick={importBrex} title="Strukturregeln aus dem BREX-Datenmodul des gewählten Uploads übernehmen">BREX importieren</button>{/if}
    <select class="newsel" aria-label="Neue Regel anlegen" onchange={(e) => { if (e.currentTarget.value) openNew(e.currentTarget.value); e.currentTarget.value = ''; }}>
      <option value="">+ Neue Regel …</option>
      {#each Object.entries(labels.ruleType) as [k, v]}<option value={k}>{v}</option>{/each}
    </select>
  {/if}
</div>
<p class="hint intro">Business Rules unterscheiden sich von Projekt zu Projekt. Sie gelten bei jeder Analyse dieses Projekts und fließen auch in den KI-Review und die Trainingsdaten ein.</p>

{#if loadError}
  <LoadError what="Die Business Rules" error={loadError} onretry={load} />
{:else if rules === null}
  <div class="empty" role="status">Regeln werden geladen …</div>
{:else if shown.length === 0}
  <div class="card empty">Keine Regeln in dieser Ansicht. Über „Neue Regel“ legen Sie eine an{hasBrex ? ' oder importieren sie aus dem BREX-Datenmodul' : ''}.</div>
{:else}
  <div class="card">
    <table>
      <thead><tr><SortTh {sorter} key="enabled">Aktiv</SortTh><SortTh {sorter} key="name">Regel</SortTh><SortTh {sorter} key="type">Typ</SortTh><SortTh {sorter} key="severity">Schwere</SortTh><th>Inhalt</th><th><span class="sr-only">Aktionen</span></th></tr></thead>
      <tbody>
        {#each sorter.apply(shown) as r (r.id)}
          <tr class:off={!r.enabled}>
            <td><input type="checkbox" checked={r.enabled} disabled={!canEdit} onchange={() => toggle(r)} aria-label={`Regel „${r.name}“ aktiv`} /></td>
            <td><strong>{r.name}</strong>{#if r.source === 'brex'} <span class="badge accent">BREX</span>{/if}</td>
            <td class="muted nowrap">{labels.ruleType[r.type] || r.type}</td>
            <td><span class="badge {r.severity}">{labels.severity[r.severity]}</span></td>
            <td class="mono sum ellipsis" title={summary(r)}>{summary(r)}</td>
            <td class="right nowrap">
              {#if canEdit}
                <button class="small" onclick={() => openEdit(r)}>Bearbeiten</button>
                <button class="small danger" onclick={() => remove(r)}>Löschen</button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

{#if edit}
  <Modal title={edit.id ? 'Regel bearbeiten' : 'Neue Regel: ' + labels.ruleType[edit.type]} wide onclose={() => (edit = null)}>
    <div class="stack">
      <div class="grid3">
        <label><span class="lbl">Name</span><input bind:value={edit.name} required /></label>
        <label><span class="lbl">Schwere</span>
          <select bind:value={edit.severity}><option value="error">Fehler</option><option value="warning">Warnung</option><option value="info">Hinweis</option></select>
        </label>
        <label class="row check"><input type="checkbox" bind:checked={edit.enabled} /> aktiv</label>
      </div>

      {#if edit.type.startsWith('xpath')}
        <label><span class="lbl">XPath-Ausdruck</span><input class="mono" bind:value={edit.cfg.xpath} placeholder="//title//emphasis" /></label>
        <label><span class="lbl">Kontext (optional)</span><input class="mono" bind:value={edit.cfg.context} placeholder="//proceduralStep (Ausdruck wird je Kontextknoten geprüft)" /></label>
        {#if edit.type === 'xpath_values'}
          <div class="grid3">
            <label><span class="lbl">Erlaubte Werte (Komma)</span><input bind:value={edit.cfg._allowed} placeholder="01, 02" /></label>
            <label><span class="lbl">Muster (RegEx)</span><input class="mono" bind:value={edit.cfg.pattern} placeholder="[A-Z]{2}\d{3}" /></label>
            <label><span class="lbl">Bereiche (a~b, Komma)</span><input bind:value={edit.cfg._ranges} placeholder="001~099" /></label>
          </div>
        {/if}
        <label><span class="lbl">Meldung</span><input bind:value={edit.cfg.message} placeholder="Was ist falsch und wie wird es korrigiert?" /></label>
        <p class="hint">
          {#if edit.type === 'xpath_forbidden'}Jeder Treffer erzeugt einen Befund.{:else if edit.type === 'xpath_required'}Befund, wenn der Ausdruck (je Datenmodul bzw. je Kontext) nichts findet.{:else}Jeder gefundene Wert (Attribut/Element) muss erlaubt sein.{/if}
          XPath 1.0, Auswertung ab Dokumentwurzel.
        </p>
      {:else if edit.type === 'forbidden_terms'}
        <table class="terms">
          <thead><tr><th>Begriff</th><th>Ersatz / Vorschlag</th><th>Hinweis</th><th></th></tr></thead>
          <tbody>
            {#each edit.cfg.terms as t, i}
              <tr>
                <td><input bind:value={t.term} /></td>
                <td><input bind:value={t.replacement} /></td>
                <td><input bind:value={t.note} /></td>
                <td><button class="ghost small" onclick={() => edit.cfg.terms.splice(i, 1)}>✕</button></td>
              </tr>
            {/each}
          </tbody>
        </table>
        <div class="row wrap">
          <button class="small" onclick={() => edit.cfg.terms.push({ term: '', replacement: '', note: '' })}>+ Begriff</button>
          <span class="spacer"></span>
          <label class="row check">Bereich
            <select bind:value={edit.cfg.scope} style="width:auto"><option value="all">Alles</option><option value="titles">Nur Überschriften</option><option value="text">Nur Fließtext</option></select>
          </label>
          <label class="row check"><input type="checkbox" bind:checked={edit.cfg.caseSensitive} /> Groß-/Kleinschreibung beachten</label>
          <label class="row check"><input type="checkbox" bind:checked={edit.cfg.partialWord} /> auch Wortteile</label>
        </div>
      {:else if edit.type === 'title_case'}
        <div class="grid3">
          <label><span class="lbl">Schreibweise</span>
            <select bind:value={edit.cfg.style}><option value="sentence">Satzschreibung</option><option value="title">Titelschreibung</option><option value="upper">GROSSBUCHSTABEN</option></select>
          </label>
          <label><span class="lbl">Elemente (Komma)</span><input bind:value={edit.cfg._elements} placeholder="levelledPara, figure, table, dmTitle" /></label>
          <label><span class="lbl">Ausnahmen (Eigennamen)</span><input bind:value={edit.cfg._exceptions} placeholder="MTU, Pratt, Whitney" /></label>
        </div>
      {:else if edit.type === 'sentence_length'}
        <div class="grid3">
          <label><span class="lbl">Max. Wörter in Arbeitsschritten</span><input type="number" bind:value={edit.cfg.procedural} /></label>
          <label><span class="lbl">Max. Wörter in Beschreibungen</span><input type="number" bind:value={edit.cfg.descriptive} /></label>
        </div>
        <p class="hint">ASD-STE100 empfiehlt 20 Wörter für prozeduralen und 25 Wörter für beschreibenden Text.</p>
      {:else if edit.type === 'dictionary'}
        <label><span class="lbl">Zulässige Fachbegriffe (ein Begriff pro Zeile)</span><textarea rows="10" bind:value={edit.cfg._words}></textarea></label>
        <p class="hint">Diese Begriffe werden von der Tippfehler-Erkennung nie als Fehler gemeldet (z. B. Herstellernamen, Bauteilbezeichnungen).</p>
      {/if}

      {#if edit.type !== 'dictionary'}
        <details>
          <summary>Gültigkeit einschränken</summary>
          <div class="grid3 applies">
            <label><span class="lbl">Infocodes (Präfix, Komma)</span><input bind:value={edit.cfg._infoCodes} placeholder="520, 720" /></label>
            <label><span class="lbl">Inhaltstyp (Komma)</span><input bind:value={edit.cfg._contents} placeholder="procedure, description" /></label>
            <label><span class="lbl">DMC-Muster (RegEx)</span><input class="mono" bind:value={edit.cfg._dmcPattern} placeholder="-29-" /></label>
          </div>
        </details>
      {/if}

      {#if error}<div class="error-box">{error}</div>{/if}
      <div class="row"><span class="spacer"></span>
        <button onclick={() => (edit = null)}>Abbrechen</button>
        <button class="primary" onclick={saveRule}>Speichern</button>
      </div>
    </div>
  </Modal>
{/if}

{#if transfer}
  <Modal title="Regeln übernehmen" onclose={() => (transfer = null)}>
    <form class="stack" onsubmit={runTransfer} novalidate>
      <p class="hint">Regeln mit gleichem Namen und Typ, die es hier schon gibt, werden übersprungen. Bestehende Regeln bleiben unverändert.</p>
      <fieldset class="stack">
        <legend class="lbl">Quelle</legend>
        <label class="row check"><input type="radio" name="src" value="project" bind:group={transfer.mode} /> Aus einem anderen Projekt</label>
        <label class="row check"><input type="radio" name="src" value="file" bind:group={transfer.mode} /> Aus einer exportierten Regeldatei (.json)</label>
      </fieldset>
      {#if transfer.mode === 'project'}
        {#if transfer.projects === null}
          <p class="muted" role="status">Projekte werden geladen …</p>
        {:else if transfer.projects.length === 0}
          <p class="muted">Sie haben keinen Zugriff auf weitere Projekte.</p>
        {:else}
          <label><span class="lbl">Quellprojekt</span>
            <select bind:value={transfer.from}>
              <option value="">Projekt wählen</option>
              {#each transfer.projects as p}<option value={p.id}>{p.name}</option>{/each}
            </select>
          </label>
        {/if}
      {:else}
        <label><span class="lbl">Regeldatei</span><input type="file" accept=".json,application/json" onchange={pickFile} /></label>
        {#if transfer.file}<p class="hint">{transfer.fileName}: {transfer.file.rules?.length ?? 0} Regeln aus „{transfer.file.project || 'unbekannt'}“</p>{/if}
      {/if}
      {#if transfer.error}<p class="error-box" role="alert">{transfer.error}</p>{/if}
      <div class="row"><span class="spacer"></span>
        <button type="button" onclick={() => (transfer = null)}>Abbrechen</button>
        <button class="primary" type="submit" disabled={transfer.busy}>{transfer.busy ? 'Übernimmt …' : 'Übernehmen'}</button>
      </div>
    </form>
  </Modal>
{/if}

<style>
  fieldset { border: none; margin: 0; padding: 0; min-width: 0; }
  .check { gap: var(--s-2); }
  .bar { margin-bottom: 8px; }
  .intro { margin-bottom: 14px; }
  .newsel { width: auto; }
  tr.off td { color: var(--muted); background: var(--surface-2); }
  tr.off strong { font-weight: 500; }
  .sum { max-width: 380px; color: var(--ink-2); }
  .terms td { padding: 4px; border: none; }
  .check { gap: 6px; font-size: 13.5px; }
  .applies { margin-top: 10px; }
  details summary { cursor: pointer; color: var(--accent); font-size: 13.5px; }
</style>
