<script>
  // Release of an upload: quality gate result, owner decision, decision history,
  // and the project's release criteria.
  import { onMount } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import { href } from '../lib/router.svelte.js';
  import { toast, toastError } from '../lib/state.svelte.js';
  import Modal from './Modal.svelte';
  import LoadError from './LoadError.svelte';

  let { project, snapshot, onchange } = $props();
  const isOwner = $derived(project.myRole === 'owner');

  let rel = $state(null);
  let relError = $state(null);
  let gate = $state(null);
  let gateError = $state(null);
  let form = $state(null);
  let saving = $state(false);
  let decide = $state(null); // 'approved' | 'rejected'
  let comment = $state('');
  let decideError = $state('');

  const stateText = { passed: 'Kriterien erfüllt', failed: 'Kriterien nicht erfüllt', pending: 'Noch nicht bewertbar' };
  const stateBadge = { passed: 'ok', failed: 'error', pending: 'info' };

  onMount(() => { loadRelease(); loadGate(); });

  async function loadRelease() {
    relError = null;
    if (!snapshot) return;
    try { rel = await api.get(`/snapshots/${snapshot.id}/release`); } catch (e) { relError = e; }
  }
  async function loadGate() {
    gateError = null;
    try {
      gate = await api.get(`/projects/${project.id}/gate`);
      form = toForm(gate);
    } catch (e) { gateError = e; }
  }
  const num = (v) => (v === null || v === undefined ? '' : String(v));
  function toForm(g) {
    return { maxErrors: num(g.maxErrors), maxWarnings: num(g.maxWarnings), maxInfo: num(g.maxInfo),
      blocking: Object.fromEntries(Object.keys(labels.category).map((k) => [k, g.blockingCategories.includes(k)])), requireLlm: g.requireLlm };
  }
  const parse = (v) => (String(v).trim() === '' ? null : Number(v));
  const formError = $derived(
    form && ['maxErrors', 'maxWarnings', 'maxInfo'].some((k) => { const v = parse(form[k]); return v !== null && (!Number.isInteger(v) || v < 0); })
      ? 'Grenzwerte sind ganze Zahlen ab 0. Leer lassen heißt: nicht begrenzt.' : ''
  );

  async function saveGate(e) {
    e.preventDefault();
    if (formError) return;
    saving = true;
    try {
      gate = await api.put(`/projects/${project.id}/gate`, {
        maxErrors: parse(form.maxErrors), maxWarnings: parse(form.maxWarnings), maxInfo: parse(form.maxInfo),
        blockingCategories: Object.entries(form.blocking).filter(([, v]) => v).map(([k]) => k), requireLlm: form.requireLlm
      });
      form = toForm(gate);
      toast('Freigabekriterien gespeichert. Sie gelten ab sofort für alle Uploads dieses Projekts.');
      loadRelease();
    } catch (err) { toastError(err); } finally { saving = false; }
  }

  function openDecide(kind) { decide = kind; comment = ''; decideError = ''; }
  const commentRequired = $derived(decide === 'rejected' || rel?.gate.state === 'failed');

  async function submitDecision(e) {
    e.preventDefault();
    if (commentRequired && !comment.trim()) {
      decideError = decide === 'rejected' ? 'Bitte begründen, warum der Upload abgelehnt wird.' : 'Die Kriterien sind nicht erfüllt. Bitte begründen, warum Sie trotzdem freigeben.';
      return;
    }
    try {
      rel = await api.post(`/snapshots/${snapshot.id}/release`, { decision: decide, comment });
      toast(decide === 'approved' ? `„${snapshot.label}“ freigegeben` : `„${snapshot.label}“ abgelehnt`);
      decide = null;
      onchange?.();
    } catch (err) { decideError = err.message; }
  }
  const limitText = (l) => (l === null || l === undefined ? 'nicht begrenzt' : fmt.num(l));
</script>

