<script>
  // Global search (Strg+K): pages, projects, data modules, open findings and rules.
  // WAI-ARIA combobox with listbox; arrows move, Enter opens, Escape closes.
  import { onMount, tick } from 'svelte';
  import { api, labels } from '../lib/api.js';
  import { go, route } from '../lib/router.svelte.js';
  import { session } from '../lib/state.svelte.js';
  import { projectNav, projectSections, ui } from '../lib/nav.svelte.js';

  let q = $state('');
  let hits = $state({ projects: [], files: [], findings: [], rules: [] });
  let loading = $state(false);
  let error = $state('');
  let active = $state(0);
  let input = $state();
  let timer;
  let opener;

  const pages = $derived.by(() => {
    const list = [
      { label: 'Projekte', detail: 'Seite', go: () => go('/') },
      { label: 'Einstellungen', detail: 'Seite', go: () => go('/einstellungen') },
      { label: 'Handbuch', detail: 'Seite', go: () => go('/hilfe') },
      { label: 'Konto und API-Tokens', detail: 'Seite', go: () => go('/account') }
    ];
    if (session.user?.role === 'admin') list.push({ label: 'Administration', detail: 'Seite', go: () => go('/admin') });
    const p = projectNav.project;
    if (p && route.parts[0] === 'p') {
      for (const [key, label] of projectSections) {
        if (key === 'audit' && !['owner', 'editor'].includes(p.myRole)) continue;
        list.push({ label, detail: p.name, go: () => go(`/p/${p.id}/${key}`, projectNav.snapshotId ? { snap: projectNav.snapshotId } : undefined) });
      }
    }
    const needle = q.trim().toLowerCase();
    return needle ? list.filter((x) => (x.label + ' ' + x.detail).toLowerCase().includes(needle)) : list;
  });

  const groups = $derived([
    ['Seiten', pages],
    ['Projekte', hits.projects.map((h) => ({ label: h.title, detail: h.detail || 'Projekt', go: () => go(`/p/${h.projectId}/overview`) }))],
    ['Datenmodule und Grafiken', hits.files.map((h) => ({ label: h.title, detail: `${h.detail} · ${h.project}`, mono: true,
      go: () => go(`/p/${h.projectId}/overview`, { snap: h.snapshotId, view: h.fileId }) }))],
    ['Offene Befunde', hits.findings.map((h) => ({ label: h.title, detail: `${labels.severity[h.severity]} · ${h.detail} · ${h.project}`,
      go: () => go(`/p/${h.projectId}/findings`, { snap: h.snapshotId, q: h.title, status: '' }) }))],
    ['Business Rules', hits.rules.map((h) => ({ label: h.title, detail: `${labels.ruleType[h.detail] || h.detail} · ${h.project}`,
      go: () => go(`/p/${h.projectId}/rules`, { edit: h.id }) }))]
  ].filter(([, items]) => items.length));

  const flat = $derived(groups.flatMap(([, items]) => items));

  onMount(() => {
    opener = document.activeElement;
    input?.focus();
    return () => { if (opener instanceof HTMLElement && document.contains(opener)) opener.focus(); };
  });

  function onInput() {
    active = 0;
    clearTimeout(timer);
    error = '';
    if (q.trim().length < 2) { hits = { projects: [], files: [], findings: [], rules: [] }; return; }
    timer = setTimeout(search, 200);
  }
  async function search() {
    loading = true;
    try { hits = await api.get(`/search?q=${encodeURIComponent(q.trim())}`); } catch (e) { error = e.message; } finally { loading = false; }
  }
  function close() { ui.paletteOpen = false; }
  function choose(item) { close(); item.go(); }
  async function key(e) {
    if (e.key === 'ArrowDown') { e.preventDefault(); active = Math.min(active + 1, flat.length - 1); await tick(); scrollActive(); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); active = Math.max(active - 1, 0); await tick(); scrollActive(); }
    else if (e.key === 'Enter' && flat[active]) { e.preventDefault(); choose(flat[active]); }
    else if (e.key === 'Escape') { e.preventDefault(); close(); }
    else if (e.key === 'Tab') { e.preventDefault(); }
  }
  function scrollActive() { document.getElementById(`cp-${active}`)?.scrollIntoView({ block: 'nearest' }); }
  const indexOf = (item) => flat.indexOf(item);
