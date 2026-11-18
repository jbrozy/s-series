<script>
  // Printable inspection report for one upload. "Save as PDF" uses the
  // browser's print dialog, so it works offline.
  import { onMount } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import { href } from '../lib/router.svelte.js';
  import LoadError from '../components/LoadError.svelte';

  let { pid, sid } = $props();
  let rep = $state(null);
  let error = $state(null);

  onMount(load);
  async function load() {
    error = null;
    if (!sid) {
      try { sid = (await api.get(`/projects/${pid}/snapshots`))[0]?.id; } catch (e) { error = e; return; }
      if (!sid) { error = 'Dieses Projekt hat noch keinen Upload.'; return; }
    }
    try { rep = await api.get(`/snapshots/${sid}/report`); } catch (e) { error = e; }
  }

  const sevOrder = ['error', 'warning', 'info'];
  const stateText = { passed: 'erfüllt', failed: 'nicht erfüllt', pending: 'noch nicht bewertbar' };
  const checksRun = (o) => [['lists', 'Listen'], ['titles', 'Überschriften'], ['references', 'Verweise'], ['consistency', 'Konsistenz'],
    ['rules', 'Business Rules'], ['llm', 'KI-Review'], ['images', 'Bilder']].filter(([k]) => o?.[k]).map(([, v]) => v).join(', ');
  const rowTotal = (m) => sevOrder.reduce((a, s) => a + (m?.[s] || 0), 0);
  const limitText = (l) => (l === null || l === undefined ? 'nicht begrenzt' : fmt.num(l));
</script>

<svelte:head><title>{rep ? `Prüfbericht ${rep.snapshot.label} · ${rep.project.name}` : 'Prüfbericht'} · S-Check</title></svelte:head>

