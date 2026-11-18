<script>
  import { api } from '../lib/api.js';
  import { toastError } from '../lib/state.svelte.js';
  import FileViewer from './FileViewer.svelte';
  // loaded on demand: keeps the graph library out of the main bundle
  const graphModule = import('./KnowledgeGraph.svelte');

  let { project, snapshot } = $props();
  let view = $state('graph');
  let q = $state('');
  let hits = $state(null);
  let question = $state('');
  let answer = $state(null);
  let asking = $state(false);
  let viewer = $state(null);

  async function search(e) {
    e?.preventDefault();
    if (!q.trim()) return;
    try { hits = await api.get(`/snapshots/${snapshot.id}/search?q=${encodeURIComponent(q)}&k=25`); } catch (err) { toastError(err); }
  }

  async function ask(e) {
    e.preventDefault();
    if (!question.trim()) return;
    asking = true;
    answer = null;
    try { answer = await api.post(`/snapshots/${snapshot.id}/ask`, { question }); } catch (err) { toastError(err); } finally { asking = false; }
  }

  const modeLabel = { text: 'Volltext', semantic: 'Semantisch', hybrid: 'Hybrid' };
</script>

<div class="seg viewsel" role="group" aria-label="Ansicht der Wissensbasis">
  <button aria-pressed={view === 'graph'} class:on={view === 'graph'} onclick={() => (view = 'graph')}>Wissensgraph</button>
  <button aria-pressed={view === 'search'} class:on={view === 'search'} onclick={() => (view = 'search')}>Suche & Fragen</button>
</div>

{#if view === 'graph'}
  {#await graphModule}
    <div class="card empty">Graph wird geladen …</div>
  {:then m}
    <m.default {project} {snapshot} />
  {:catch e}
    <div class="error-box">Graph konnte nicht geladen werden: {e.message}</div>
  {/await}
{:else}
<p class="hint intro">
  Die Datenmodule (XML oder HTML-Export) werden beim Upload in Abschnitte mit Strukturkontext (DMC › Titel › Unterabschnitt) zerlegt. So lassen sie sich durchsuchen
  und als Wissensbasis für Fragen und den KI-Review nutzen. Das klappt auch ohne Embedding-Modell (Volltext und Ähnlichkeitssuche).
</p>

<div class="layout">
  <section class="card card-pad stack">
    <h2 class="h3">Suche</h2>
    <form class="row" onsubmit={search}>
      <input class="grow" bind:value={q} placeholder="z. B. Anzugsmoment, Hydraulikdruck, ICN-…" />
      <button class="primary" type="submit">Suchen</button>
    </form>
    {#if hits}
      {#if !hits.length}<p class="muted">Keine Treffer.</p>{/if}
      <ul class="hits">
        {#each hits as h}
          <li>
            <div class="row">
              <button class="link mono grow ellipsis" onclick={() => (viewer = { file: { id: h.fileId, dmc: h.dmc, path: h.dmc }, line: h.line })}>{h.breadcrumb}</button>
              <span class="badge outline">{modeLabel[h.mode]}</span>
            </div>
            <p class="txt">{h.text.length > 360 ? h.text.slice(0, 360) + ' …' : h.text}</p>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card card-pad stack">
    <h2 class="h3">Frage an die Dokumentation</h2>
    <form class="stack" onsubmit={ask}>
      <textarea rows="3" bind:value={question} placeholder="z. B. Welche Betriebsstoffe werden beim Ausbau der Hydraulikpumpe benötigt?"></textarea>
      <div class="row"><span class="hint grow">Benötigt ein konfiguriertes Chat-Modell.</span><button class="primary" disabled={asking}>{asking ? 'Denke nach …' : 'Fragen'}</button></div>
    </form>
    {#if answer}
      <div class="answer">{answer.answer}</div>
      {#if answer.sources?.length}
        <h4>Quellen</h4>
        <ol class="src">
          {#each answer.sources as s}
            <li><button class="link mono" onclick={() => (viewer = { file: { id: s.fileId, dmc: s.dmc, path: s.dmc }, line: s.line })}>{s.breadcrumb}</button></li>
          {/each}
        </ol>
      {/if}
    {/if}
  </section>
</div>

{/if}

{#if viewer}
  <FileViewer snapshotId={snapshot.id} file={viewer.file} line={viewer.line} onclose={() => (viewer = null)} />
{/if}

<style>
  .viewsel { display: inline-flex; border: 1px solid var(--line-strong); border-radius: var(--r-1); overflow: hidden; margin-bottom: 14px; }
  .viewsel button { border: none; border-radius: 0; }
  .viewsel button.on { background: var(--accent); color: #fff; }
  .intro { margin-bottom: 14px; max-width: 900px; }
  .layout { display: grid; grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr); gap: 16px; align-items: start; }
  .hits { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
  .hits li { border-bottom: 1px solid var(--line); padding-bottom: 10px; }
  .txt { font-size: 13.5px; color: var(--ink-2); margin-top: 4px; }
  .link { border: none; background: none; padding: 0; color: var(--accent); cursor: pointer; text-align: left; }
  .link:hover { text-decoration: underline; background: none !important; }
  .answer { white-space: pre-wrap; background: var(--surface-2); border: 1px solid var(--line); border-radius: var(--r-1); padding: 12px 14px; }
  .src { margin: 0; padding-left: 20px; font-size: 13px; }
  @media (max-width: 1000px) { .layout { grid-template-columns: minmax(0, 1fr); } }
</style>