</script>

<div class="backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
  <div class="palette" role="dialog" aria-modal="true" aria-label="Suche">
    <div class="field">
      <label for="cp-input" class="sr-only">Suchen nach Seiten, Projekten, Datenmodulen, Befunden und Regeln</label>
      <input id="cp-input" bind:this={input} bind:value={q} oninput={onInput} onkeydown={key} autocomplete="off" spellcheck="false"
        role="combobox" aria-expanded={flat.length > 0} aria-controls="cp-list" aria-activedescendant={flat.length ? `cp-${active}` : undefined}
        aria-describedby="cp-help" placeholder="Seite, Projekt, DMC, Befund oder Regel …" />
      <kbd>Esc</kbd>
    </div>
    <p id="cp-help" class="sr-only">Pfeiltasten wählen, Eingabetaste öffnet, Escape schließt.</p>
    <div class="results" id="cp-list" role="listbox" aria-label="Treffer">
      {#each groups as [title, items]}
        <div role="group" aria-label={title}>
          <div class="gtitle" aria-hidden="true">{title}</div>
          {#each items as item}
            {@const i = indexOf(item)}
            <!-- keyboard selection runs through the input (aria-activedescendant), options take pointer input only -->
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <div id={`cp-${i}`} role="option" tabindex="-1" aria-selected={i === active} class="opt" class:active={i === active}
              onmousemove={() => (active = i)} onclick={() => choose(item)}>
              <span class="label" class:mono={item.mono}>{item.label}</span>
              <span class="detail">{item.detail}</span>
            </div>
          {/each}
        </div>
      {/each}
      {#if loading}<p class="note" role="status">Suche läuft …</p>{/if}
      {#if error}<p class="note err" role="alert">{error}</p>{/if}
      {#if !loading && !error && q.trim().length >= 2 && flat.length === 0}
        <p class="note">Keine Treffer für „{q.trim()}“. Gesucht wird in Projekten, im neuesten Upload jedes Projekts und in den Business Rules.</p>
      {/if}
    </div>
    <div class="foot"><span><kbd>↑</kbd> <kbd>↓</kbd> wählen</span><span><kbd>Enter</kbd> öffnen</span><span><kbd>Esc</kbd> schließen</span></div>
  </div>
</div>

<style>
  .backdrop { position: fixed; inset: 0; background: var(--overlay); z-index: 60; display: flex; justify-content: center; align-items: flex-start; padding: 10vh var(--s-4) var(--s-4); }
  .palette { width: min(680px, 100%); max-height: 76vh; display: flex; flex-direction: column; background: var(--surface); border: 1px solid var(--line);
    border-radius: var(--r-2); box-shadow: var(--e-3); overflow: hidden; }
  .field { display: flex; align-items: center; gap: var(--s-2); padding: var(--s-2) var(--s-3); border-bottom: 1px solid var(--line); }
  .field input { border: none; font-size: var(--t-lg); min-height: 44px; padding: 0 var(--s-2); }
  .field input:focus-visible { outline: none; }
  .palette:focus-within .field { box-shadow: inset 0 -2px 0 var(--focus); }
  .results { overflow: auto; padding: var(--s-2); }
  .gtitle { font-size: var(--t-xs); font-weight: 600; color: var(--muted); padding: var(--s-2) var(--s-3) var(--s-1); }
  .opt { display: flex; flex-direction: column; padding: var(--s-2) var(--s-3); border-radius: var(--r-1); cursor: pointer; }
  .opt.active { background: var(--accent-soft); box-shadow: inset 3px 0 0 var(--accent); }
  .label { color: var(--ink); font-weight: 600; overflow-wrap: anywhere; }
  .detail { color: var(--muted); font-size: var(--t-sm); }
  .note { padding: var(--s-3); color: var(--muted); font-size: var(--t-sm); }
  .note.err { color: var(--error); }
  .foot { display: flex; gap: var(--s-4); padding: var(--s-2) var(--s-4); border-top: 1px solid var(--line); background: var(--surface-2); font-size: var(--t-xs); color: var(--muted); flex-wrap: wrap; }
  @media (max-width: 760px) { .backdrop { padding: var(--s-2); } .palette { max-height: 92vh; } }
</style>