<div class="page report">
  <div class="toolbar no-print row wrap">
    <a href={href(`/p/${pid}/release`, sid ? { snap: sid } : {})}>Zurück zur Freigabe</a>
    <span class="spacer"></span>
    <button class="primary" onclick={() => window.print()} disabled={!rep}>Drucken oder als PDF speichern</button>
  </div>

  {#if error}
    <LoadError what="Der Prüfbericht" {error} onretry={load} />
  {:else if !rep}
    <p class="empty" role="status">Prüfbericht wird erstellt …</p>
  {:else}
    {@const rel = rep.release}
    <header class="rhead">
      <p class="product">S-Check · Inhaltliche Prüfung {rep.project.standard}</p>
      <h1>Prüfbericht</h1>
      <dl class="ident">
        <div><dt>Projekt</dt><dd>{rep.project.name}</dd></div>
        <div><dt>Upload</dt><dd>{rep.snapshot.label}</dd></div>
        <div><dt>Hochgeladen</dt><dd>{fmt.date(rep.snapshot.createdAt)} von {rep.snapshot.uploadedBy}</dd></div>
        <div><dt>Bericht erstellt</dt><dd>{fmt.date(rep.generatedAt)} von {rep.generatedBy}</dd></div>
      </dl>
    </header>

    <section class="sect">
      <h2>1 Ergebnis</h2>
      <div class="result r-{rel.current && !rel.stale ? rel.current.decision : 'open'}">
        {#if rel.current}
          <p class="big">{rel.current.decision === 'approved' ? 'Freigegeben' : 'Abgelehnt'}</p>
          <p>von {rel.current.userName} am {fmt.date(rel.current.decidedAt)}{#if rel.current.comment}. Begründung: „{rel.current.comment}“{/if}</p>
          {#if rel.stale}<p class="warn">Die Entscheidung bezieht sich auf eine ältere Analyse dieses Uploads.</p>{/if}
        {:else}
          <p class="big">Nicht entschieden</p>
          <p>Für diesen Upload liegt noch keine Freigabeentscheidung vor.</p>
        {/if}
      </div>
      <p>Freigabekriterien: <strong>{stateText[rel.gate.state]}</strong>{#if rel.gate.state === 'pending'} ({rel.gate.reason}){/if}</p>
      {#if rel.gate.checks.length}
        <table>
          <thead><tr><th>Kriterium</th><th class="right">Erlaubt</th><th class="right">Ist</th><th>Ergebnis</th></tr></thead>
          <tbody>
            {#each rel.gate.checks as c}
              <tr><td>{c.label}</td><td class="right">{c.key === 'llm' ? 'erforderlich' : limitText(c.limit)}</td>
                <td class="right">{c.key === 'llm' ? (c.actual ? 'ja' : 'nein') : fmt.num(c.actual)}</td><td>{c.ok ? 'erfüllt' : 'nicht erfüllt'}</td></tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </section>

    <section class="sect">
      <h2>2 Geprüfter Stand</h2>
      <dl class="facts">
        <div><dt>Dateien</dt><dd>{fmt.num(rep.snapshot.fileCount)} ({fmt.bytes(rep.snapshot.totalBytes)})</dd></div>
        <div><dt>Datenmodule</dt><dd>{fmt.num(rep.snapshot.kinds?.dm || 0)}</dd></div>
        <div><dt>Publikationsmodule</dt><dd>{fmt.num(rep.snapshot.kinds?.pm || 0)}</dd></div>
        <div><dt>Grafiken</dt><dd>{fmt.num(rep.snapshot.kinds?.icn || 0)}</dd></div>
        {#if rep.run}
          <div><dt>Analyse</dt><dd>{fmt.date(rep.run.finishedAt || rep.run.createdAt)}, Status {rep.run.status === 'done' ? 'abgeschlossen' : rep.run.status}</dd></div>
          <div><dt>Prüfungen</dt><dd>{checksRun(rep.run.options) || 'keine'}</dd></div>
        {/if}
        {#if rep.rules}<div><dt>Business Rules</dt><dd>{rep.rules.active} aktiv von {rep.rules.total}</dd></div>{/if}
      </dl>
    </section>

    {#if rep.matrix}
      <section class="sect">
        <h2>3 Offene Befunde nach Kategorie</h2>
        <table class="matrix">
          <thead><tr><th>Kategorie</th>{#each sevOrder as s}<th class="right">{labels.severity[s]}</th>{/each}<th class="right">Summe</th></tr></thead>
          <tbody>
            {#each Object.keys(labels.category).filter((k) => rep.matrix[k]) as k}
              <tr><td>{labels.category[k]}</td>{#each sevOrder as s}<td class="right">{rep.matrix[k][s] || 0}</td>{/each}<td class="right"><strong>{rowTotal(rep.matrix[k])}</strong></td></tr>
            {:else}
              <tr><td colspan="5">Keine offenen Befunde.</td></tr>
            {/each}
          </tbody>
        </table>
        <p class="note">Bearbeitungsstand aller Befunde: {Object.entries(rep.statusCount).map(([k, v]) => `${labels.status[k]} ${v}`).join(', ') || 'keine'}.</p>
        {#if rep.comparison}
          <p class="note">Gegenüber „{rep.comparison.previousLabel}“: {rep.comparison.new} neu, {rep.comparison.persisting} unverändert, {rep.comparison.resolved} nicht mehr vorhanden.</p>
        {/if}
        {#if Object.keys(rep.images || {}).length}
          <p class="note">Bilder: {Object.entries(rep.images).map(([k, v]) => `${labels.imageStatus[k] || k} ${v}`).join(', ')}.</p>
        {/if}
      </section>

      <section class="sect">
        <h2>4 Offene Befunde ({rep.open.length})</h2>
        {#if rep.open.length}
          <table class="flist">
            <thead><tr><th>Schwere</th><th>Fundstelle</th><th>Befund</th><th>Bearbeitung</th></tr></thead>
            <tbody>
              {#each rep.open as f}
                <tr>
                  <td class="nowrap">{labels.severity[f.severity]}{#if f.change === 'new'}<br /><span class="small">neu</span>{/if}</td>
                  <td class="loc"><span class="mono">{f.dmc || f.file}</span>{#if f.line}, Zeile {f.line}{/if}</td>
                  <td>{f.message}{#if f.suggestion}<br /><span class="small">Vorschlag: {f.suggestion}</span>{/if}</td>
                  <td class="small">{labels.status[f.status]}{#if f.assigneeName}, {f.assigneeName}{/if}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {:else}
          <p>Keine offenen Befunde.</p>
        {/if}
      </section>

      <section class="sect">
        <h2>5 Verworfene Befunde mit Begründung ({rep.dismissed.length})</h2>
        {#if rep.dismissed.length}
          <table class="flist">
            <thead><tr><th>Fundstelle</th><th>Befund</th><th>Begründung</th></tr></thead>
            <tbody>
              {#each rep.dismissed as f}
                <tr><td class="loc"><span class="mono">{f.dmc || f.file}</span>{#if f.line}, Zeile {f.line}{/if}</td><td>{f.message}</td><td>{f.statusNote || 'keine Angabe'}</td></tr>
              {/each}
            </tbody>
          </table>
        {:else}
          <p>Keine Befunde verworfen.</p>
        {/if}
      </section>
    {/if}

    {#if rel.history.length}
      <section class="sect">
        <h2>{rep.matrix ? 6 : 3} Entscheidungsverlauf</h2>
        <table>
          <thead><tr><th>Zeitpunkt</th><th>Entscheidung</th><th>Person</th><th>Begründung</th></tr></thead>
          <tbody>
            {#each rel.history as h}
              <tr><td class="nowrap">{fmt.date(h.decidedAt)}</td><td>{h.decision === 'approved' ? 'Freigegeben' : 'Abgelehnt'}</td><td>{h.userName}</td><td>{h.comment || 'keine'}</td></tr>
            {/each}
          </tbody>
        </table>
      </section>
    {/if}

    <footer class="rfoot">Erstellt mit S-Check am {fmt.date(rep.generatedAt)}. Der Bericht gibt den Stand der Datenbank zu diesem Zeitpunkt wieder.</footer>
  {/if}
</div>

<style>
  .report { max-width: 960px; }
  .toolbar { margin-bottom: var(--s-5); }
  .rhead { border-top: 3px solid var(--rule); padding-top: var(--s-4); margin-bottom: var(--s-6); }
  .product { color: var(--muted); font-size: var(--t-sm); }
  .rhead h1 { font-size: 36px; margin: var(--s-1) 0 var(--s-4); }
  .ident, .facts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s-2) var(--s-6); margin: 0; }
  .ident div, .facts div { display: grid; grid-template-columns: 150px minmax(0, 1fr); gap: var(--s-3); border-bottom: 1px solid var(--line); padding: var(--s-1) 0; }
  dt { color: var(--muted); font-family: var(--font-head); font-weight: 600; }
  dd { margin: 0; overflow-wrap: anywhere; }
  .sect { margin-bottom: var(--s-6); display: flex; flex-direction: column; gap: var(--s-3); }
  .sect h2 { font-size: 22px; border-bottom: 1px solid var(--ink-2); padding-bottom: var(--s-1); }
  .result { padding: var(--s-3) var(--s-4); border: 2px solid var(--ink-2); border-radius: var(--r-1); }
  .r-approved { border-color: var(--ok); background: var(--ok-soft); }
  .r-rejected { border-color: var(--error); background: var(--error-soft); }
  .big { font-family: var(--font-head); font-size: 26px; font-weight: 600; }
  .warn { color: var(--warning); font-weight: 500; }
  .note { color: var(--ink-2); font-size: var(--t-sm); }
  .small { font-size: var(--t-xs); color: var(--muted); }
  .loc { min-width: 220px; }
  .flist td { font-size: var(--t-sm); }
  .rfoot { margin-top: var(--s-7); padding-top: var(--s-3); border-top: 1px solid var(--line); color: var(--muted); font-size: var(--t-sm); }
  @media (max-width: 760px) {
    /* report tables wrap instead of scrolling sideways */
    .report :global(table) { display: table; table-layout: fixed; }
    .report :global(th), .report :global(td) { overflow-wrap: anywhere; padding-left: var(--s-2); padding-right: var(--s-2); }
    .report :global(td .mono) { white-space: normal; overflow-wrap: anywhere; }
    .loc { min-width: 0; }
    .ident, .facts { grid-template-columns: minmax(0, 1fr); }
    .ident div, .facts div { grid-template-columns: 120px minmax(0, 1fr); }
    .rhead h1 { font-size: 28px; }
  }
  @media print {
    .report { max-width: none; padding: 0; }
    .sect { break-inside: auto; }
    tr { break-inside: avoid; }
    h2 { break-after: avoid; }
    .result { break-inside: avoid; }
  }
</style>
