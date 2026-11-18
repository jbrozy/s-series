<script>
  import { onMount, onDestroy } from 'svelte';
  import { api, labels } from '../lib/api.js';
  import { href } from '../lib/router.svelte.js';
  import { toast, toastError } from '../lib/state.svelte.js';

  let { project, snapshot, canEdit } = $props();
  let images = $state(null);
  let filter = $state('attention');
  let timer;

  const groups = {
    attention: ['review', 'failed', 'unsupported', 'pending', 'detecting'],
    review: ['review'],
    anonymized: ['anonymized'],
    clean: ['clean'],
    all: null
  };
  const shown = $derived((images || []).filter((i) => !groups[filter] || groups[filter].includes(i.status)));
  const counts = $derived(Object.fromEntries(Object.entries(groups).map(([k, v]) => [k, (images || []).filter((i) => !v || v.includes(i.status)).length])));

  onMount(load);
  onDestroy(() => clearTimeout(timer));

  async function load() {
    try { images = await api.get(`/snapshots/${snapshot.id}/images`); } catch (e) { toastError(e); images = []; }
    clearTimeout(timer);
    if (images.some((i) => i.status === 'detecting')) timer = setTimeout(load, 2500);
  }

  async function detectAll() {
    const pending = images.filter((i) => ['pending', 'failed'].includes(i.status));
    for (const im of pending) await api.post(`/images/${im.id}/detect?async=1`).catch(() => {});
    toast(`${pending.length} Bilder zur Erkennung eingereiht`);
    setTimeout(load, 800);
  }

  // keep the distinguishing end of long ICN identifiers visible
  function shortIcn(s) {
    return s.length <= 34 ? s : s.slice(0, 12) + '…' + s.slice(-21);
  }
  const statusClass = { review: 'error', failed: 'error', unsupported: 'warning', anonymized: 'ok', clean: 'ok', pending: '', detecting: 'info' };
</script>

<div class="row wrap bar">
  {#each [['attention', 'Zu prüfen'], ['review', 'Körperteile erkannt'], ['anonymized', 'Anonymisiert'], ['clean', 'Unbedenklich'], ['all', 'Alle']] as [k, l]}
    <button class="chip {filter === k ? 'active' : ''}" aria-pressed={filter === k} onclick={() => (filter = k)}>{l} <strong>{counts[k] || 0}</strong></button>
  {/each}
  <span class="spacer"></span>
  {#if canEdit && images?.some((i) => ['pending', 'failed'].includes(i.status))}
    <button onclick={detectAll}>Ausstehende jetzt prüfen</button>
  {/if}
</div>

<p class="hint intro">
  Menschliche Körperteile sind in Abbildungen nicht zulässig und müssen unkenntlich gemacht werden. Hände und andere Gliedmaßen werden
  dabei als neutrale Silhouette dargestellt, damit weiterhin erkennbar ist, <em>dass</em> es sich um eine Hand handelt. Gesichter werden verpixelt.
</p>

{#if images === null}
  <div class="empty">Lade …</div>
{:else if shown.length === 0}
  <div class="card empty">Keine Bilder in dieser Ansicht.</div>
{:else}
  <div class="grid">
    {#each shown as im (im.id)}
      <a class="card tile" href={href(`/p/${project.id}/images/${im.id}`, { snap: snapshot.id })}>
        <div class="thumb">
          {#if im.status === 'unsupported'}
            <span class="vec">{im.path.split('.').pop().toUpperCase()}</span>
          {:else}
            <img loading="lazy" alt={im.icn} src={im.anonymized ? `/api/images/${im.id}/anonymized?t=${Date.parse(im.updatedAt)}` : `/api/images/${im.id}/original`} />
          {/if}
        </div>
        <div class="meta">
          <span class="mono icn" title={im.icn}>{shortIcn(im.icn)}</span>
          <div class="row">
            <span class="badge {statusClass[im.status]}">{labels.imageStatus[im.status] || im.status}</span>
            {#if im.detections.length}<span class="hint">{im.detections.map((d) => labels.bodyPart[d.label] || d.label).filter((v, i, a) => a.indexOf(v) === i).join(', ')}</span>{/if}
          </div>
          {#if im.error && im.status !== 'unsupported'}<span class="hint err ellipsis" title={im.error}>{im.error}</span>{/if}
        </div>
      </a>
    {/each}
  </div>
{/if}

<style>
  .bar { margin-bottom: 10px; }
  .intro { margin-bottom: 14px; max-width: 900px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(230px, 1fr)); gap: 14px; }
  .tile { color: var(--ink); overflow: hidden; display: flex; flex-direction: column; }
  .tile:hover { text-decoration: none; border-color: var(--accent); }
  .thumb { height: 170px; background: #eceef2 repeating-conic-gradient(#f4f5f7 0 25%, #eceef2 0 50%) 0 0 / 16px 16px; display: grid; place-items: center; overflow: hidden; }
  .thumb img { max-width: 100%; max-height: 100%; object-fit: contain; }
  .vec { font-weight: 700; color: var(--muted); font-size: 20px; }
  .meta { padding: 10px 12px; display: flex; flex-direction: column; gap: 6px; font-size: 12.5px; }
  .err { color: var(--error); }
  .icn { overflow-wrap: anywhere; }
</style>
