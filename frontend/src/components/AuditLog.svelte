<script>
  // Audit trail: who changed what and when. Used per project and for administrators.
  import { onMount } from 'svelte';
  import { api, fmt } from '../lib/api.js';
  import LoadError from './LoadError.svelte';

  let { pid = '', admin = false } = $props();

  const areas = [
    ['', 'Alle Bereiche'],
    ['finding.', 'Befunde'],
    ['release.', 'Freigaben'],
    ['gate.', 'Freigabekriterien'],
    ['rule.', 'Business Rules'],
    ['snapshot.', 'Uploads'],
    ['run.', 'Analysen'],
    ['report.', 'Prüfberichte'],
    ['image.', 'Bilder'],
    ['member.', 'Mitglieder'],
    ['project.', 'Projekt'],
    ...(admin ? [['auth.', 'Anmeldungen'], ['user.', 'Konten'], ['token.', 'API-Tokens'], ['settings.', 'Einstellungen']] : [])
  ];
  const areaOf = (action) => areas.find(([k]) => k && action.startsWith(k))?.[1] || action;

  let area = $state('');
  let q = $state('');
  let from = $state('');
  let to = $state('');
  let offset = $state(0);
  const limit = 50;
  let data = $state(null);
  let loadError = $state(null);
  let loading = $state(false);
  let timer;

  const base = admin ? '/admin/audit' : `/projects/${pid}/audit`;
  const params = $derived(
    new URLSearchParams(Object.entries({ action: area, q, from, to }).filter(([, v]) => v)).toString()
  );

  onMount(load);

  async function load() {
    loading = true;
    loadError = null;
    try {
      data = await api.get(`${base}?${params}&limit=${limit}&offset=${offset}`);
    } catch (e) {
      loadError = e;
    } finally {
      loading = false;
    }
  }
  function refilter() { offset = 0; load(); }
  function onSearch() { clearTimeout(timer); timer = setTimeout(refilter, 300); }
  function page(d) { offset = Math.max(0, offset + d * limit); load(); }

  function detailText(e) {
    const d = e.details || {};
    const keys = Object.keys(d).filter((k) => d[k] !== null && d[k] !== '' && !(Array.isArray(d[k]) && !d[k].length));
    return keys.length ? JSON.stringify(d, null, 2) : '';
  }
</script>

<section class="audit">
  <div class="filters row wrap">
    <label class="f-area"><span class="lbl">Bereich</span>
      <select bind:value={area} onchange={refilter} aria-label="Bereich">
        {#each areas as [k, v]}<option value={k}>{v}</option>{/each}
      </select>
    </label>
    <label class="f-q grow"><span class="lbl">Suche</span>
      <input bind:value={q} oninput={onSearch} placeholder="Person, DMC, Regel, Beschreibung …" />
    </label>
    <label class="f-date"><span class="lbl">Von</span><input type="date" bind:value={from} onchange={refilter} /></label>
    <label class="f-date"><span class="lbl">Bis</span><input type="date" bind:value={to} onchange={refilter} /></label>
    <a class="btn csv" href={`/api${base}?${params}&format=csv`}>Als CSV exportieren</a>
  </div>

  {#if loadError}
    <LoadError what="Das Protokoll" error={loadError} onretry={load} />
  {:else if !data}
    <p class="empty" role="status">Protokoll wird geladen …</p>
  {:else if data.items.length === 0}
    <p class="card empty">
      {params ? 'Keine Einträge für diese Filter.' : 'Noch keine protokollierten Änderungen. Jede Änderung an Befunden, Regeln, Freigaben, Uploads und Mitgliedern erscheint hier.'}
    </p>
  {:else}
    <div class="card list" class:loading aria-busy={loading}>
      <table>
        <thead>
          <tr>
            <th class="c-at">Zeitpunkt</th><th class="c-who">Person</th>
            {#if admin}<th>Projekt</th>{/if}
            <th class="c-area">Bereich</th><th>Änderung</th>
          </tr>
        </thead>
        <tbody>
          {#each data.items as e (e.id)}
            {@const det = detailText(e)}
            <tr>
              <td class="nowrap c-at">{fmt.date(e.at)}</td>
              <td class="c-who">
                {e.userName || 'unbekannt'}
                {#if e.viaToken}<span class="via">per API-Token „{e.viaToken}“</span>{/if}
              </td>
              {#if admin}<td class="c-proj">{e.projectName || 'kein Projekt'}</td>{/if}
              <td class="c-area">{areaOf(e.action)}</td>
              <td>
                {e.summary}
                {#if det}
                  <details><summary>Details</summary><pre>{det}</pre></details>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <div class="row pager">
      <span class="muted">{fmt.num(offset + 1)} bis {fmt.num(Math.min(offset + limit, data.total))} von {fmt.num(data.total)}</span>
      <span class="spacer"></span>
      <button class="small" disabled={offset === 0} onclick={() => page(-1)}>Neuere</button>
      <button class="small" disabled={offset + limit >= data.total} onclick={() => page(1)}>Ältere</button>
    </div>
  {/if}
</section>

<style>
  .audit { display: flex; flex-direction: column; gap: var(--s-4); }
  .filters { align-items: flex-end; gap: var(--s-3); }
  .f-area { width: 200px; }
  .f-q { min-width: 220px; }
  .f-date { width: 160px; }
  .list.loading { opacity: .6; }
  .c-at { width: 150px; }
  .c-who { width: 200px; }
  .c-area { width: 140px; color: var(--ink-2); }
  .via { display: block; font-size: var(--t-xs); color: var(--muted); }
  details { margin-top: var(--s-1); }
  summary { font-size: var(--t-sm); color: var(--accent); width: fit-content; }
  pre { margin: var(--s-1) 0 0; padding: var(--s-2) var(--s-3); background: var(--surface-2); border: 1px solid var(--line); border-radius: var(--r-1);
    font-family: var(--mono); font-size: var(--t-xs); white-space: pre-wrap; overflow-wrap: anywhere; max-height: 260px; overflow: auto; }
  @media (max-width: 760px) {
    .f-area, .f-date, .f-q, .csv { width: 100%; flex: 1 1 100%; }
    /* entries as stacked records: the change text matters most */
    table, tbody, tr, td { display: block; width: auto; }
    thead { display: none; }
    tr { padding: var(--s-2) var(--s-3); border-bottom: 1px solid var(--line); }
    td { border: none; padding: 0; }
    .c-at, .c-who, .c-proj, .c-area { display: inline; width: auto; font-size: var(--t-sm); color: var(--muted); }
    .c-who::before, .c-proj::before, .c-area::before { content: ' · '; }
    .via { display: inline; }
    .via::before { content: ' '; }
    td:last-child { margin-top: var(--s-1); }
  }
</style>
