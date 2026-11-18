<script>
  // History of one finding across uploads: status changes, assignments and comments.
  import { onMount } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import Modal from './Modal.svelte';
  import LoadError from './LoadError.svelte';

  let { finding, canEdit, onclose, oncount } = $props();
  let items = $state(null);
  let error = $state(null);
  let body = $state('');
  let sending = $state(false);
  let sendError = $state('');

  onMount(load);
  async function load() {
    error = null;
    try { items = await api.get(`/findings/${finding.id}/activity`); } catch (e) { error = e; }
  }

  async function send(e) {
    e.preventDefault();
    if (!body.trim()) { sendError = 'Bitte einen Kommentar eingeben.'; return; }
    sending = true;
    sendError = '';
    try {
      items = await api.post(`/findings/${finding.id}/comments`, { body });
      body = '';
      oncount?.(items.filter((a) => a.kind === 'comment').length);
    } catch (err) { sendError = err.message; } finally { sending = false; }
  }
</script>

<Modal title="Verlauf des Befunds" wide {onclose}>
  <div class="stack">
    <div class="subject">
      <span class="badge {finding.severity}">{labels.severity[finding.severity]}</span>
      <code>{finding.code}</code>
      <p>{finding.message}</p>
      <p class="hint">{finding.dmc || finding.file}. Kommentare und Zuweisung bleiben am Befund, solange er in späteren Uploads wieder auftritt.</p>
    </div>

    {#if error}
      <LoadError what="Der Verlauf" {error} onretry={load} />
    {:else if !items}
      <p class="empty" role="status">Verlauf wird geladen …</p>
    {:else if items.length === 0}
      <p class="muted">Noch keine Änderungen oder Kommentare.{canEdit ? ' Schreiben Sie den ersten Kommentar.' : ''}</p>
    {:else}
      <ol class="timeline">
        {#each items as a}
          <li class:comment={a.kind === 'comment'}>
            <div class="meta"><strong>{a.user || 'unbekannt'}</strong> <span class="muted">{fmt.date(a.at)}</span></div>
            {#if a.kind === 'comment'}<p class="text">{a.text}</p>{:else}<p class="event">{a.text}</p>{/if}
          </li>
        {/each}
      </ol>
    {/if}

    {#if canEdit}
      <form class="stack" onsubmit={send} novalidate>
        <label>
          <span class="lbl">Kommentar</span>
          <textarea rows="3" bind:value={body} aria-invalid={!!sendError} aria-describedby={sendError ? 'c-err' : undefined}
            placeholder="z. B. Korrektur im nächsten Issue, Rücksprache mit Konstruktion läuft"></textarea>
        </label>
        {#if sendError}<p class="error-box" id="c-err" role="alert">{sendError}</p>{/if}
        <div class="row"><span class="spacer"></span><button class="primary" type="submit" disabled={sending}>{sending ? 'Speichert …' : 'Kommentar speichern'}</button></div>
      </form>
    {/if}
  </div>
</Modal>

<style>
  .subject { display: flex; flex-wrap: wrap; gap: var(--s-2); align-items: baseline; padding-bottom: var(--s-3); border-bottom: 1px solid var(--line); }
  .subject p { flex-basis: 100%; }
  .subject code { color: var(--muted); }
  .timeline { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
  .timeline li { padding: var(--s-2) 0 var(--s-2) var(--s-4); border-left: 2px solid var(--line-strong); position: relative; }
  .timeline li::before { content: ''; position: absolute; left: -5px; top: 14px; width: 8px; height: 8px; background: var(--surface); border: 2px solid var(--line-strong); border-radius: 50%; }
  .timeline li.comment::before { background: var(--accent); border-color: var(--accent); }
  .meta { font-size: var(--t-sm); }
  .text { white-space: pre-wrap; overflow-wrap: anywhere; }
  .event { color: var(--ink-2); font-size: var(--t-sm); }
  textarea { font-family: var(--font); font-size: var(--t-md); }
</style>