<div class="layout">
  <section class="ruled stack">
    <h2 class="h3">Freigabe von „{snapshot?.label || 'kein Upload'}“</h2>
    {#if !snapshot}
      <p class="muted">Noch kein Upload vorhanden. Nach dem ersten Upload und der Analyse lässt er sich hier freigeben.</p>
    {:else if relError}
      <LoadError what="Der Freigabestatus" error={relError} onretry={loadRelease} />
    {:else if !rel}
      <p class="empty" role="status">Freigabestatus wird geladen …</p>
    {:else}
      <div class="verdict v-{rel.current && !rel.stale ? rel.current.decision : 'none'}">
        {#if rel.current}
          <p class="decision">
            <strong>{rel.current.decision === 'approved' ? 'Freigegeben' : 'Abgelehnt'}</strong>
            von {rel.current.userName} am {fmt.date(rel.current.decidedAt)}
          </p>
          {#if rel.current.comment}<p class="why">„{rel.current.comment}“</p>{/if}
          {#if rel.stale}
            <p class="stale">Seit dieser Entscheidung wurde neu analysiert. Sie bezieht sich auf ein älteres Ergebnis; bitte neu entscheiden.</p>
          {/if}
        {:else}
          <p class="decision"><strong>Noch nicht entschieden.</strong>
            {isOwner ? 'Sie können freigeben oder ablehnen, sobald die Analyse abgeschlossen ist.' : 'Freigeben oder ablehnen können die Eigentümer des Projekts.'}</p>
        {/if}
      </div>

      <div class="gatehead row wrap">
        <h3 class="grow">Freigabekriterien für diesen Stand</h3>
        <span class="badge {stateBadge[rel.gate.state]}">{stateText[rel.gate.state]}</span>
      </div>
      {#if rel.gate.state === 'pending' || (rel.gate.state === 'failed' && !rel.gate.checks.length)}
        <p class="muted">{rel.gate.reason}</p>
      {:else}
        <table class="checks">
          <thead><tr><th>Kriterium</th><th class="right">Erlaubt</th><th class="right">Ist</th><th>Ergebnis</th></tr></thead>
          <tbody>
            {#each rel.gate.checks as c}
              <tr class:bad={!c.ok}>
                <td>{c.label}</td>
                <td class="right">{c.key === 'llm' ? 'erforderlich' : limitText(c.limit)}</td>
                <td class="right">{c.key === 'llm' ? (c.actual ? 'ja' : 'nein') : fmt.num(c.actual)}</td>
                <td>{#if c.ok}<span class="badge ok">erfüllt</span>{:else}<span class="badge error">nicht erfüllt</span>{/if}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <p class="hint">Gezählt werden offene und bestätigte Befunde. Mit Begründung verworfene und als behoben markierte Befunde zählen nicht.</p>
      {/if}

      <div class="row wrap actions">
        {#if isOwner}
          <button class="primary" disabled={rel.gate.state === 'pending'} onclick={() => openDecide('approved')}>Freigeben …</button>
          <button class="danger" disabled={rel.gate.state === 'pending'} onclick={() => openDecide('rejected')}>Ablehnen …</button>
        {/if}
        <a class="btn" href={href(`/p/${project.id}/report`, { snap: snapshot.id })}>Prüfbericht öffnen</a>
      </div>

      {#if rel.history.length > 1}
        <h3>Frühere Entscheidungen zu diesem Upload</h3>
        <ol class="history">
          {#each rel.history.slice(1) as h}
            <li><strong>{h.decision === 'approved' ? 'Freigegeben' : 'Abgelehnt'}</strong> von {h.userName}, {fmt.date(h.decidedAt)}{#if h.comment}: „{h.comment}“{/if}</li>
          {/each}
        </ol>
      {/if}
    {/if}
  </section>

  <section class="ruled stack">
    <h2 class="h3">Freigabekriterien des Projekts</h2>
    {#if gateError}
      <LoadError what="Die Freigabekriterien" error={gateError} onretry={loadGate} />
    {:else if !form}
      <p class="empty" role="status">Kriterien werden geladen …</p>
    {:else}
      {#if !gate.configured}
        <p class="hint">Noch nicht festgelegt. Bis dahin gilt: keine offenen Fehler.</p>
      {:else}
        <p class="hint">Zuletzt geändert {fmt.date(gate.updatedAt)}{gate.updatedBy ? ` von ${gate.updatedBy}` : ''}.</p>
      {/if}
      <form class="stack" onsubmit={saveGate} novalidate>
        <fieldset disabled={!isOwner} class="stack">
          <legend class="sr-only">Grenzwerte für offene Befunde</legend>
          <div class="limits">
            <label><span class="lbl">Höchstens offene Fehler</span><input inputmode="numeric" bind:value={form.maxErrors} placeholder="nicht begrenzt" /></label>
            <label><span class="lbl">Höchstens offene Warnungen</span><input inputmode="numeric" bind:value={form.maxWarnings} placeholder="nicht begrenzt" /></label>
            <label><span class="lbl">Höchstens offene Hinweise</span><input inputmode="numeric" bind:value={form.maxInfo} placeholder="nicht begrenzt" /></label>
          </div>
          {#if formError}<p class="error-box" role="alert">{formError}</p>{/if}
          <div>
            <span class="lbl block">Kategorien, in denen kein Befund offen sein darf</span>
            <div class="cats">
              {#each Object.entries(labels.category) as [k, v]}
                <label class="row check"><input type="checkbox" bind:checked={form.blocking[k]} /> {v}</label>
              {/each}
            </div>
          </div>
          <label class="row check"><input type="checkbox" bind:checked={form.requireLlm} /> Freigabe nur nach einer Analyse mit KI-Review</label>
        </fieldset>
        {#if isOwner}
          <div class="row"><button class="primary" type="submit" disabled={saving}>{saving ? 'Speichert …' : 'Kriterien speichern'}</button></div>
        {:else}
          <p class="hint">Ändern können die Eigentümer des Projekts.</p>
        {/if}
      </form>
    {/if}
  </section>
</div>

{#if decide}
  <Modal title={decide === 'approved' ? `„${snapshot.label}“ freigeben` : `„${snapshot.label}“ ablehnen`} onclose={() => (decide = null)}>
    <form class="stack" onsubmit={submitDecision} novalidate>
      {#if decide === 'approved' && rel.gate.state === 'failed'}
        <p class="warnbox">Die Freigabekriterien sind nicht erfüllt. Eine Freigabe ist trotzdem möglich, wird aber mit Ihrer Begründung im Protokoll und im Prüfbericht vermerkt.</p>
      {/if}
      <label>
        <span class="lbl">Begründung{commentRequired ? '' : ' (optional)'}</span>
        <textarea rows="4" bind:value={comment} aria-invalid={!!decideError} aria-describedby={decideError ? 'decide-err' : undefined}
          placeholder={decide === 'approved' ? 'z. B. Restliche Warnungen mit Kunde abgestimmt (Protokoll 2026-11-18)' : 'z. B. Verweise auf fehlende Datenmodule müssen vor Auslieferung behoben werden'}></textarea>
      </label>
      {#if decideError}<p class="error-box" id="decide-err" role="alert">{decideError}</p>{/if}
      <div class="row"><span class="spacer"></span>
        <button type="button" onclick={() => (decide = null)}>Abbrechen</button>
        <button type="submit" class={decide === 'approved' ? 'primary' : 'danger'}>{decide === 'approved' ? 'Freigeben' : 'Ablehnen'}</button>
      </div>
    </form>
  </Modal>
{/if}

<style>
  .layout { display: grid; grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr); gap: var(--s-6); align-items: start; }
  .verdict { padding: var(--s-3) var(--s-4); background: var(--surface); border: 1px solid var(--line-strong); border-radius: var(--r-1); display: flex; flex-direction: column; gap: var(--s-1); }
  .v-approved { border-color: var(--ok); background: var(--ok-soft); }
  .v-rejected { border-color: var(--error); background: var(--error-soft); }
  .decision strong { font-family: var(--font-head); font-size: var(--t-lg); margin-right: var(--s-1); }
  .why { color: var(--ink-2); }
  .stale { color: var(--warning); font-weight: 500; }
  .gatehead { margin-top: var(--s-2); }
  .gatehead h3 { font-size: var(--t-lg); }
  .checks tr.bad td { background: var(--error-soft); }
  .actions { margin-top: var(--s-2); }
  .history { margin: 0; padding-left: var(--s-5); display: flex; flex-direction: column; gap: var(--s-1); font-size: var(--t-sm); }
  .limits { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--s-3); }
  .block { display: block; margin-bottom: var(--s-2); font-family: var(--font-head); font-size: var(--t-sm); font-weight: 600; color: var(--ink-2); }
  .cats { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s-2) var(--s-4); }
  .check { gap: var(--s-2); }
  fieldset { border: none; margin: 0; padding: 0; min-width: 0; }
  .warnbox { background: var(--warning-soft); color: var(--warning); padding: var(--s-2) var(--s-3); border-radius: var(--r-1); font-size: var(--t-sm); }
  @media (max-width: 1000px) { .layout { grid-template-columns: minmax(0, 1fr); } }
  @media (max-width: 640px) { .limits, .cats { grid-template-columns: minmax(0, 1fr); } }
</style>
